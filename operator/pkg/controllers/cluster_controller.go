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
	"os"
	"sync"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/api/env"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

	requeueInterval = 10 * time.Second
	errBackOffTime  = 10 * time.Second

	userAgent = "ais-operator"
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
	userName := os.Getenv(env.AuthN.AdminUsername)
	if userName == "" {
		userName = AuthNAdminUser
	}
	pass := os.Getenv(env.AuthN.AdminPassword)
	if pass == "" {
		pass = AuthNAdminPass
	}
	host := os.Getenv(AuthNServiceHostVar)
	if host == "" {
		host = AuthNServiceHostName
	}
	port := os.Getenv(AuthNServicePortVar)
	if port == "" {
		port = AuthNServicePort
	}

	return authNConfig{
		adminUser: userName,
		adminPass: pass,
		port:      port,
		host:      host,
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

	if !r.isInitialized(ais) {
		if result, err := r.initializeCR(ctx, ais); err != nil {
			return result, err
		}
	}

	if ais.ShouldDecommission() {
		_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionDecommissioning})
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonDeleted, "Decommissioning...")
	}

	if ais.ShouldShutdown() {
		_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionShuttingDown})
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonUpdated, "Shutting down...")
	}

	switch {
	case ais.HasState(aisv1.ConditionShuttingDown):
		return r.shutdownCluster(ctx, ais)
	case ais.HasState(aisv1.ConditionDecommissioning):
		return r.decommissionCluster(ctx, ais)
	case ais.HasState(aisv1.ConditionCleanup):
		return r.cleanupClusterRes(ctx, ais)
	case !ais.IsConditionTrue(aisv1.ConditionCreated.Str()):
		return r.bootstrapNew(ctx, ais)
	default:
		return r.handleCREvents(ctx, ais)
	}
}

func (r *AIStoreReconciler) getLogger(ais *aisv1.AIStore) logr.Logger {
	return r.log.WithValues("namespace", ais.Namespace, "name", ais.Name)
}

func (r *AIStoreReconciler) initializeCR(ctx context.Context, ais *aisv1.AIStore) (result reconcile.Result, err error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(ais, aisFinalizer) {
		logger.Info("Updating finalizer")
		controllerutil.AddFinalizer(ais, aisFinalizer)
		if err = r.client.Update(ctx, ais); err != nil {
			logger.Error(err, "Failed to update finalizer")
			return result, err
		}
		logger.Info("Successfully updated finalizer")
	}

	logger.Info("Updating state and setting condition", "state", aisv1.ConditionInitialized)
	ais.SetConditionInitialized()
	retry, err := r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionInitialized})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ConditionInitialized)
		return reconcile.Result{Requeue: retry}, err
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
	if err = r.scaleProxiesToZero(ctx, ais); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.scaleTargetsToZero(ctx, ais); err != nil {
		return reconcile.Result{}, err
	}
	_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionShutdown})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ConditionShutdown)
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
			logger.Info("Unable to gracefully decommission AIStore, retrying until cluster is not running")
		}
		return reconcile.Result{Requeue: true, RequeueAfter: requeueInterval}, nil
	}
	_, err := r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionCleanup})
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ConditionCleanup)
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
		r.recordError(ais, err, "Failed to cleanup AIS Resources")
		return r.manageError(ctx, ais, aisv1.InstanceDeletionError, err)
	}
	if updated {
		return reconcile.Result{RequeueAfter: requeueInterval}, nil
	}
	logger.Info("Removing AIS finalizer")
	controllerutil.RemoveFinalizer(ais, aisFinalizer)
	err = r.client.UpdateIfExists(ctx, ais)
	if err != nil {
		r.recordError(ais, err, "Failed to update instance")
		return r.manageError(ctx, ais, aisv1.ResourceUpdateError, err)
	}
	return reconcile.Result{}, nil
}

func (r *AIStoreReconciler) isClusterRunning(ctx context.Context, ais *aisv1.AIStore) bool {
	// Consider cluster running if both proxy and target ss have ready pods
	return r.ssHasReadyReplicas(ctx, target.StatefulSetNSName(ais)) && r.ssHasReadyReplicas(ctx, proxy.StatefulSetNSName(ais))
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
		logger.Error(err, "Failed to get API parameters")
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
		logger.Error(err, "Failed to get API parameters")
		return err
	}
	logger.Info("Attempting graceful shutdown of cluster")
	err = aisapi.ShutdownCluster(*params)
	if err != nil {
		logger.Error(err, "Failed to gracefully shutdown cluster")
	}
	return err
}

func (r *AIStoreReconciler) bootstrapNew(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var changed bool

	globalCM, err := cmn.NewGlobalCM(ais, ais.Spec.ConfigToUpdate)
	if err != nil {
		r.recordError(ais, err, "Failed to construct global config")
		return r.manageError(ctx, ais, aisv1.ConfigBuildError, err)
	}

	// Verify the Kubernetes cluster can support this deployment.
	err = r.verifyDeployment(ctx, ais)
	if err != nil {
		r.recordError(ais, err, "Failed to verify desired deployment compatibility with K8s cluster")
		// Don't use manageError, let k8s do a full exponential backoff by returning the error
		ais.IncErrorCount()
		ais.SetConditionError(aisv1.IncompatibleSpecError, err)
		return result, err
	}

	// 1. Create rbac resources
	err = r.createRBACResources(ctx, ais)
	if err != nil {
		return r.manageError(ctx, ais, aisv1.RBACManagementError, err)
	}

	// 2. Check if the cluster needs external access.
	// If yes, create a LoadBalancer services for targets and proxies and wait for external IP to be allocated.
	if ais.Spec.EnableExternalLB {
		var proxyReady, targetReady, retry bool
		proxyReady, err = r.enableProxyExternalService(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to enable proxy external service")
			return r.manageError(ctx, ais, aisv1.ExternalServiceError, err)
		}
		targetReady, err = r.enableTargetExternalService(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to enable target external service")
			return r.manageError(ctx, ais, aisv1.ExternalServiceError, err)
		}
		// When external access is enabled, we need external IPs of all the targets before deploying AIS cluster.
		// To ensure correct behavior of cluster, we requeue the reconciler till we have all the external IPs.
		if !targetReady || !proxyReady {
			if !ais.HasState(aisv1.ConditionInitializingLBService) && !ais.HasState(aisv1.ConditionPendingLBService) {
				retry, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionInitializingLBService})
				if !retry && err == nil {
					r.recorder.Event(ais, corev1.EventTypeNormal,
						EventReasonInitialized, "Successfully initialized LoadBalancer service")
				}
			} else {
				retry, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionPendingLBService})
				if !retry && err == nil {
					str := fmt.Sprintf("Waiting for LoadBalancer service to be ready; proxy ready=%t, target ready=%t", proxyReady, targetReady)
					r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonWaiting, str)
				}
			}
			result.RequeueAfter = requeueInterval
			return
		}
	}

	// 3. Deploy statsd config map. Required by both proxies and targets
	statsDCM := statsd.NewStatsDCM(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, ais, statsDCM); err != nil {
		r.recordError(ais, err, "Failed to deploy StatsD ConfigMap")
		return r.manageError(ctx, ais, aisv1.ResourceCreationError, err)
	}

	// 4. Deploy global cluster config map.
	if _, err = r.client.CreateOrUpdateResource(ctx, ais, globalCM); err != nil {
		r.recordError(ais, err, "Failed to deploy global cluster ConfigMap")
		return r.manageError(ctx, ais, aisv1.ResourceCreationError, err)
	}

	// 5. Bootstrap proxies
	if changed, err = r.initProxies(ctx, ais); err != nil {
		r.recordError(ais, err, "Failed to create Proxy resources")
		return r.manageError(ctx, ais, aisv1.ProxyCreationError, err)
	} else if changed {
		result.RequeueAfter = requeueInterval
		return
	}

	// 6. Bootstrap targets
	if changed, err = r.initTargets(ctx, ais); err != nil {
		r.recordError(ais, err, "Failed to create Target resources")
		return r.manageError(ctx, ais, aisv1.TargetCreationError, err)
	} else if changed {
		result.RequeueAfter = requeueInterval
		return
	}

	ais.SetConditionCreated()
	result.Requeue, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionCreated})
	if err != nil {
		return
	}
	if !result.Requeue {
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonCreated, "Successfully created AIS cluster")
	}
	return
}

// handlerCREvents matches the AIS cluster state obtained from reconciler request against the existing cluster state.
// It applies changes to cluster resources to ensure the request state is reached.
// Stages:
//  1. Check if the proxy daemon resources have a state (e.g. replica count) that matches the latest `ais` cluster spec.
//     If not, update the state to match the request spec and requeue the request. If they do, proceed to next set of checks.
//  2. Similarly, check the resource state for targets and ensure the state matches the reconciler request.
//  3. If both proxy and target daemons have expected state, keep requeuing the event until all the pods are ready.
func (r *AIStoreReconciler) handleCREvents(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	// Ensure correct RBAC resources exists
	err = r.createRBACResources(ctx, ais)
	if err != nil {
		return r.manageError(ctx, ais, aisv1.RBACManagementError, err)
	}

	var proxyReady, targetReady bool
	if proxyReady, err = r.handleProxyState(ctx, ais); err != nil {
		return
	}
	if !proxyReady {
		goto requeue
	}

	if targetReady, err = r.handleTargetState(ctx, ais); err != nil {
		return
	}

	if targetReady && proxyReady {
		return r.manageSuccess(ctx, ais)
	}

requeue:
	// We requeue till the AIStore cluster becomes ready.
	// TODO: Remove explicit requeue after enabling event watchers for owned resources (e.g. proxy/target statefulsets).
	if ais.IsConditionTrue(aisv1.ConditionReady.Str()) {
		ais.UnsetConditionReady(aisv1.ConditionUpgrading.Str(), "Waiting for cluster to upgrade")
		_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionUpgrading})
	}
	result.RequeueAfter = 5 * time.Second
	return
}

func (r *AIStoreReconciler) patchRole(ctx context.Context, ais *aisv1.AIStore, role *rbacv1.Role) error {
	sliceContains := func(keys []string, e string) bool {
		for _, v := range keys {
			if v == e {
				return true
			}
		}
		return false
	}
	existingRole, err := r.client.GetRole(ctx, types.NamespacedName{Namespace: role.Namespace, Name: role.Name})
	if err != nil {
		r.recordError(ais, err, "Failed to fetch Role")
		return err
	}

	for _, rule := range existingRole.Rules {
		if sliceContains(rule.Resources, cmn.ResourceTypePodsExec) {
			return nil
		}
	}
	if err = r.client.UpdateIfExists(ctx, role); err != nil {
		r.recordError(ais, err, "Failed updating Role")
	}
	return err
}

func (r *AIStoreReconciler) createRBACResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, nil, sa); err != nil {
		r.recordError(ais, err, "Failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	var (
		role   = cmn.NewAISRBACRole(ais)
		exists bool
	)

	if exists, err = r.client.CreateOrUpdateResource(ctx, nil, role); err != nil {
		r.recordError(ais, err, "Failed to create Role")
		return
	}

	// If the role already exists, ensure it has `pods/exec`.
	if exists {
		err = r.patchRole(ctx, ais, role)
		if err != nil {
			return
		}
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRBACRoleBinding(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, nil, rb); err != nil {
		r.recordError(ais, err, "Failed to create RoleBinding")
		return
	}

	// 4. Create AIS ClusterRole
	cluRole := cmn.NewAISRBACClusterRole(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, nil, cluRole); err != nil {
		r.recordError(ais, err, "Failed to create ClusterRole")
		return
	}

	// 5. Create binding for ClusterRole
	crb := cmn.NewAISRBACClusterRoleBinding(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, nil, crb); err != nil {
		r.recordError(ais, err, "Failed to create ClusterRoleBinding")
		return
	}

	return
}

func (r *AIStoreReconciler) isInitialized(ais *aisv1.AIStore) bool {
	r.getLogger(ais).Info("State: " + ais.Status.State.Str())
	return ais.Status.State != ""
}

func (r *AIStoreReconciler) setStatus(ctx context.Context, ais *aisv1.AIStore, status aisv1.AIStoreStatus) (retry bool, err error) {
	logger := logf.FromContext(ctx)
	if status.State != "" {
		logger.Info("Updating AIS state", "state", status.State)
		ais.SetState(status.State)
	}

	if err = r.client.Status().Update(ctx, ais); err != nil {
		if k8serrors.IsConflict(err) {
			logger.Info("Conflict updating CR status")
			return true, nil
		}
		r.recordError(ais, err, "Failed to update CR status")
	}

	return
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
	var exists bool
	r.mu.RLock()
	baseParams, exists = r.clientParams[ais.NamespacedName().String()]
	if exists && hasValidBaseParams(baseParams, ais) {
		r.mu.RUnlock()
		return
	}
	r.mu.RUnlock()
	baseParams, err = r.newAISBaseParams(ctx, ais)
	if err != nil {
		return
	}
	r.mu.Lock()
	r.clientParams[ais.NamespacedName().String()] = baseParams
	r.mu.Unlock()
	return
}

// misc helpers
func (r *AIStoreReconciler) manageError(ctx context.Context,
	ais *aisv1.AIStore, reason aisv1.ErrorReason, err error,
) (ctrl.Result, error) {
	var requeueAfter time.Duration
	condition, _ := ais.GetLastCondition()

	if reason.Equals(condition.Reason) {
		requeueAfter = errBackOffTime
	} else {
		// If the error with given reason occurred for the first time,
		// requeue immediately and reset the error count
		ais.ResetErrorCount()
	}

	ais.IncErrorCount()
	ais.SetConditionError(reason, err)
	if retry, statusErr := r.setStatus(ctx, ais, aisv1.AIStoreStatus{}); statusErr != nil || retry {
		// Status update failed, requeue immediately.
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, err
}

func (r *AIStoreReconciler) manageSuccess(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	ais.SetConditionSuccess()
	if !ais.IsConditionTrue(aisv1.ConditionReady.Str()) {
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonReady, "Created AIS cluster")
		ais.SetConditionReady()
	}
	if !ais.HasState(aisv1.ConditionReady) {
		result.Requeue, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionReady})
	}

	return
}

func (r *AIStoreReconciler) recordError(ais *aisv1.AIStore, err error, msg string) {
	r.getLogger(ais).Error(err, msg)
	r.recorder.Eventf(ais, corev1.EventTypeWarning, EventReasonFailed, "%s, err: %v", msg, err)
}

func (r *AIStoreReconciler) verifyDeployment(ctx context.Context, ais *aisv1.AIStore) error {
	if err := r.verifyNodesAvailable(ctx, ais, aisapc.Proxy); err != nil {
		return err
	}
	if err := r.verifyNodesAvailable(ctx, ais, aisapc.Target); err != nil {
		return err
	}
	return r.verifyRequiredStorageClasses(ctx, ais)
}

func (r *AIStoreReconciler) verifyNodesAvailable(ctx context.Context, ais *aisv1.AIStore, daeType string) error {
	var (
		requiredSize int
		nodeSelector map[string]string
		nodes        *corev1.NodeList
		err          error
	)
	switch daeType {
	case aisapc.Proxy:
		requiredSize = int(ais.GetProxySize())
		nodeSelector = ais.Spec.ProxySpec.NodeSelector
	case aisapc.Target:
		if ais.AllowTargetSharedNodes() {
			return nil
		}
		requiredSize = int(ais.GetTargetSize())
		nodeSelector = ais.Spec.TargetSpec.NodeSelector
	default:
		return nil
	}

	// Check that desired nodes matching this selector does not exceed available K8s cluster nodes
	nodes, err = r.client.ListNodesMatchingSelector(ctx, nodeSelector)
	if err != nil {
		r.recordError(ais, err, "Failed to list nodes matching provided selector")
		return err
	}
	if len(nodes.Items) >= requiredSize {
		return nil
	}
	return fmt.Errorf("spec for AIS %s requires more K8s nodes matching the given selector: expected '%d' but found '%d'", daeType, requiredSize, len(nodes.Items))
}

// Ensure all storage classes requested by the AIS resource are available in the cluster
func (r *AIStoreReconciler) verifyRequiredStorageClasses(ctx context.Context, ais *aisv1.AIStore) error {
	scMap, err := r.client.GetStorageClasses(ctx)
	if err != nil {
		return err
	}
	requiredClasses := []*string{ais.Spec.StateStorageClass}
	for _, requiredClass := range requiredClasses {
		if requiredClass != nil {
			if _, exists := scMap[*requiredClass]; !exists {
				return fmt.Errorf("required storage class '%s' not found", *requiredClass)
			}
		}
	}
	return nil
}

func (r *AIStoreReconciler) primaryBaseParams(ctx context.Context, ais *aisv1.AIStore) (params *aisapi.BaseParams, err error) {
	baseParams, err := r.getAPIParams(ctx, ais)
	if err != nil {
		return nil, err
	}
	smap, err := aisapi.GetClusterMap(*baseParams)
	if err != nil {
		return nil, err
	}
	return _baseParams(smap.Primary.URL(aiscmn.NetPublic), baseParams.Token), nil
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
	token, err = r.getAdminToken(ais, scheme)

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
