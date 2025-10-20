// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/pkg/services"
	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	aisFinalizer            = "finalize.ais"
	aisShutdownRequeueDelay = 5 * time.Second
)

type (
	// AIStoreReconciler reconciles a AIStore object
	AIStoreReconciler struct {
		k8sClient     *aisclient.K8sClient
		log           logr.Logger
		recorder      record.EventRecorder
		clientManager services.AISClientManagerInterface
	}
)

func NewAISReconciler(c *aisclient.K8sClient, recorder record.EventRecorder, logger logr.Logger, clientManager services.AISClientManagerInterface) *AIStoreReconciler {
	return &AIStoreReconciler{
		k8sClient:     c,
		log:           logger,
		recorder:      recorder,
		clientManager: clientManager,
	}
}

func NewAISReconcilerFromMgr(mgr manager.Manager, aisClientTLSOpts services.AISClientTLSOpts, logger logr.Logger) *AIStoreReconciler {
	c := aisclient.NewClientFromMgr(mgr)
	recorder := mgr.GetEventRecorderFor("ais-controller")
	clientManager := services.NewAISClientManager(c, aisClientTLSOpts)
	return NewAISReconciler(c, recorder, logger, clientManager)
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
	ais, err := r.k8sClient.GetAIStoreCR(ctx, req.NamespacedName)
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
	logger.Info("Reconciling AIStore", "state", ais.Status.State)

	if ais.HasState("") {
		if err := r.initializeCR(ctx, ais); err != nil {
			return reconcile.Result{}, err
		}
	}

	if ais.IsTargetAutoScaling() || ais.IsProxyAutoScaling() {
		if err := r.determineAutoScaleStatus(ctx, ais); err != nil {
			return reconcile.Result{}, err
		}
	}

	if ais.ShouldDecommission() {
		err = r.updateStatusWithState(ctx, ais, aisv1.ClusterDecommissioning)
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonDeleted, "Decommissioning...")
	}

	if ais.ShouldStartShutdown() {
		logger.Info("Disabling rebalance before shutting down cluster")
		err = r.disableRebalance(ctx, ais, aisv1.ReasonShutdown, "Disabling rebalance before shutdown")
		if err != nil {
			logger.Error(err, "Failed to disable rebalance before shutdown")
			return reconcile.Result{}, err
		}
		err = r.updateStatusWithState(ctx, ais, aisv1.ClusterShuttingDown)
		if err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonUpdated, "Shutting down...")
	}

	switch {
	case ais.HasState(aisv1.ClusterShuttingDown):
		if !ais.ShouldBeShutdown() {
			// Aborts shutdown process -- reset state and reconcile back to normal
			err = r.updateStatusWithState(ctx, ais, aisv1.ClusterUpgrading)
			return reconcile.Result{}, err
		}
		return r.shutdownCluster(ctx, ais)
	case ais.HasState(aisv1.ClusterShutdown):
		// Remain in shutdown state unless the spec field changes
		if ais.ShouldBeShutdown() {
			return reconcile.Result{}, nil
		}
	case ais.HasState(aisv1.ClusterDecommissioning):
		return r.decommissionCluster(ctx, ais)
	case ais.HasState(aisv1.ClusterCleanup):
		return r.cleanupClusterRes(ctx, ais)
	case ais.HasState(aisv1.HostCleanup):
		return r.cleanupHost(ctx, ais)
	case ais.HasState(aisv1.ClusterFinalized):
		return r.finalize(ctx, ais)
	}

	if result, err := r.ensurePrereqs(ctx, ais); err != nil || !result.IsZero() {
		return result, err
	}

	if ais.Status.IntraClusterURL == "" {
		err = r.updateIntraClusterURLStatus(ctx, ais)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if !ais.IsConditionTrue(aisv1.ConditionCreated) {
		return r.bootstrapNew(ctx, ais)
	}

	return r.handleCREvents(ctx, ais)
}

func (r *AIStoreReconciler) determineAutoScaleStatus(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	autoScaleStatus := aisv1.AutoScaleStatus{}
	// If autoScale is enabled figure out target nodes and proxy nodes
	if ais.IsTargetAutoScaling() {
		nodes, err := r.k8sClient.ListNodesMatchingSelector(ctx, ais.Spec.TargetSpec.NodeSelector)
		if err != nil {
			logger.Error(err, "Unable to fetch nodes for autoScaleStatus target")
			return err
		}

		targetNodes := make([]string, 0, len(nodes.Items))
		for i := range nodes.Items {
			node := nodes.Items[i]

			if !toleratesTaints(ais.Spec.TargetSpec.Tolerations, &node) {
				continue
			}
			targetNodes = append(targetNodes, node.Name)
		}
		logger.Info("Discovered autoScaleStatus target nodes", "targetNodes", targetNodes)
		slices.Sort(targetNodes)
		autoScaleStatus.ExpectedTargetNodes = targetNodes
	}

	if ais.IsProxyAutoScaling() {
		nodes, err := r.k8sClient.ListNodesMatchingSelector(ctx, ais.Spec.ProxySpec.NodeSelector)
		if err != nil {
			logger.Error(err, "Unable to fetch nodes for autoScaleStatus proxy")
			return err
		}
		proxyNodes := make([]string, 0, len(nodes.Items))
		for i := range nodes.Items {
			node := nodes.Items[i]

			if !toleratesTaints(ais.Spec.ProxySpec.Tolerations, &node) {
				continue
			}
			proxyNodes = append(proxyNodes, node.Name)
		}
		logger.Info("Discovered autoScaleStatus proxy nodes", "proxyNodes", proxyNodes)
		slices.Sort(proxyNodes)
		autoScaleStatus.ExpectedProxyNodes = proxyNodes
	}

	return r.updateAutoScaleStatus(ctx, ais, autoScaleStatus)
}

func toleratesTaints(tolerations []corev1.Toleration, node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		isTolerated := false
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(&taint) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false
		}
	}
	return true
}

func (r *AIStoreReconciler) initializeCR(ctx context.Context, ais *aisv1.AIStore) (err error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(ais, aisFinalizer) {
		logger.Info("Updating finalizer")
		original := ais.DeepCopy()
		controllerutil.AddFinalizer(ais, aisFinalizer)
		if err = r.k8sClient.Patch(ctx, ais, k8sclient.MergeFrom(original)); err != nil {
			logger.Error(err, "Failed to update finalizer")
			return err
		}
		logger.Info("Successfully updated finalizer")
	}

	logger.Info("Updating state and setting condition", "state", aisv1.ConditionInitialized)
	ais.SetCondition(aisv1.ConditionInitialized)
	err = r.updateStatusWithState(ctx, ais, aisv1.ClusterInitialized)
	if err != nil {
		logger.Error(err, "Failed to update state", "state", aisv1.ConditionInitialized)
		return err
	}
	logger.Info("Successfully updated state")

	return
}

func (r *AIStoreReconciler) shutdownCluster(ctx context.Context, ais *aisv1.AIStore) (result reconcile.Result, err error) {
	logger := logf.FromContext(ctx)

	// Scale proxy statefulset to 0 and wait for it to finish
	if _, err = r.k8sClient.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), 0); err != nil {
		return reconcile.Result{}, err
	}
	proxyFinished, err := r.k8sClient.IsStatefulSetSize(ctx, proxy.StatefulSetNSName(ais), 0)
	if err != nil || !proxyFinished {
		return reconcile.Result{RequeueAfter: aisShutdownRequeueDelay}, err
	}

	// Scale target statefulset to 0 and wait for it to finish
	if _, err = r.k8sClient.UpdateStatefulSetReplicas(ctx, target.StatefulSetNSName(ais), 0); err != nil {
		return reconcile.Result{}, err
	}
	targetFinished, err := r.k8sClient.IsStatefulSetSize(ctx, target.StatefulSetNSName(ais), 0)
	if err != nil || !targetFinished {
		return reconcile.Result{RequeueAfter: aisShutdownRequeueDelay}, err
	}

	err = r.updateStatusWithState(ctx, ais, aisv1.ClusterShutdown)
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
	err := r.updateStatusWithState(ctx, ais, aisv1.ClusterCleanup)
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
	err = r.updateStatusWithState(ctx, ais, aisv1.HostCleanup)
	return reconcile.Result{}, err
}

func (r *AIStoreReconciler) cleanupHost(ctx context.Context, ais *aisv1.AIStore) (reconcile.Result, error) {
	// Get cleanup jobs
	jobs, err := r.listCleanupJobs(ctx, ais.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Delete all finished or expired jobs
	remainingJobs, err := r.deleteFinishedJobs(ctx, jobs)
	if err != nil {
		return reconcile.Result{}, err
	}
	// If some still running, requeue
	if len(remainingJobs.Items) > 0 {
		return reconcile.Result{Requeue: true}, nil
	}
	// If all are gone, move to finalized stage
	err = r.updateStatusWithState(ctx, ais, aisv1.ClusterFinalized)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, err
}

func (r *AIStoreReconciler) finalize(ctx context.Context, ais *aisv1.AIStore) (result reconcile.Result, err error) {
	logger := logf.FromContext(ctx)
	logger.Info("Removing AIS finalizer")

	original := ais.DeepCopy()
	updated := controllerutil.RemoveFinalizer(ais, aisFinalizer)
	if !updated {
		return
	}
	if err = r.k8sClient.PatchIfExists(ctx, ais, k8sclient.MergeFrom(original)); err != nil {
		r.recordError(ctx, ais, err, "Failed to update instance")
		return
	}

	// Do not requeue if we've removed the finalizer -- CR should be removed
	return reconcile.Result{Requeue: false}, nil
}

func (r *AIStoreReconciler) isClusterRunning(ctx context.Context, ais *aisv1.AIStore) bool {
	// Consider cluster running if both proxy and target ss have ready pods
	return r.ssHasReadyReplicas(ctx, target.StatefulSetNSName(ais)) && r.ssHasReadyReplicas(ctx, proxy.StatefulSetNSName(ais))
}

func (r *AIStoreReconciler) ssHasReadyReplicas(ctx context.Context, name types.NamespacedName) bool {
	ss, err := r.k8sClient.GetStatefulSet(ctx, name)
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
	cleanupData := ais.Spec.CleanupData != nil && *ais.Spec.CleanupData
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return err
	}
	err = apiClient.DecommissionCluster(cleanupData)
	if err != nil {
		logger.Error(err, "Failed to gracefully decommission cluster")
	}
	return err
}

func (r *AIStoreReconciler) attemptGracefulShutdown(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return err
	}
	logger.Info("Attempting graceful shutdown of cluster")
	err = apiClient.ShutdownCluster()
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

	globalCM, err := cmn.NewGlobalCM(ais)
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
	if err = r.k8sClient.CreateOrUpdateResource(ctx, ais, statsDCM); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy StatsD ConfigMap")
		return err
	}

	// 3. Deploy global cluster ConfigMap.
	if err = r.k8sClient.CreateOrUpdateResource(ctx, ais, globalCM); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy global cluster ConfigMap")
		return err
	}

	// FIXME: We should also move the logic from `bootstrapNew` and `handleCREvents`.

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
				err = r.updateStatusWithState(ctx, ais, aisv1.ClusterInitializingLBService)
				if err == nil {
					r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, "Successfully initialized LoadBalancer service")
				}
			} else {
				err = r.updateStatusWithState(ctx, ais, aisv1.ClusterPendingLBService)
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
	if result, err = r.initProxies(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Proxy resources")
		return
	} else if !result.IsZero() {
		return
	}

	// 2. Bootstrap targets
	if result, err = r.initTargets(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Target resources")
		return
	} else if !result.IsZero() {
		return
	}

	ais.SetCondition(aisv1.ConditionCreated)
	err = r.updateStatusWithState(ctx, ais, aisv1.ClusterCreated)
	if err != nil {
		return
	}

	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonCreated, "Successfully created AIS cluster")
	return
}

// handleCREvents matches the AIS cluster state obtained from reconciler request against the existing cluster state.
// It applies changes to cluster resources to ensure the request state is reached.
// Stages:
//  1. Check if the proxy daemon resources have a state (e.g. replica count) that matches the latest cluster spec.
//     If not, update the state to match the request spec and requeue the request. If they do, proceed to next set of checks.
//  2. Similarly, check the resource state for targets and ensure the state matches the reconciler request.
//  3. Check if config is properly updated in the cluster.
//  4. If expected state is not yet met we should reconcile until everything is ready.
func (r *AIStoreReconciler) handleCREvents(ctx context.Context, ais *aisv1.AIStore) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if res, err := r.handleProxyState(ctx, ais); err != nil {
		return res, err
	} else if !res.IsZero() {
		return r.updateStatusAndRequeue(ctx, ais, res)
	}

	if res, err := r.handleTargetState(ctx, ais); err != nil {
		return res, err
	} else if !res.IsZero() {
		return r.updateStatusAndRequeue(ctx, ais, res)
	}

	ready, err := r.checkAISClusterReady(ctx, ais)
	if err != nil {
		return ctrl.Result{}, err
	} else if !ready {
		return r.updateStatusAndRequeue(ctx, ais, ctrl.Result{Requeue: true})
	}

	// Enable the rebalance condition (still respects the spec desired rebalance.Enabled property)
	err = r.enableRebalanceCondition(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to enable rebalance condition")
		return ctrl.Result{}, err
	}

	shouldRequeue, err := r.handleConfigState(ctx, ais, false /*force*/)
	if err != nil {
		return ctrl.Result{}, err
	} else if shouldRequeue {
		return r.updateStatusAndRequeue(ctx, ais, ctrl.Result{Requeue: true})
	}

	return ctrl.Result{}, r.handleSuccessfulReconcile(ctx, ais)
}

// updateStatusAndRequeue updates the cluster status to indicate it's upgrading when we need to requeue.
func (r *AIStoreReconciler) updateStatusAndRequeue(ctx context.Context, ais *aisv1.AIStore, result ctrl.Result) (ctrl.Result, error) {
	if !ais.IsConditionTrue(aisv1.ConditionReady) {
		return result, nil
	}

	ais.SetConditionFalse(aisv1.ConditionReady, aisv1.ReasonUpgrading, "Waiting for cluster to upgrade")
	if err := r.updateStatusWithState(ctx, ais, aisv1.ClusterUpgrading); err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}

// handleConfigState properly reconciles the AIS cluster config with the `.spec.configToUpdate` field and any other
// operator provided configs. It also updates the restart config annotation on the AIS resource to indicate that
// statefulsets should begin a rollout.
//
// The ConfigMap that also contains the global config is updated earlier, but
// this synchronizes any changes to the active loaded config in the cluster.
func (r *AIStoreReconciler) handleConfigState(ctx context.Context, ais *aisv1.AIStore, forceSync bool) (requeue bool, err error) {
	logger := logf.FromContext(ctx)
	// Get the config provided in spec plus any additional ones set by the operator
	conf, err := cmn.GenerateGlobalConfig(ais)
	if err != nil {
		logger.Error(err, "Error generating global config")
		return
	}

	newConfHash, err := r.updateClusterConfig(ctx, ais, conf, forceSync)
	if err != nil {
		return
	}
	restartAnnot, err := calcRestartConfigAnnotation(ais.Annotations[cmn.RestartConfigHashAnnotation], conf)
	if err != nil {
		logger.Error(err, "Error hashing restart configs")
		return
	}
	confChanged := newConfHash != ais.Annotations[cmn.ConfigHashAnnotation]
	restartChanged := restartAnnot != ais.Annotations[cmn.RestartConfigHashAnnotation]
	// We only care about re-queueing if the restart annotation changes and is not initial -- regular config is done syncing at this point
	requeue = restartChanged && !strings.HasSuffix(restartAnnot, cmn.RestartConfigHashInitial)
	// If nothing changed, we're done
	if !requeue && !confChanged {
		return
	}
	err = r.patchAISAnnotations(ctx, ais, newConfHash, restartAnnot)
	if err != nil {
		logger.Error(err, "Error patching AIS with latest annotations")
	}
	return
}

func (r *AIStoreReconciler) patchAISAnnotations(ctx context.Context, ais *aisv1.AIStore, confHash, restartHash string) error {
	original := ais.DeepCopy()
	if ais.Annotations == nil {
		ais.Annotations = map[string]string{}
	}
	if confHash != "" {
		ais.Annotations[cmn.ConfigHashAnnotation] = confHash
	}
	ais.Annotations[cmn.RestartConfigHashAnnotation] = restartHash
	return r.k8sClient.Patch(ctx, ais, k8sclient.MergeFrom(original))
}

// Given cluster config, compute the hash, update the cluster if it does not match, and return hash if changed
func (r *AIStoreReconciler) updateClusterConfig(ctx context.Context, ais *aisv1.AIStore, conf *aiscmn.ConfigToSet, forceSync bool) (newHash string, err error) {
	logger := logf.FromContext(ctx)
	confHash, err := cmn.HashGlobalConfig(conf)
	if err != nil {
		logger.Error(err, "Error hashing global config")
		return
	}

	// Hash is same and not forcing, do nothing
	if !forceSync && ais.Annotations[cmn.ConfigHashAnnotation] == confHash {
		return confHash, nil
	}
	// Update active cluster config to the new global config
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return
	}

	logger.Info("Updating cluster config to match spec via API")
	err = apiClient.SetClusterConfigUsingMsg(conf, false /*transient*/)
	if err != nil {
		return "", fmt.Errorf("failed to update cluster config: %w", err)
	}
	return confHash, nil
}

// Given a hash of configs that cause restart, return the annotation we should store in AIS
func calcRestartConfigAnnotation(annot string, conf *aiscmn.ConfigToSet) (string, error) {
	restartHash, err := cmn.HashRestartConfigs(conf)
	if err != nil {
		return "", err
	}
	switch {
	case annot == "":
		return restartHash + cmn.RestartConfigHashInitial, nil
	case strings.HasSuffix(annot, cmn.RestartConfigHashInitial):
		currentHash := strings.TrimSuffix(annot, cmn.RestartConfigHashInitial)
		// If annotation has initial tag, only update if hash part changes (no longer initial)
		if currentHash == restartHash {
			return annot, nil
		}
		return restartHash, nil
	default:
		return restartHash, nil
	}
}

func (r *AIStoreReconciler) createOrUpdateRBACResources(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Create service account if not exists
	sa := cmn.NewAISServiceAccount(ais)
	if err = r.k8sClient.CreateOrUpdateResource(ctx, nil, sa); err != nil {
		r.recordError(ctx, ais, err, "Failed to create ServiceAccount")
		return
	}

	// 2. Create AIS Role
	role := cmn.NewAISRBACRole(ais)
	if err = r.k8sClient.CreateOrUpdateResource(ctx, nil, role); err != nil {
		r.recordError(ctx, ais, err, "Failed to create Role")
		return
	}

	// 3. Create binding for the Role
	rb := cmn.NewAISRBACRoleBinding(ais)
	if err = r.k8sClient.CreateOrUpdateResource(ctx, nil, rb); err != nil {
		r.recordError(ctx, ais, err, "Failed to create RoleBinding")
		return
	}

	// Delete any previously created cluster roles and bindings for this cluster
	crbName := types.NamespacedName{Namespace: ais.Namespace, Name: cmn.ClusterRoleBindingName(ais)}
	if _, err = r.k8sClient.DeleteCRBindingIfExists(ctx, crbName); err != nil {
		r.recordError(ctx, ais, err, "Failed to delete ClusterRoleBinding")
	}
	crName := types.NamespacedName{Namespace: ais.Namespace, Name: cmn.ClusterRoleName(ais)}
	if _, err = r.k8sClient.DeleteClusterRoleIfExists(ctx, crName); err != nil {
		r.recordError(ctx, ais, err, "Failed to delete ClusterRole")
	}

	return
}

func (r *AIStoreReconciler) disableRebalance(ctx context.Context, ais *aisv1.AIStore, reason aisv1.ClusterConditionReason, msg string) error {
	logf.FromContext(ctx).Info("Disabling rebalance condition")
	ais.SetConditionFalse(aisv1.ConditionReadyRebalance, reason, msg)
	err := r.patchStatus(ctx, ais)
	if err != nil {
		return err
	}
	// Also disable in the live cluster (don't wait for config sync)
	// This function will update the annotation so future reconciliations can tell the config has been updated
	// Force to ensure we still disable rebalance when set disabled in spec (in case it was enabled manually)
	_, err = r.handleConfigState(ctx, ais, true /*force*/)
	return err
}

func (r *AIStoreReconciler) enableRebalanceCondition(ctx context.Context, ais *aisv1.AIStore) error {
	if ais.IsConditionTrue(aisv1.ConditionReadyRebalance) {
		return nil
	}
	logf.FromContext(ctx).Info("Enabling rebalance condition")
	// Note this does not force-enable rebalance, only allows the value from spec to be used again
	ais.SetCondition(aisv1.ConditionReadyRebalance)
	return r.patchStatus(ctx, ais)
}

func (r *AIStoreReconciler) updateAutoScaleStatus(ctx context.Context, ais *aisv1.AIStore, status aisv1.AutoScaleStatus) error {
	if slices.Equal(ais.Status.AutoScaleStatus.ExpectedProxyNodes, status.ExpectedProxyNodes) && slices.Equal(ais.Status.AutoScaleStatus.ExpectedTargetNodes, status.ExpectedTargetNodes) {
		return nil
	}
	logf.FromContext(ctx).Info("Updating autoScaleStatus", "status", status)
	ais.Status.AutoScaleStatus = status
	return r.patchStatus(ctx, ais)
}

func (r *AIStoreReconciler) updateIntraClusterURLStatus(ctx context.Context, ais *aisv1.AIStore) error {
	intraClusterURL := ais.GetIntraClusterURL()
	logf.FromContext(ctx).WithValues("intraClusterURL", intraClusterURL).Info("Updating AIS with intraClusterURL")
	ais.Status.IntraClusterURL = intraClusterURL
	return r.patchStatus(ctx, ais)
}

func (r *AIStoreReconciler) updateStatusWithState(ctx context.Context, ais *aisv1.AIStore, state aisv1.ClusterState) error {
	logf.FromContext(ctx).Info("Updating AIS state", "state", state)
	ais.SetState(state)
	return r.patchStatus(ctx, ais)
}

func (r *AIStoreReconciler) patchStatus(ctx context.Context, ais *aisv1.AIStore) error {
	patchBytes, err := json.Marshal(map[string]interface{}{
		"status": ais.Status,
	})
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to marshal AIS status")
		return err
	}
	patch := k8sclient.RawPatch(types.MergePatchType, patchBytes)

	err = r.k8sClient.Status().Patch(ctx, ais, patch)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to patch CR status")
	}
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodePredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !reflect.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
		},
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&aisv1.AIStore{}).
		Watches(&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.findAISClustersForNode),
			builder.WithPredicates(nodePredicate),
		).
		Owns(&apiv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *AIStoreReconciler) findAISClustersForNode(ctx context.Context, o k8sclient.Object) []reconcile.Request {
	logger := r.log.WithName("node-mapper").WithValues("object", o.GetName())

	// convert to node
	node, ok := o.(*corev1.Node)
	if !ok {
		logger.Error(fmt.Errorf("unexpected object type"), "Expected Node", "got", fmt.Sprintf("%T", o))
		return nil
	}

	logger.Info("Finding ais clusters for node")

	// Find all autoScale AIStore clusters
	aisList := &aisv1.AIStoreList{}
	if err := r.k8sClient.List(ctx, aisList); err != nil {
		logger.Error(err, "Failed to list ais crs")
		return nil
	}

	var requests []reconcile.Request

	for idx := range len(aisList.Items) {
		ais := aisList.Items[idx]
		// the match funcs check for two things: is the node in already in the node list and does it match the node selector
		// We check the expectedNode list, as we could be here because the node change labels.
		// If the node change labels we queue up the reconcile and the reconcile function will handle it.
		if ais.IsProxyAutoScaling() {
			if r.nodeMatchesForProxy(node, &ais) {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ais.Namespace, Name: ais.Name}})
				continue
			}
		}
		if ais.IsTargetAutoScaling() {
			if r.nodeMatchesForTarget(node, &ais) {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ais.Namespace, Name: ais.Name}})
				continue
			}
		}

		logger.Info("Skipping non-autoScaleStatus AIStore cr", "cr", ais.GetName())
	}

	if len(requests) > 0 {
		// fine to log requests here as it's only the name and namespace of each ais cr
		logger.Info("Found new nodes for ais crs", "cluster-count", len(requests), "crs", requests)
	}

	return requests
}

func (r *AIStoreReconciler) nodeMatchesForProxy(node *corev1.Node, ais *aisv1.AIStore) bool {
	logger := r.log.WithName("node-mapper").WithValues("node", node.Name)

	if ais.IsProxyAutoScaling() && slices.Contains(ais.Status.AutoScaleStatus.ExpectedProxyNodes, node.Name) {
		logger.Info("Node is in expected proxy nodes", "cr", ais.GetName())
		return true
	}
	if ais.IsProxyAutoScaling() && r.nodeMatchesSelector(node, ais.Spec.ProxySpec.NodeSelector) {
		logger.Info("Node selector found for proxy", "cr", ais.GetName())
		return true
	}
	return false
}

func (r *AIStoreReconciler) nodeMatchesForTarget(node *corev1.Node, ais *aisv1.AIStore) bool {
	logger := r.log.WithName("node-mapper").WithValues("node", node.Name)
	if ais.IsTargetAutoScaling() && slices.Contains(ais.Status.AutoScaleStatus.ExpectedTargetNodes, node.Name) {
		logger.Info("Node is in expected target nodes", "cr", ais.GetName())
		return true
	}
	if ais.IsTargetAutoScaling() && r.nodeMatchesSelector(node, ais.Spec.TargetSpec.NodeSelector) {
		logger.Info("Node selector found for target", "cr", ais.GetName())
		return true
	}
	return false
}

func (*AIStoreReconciler) nodeMatchesSelector(node *corev1.Node, selector map[string]string) bool {
	nodeLabels := labels.Set(node.Labels)
	selectorLabels := labels.Set(selector)
	return selectorLabels.AsSelector().Matches(nodeLabels)
}

func (r *AIStoreReconciler) handleSuccessfulReconcile(ctx context.Context, ais *aisv1.AIStore) (err error) {
	var needsUpdate bool
	if !ais.IsConditionTrue(aisv1.ConditionReady) {
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonReady, "Successfully reconciled AIStore cluster")
		ais.SetCondition(aisv1.ConditionReady)
		needsUpdate = true
	}
	if !ais.HasState(aisv1.ClusterReady) {
		needsUpdate = true
	}
	if needsUpdate {
		err = r.updateStatusWithState(ctx, ais, aisv1.ClusterReady)
	}
	return
}

func (r *AIStoreReconciler) checkAISClusterReady(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	logger := logf.FromContext(ctx)
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get client to check cluster readiness")
		return
	}
	err = apiClient.Health(true /*readyToRebalance*/)
	if err != nil {
		logger.Info("AIS cluster is not ready", "health_error", err.Error())
		return
	}
	return true, nil
}

func (r *AIStoreReconciler) recordError(ctx context.Context, ais *aisv1.AIStore, err error, msg string) {
	logf.FromContext(ctx).Error(err, msg)
	r.recorder.Eventf(ais, corev1.EventTypeWarning, EventReasonFailed, "%s, err: %v", msg, err)
}

func shouldUpdatePodTemplate(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if len(desired.Spec.Containers) != len(current.Spec.Containers) {
		return true, "updating desired containers"
	}

	for _, daemon := range []struct {
		desiredContainer *corev1.Container
		currentContainer *corev1.Container
	}{
		{&desired.Spec.InitContainers[0], &current.Spec.InitContainers[0]},
		{&desired.Spec.Containers[0], &current.Spec.Containers[0]},
	} {
		if daemon.desiredContainer.Image != daemon.currentContainer.Image {
			return true, fmt.Sprintf("updating image for %q container", daemon.desiredContainer.Name)
		}
		if !equality.Semantic.DeepEqual(daemon.desiredContainer.Env, daemon.currentContainer.Env) {
			return true, fmt.Sprintf("updating env variables for %q container", daemon.desiredContainer.Name)
		}
		if shouldUpdateResources(&daemon.desiredContainer.Resources, &daemon.currentContainer.Resources) {
			return true, fmt.Sprintf("updating resource requests/limits for %q container", daemon.desiredContainer.Name)
		}
	}

	if shouldUpdateAnnotations(desired.Annotations, current.Annotations) {
		return true, "updating annotations"
	}

	if !equality.Semantic.DeepEqual(desired.Labels, current.Labels) {
		return true, "updating labels"
	}

	// Both `desired.Spec.SecurityContext` and `current.Spec.SecurityContext` are
	// expected to be non-nil here as `SecurityContext` should be set by default.
	if !equality.Semantic.DeepEqual(desired.Spec.SecurityContext, current.Spec.SecurityContext) {
		return true, "updating security context"
	}

	// We already know desired number of containers matches current here,
	// so if using sidecar, compare the images of the sidecar container.
	if len(desired.Spec.Containers) > 1 {
		if desired.Spec.Containers[1].Image != current.Spec.Containers[1].Image {
			return true, fmt.Sprintf("updating image for %q container", desired.Spec.Containers[1].Name)
		}
	}

	return false, ""
}

func shouldUpdateResources(desired, current *corev1.ResourceRequirements) bool {
	// TODO: Remove check in next major version (causes cluster restart)
	// If we already have ephemeral storage request, do a full comparison
	if current.Requests.StorageEphemeral() != nil && !current.Requests.StorageEphemeral().IsZero() {
		return !equality.Semantic.DeepEqual(desired, current)
	}
	// Do not sync if the only change is *adding* ephemeral storage request
	desFiltered := desired.DeepCopy()
	delete(desFiltered.Requests, corev1.ResourceEphemeralStorage)
	return !equality.Semantic.DeepEqual(desFiltered, current)
}

func shouldUpdateAnnotations(desired, current map[string]string) bool {
	if equality.Semantic.DeepDerivative(desired, current) {
		return false
	}
	restartHash, exists := desired[cmn.RestartConfigHashAnnotation]
	// At this point annotations are not equal -- If the restart hash does not exist trigger sync
	if !exists {
		return true
	}
	// If the hash is different and NOT initial, trigger sync
	nonInitial := !strings.HasSuffix(restartHash, cmn.RestartConfigHashInitial)
	if nonInitial && restartHash != current[cmn.RestartConfigHashAnnotation] {
		return true
	}
	// Compare the desired to current WITHOUT the restart hash and trigger if not equivalent
	desiredCopy := make(map[string]string)
	for k, v := range desired {
		desiredCopy[k] = v
	}
	delete(desiredCopy, cmn.RestartConfigHashAnnotation)
	return !equality.Semantic.DeepDerivative(desiredCopy, current)
}

func syncPodTemplate(desired, current *corev1.PodTemplateSpec) (updated bool) {
	for _, daemon := range []struct {
		desiredContainer *corev1.Container
		currentContainer *corev1.Container
	}{
		{&desired.Spec.InitContainers[0], &current.Spec.InitContainers[0]},
		{&desired.Spec.Containers[0], &current.Spec.Containers[0]},
	} {
		if equality.Semantic.DeepDerivative(*daemon.desiredContainer, *daemon.currentContainer) {
			continue
		}
		*daemon.currentContainer = *daemon.desiredContainer
		updated = true
	}

	if !equality.Semantic.DeepDerivative(desired.Annotations, current.Annotations) {
		current.Annotations = desired.Annotations
		updated = true
	}

	if !equality.Semantic.DeepEqual(desired.Labels, current.Labels) {
		current.Labels = desired.Labels
		updated = true
	}

	if !equality.Semantic.DeepEqual(desired.Spec.SecurityContext, current.Spec.SecurityContext) {
		current.Spec.SecurityContext = desired.Spec.SecurityContext
		updated = true
	}

	if syncSidecarContainer(desired, current) {
		updated = true
	}

	return
}

func findAISNodeByPodName(nodeMap aismeta.NodeMap, podName string) (*aismeta.Snode, error) {
	for _, node := range nodeMap {
		if strings.HasPrefix(node.ControlNet.Hostname, podName) {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no matching AIS node found for pod %q", podName)
}

func syncSidecarContainer(desired, current *corev1.PodTemplateSpec) (updated bool) {
	// We have no sidecar, and don't want one
	if len(desired.Spec.Containers) < 2 && len(current.Spec.Containers) < 2 {
		return false
	}
	// We want to remove the sidecar
	if len(desired.Spec.Containers) < 2 && len(current.Spec.Containers) > 1 {
		current.Spec.Containers = current.Spec.Containers[:1]
		return true
	}
	// Add a new sidecar
	if len(desired.Spec.Containers) > 1 && len(current.Spec.Containers) < 2 {
		current.Spec.Containers = append(current.Spec.Containers, desired.Spec.Containers[1])
		return true
	}
	// If sidecar is already updated, no change
	if equality.Semantic.DeepDerivative(desired.Spec.Containers[1], current.Spec.Containers[1]) {
		return false
	}
	current.Spec.Containers[1] = desired.Spec.Containers[1]
	return true
}
