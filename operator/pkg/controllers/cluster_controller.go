// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aisapi "github.com/NVIDIA/aistore/api"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	aisclient "github.com/ais-operator/pkg/client"
)

const (
	aisFinalizer = "finalize.ais"

	requeueInterval = 10 * time.Second
	errBackOffTime  = 10 * time.Second
)

type (

	// AIStoreReconciler reconciles a AIStore object
	AIStoreReconciler struct {
		sync.RWMutex
		client       *aisclient.K8sClient
		log          logr.Logger
		recorder     record.EventRecorder
		clientParams map[string]*aisapi.BaseParams
		isExternal   bool // manager is deployed externally to K8s cluster
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
	ais, err := r.client.GetAIStoreCR(ctx, req.NamespacedName)
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

	if !r.isInitialized(ais) {
		if !controllerutil.ContainsFinalizer(ais, aisFinalizer) {
			controllerutil.AddFinalizer(ais, aisFinalizer)
			err = r.client.Update(ctx, ais)
			return reconcile.Result{}, err
		}

		ais.SetConditionInitialized()
		retry, err := r.setStatus(ctx, ais,
			aisv1.AIStoreStatus{State: aisv1.ConditionInitialized})
		return reconcile.Result{Requeue: retry}, err
	}

	if !ais.GetDeletionTimestamp().IsZero() {
		if !hasFinalizer(ais) {
			return reconcile.Result{}, nil
		}
		updated, err := r.cleanup(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to delete instance")
			return r.manageError(ctx, ais, aisv1.InstanceDeletionError, err)
		}
		if updated {
			return reconcile.Result{RequeueAfter: requeueInterval}, nil
		}
		controllerutil.RemoveFinalizer(ais, aisFinalizer)
		err = r.client.UpdateIfExists(ctx, ais)
		if err != nil {
			r.recordError(ais, err, "Failed to update instance")
			return r.manageError(ctx, ais, aisv1.ResourceUpdateError, err)
		}
		return reconcile.Result{}, nil
	}

	if r.isNewCR(ais) {
		return r.bootstrapNew(ctx, ais)
	}

	return r.handleCREvents(ctx, ais)
}

func (r *AIStoreReconciler) cleanup(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) { return r.cleanupTarget(ctx, ais) },
		func() (bool, error) { return r.cleanupProxy(ctx, ais) },
		func() (bool, error) { return r.client.DeleteConfigMapIfExists(ctx, statsd.ConfigMapNSName(ais)) },
		func() (bool, error) { return r.cleanupRBAC(ctx, ais) },
		func() (bool, error) { return r.cleanupVolumes(ctx, ais) },
	)
}

func (r *AIStoreReconciler) cleanupVolumes(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	if ais.Spec.DeletePVCs == nil || !*ais.Spec.DeletePVCs {
		return
	}
	return r.client.DeleteAllPVCsIfExist(ctx, ais.Namespace, target.PodLabels(ais))
}

func (r *AIStoreReconciler) cleanupRBAC(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) {
			crb := cmn.NewAISRBACClusterRoleBinding(ais)
			return r.client.DeleteResourceIfExists(ctx, crb)
		},
		func() (bool, error) {
			cluRole := cmn.NewAISRBACClusterRole(ais)
			return r.client.DeleteResourceIfExists(ctx, cluRole)
		},
		func() (bool, error) {
			rb := cmn.NewAISRBACRoleBinding(ais)
			return r.client.DeleteResourceIfExists(ctx, rb)
		},
		func() (bool, error) {
			role := cmn.NewAISRBACRole(ais)
			return r.client.DeleteResourceIfExists(ctx, role)
		},
		func() (bool, error) {
			sa := cmn.NewAISServiceAccount(ais)
			return r.client.DeleteResourceIfExists(ctx, sa)
		},
	)
}

func hasFinalizer(ais *aisv1.AIStore) bool {
	for _, fin := range ais.GetFinalizers() {
		if fin == aisFinalizer {
			return true
		}
	}
	return false
}

func (r *AIStoreReconciler) bootstrapNew(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var (
		configToUpdate *aiscmn.ConfigToUpdate
		changed        bool
	)

	if ais.Spec.ConfigToUpdate != nil {
		if configToUpdate, err = r.getConfigToUpdate(ais.Spec.ConfigToUpdate); err != nil {
			return ctrl.Result{}, err
		}
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
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, statsDCM); err != nil {
		r.recordError(ais, err, "Failed to deploy StatsD ConfigMap")
		return r.manageError(ctx, ais, aisv1.ResourceCreationError, err)
	}

	// 4. Deploy global cluster config map.
	globalCM, err := cmn.NewGlobalCM(ais, configToUpdate)
	if err != nil {
		r.recordError(ais, err, "Failed to construct global config")
		return r.manageError(ctx, ais, aisv1.ConfigBuildError, err)
	}
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, globalCM); err != nil {
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
// 1. Check if the proxy daemon resources have a state (e.g. replica count) that matches the latest `ais` cluster spec.
//    If not, update the state to match the request spec and requeue the request. If they do, proceed to next set of checks.
// 2. Similarly, check the resource state for targets and ensure the state matches the reconciler request.
// 3. If both proxy and target daemons have expected state, keep requeuing the event until all the pods are ready.
func (r *AIStoreReconciler) handleCREvents(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var proxyReady, targetReady bool
	if proxyReady, err = r.handleProxyState(ctx, ais); err != nil {
		return
	}

	if targetReady, err = r.handleTargetState(ctx, ais); err != nil {
		return
	}

	if targetReady && proxyReady {
		return r.manageSuccess(ctx, ais)
	}

	// We requeue till the AIStore cluster becomes ready.
	// TODO: Remove explicit requeue after enabling event watchers for owned resources (e.g. proxy/target statefulsets).
	if ais.IsConditionTrue(aisv1.ConditionReady.Str()) {
		ais.UnsetConditionReady(aisv1.ConditionUpgrading.Str(), "Waiting for cluster to upgrade")
		_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionUpgrading})
	}
	result.RequeueAfter = 5 * time.Second
	return
}

func (r *AIStoreReconciler) createRBACResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, sa); err != nil {
		r.recordError(ais, err, "Failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	role := cmn.NewAISRBACRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, role); err != nil {
		r.recordError(ais, err, "Failed to create Role")
		return
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRBACRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, rb); err != nil {
		r.recordError(ais, err, "Failed to create RoleBinding")
		return
	}

	// 4. Create AIS ClusterRole
	cluRole := cmn.NewAISRBACClusterRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, cluRole); err != nil {
		errMsg := "Failed to create ClusterRole"
		r.recordError(ais, err, errMsg)
		return
	}

	// 5. Create binding for ClusterRole
	crb := cmn.NewAISRBACClusterRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, crb); err != nil {
		r.recordError(ais, err, "Failed to create ClusterRoleBinding")
	}
	return
}

func (r *AIStoreReconciler) isInitialized(ais *aisv1.AIStore) bool {
	r.log.Info("State: " + ais.Status.State.Str())
	return ais.Status.State != ""
}

func (r *AIStoreReconciler) setStatus(ctx context.Context, ais *aisv1.AIStore, status aisv1.AIStoreStatus) (retry bool, err error) {
	if status.State != "" {
		ais.SetState(status.State)
	}

	if err = r.client.Status().Update(ctx, ais); err != nil {
		if errors.IsConflict(err) {
			r.log.Info("Versions conflict updating CR")
			return true, nil
		}
		r.recordError(ais, err, "Failed to update CR status")
	}

	return
}

func (r *AIStoreReconciler) isNewCR(ais *aisv1.AIStore) (isNew bool) {
	return !ais.IsConditionTrue(aisv1.ConditionCreated.Str())
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aisv1.AIStore{}).
		Complete(r)
}

// getAPIParams gets BaseAPIParams for the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
// nolint:unused // will be used lated
func (r *AIStoreReconciler) getAPIParams(ctx context.Context,
	ais *aisv1.AIStore) (baseParams *aisapi.BaseParams, err error) {
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
func (r *AIStoreReconciler) manageError(ctx context.Context,
	ais *aisv1.AIStore, reason aisv1.ErrorReason, err error) (ctrl.Result, error) {
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
	if retry, err := r.setStatus(ctx, ais, aisv1.AIStoreStatus{}); err != nil || retry {
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
	if ais.Status.State != aisv1.ConditionReady {
		result.Requeue, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionReady})
	}

	return
}

func (r *AIStoreReconciler) getConfigToUpdate(cfg *aisv1.ConfigToUpdate) (toUpdate *aiscmn.ConfigToUpdate, err error) {
	toUpdate = &aiscmn.ConfigToUpdate{}
	err = cos.MorphMarshal(cfg, toUpdate)
	return toUpdate, err
}

func (r *AIStoreReconciler) recordError(ais *aisv1.AIStore, err error, msg string) {
	r.log.Error(err, msg)
	r.recorder.Eventf(ais, corev1.EventTypeWarning, EventReasonFailed, "%s, err: %v", msg, err)
}

// nolint:unused // will be used lated
func (r *AIStoreReconciler) newAISBaseParams(ctx context.Context,
	ais *aisv1.AIStore) (params *aisapi.BaseParams, err error) {
	// TODO:
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
