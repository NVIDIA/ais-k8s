// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	aisapi "github.com/NVIDIA/aistore/api"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	aisclient "github.com/ais-operator/pkg/client"
)

const (
	aisFinalizer = "finalize.ais"
	// Duration to requeue reconciler for status update.
	statusRetryInterval = 10 * time.Second
)

type (

	// AIStoreReconciler reconciles a AIStore object
	AIStoreReconciler struct {
		sync.RWMutex
		client       *aisclient.K8SClient
		log          logr.Logger
		recorder     record.EventRecorder
		clientParams map[string]*aisapi.BaseParams
		isExternal   bool // manager is deployed externally to K8s cluster
	}

	daemonState struct {
		isUpdated bool
		isReady   bool
	}
)

func NewAISReconciler(mgr manager.Manager, logger logr.Logger, isExternal bool) *AIStoreReconciler {
	return &AIStoreReconciler{
		client:       aisclient.NewClientFromMgr(mgr),
		log:          logger,
		recorder:     mgr.GetEventRecorderFor("ais-controller"),
		clientParams: make(map[string]*aisapi.BaseParams, 16),
		isExternal:   isExternal,
	}
}

// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ais.nvidia.com,resources=aistores/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AIStoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.log.WithValues("aistore", req.NamespacedName)
	var (
		configSpec *aisv1.AISConfig
		cfgVersion string
		ais, err   = r.client.GetAIStoreCR(ctx, req.NamespacedName)
	)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if ais.Spec.ConfigCRName != nil {
		configSpec, err = r.client.GetAIStoreConfCR(ctx, types.NamespacedName{Name: *ais.Spec.ConfigCRName, Namespace: ais.Namespace})
		if err != nil {
			return reconcile.Result{}, err
		}
		var isOwner bool
		for _, ref := range configSpec.OwnerReferences {
			if ref.UID == ais.UID {
				isOwner = true
				break
			}
		}

		if !isOwner {
			// NOTE: AIStore CR owns the AISConfig CR it references. Ensure that owner reference can be set.
			// AISConfig CR can only be owned by a single AIStore CR, if not setting owner reference would fail.
			if err = controllerutil.SetControllerReference(ais, configSpec, r.client.Scheme()); err != nil {
				r.recordError(ais, err, "Failed to set controller reference")
				return reconcile.Result{}, err
			}
			if err = r.client.Update(ctx, configSpec); err != nil {
				r.recordError(ais, err, "Failed to update configSpec")
				return reconcile.Result{}, err
			}
			// reference updated, reque the request
			return reconcile.Result{Requeue: true}, nil
		}
		cfgVersion = configSpec.GetResourceVersion()
	}

	if !r.isInitialized(ais) {
		ais.SetConditionInitialized()
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionInitialized, ConfigResourceVersion: cfgVersion})
		controllerutil.AddFinalizer(ais, aisFinalizer)
		return reconcile.Result{}, err
	}

	if !ais.GetDeletionTimestamp().IsZero() {
		if !hasFinalizer(ais) {
			return reconcile.Result{}, nil
		}
		err := r.cleanup(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to delete instance")
			return reconcile.Result{}, err
		}
		controllerutil.RemoveFinalizer(ais, aisFinalizer)
		err = r.client.UpdateIfExists(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to update instance")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if r.isNewCR(ctx, ais) {
		return r.bootstrapNew(ctx, ais, configSpec)
	}

	// Check if AIS config CR is updated
	if configSpec != nil {
		if updated, err := r.handleConfigChange(ctx, ais, configSpec); updated || err != nil {
			return reconcile.Result{}, err
		}
	}
	return r.handleCREvents(ctx, ais)
}

func (r *AIStoreReconciler) cleanup(ctx context.Context, ais *aisv1.AIStore) error {
	if err := r.cleanupTarget(ctx, ais); err != nil {
		return err
	}

	if err := r.cleanupProxy(ctx, ais); err != nil {
		return err
	}

	// clean-up statsd
	return r.client.DeleteConfigMapIfExists(ctx, statsd.ConfigMapNSName(ais))
}

func hasFinalizer(ais *aisv1.AIStore) bool {
	for _, fin := range ais.GetFinalizers() {
		if fin == aisFinalizer {
			return true
		}
	}
	return false
}

func (r *AIStoreReconciler) bootstrapNew(ctx context.Context, ais *aisv1.AIStore, configSpec *aisv1.AISConfig) (result ctrl.Result, err error) {
	var (
		configToUpdate *aiscmn.ConfigToUpdate
		changed        bool
	)

	if ais.Spec.ConfigCRName != nil {
		if configToUpdate, err = r.getConfigToUpdate(configSpec); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Create rbac resources
	err = r.createRbacResources(ctx, ais)
	if err != nil {
		return
	}

	// 2. Check if the cluster needs external access.
	// If yes, create a LoadBalancer services for targets and proxies and wait for external IP to be allocated.
	if ais.Spec.EnableExternalLB {
		var proxyReady, targetReady bool
		proxyReady, err = r.enableProxyExternalService(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to enable proxy external service")
			return
		}
		targetReady, err = r.enableTargetExternalService(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to enable target external service")
			return
		}
		// When external access is enabled, we need external IPs of all the targets before deploying the AIS cluster resources (proxies & targets).
		// To ensure correct behavior of cluster, we requeue the reconciler till all the external services are assigned an external IP.
		if !targetReady || !proxyReady {
			if !ais.HasState(aisv1.ConditionInitializingLBService) {
				err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionInitializingLBService})
				r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, "Successfully initialized LoadBalancer service")
			} else {
				err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionPendingLBService})
				r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonWaiting, "Waiting for LoadBalancer service to be ready")
			}
			result.Requeue = true
			result.RequeueAfter = 10 * time.Second
			return
		}
	}

	// 3. Deploy statsd config map. Required by both proxies and targets
	statsDCM := statsd.NewStatsDCM(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, statsDCM); err != nil {
		r.recordError(ais, err, "Failed to deploy StatsD ConfigMap")
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionFailed})
		return
	}

	// 4. Bootstrap proxies
	if changed, err = r.initProxies(ctx, ais, configToUpdate); err != nil {
		r.recordError(ais, err, "Failed to create Proxy resources")
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionFailed})
		return
	} else if changed {
		result.Requeue = true
		return
	}

	// 5. Bootstrap targets
	if changed, err = r.initTargets(ctx, ais, configToUpdate); err != nil {
		r.recordError(ais, err, "Failed to create Target resources")
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionFailed})
		return
	} else if changed {
		result.Requeue = true
		return
	}
	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonCreated, "Successfully created AIS cluster")
	ais.SetConditionReady()
	err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionCreated})
	return
}

// handlerCREvents matches the AIS cluster state obtained from reconciler request against the existing cluster state.
// It applies changes to cluster resources to ensure the request state is reached.
// Stages:
// 1. Check if the proxy daemon resources have a state (e.g. replica count) that matches the latest `ais` cluster spec.
//    If not, update the state to match the request spec and requeue the reconciler request. If they do, proceed to next set of checks.
// 2. Similarly, check the resource state for targets and ensure the state matches the reconciler request.
// 3. If the both proxy and target daemons have expected state, check if they have reached the ready state.
//    If the resources aren't ready, requeue the reconciler till ready state is reached and update the status of AIS cluster resource.
func (r *AIStoreReconciler) handleCREvents(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var proxyState, targetState daemonState
	if proxyState, err = r.handleProxyState(ctx, ais); err != nil {
		return
	}
	if proxyState.isUpdated {
		goto updated
	}

	if targetState, err = r.handleTargetState(ctx, ais); err != nil {
		return
	}

	if targetState.isUpdated {
		goto updated
	}

	if targetState.isReady && proxyState.isReady {
		if !ais.IsConditionTrue(string(aisv1.ConditionReady)) {
			r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonReady, "Created AIS cluster")
			ais.SetConditionReady()
			err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionReady})
		}

		if err != nil {
			return
		}
		return
	}

	result.RequeueAfter = statusRetryInterval
	// We requeue till the AIStore cluster becomes ready.
	// TODO: Remove explicit requeue after enabling event watchers for owned resources (e.g. proxy/target statefulsets).
updated:
	if !ais.IsConditionTrue(string(aisv1.ConditionReady)) {
		ais.UnsetConditionReady(string(aisv1.ConditionUpgrading), "Waiting for cluster to upgrade")
		err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionUpgrading})
	}
	result.Requeue = true
	return
}

func (r *AIStoreReconciler) createRbacResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, sa); err != nil {
		r.recordError(ais, err, "Failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	role := cmn.NewAISRbacRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, role); err != nil {
		r.recordError(ais, err, "Failed to create Role")
		return
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRbacRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, rb); err != nil {
		r.recordError(ais, err, "Failed to create RoleBinding")
		return
	}

	// 4. Create AIS ClusterRole
	cluRole := cmn.NewAISRbacClusterRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, cluRole); err != nil {
		errMsg := "Failed to create ClusterRole"
		r.recordError(ais, err, errMsg)
		return
	}

	// 5. Create binding for ClusterRole
	crb := cmn.NewAISRbacClusterRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, crb); err != nil {
		r.recordError(ais, err, "Failed to create ClusterRoleBinding")
	}
	return
}

func (r *AIStoreReconciler) isInitialized(ais *aisv1.AIStore) bool {
	r.log.Info("State: " + string(ais.Status.State))
	return ais.Status.State != ""
}

func (r *AIStoreReconciler) setStatus(ctx context.Context, ais *aisv1.AIStore, status aisv1.AIStoreStatus) (err error) {
	if status.State != "" {
		ais.SetState(status.State)
	}

	if status.ConfigResourceVersion != "" {
		ais.Status.ConfigResourceVersion = status.ConfigResourceVersion
	}
	err = r.client.Status().Update(ctx, ais)
	if err != nil {
		r.recordError(ais, err, "Failed to update CR status")
	}
	return
}

func (r *AIStoreReconciler) isNewCR(ctx context.Context, ais *aisv1.AIStore) (isNew bool) {
	return !ais.IsConditionTrue(string(aisv1.ConditionCreated))
}

// handleConfigChange checks if the `AISConfig` CR has been updated.
// We check for updates using the last ResourceVersion recorded in `AIStore` CR's status against the current ResourceVersion of `AISConfig`.
// If we observe a higher version, we use the AIS API to update cluster config, and set the `ConfigResourceVersion` status field to the latest version.
func (r *AIStoreReconciler) handleConfigChange(ctx context.Context, ais *aisv1.AIStore, configSpec *aisv1.AISConfig) (updated bool, err error) {
	if ais.Status.ConfigResourceVersion == configSpec.GetResourceVersion() {
		return
	}

	currVer, _ := strconv.ParseInt(ais.Status.ConfigResourceVersion, 10, 64)
	newVer, _ := strconv.ParseInt(configSpec.GetResourceVersion(), 10, 64)
	if newVer < currVer {
		return
	}
	var toUpdate *aiscmn.ConfigToUpdate
	updated = true
	toUpdate, err = r.getConfigToUpdate(configSpec)
	if err != nil {
		r.recordError(ais, err, "Failed to convert config CR to key-value pair")
		return
	}

	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		r.recordError(ais, err, fmt.Sprintf("Failed to fetch BaseParams for cluster %q", ais.NamespacedName().String()))
		return
	}

	// Config has changed, ensure config is applied to AIS deployment.
	err = aisapi.SetClusterConfigUsingMsg(*params, toUpdate)
	if err != nil {
		r.recordError(ais, err, "Failed to update AIS cluster config")
		return
	}

	err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{ConfigResourceVersion: configSpec.GetResourceVersion()})
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aisv1.AIStore{}).
		Watches(&source.Kind{Type: &aisv1.AISConfig{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &aisv1.AIStore{},
		}).
		Complete(r)
}

// getAPIParams gets BaseAPIParams for the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
func (r *AIStoreReconciler) getAPIParams(ctx context.Context, ais *aisv1.AIStore) (baseParams *aisapi.BaseParams, err error) {
	var exists bool
	r.RLock()
	if baseParams, exists = r.clientParams[ais.NamespacedName().String()]; exists {
		r.RUnlock()
		return
	}
	r.RUnlock()
	baseParams, err = r.newAISBaseParams(ctx, ais)
	if err != nil {
		return
	}
	r.Lock()
	r.clientParams[ais.NamespacedName().String()] = baseParams
	r.Unlock()
	return
}

// misc helpers
func (r *AIStoreReconciler) getConfigToUpdate(cfg *aisv1.AISConfig) (toUpdate *aiscmn.ConfigToUpdate, err error) {
	toUpdate = &aiscmn.ConfigToUpdate{}
	if err = aiscmn.MorphMarshal(cfg.Spec, toUpdate); err != nil {
		return nil, err
	}
	return toUpdate, err
}

func (r *AIStoreReconciler) recordError(ais *aisv1.AIStore, err error, msg string) {
	r.log.Error(err, msg)
	r.recorder.Eventf(ais, corev1.EventTypeWarning, EventReasonFailed, "%s, err: %v", msg, err)
}

func (r *AIStoreReconciler) newAISBaseParams(ctx context.Context, ais *aisv1.AIStore) (params *aisapi.BaseParams, err error) {
	// TODOs:
	// 1. Get timeout from config
	// 2. `UseHTTPS` should be set based on cluster config
	// 3. Should handle auth
	var (
		serviceHostname string
		client          = aiscmn.NewClient(aiscmn.TransportArgs{
			Timeout:          600 * time.Second,
			IdleConnsPerHost: 100,
			UseHTTPProxyEnv:  true,
			UseHTTPS:         false,
		})
	)

	// If LoadBalancer is configured and `isExternal` flag is set use the LB service to contact the API.
	if r.isExternal && ais.Spec.EnableExternalLB {
		var proxyLBSVC *corev1.Service
		proxyLBSVC, err = r.client.GetServiceByName(ctx, proxy.LoadBalancerSVCNSName(ais))
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
	params = &aisapi.BaseParams{
		Client: client,
		URL:    fmt.Sprintf("http://%s:%s", serviceHostname, ais.Spec.ProxySpec.ServicePort.String()),
	}
	return
}
