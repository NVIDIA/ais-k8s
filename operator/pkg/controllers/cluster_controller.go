// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"context"
	"time"

	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
		client *aisclient.K8SClient
		log    logr.Logger
	}

	daemonState struct {
		isUpdated bool
		isReady   bool
	}
)

func NewAISReconciler(mgr manager.Manager, logger logr.Logger) *AIStoreReconciler {
	return &AIStoreReconciler{
		client: aisclient.NewClientFromMgr(mgr),
		log:    logger,
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
		err = r.setState(ctx, ais, aisv1.AIStoreConiditionInitialized)
		controllerutil.AddFinalizer(ais, aisFinalizer)
		return reconcile.Result{}, err
	}

	if !ais.GetDeletionTimestamp().IsZero() {
		if !hasFinalizer(ais) {
			return reconcile.Result{}, nil
		}
		err := r.cleanup(ctx, ais)
		if err != nil {
			r.log.Error(err, "unable to delete instance", "instance", ais)
			return reconcile.Result{}, err
		}
		controllerutil.RemoveFinalizer(ais, aisFinalizer)
		err = r.client.UpdateIfExists(ctx, ais)
		if err != nil {
			r.log.Error(err, "unable to update instance", "instance", ais)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if r.isNewCR(ctx, ais) {
		return r.bootstrapNew(ctx, ais)
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

func (r *AIStoreReconciler) bootstrapNew(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	var (
		configToUpdate *aiscmn.ConfigToUpdate
		changed        bool
	)

	if ais.Spec.ConfigCRName != nil {
		if configToUpdate, err = r.getConfigToUpdate(ctx, types.NamespacedName{
			Name:      *ais.Spec.ConfigCRName,
			Namespace: ais.Namespace,
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Create rbac resources
	err = r.createRbacResources(ctx, ais)
	if err != nil {
		return
	}

	// 2. Check if the cluster needs external access, if yes, create a LoadBalancer services for targets and proxies and wait for external IP to be allocated.
	if ais.Spec.EnableExternalLB {
		var proxyReady, targetReady bool
		proxyReady, err = r.enableProxyExternalService(ctx, ais)
		if err != nil {
			r.log.Error(err, "failed to enable proxy external service")
			return
		}
		targetReady, err = r.enableTargetExternalService(ctx, ais)
		if err != nil {
			r.log.Error(err, "failed to enable target external service")
			return
		}
		// When external access is enabled, we need external IPs of all the targets before deploying the AIS cluster resources (proxies & targets).
		// To ensure correct behavior of cluster, we requeue the reconciler till all the external services are assigned an external IP.
		if !targetReady || !proxyReady {
			if !ais.HasState(aisv1.AIStoreConditionInitializingLBService) {
				err = r.setState(ctx, ais, aisv1.AIStoreConditionInitializingLBService)
			}
			result.Requeue = true
			result.RequeueAfter = 10 * time.Second
			return
		}
	}

	// 3. Deploy statsd config map. Required by both proxies and targets
	statsDCM := statsd.NewStatsDCM(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, statsDCM); err != nil {
		r.log.Error(err, "failed to deploy StatsD ConfigMap")
		err = r.setState(ctx, ais, aisv1.AIStoreConditionFailed)
		return
	}

	// 4. Bootstrap proxies
	if changed, err = r.initProxies(ctx, ais, configToUpdate); err != nil {
		r.log.Error(err, "failed to create Proxy resources")
		err = r.setState(ctx, ais, aisv1.AIStoreConditionFailed)
		return
	} else if changed {
		result.Requeue = true
		return
	}

	// 5. Bootstrap targets
	if changed, err = r.initTargets(ctx, ais, configToUpdate); err != nil {
		r.log.Error(err, "failed to create Target resources")
		err = r.setState(ctx, ais, aisv1.AIStoreConditionFailed)
		return
	} else if changed {
		result.Requeue = true
		return
	}
	err = r.setState(ctx, ais, aisv1.AIStoreConditionCreated)
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
		if !ais.HasState(aisv1.AIStoreConditionReady) {
			err = r.setState(ctx, ais, aisv1.AIStoreConditionReady)
		}
		return
	}

	result.RequeueAfter = statusRetryInterval
	// We requeue till the AIStore cluster becomes ready.
	// TODO: Remove explicit requeue after enabling event watchers for owned resources (e.g. proxy/target statefulsets).
updated:
	result.Requeue = true
	return
}

func (r *AIStoreReconciler) createRbacResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, sa); err != nil {
		r.log.Error(err, "failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	role := cmn.NewAISRbacRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, role); err != nil {
		r.log.Error(err, "failed to create Role")
		return
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRbacRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, rb); err != nil {
		r.log.Error(err, "failed to create RoleBinding")
		return
	}

	// 4. Create AIS ClusterRole
	cluRole := cmn.NewAISRbacClusterRole(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, cluRole); err != nil {
		r.log.Error(err, "failed to create ClusterRole")
		return
	}

	// 5. Create binding for ClusterRole
	crb := cmn.NewAISRbacClusterRoleBinding(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, nil, crb); err != nil {
		r.log.Error(err, "failed to create ClusterRoleBinding")
	}
	return
}

func (r *AIStoreReconciler) isInitialized(ais *aisv1.AIStore) bool {
	r.log.Info("State: " + string(ais.Status.State))
	return ais.Status.State != ""
}

func (r *AIStoreReconciler) setState(ctx context.Context, ais *aisv1.AIStore, state aisv1.AIStoreCondition) error {
	ais.SetState(state)
	return r.client.Status().Update(ctx, ais)
}

func (r *AIStoreReconciler) isNewCR(ctx context.Context, ais *aisv1.AIStore) (exists bool) {
	// TODO: check for other conditions
	return !ais.HasState(aisv1.AIStoreConditionCreated) && !ais.HasState(aisv1.AIStoreConditionReady)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aisv1.AIStore{}).
		Complete(r)
}

// misc helpers
func (r *AIStoreReconciler) getConfigToUpdate(ctx context.Context, name types.NamespacedName) (*aiscmn.ConfigToUpdate, error) {
	toUpdate := &aiscmn.ConfigToUpdate{}
	cfg, err := r.client.GetAIStoreConfCR(ctx, name)
	if err != nil {
		return nil, err
	}
	if err = aiscmn.MorphMarshal(cfg.Spec, toUpdate); err != nil {
		return nil, err
	}
	return toUpdate, err
}
