// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/env"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	aisFinalizer = "finalize.ais"
	userAgent    = "ais-operator"

	configHashAnnotation = "config.aistore.nvidia.com/hash"
)

type (
	// AIStoreReconciler reconciles a AIStore object
	AIStoreReconciler struct {
		mu           sync.RWMutex
		client       *aisclient.K8sClient
		log          logr.Logger
		recorder     record.EventRecorder
		clientParams map[string]*aisapi.BaseParams
		isExternal   bool // manager is deployed externally to K8s cluster
		// AuthN Server Config
		authN authNConfig
	}
)

func NewAISReconciler(c *aisclient.K8sClient, recorder record.EventRecorder, logger logr.Logger, isExternal bool) *AIStoreReconciler {
	return &AIStoreReconciler{
		client:       c,
		log:          logger,
		recorder:     recorder,
		clientParams: make(map[string]*aisapi.BaseParams, 16),
		isExternal:   isExternal,
		authN:        newAuthNConfig(),
	}
}
func newAuthNConfig() authNConfig {
	protocol := "http"
	if useHTTPS, err := cos.IsParseEnvBoolOrDefault(env.AuthN.UseHTTPS, false); err == nil && useHTTPS {
		protocol = "https"
	}

	return authNConfig{
		adminUser: cos.GetEnvOrDefault(env.AuthN.AdminUsername, AuthNAdminUser),
		adminPass: cos.GetEnvOrDefault(env.AuthN.AdminPassword, AuthNAdminPass),
		host:      cos.GetEnvOrDefault(AuthNServiceHostVar, AuthNServiceHostName),
		port:      cos.GetEnvOrDefault(AuthNServicePortVar, AuthNServicePort),
		protocol:  protocol,
	}
}

func NewAISReconcilerFromMgr(mgr manager.Manager, logger logr.Logger, isExternal bool) *AIStoreReconciler {
	c := aisclient.NewClientFromMgr(mgr)
	recorder := mgr.GetEventRecorderFor("ais-controller")
	return NewAISReconciler(c, recorder, logger, isExternal)
}

// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AIStoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.log.WithValues("namespace", req.Namespace, "name", req.Name)
	ctx = logf.IntoContext(ctx, logger)
	logger.Info("Reconciling AIStore")

	ais, err := r.client.GetAIStoreCR(ctx, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Unable to fetch AIStore")
		return reconcile.Result{}, err
	}

	if ais.HasState("") {
		if err := r.initializeCR(ctx, ais); err != nil {
			return reconcile.Result{}, err
		}
	}

	if ais.ShouldDecommission() {
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterDecommissioning})
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonDeleted, "Decommissioning...")
	}

	if ais.ShouldShutdown() {
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterShuttingDown})
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonUpdated, "Shutting down...")
	}

	switch {
	case ais.HasState(aisv1.ClusterShuttingDown):
		return r.shutdownCluster(ctx, ais)
	case ais.HasState(aisv1.ClusterDecommissioning):
		return r.decommissionCluster(ctx, ais)
	case ais.HasState(aisv1.ClusterCleanup):
		return r.cleanupClusterRes(ctx, ais)
	}

	if result, err := r.ensurePrereqs(ctx, ais); err != nil || !result.IsZero() {
		return result, err
	}

	if !ais.IsConditionTrue(aisv1.ConditionCreated) {
		return r.bootstrapNew(ctx, ais)
	}
	return r.handleCREvents(ctx, ais)
}

func (r *AIStoreReconciler) initializeCR(ctx context.Context, ais *aisv1.AIStore) (err error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(ais, aisFinalizer) {
		logger.Info("Updating finalizer")
		controllerutil.AddFinalizer(ais, aisFinalizer)
		if err = r.client.Update(ctx, ais); err != nil {
			logger.Error(err, "Failed to update finalizer")
			return err
		}
		logger.Info("Successfully updated finalizer")
	}

	logger.Info("Updating state and setting condition", "state", aisv1.ConditionInitialized)
	ais.SetCondition(aisv1.ConditionInitialized)
	err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterInitialized})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ConditionInitialized)
		return err
	}
	logger.Info("Successfully updated state")

	return
}

func (r *AIStoreReconciler) shutdownCluster(ctx context.Context, ais *aisv1.AIStore) (reconcile.Result, error) {
	var err error
	logger := logf.FromContext(ctx)

	logger.Info("Starting shutdown of AIS cluster")
	if err = r.attemptGracefulShutdown(ctx, ais); err != nil {
		logger.Error(err, "Graceful shutdown failed")
	}
	//TODO: wait for AIS graceful shutdown to finish before scaling down
	if err = r.scaleStatefulSetToZero(ctx, proxy.StatefulSetNSName(ais)); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.scaleStatefulSetToZero(ctx, target.StatefulSetNSName(ais)); err != nil {
		return reconcile.Result{}, err
	}
	err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterShutdown})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ClusterShutdown)
		return reconcile.Result{}, err
	}
	logger.Info("AIS cluster shutdown completed")
	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonShutdownCompleted, "Shutdown completed")
	return reconcile.Result{}, nil
}

func (r *AIStoreReconciler) decommissionCluster(ctx context.Context, ais *aisv1.AIStore) (reconcile.Result, error) {
	logger := logf.FromContext(ctx)
	if r.isClusterRunning(ctx, ais) {
		err := r.decommissionAIS(ctx, ais)
		if err != nil {
			logger.Error(err, "Unable to gracefully decommission AIStore, retrying until cluster is not running")
		}
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	err := r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterCleanup})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ClusterCleanup)
		return reconcile.Result{}, err
	}
	logger.Info("AIS cluster decommission completed")
	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonDecommissionCompleted, "Decommission completed")
	return reconcile.Result{}, nil
}

func (r *AIStoreReconciler) cleanupClusterRes(ctx context.Context, ais *aisv1.AIStore) (reconcile.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(ais, aisFinalizer) {
		logger.Info("No finalizer remaining on AIS")
		return reconcile.Result{}, nil
	}
	logger.Info("Deleting AIS cluster resources")
	updated, err := r.cleanup(ctx, ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to cleanup AIS Resources")
		return reconcile.Result{}, err
	}
	if updated {
		// It is better to delay the requeue little bit since cleanup can take some time.
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	logger.Info("Removing AIS finalizer")
	controllerutil.RemoveFinalizer(ais, aisFinalizer)
	err = r.client.UpdateIfExists(ctx, ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to update instance")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *AIStoreReconciler) isClusterRunning(ctx context.Context, ais *aisv1.AIStore) bool {
	// Consider cluster running if both proxy and target ss have ready pods
	return r.ssHasReadyReplicas(ctx, target.StatefulSetNSName(ais)) && r.ssHasReadyReplicas(ctx, proxy.StatefulSetNSName(ais))
}

func (r *AIStoreReconciler) scaleStatefulSetToZero(ctx context.Context, name types.NamespacedName) error {
	logger := logf.FromContext(ctx).WithValues("statefulset", name.String())
	logger.Info("Scaling statefulset to zero")
	changed, err := r.client.UpdateStatefulSetReplicas(ctx, name, 0)
	if err != nil {
		logger.Error(err, "Failed to scale statefulset to zero")
	} else if changed {
		logger.Info("StatefulSet set to size 0")
	} else {
		logger.Info("StatefulSet already at size 0")
	}
	return err
}

func (r *AIStoreReconciler) ssHasReadyReplicas(ctx context.Context, name types.NamespacedName) bool {
	ss, err := r.client.GetStatefulSet(ctx, name)
	if k8serrors.IsNotFound(err) {
		return false
	}
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to get statefulset", "statefulset", name)
		// Assume the ss has ready replicas unless we can confirm otherwise
		return true
	}
	return ss.Status.ReadyReplicas > 0
}

func (r *AIStoreReconciler) decommissionAIS(ctx context.Context, ais *aisv1.AIStore) error {
	var err error
	logger := logf.FromContext(ctx)

	if ais.ShouldCleanupMetadata() {
		err = r.attemptGracefulDecommission(ctx, ais)
		if err != nil {
			logger.Info("Failed to decommission cluster")
		}
	} else {
		// We are "decommissioning" on the operator side and will still delete the statefulsets
		// This call to the AIS API preserves metadata for a future cluster, where decommission call would delete it all
		err = r.attemptGracefulShutdown(ctx, ais)
		if err != nil {
			logger.Info("Failed to shutdown cluster")
		}
	}
	return err
}

func (r *AIStoreReconciler) attemptGracefulDecommission(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting graceful decommission of cluster")
	var (
		baseParams *aisapi.BaseParams
		err        error
	)
	if r.isExternal {
		baseParams, err = r.getAPIParams(ctx, ais)
	} else {
		baseParams, err = r.primaryBaseParams(ctx, ais)
	}
	if err != nil {
		return err
	}
	cleanupData := ais.Spec.CleanupData != nil && *ais.Spec.CleanupData
	err = aisapi.DecommissionCluster(*baseParams, cleanupData)
	if err != nil {
		logger.Error(err, "Failed to gracefully decommission cluster")
	}
	return err
}

func (r *AIStoreReconciler) attemptGracefulShutdown(ctx context.Context, ais *aisv1.AIStore) error {
	var (
		params *aisapi.BaseParams
		err    error
	)
	logger := logf.FromContext(ctx)
	logger.Info("Attempting graceful shutdown")
	if r.isExternal {
		params, err = r.getAPIParams(ctx, ais)
	} else {
		params, err = r.primaryBaseParams(ctx, ais)
	}
	if err != nil {
		return err
	}
	logger.Info("Attempting graceful shutdown of cluster")
	err = aisapi.ShutdownCluster(*params)
	if err != nil {
		logger.Error(err, "Failed to gracefully shutdown cluster")
	}
	return err
}

// reconcileResources is responsible for reconciling all resources that given
// AIStore CRD is managing. It handles initial reconcile as well as any updates.
func (r *AIStoreReconciler) reconcileResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	_, err = ais.ValidateSpec(ctx)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to validate AIStore spec")
		return err
	}

	globalCM, err := cmn.NewGlobalCM(ais, ais.Spec.ConfigToUpdate)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to construct global config")
		return err
	}

	// 1. Deploy RBAC resources.
	err = r.createOrUpdateRBACResources(ctx, ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to create/update RBAC resources")
		return err
	}

	// 2. Deploy statsd ConfigMap. Required by both proxies and targets.
	statsDCM := statsd.NewStatsDCM(ais)
	if err = r.client.CreateOrUpdateResource(ctx, ais, statsDCM); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy StatsD ConfigMap")
		return err
	}

	// 3. Deploy global cluster ConfigMap.
	if err = r.client.CreateOrUpdateResource(ctx, ais, globalCM); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy global cluster ConfigMap")
		return err
	}

	// FIXME: We should also move the logic from `bootstrapNew` and `handleCREvents`.

	// FIXME: To make sure that we don't forget to update StatefulSets we should
	//  add annotations with hashes of the configmaps - thanks to this even if we
	//  would restart on next reconcile we can compare hashes.

	return nil
}

func (r *AIStoreReconciler) ensurePrereqs(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	// 1. Reconcile basic resources like RBAC and ConfigMaps.
	if err = r.reconcileResources(ctx, ais); err != nil {
		return result, err
	}

	// 2. Check if the cluster needs external access.
	// If yes, create a LoadBalancer services for targets and proxies and wait for external IP to be allocated.
	if ais.Spec.EnableExternalLB {
		var proxyReady, targetReady bool
		proxyReady, err = r.enableProxyExternalService(ctx, ais)
		if err != nil {
			r.recordError(ctx, ais, err, "Failed to enable proxy external service")
			return result, err
		}
		err = r.enableTargetExternalService(ctx, ais)
		if err != nil {
			r.recordError(ctx, ais, err, "Failed to enable target external service")
			return result, err
		}
		// When external access is enabled, we need external IPs of all the targets before deploying AIS cluster.
		// To ensure correct behavior of cluster, we requeue the reconciler till we have all the external IPs.
		if !proxyReady {
			if !ais.HasState(aisv1.ClusterInitializingLBService) && !ais.HasState(aisv1.ClusterPendingLBService) {
				err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterInitializingLBService})
				if err == nil {
					r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, "Successfully initialized LoadBalancer service")
				}
			} else {
				err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterPendingLBService})
				if err == nil {
					r.recorder.Eventf(
						ais, corev1.EventTypeNormal, EventReasonWaiting,
						"Waiting for LoadBalancer service to be ready; proxy ready=%t, target ready=%t", proxyReady, targetReady,
					)
				}
			}
			result.Requeue = true
			return
		}
	}

	err = r.ensureProxyPrereqs(ctx, ais)
	if err != nil {
		return
	}
	err = r.ensureTargetPrereqs(ctx, ais)
	return
}

func (r *AIStoreReconciler) bootstrapNew(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	// 1. Bootstrap proxies
	if result.Requeue, err = r.initProxies(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Proxy resources")
		return result, err
	} else if result.Requeue {
		return
	}

	// 2. Bootstrap targets
	if result.Requeue, err = r.initTargets(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Target resources")
		return result, err
	} else if result.Requeue {
		return
	}

	ais.SetCondition(aisv1.ConditionCreated)
	err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterCreated})
	if err != nil {
		return
	}

	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonCreated, "Successfully created AIS cluster")
	return
}

// handlerCREvents matches the AIS cluster state obtained from reconciler request against the existing cluster state.
// It applies changes to cluster resources to ensure the request state is reached.
// Stages:
//  1. Check if the proxy daemon resources have a state (e.g. replica count) that matches the latest cluster spec.
//     If not, update the state to match the request spec and requeue the request. If they do, proceed to next set of checks.
//  2. Similarly, check the resource state for targets and ensure the state matches the reconciler request.
//  3. Check if config is properly updated in the cluster.
//  4. If expected state is not yet met we should reconcile until everything is ready.
func (r *AIStoreReconciler) handleCREvents(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var proxyReady, targetReady, configReady bool
	if proxyReady, err = r.handleProxyState(ctx, ais); err != nil {
		return
	} else if !proxyReady {
		goto requeue
	}

	if targetReady, err = r.handleTargetState(ctx, ais); err != nil {
		return
	} else if !targetReady {
		goto requeue
	}

	if configReady, err = r.handleConfigState(ctx, ais); err != nil {
		return
	} else if !configReady {
		goto requeue
	}

	return r.handleSuccessfulReconcile(ctx, ais)

requeue:
	// We requeue till the AIStore cluster becomes ready.
	if ais.IsConditionTrue(aisv1.ConditionReady) {
		ais.UnsetConditionReady(aisv1.ReasonUpgrading, "Waiting for cluster to upgrade")
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ClusterUpgrading})
	}
	return
}

// handleConfigState properly reconciles any changes in `.spec.configToUpdate` field.
//
// ConfigMap that also contain the value of this field is updated earlier, but
// we have to make sure that the cluster also has expected config.
func (r *AIStoreReconciler) handleConfigState(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	currentHash := ais.Spec.ConfigToUpdate.Hash()
	if ais.Annotations[configHashAnnotation] == currentHash {
		return true, nil
	}

	// Update cluster config based on what we have in the CRD spec.
	baseParams, err := r.getAPIParams(ctx, ais)
	if err != nil {
		return false, err
	}
	configToSet, err := ais.Spec.ConfigToUpdate.Convert()
	if err != nil {
		return false, err
	}
	err = aisapi.SetClusterConfigUsingMsg(*baseParams, configToSet, false /*transient*/)
	if err != nil {
		return false, err
	}

	// Finally update CRD with proper annotation.
	if ais.Annotations == nil {
		ais.Annotations = map[string]string{}
	}
	ais.Annotations[configHashAnnotation] = currentHash
	if err := r.client.Update(ctx, ais); err != nil {
		return false, err
	}

	return true, nil
}

func (r *AIStoreReconciler) createOrUpdateRBACResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if err = r.client.CreateOrUpdateResource(ctx, nil, sa); err != nil {
		r.recordError(ctx, ais, err, "Failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	role := cmn.NewAISRBACRole(ais)
	if err = r.client.CreateOrUpdateResource(ctx, nil, role); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Role")
		return
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRBACRoleBinding(ais)
	if err = r.client.CreateOrUpdateResource(ctx, nil, rb); err != nil {
		r.recordError(ctx, ais, err, "Failed to create RoleBinding")
		return
	}

	// 4. Create AIS ClusterRole
	cluRole := cmn.NewAISRBACClusterRole(ais)
	if err = r.client.CreateOrUpdateResource(ctx, nil, cluRole); err != nil {
		r.recordError(ctx, ais, err, "Failed to create ClusterRole")
		return
	}

	// 5. Create binding for ClusterRole
	crb := cmn.NewAISRBACClusterRoleBinding(ais)
	if err = r.client.CreateOrUpdateResource(ctx, nil, crb); err != nil {
		r.recordError(ctx, ais, err, "Failed to create ClusterRoleBinding")
		return
	}

	return
}

func (r *AIStoreReconciler) setStatus(ctx context.Context, ais *aisv1.AIStore, status aisv1.AIStoreStatus) error {
	logger := logf.FromContext(ctx)
	if status.State != "" {
		logger.Info("Updating AIS state", "state", status.State)
		ais.SetState(status.State)
	}

	if err := r.client.Status().Update(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to update CR status")
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aisv1.AIStore{}).
		Owns(&apiv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// hasValidBaseParams checks if the BaseParams are valid for the given AIS cluster configuration
func hasValidBaseParams(baseParams *aisapi.BaseParams, ais *aisv1.AIStore) bool {
	// Determine whether HTTPS should be used based on the presence of a TLS secret
	shouldUseHTTPS := ais.Spec.TLSSecretName != nil

	// Verify if the URL's protocol matches the expected protocol (HTTPS or HTTP)
	httpsCheck := cos.IsHTTPS(baseParams.URL) == shouldUseHTTPS

	// Check if the token and AuthN secret are correctly aligned:
	// - Valid if both are either set or both are unset
	authNCheck := (baseParams.Token == "" && ais.Spec.AuthNSecretName == nil) ||
		(baseParams.Token != "" && ais.Spec.AuthNSecretName != nil)

	return httpsCheck && authNCheck
}

// getAPIParams gets BaseAPIParams for the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
func (r *AIStoreReconciler) getAPIParams(ctx context.Context,
	ais *aisv1.AIStore,
) (baseParams *aisapi.BaseParams, err error) {
	r.mu.RLock()
	baseParams, exists := r.clientParams[ais.NamespacedName().String()]
	if exists && hasValidBaseParams(baseParams, ais) {
		r.mu.RUnlock()
		return
	}
	r.mu.RUnlock()
	baseParams, err = r.newAISBaseParams(ctx, ais)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to get AIS API parameters")
		return
	}
	r.mu.Lock()
	r.clientParams[ais.NamespacedName().String()] = baseParams
	r.mu.Unlock()
	return
}

func (r *AIStoreReconciler) handleSuccessfulReconcile(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var needsUpdate bool
	if !ais.IsConditionTrue(aisv1.ConditionReady) {
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonReady, "Successfully reconciled AIStore cluster")
		ais.SetCondition(aisv1.ConditionReady)
		needsUpdate = true
	}
	if !ais.HasState(aisv1.ClusterReady) {
		ais.SetState(aisv1.ClusterReady)
		needsUpdate = true
	}
	if needsUpdate {
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{})
	}
	return
}

func (r *AIStoreReconciler) recordError(ctx context.Context, ais *aisv1.AIStore, err error, msg string) {
	logf.FromContext(ctx).Error(err, msg)
	r.recorder.Eventf(ais, corev1.EventTypeWarning, EventReasonFailed, "%s, err: %v", msg, err)
}

func (r *AIStoreReconciler) primaryBaseParams(ctx context.Context, ais *aisv1.AIStore) (params *aisapi.BaseParams, err error) {
	baseParams, err := r.getAPIParams(ctx, ais)
	if err != nil {
		return nil, err
	}
	smap, err := r.GetSmap(ctx, baseParams)
	if err != nil {
		return nil, err
	}
	return _baseParams(smap.Primary.URL(aiscmn.NetPublic), baseParams.Token), nil
}

func (*AIStoreReconciler) GetSmap(ctx context.Context, params *aisapi.BaseParams) (*aismeta.Smap, error) {
	logger := logf.FromContext(ctx)
	smap, err := aisapi.GetClusterMap(*params)
	if err != nil {
		logger.Error(err, "Failed to get cluster map")
		return nil, err
	}
	return smap, nil
}

func (r *AIStoreReconciler) newAISBaseParams(ctx context.Context,
	ais *aisv1.AIStore,
) (params *aisapi.BaseParams, err error) {
	var (
		serviceHostname string
		token           string
	)
	// If LoadBalancer is configured and `isExternal` flag is set use the LB service to contact the API.
	if r.isExternal && ais.Spec.EnableExternalLB {
		var proxyLBSVC *corev1.Service
		proxyLBSVC, err = r.client.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
		if err != nil {
			return nil, err
		}

		for _, ing := range proxyLBSVC.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				serviceHostname = ing.IP
				goto createParams
			}
		}
		err = fmt.Errorf("failed to fetch LoadBalancer service %q, err: %v", proxy.LoadBalancerSVCNSName(ais), err)
		return
	}

	// When operator is deployed within K8s cluster with no external LoadBalancer,
	// use the proxy headless service to request the API.
	serviceHostname = proxy.HeadlessSVCNSName(ais).Name + "." + ais.Namespace
createParams:
	var scheme string
	if ais.Spec.TLSSecretName == nil {
		scheme = "http"
	} else {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%s", scheme, serviceHostname, ais.Spec.ProxySpec.ServicePort.String())

	// Get admin token if AuthN is enabled
	token, err = r.getAdminToken(ais)
	if err != nil {
		return nil, err
	}

	return _baseParams(url, token), nil
}

func _baseParams(url, token string) *aisapi.BaseParams {
	transportArgs := aiscmn.TransportArgs{
		Timeout:         10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := aiscmn.NewTransport(transportArgs)

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &aisapi.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.Timeout,
		},
		URL:   url,
		Token: token,
		UA:    userAgent,
	}
}
