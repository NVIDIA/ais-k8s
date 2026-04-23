// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/target"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const targetLongRequeueDelay = 10 * time.Second
const targetShortRequeueDelay = 2 * time.Second

func (r *AIStoreReconciler) ensureTargetPrereqs(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Deploy required ConfigMap
	cm, err := target.NewTargetCM(ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to generate valid target ConfigMap")
		return
	}

	if err = r.k8sClient.Apply(ctx, cm); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if _, err = r.k8sClient.CreateOrUpdateResource(ctx, ais, svc); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target SVC")
		return
	}
	return
}

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	// Deploy statefulset
	ss := target.NewTargetSS(ais, ais.GetTargetSize())
	exists, err := r.k8sClient.CreateResourceIfNotExists(ctx, ais, ss)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target statefulset")
		return
	}
	if !exists {
		msg := "Successfully initialized target nodes"
		logf.FromContext(ctx).Info(msg)
		r.recorder.Eventf(ais, ss, corev1.EventTypeNormal, EventReasonInitialized, ActionInitTargets, msg)
		result.RequeueAfter = targetShortRequeueDelay
	}
	return
}

// reconcileTargetPDB handles creating, updating, or deleting the target PDB
func (r *AIStoreReconciler) reconcileTargetPDB(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	if ais.TargetPDBEnabled() {
		pdb := target.NewTargetPDB(ais)
		changed, err := r.k8sClient.CreateOrUpdateResource(ctx, ais, pdb)
		if err != nil {
			return err
		}
		if changed {
			maxUnavailable := ais.GetTargetPDBMaxUnavailable()
			logger.Info("Reconciled target PDB", "name", pdb.Name, "maxUnavailable", maxUnavailable.String())
		}
		return nil
	}
	// PDB not enabled, delete if exists
	pdbName := target.PDBNSName(ais)
	deleted, err := r.k8sClient.DeletePDBIfExists(ctx, pdbName)
	if err != nil {
		return err
	}
	if deleted {
		logger.Info("Deleted target PDB", "name", pdbName.Name)
	}
	return nil
}

func (r *AIStoreReconciler) cleanupTarget(ctx context.Context, ais *aisv1.AIStore) (updated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) { return r.k8sClient.DeletePDBIfExists(ctx, target.PDBNSName(ais)) },
		func() (bool, error) { return r.cleanupTargetSS(ctx, ais) },
		func() (bool, error) { return r.k8sClient.DeleteServiceIfExists(ctx, target.HeadlessSVCNSName(ais)) },
		func() (bool, error) {
			return r.k8sClient.DeleteAllServicesIfExist(ctx, ais.Namespace, cmn.NewServiceLabels(ais.Name, target.ServiceLabelLB))
		},
		func() (bool, error) { return r.k8sClient.DeleteConfigMapIfExists(ctx, target.ConfigMapNSName(ais)) },
	)
}

func (r *AIStoreReconciler) cleanupTargetSS(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	logf.FromContext(ctx).Info("Cleaning up target statefulset")
	targetSS := target.StatefulSetNSName(ais)
	return r.k8sClient.DeleteStatefulSetIfExists(ctx, targetSS)
}

func (r *AIStoreReconciler) handleTargetState(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	ss, err := r.k8sClient.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return r.initTargets(ctx, ais)
		}
		return
	}

	if err = r.reconcileTargetPDB(ctx, ais); err != nil {
		r.recordError(ctx, ais, err, "Failed to sync target PDB")
		return
	}

	rolling := isRolloutInProgress(ss)
	scaling := isScalingInProgress(ss)
	rolloutNeeded, _ := shouldUpdatePodTemplate(&target.NewTargetSS(ais, ais.GetTargetSize()).Spec.Template, &ss.Spec.Template)
	scalingNeeded := isScalingNeeded(ss, ais.GetTargetSize())

	logger := logf.FromContext(ctx).WithValues(
		"statefulset", ss.Name,
		"specReplicas", *ss.Spec.Replicas, "statusReplicas", ss.Status.Replicas,
		"readyReplicas", ss.Status.ReadyReplicas, "desiredSize", ais.GetTargetSize(),
		"rolling", rolling, "scaling", scaling,
		"rolloutNeeded", rolloutNeeded, "scalingNeeded", scalingNeeded,
	)

	if policyUpdated, policyErr := r.syncTargetPVCRetentionPolicy(ctx, ais, ss); policyErr != nil {
		return ctrl.Result{}, policyErr
	} else if policyUpdated {
		ss, err = r.k8sClient.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
		if err != nil {
			return
		}
	}

	// Apply template update (blocked by scaling in progress)
	if rolloutNeeded && !scaling {
		if updated, err := r.syncTargetPodSpec(ctx, ais, ss); err != nil {
			return ctrl.Result{}, err
		} else if updated {
			return ctrl.Result{RequeueAfter: targetLongRequeueDelay}, nil
		}
	}

	// Apply scaling (blocked by rollout in progress)
	if scalingNeeded && !rolling {
		if err = r.startTargetScaling(ctx, ais, ss); err != nil {
			return ctrl.Result{}, err
		}
		if err = r.resolveStatefulSetScaling(ctx, ais); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: targetLongRequeueDelay}, nil
	}

	// Drive ongoing rollout
	if rolling {
		if res, err := r.handleTargetRollout(ctx, ais, ss); err != nil || !res.IsZero() {
			return res, err
		}
		return ctrl.Result{RequeueAfter: targetLongRequeueDelay}, nil
	}

	// Drive ongoing scaling
	if scaling {
		return ctrl.Result{RequeueAfter: targetLongRequeueDelay}, nil
	}

	// Wait for readiness
	if !r.isStatefulSetReady(ais.GetTargetSize(), ss) {
		logger.Info("Waiting for target statefulset to reach desired replicas")
		return ctrl.Result{RequeueAfter: targetLongRequeueDelay}, nil
	}
	return
}

func (r *AIStoreReconciler) resolveStatefulSetScaling(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	expectedSize := ais.GetTargetSize()
	current, ssErr := r.k8sClient.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
	if ssErr != nil {
		return ssErr
	}
	currentSize := *current.Spec.Replicas
	// Scaling up
	if expectedSize > currentSize {
		// If we have an existing cluster, check health and disable rebalance before adding multiple targets
		if expectedSize > currentSize+1 && currentSize > 0 {
			apiClient, err := r.clientManager.GetClient(ctx, ais)
			if err != nil {
				return err
			}
			if err = apiClient.Health(true /*readyToRebalance*/); err != nil {
				logger.Info("Waiting for cluster to be healthy before scaling")
				return fmt.Errorf("cannot disable rebalance before target scaling, cluster not healthy")
			}
			logger.Info("Disabling rebalance before target scale-up of > 1 new nodes")
			err = r.disableRebalance(ctx, ais, aisv1.ReasonScaling, "Disabled due to target scale-up")
			if err != nil {
				logger.Error(err, "Failed to disable rebalance before scaling")
				return err
			}
		}
		logger.Info("Scaling up target statefulset to match AIS cluster spec size", "desiredSize", expectedSize)
	} else if expectedSize < currentSize {
		// Wait for decommission to complete before scaling the StatefulSet down
		if ready, scaleErr := r.isReadyToScaleDown(ctx, ais, currentSize); scaleErr != nil || !ready {
			return scaleErr
		}
		logger.Info("Scaling down target statefulset to match AIS cluster spec size", "desiredSize", expectedSize)
	}
	_, ssErr = r.k8sClient.UpdateStatefulSetReplicas(ctx, target.StatefulSetNSName(ais), expectedSize)
	if ssErr != nil {
		return ssErr
	}
	logger.Info("Updated replica count for target statefulset")
	return nil
}

func (r *AIStoreReconciler) isReadyToScaleDown(ctx context.Context, ais *aisv1.AIStore, currentSize int32) (ready bool, err error) {
	logger := logf.FromContext(ctx)
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return
	}
	smap, err := apiClient.GetClusterMap()
	if err != nil {
		return
	}
	// If any targets are still in the smap as decommissioning, delay scaling
	for _, targetNode := range smap.Tmap {
		if smap.InMaintOrDecomm(targetNode.ID()) && !smap.InMaint(targetNode) {
			logger.Info("Delaying scaling. Target still in decommissioning state", "target", targetNode.ID())
			return
		}
	}
	// If we have the same number of target nodes as current replicas and none showed as decommissioned, don't scale
	if int32(len(smap.Tmap)) == currentSize {
		logger.Info("Delaying scaling. All target nodes are still listed as active")
		return
	}
	return true, nil
}

func (r *AIStoreReconciler) startTargetScaling(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) error {
	if *ss.Spec.Replicas < ais.GetTargetSize() {
		// Current SS has fewer replicas than expected size - scale up.
		return r.scaleUpLB(ctx, ais)
	}

	// Otherwise - scale down.
	// Ensure rebalance is enabled before decommissioning so data can migrate
	// off the targets being decommissioned.
	if err := r.enableRebalanceCondition(ctx, ais); err != nil {
		return err
	}
	if err := r.handleConfigState(ctx, ais, true /*force*/); err != nil {
		return err
	}
	err := r.scaleDownLB(ctx, ais, ss)
	if err != nil {
		return err
	}
	// Decommission target through AIS API
	return r.decommissionTargets(ctx, ais, *ss.Spec.Replicas)
}

func (r *AIStoreReconciler) decommissionTargets(ctx context.Context, ais *aisv1.AIStore, actualSize int32) error {
	logger := logf.FromContext(ctx)
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return err
	}
	smap, err := apiClient.GetClusterMap()
	if err != nil {
		return err
	}
	logger.Info("Decommissioning targets", "Smap version", smap)
	for idx := actualSize; idx > ais.GetTargetSize(); idx-- {
		podName := target.PodName(ais, idx-1)
		logger.Info("Attempting to decommission target", "podName", podName)
		node, err := findAISNodeByPodName(smap.Tmap, podName)
		if err != nil {
			// If target is not in the cluster map, fetch the pod and inspect state.
			// Skip decommission if the pod is unschedulable or in CrashLoopBackOff.
			// Otherwise, wait for the pod to start and register in the cluster map.
			pod, podErr := r.k8sClient.GetPod(ctx, types.NamespacedName{Name: podName, Namespace: ais.Namespace})
			switch {
			case k8serrors.IsNotFound(podErr):
				logger.Info("Target pod not found, skipping decommission", "podName", podName)
				continue
			case podErr != nil:
				return fmt.Errorf("failed to get pod %s: %w", podName, podErr)
			case isPodUnschedulable(pod):
				logger.Info("Target pod is unschedulable, skipping decommission", "podName", podName)
				continue
			case isPodInCrashLoopBackOff(pod):
				logger.Info("Target pod is in CrashLoopBackOff, skipping decommission", "podName", podName)
				continue
			}
			return fmt.Errorf("waiting for target %s to register in smap", podName)
		}
		if !smap.InMaintOrDecomm(node.ID()) {
			logger.Info("Decommissioning target", "nodeID", node.ID())
			_, err = apiClient.DecommissionNode(&aisapc.ActValRmNode{DaemonID: node.ID(), RmUserData: true})
			if err != nil {
				logger.Error(err, "Failed to decommission node", "nodeID", node.ID())
				return err
			}
		} else {
			logger.Info("AIS target is already in decommissioning state", "nodeID", node.ID())
		}
	}
	return nil
}

func (r *AIStoreReconciler) syncTargetPodSpec(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (updated bool, err error) {
	logger := logf.FromContext(ctx)
	updatedSS := ss.DeepCopy()
	desiredTemplate := &target.NewTargetSS(ais, ais.GetTargetSize()).Spec.Template
	if needsUpdate, reason := shouldUpdatePodTemplate(desiredTemplate, &updatedSS.Spec.Template); needsUpdate {
		// Disable rebalance condition before ANY changes that trigger a rolling upgrade
		err = r.disableRebalance(ctx, ais, aisv1.ReasonUpgrading, "Disabled due to rolling upgrade: "+reason)
		if err != nil {
			return false, fmt.Errorf("failed to disable rebalance before rolling upgrade: %w", err)
		}

		syncPodTemplate(desiredTemplate, &updatedSS.Spec.Template)
		logger.Info("Target pod template spec modified", "reason", reason)
		patch := client.MergeFrom(ss)
		err = r.k8sClient.Patch(ctx, updatedSS, patch)
		if err == nil {
			logger.Info("Target statefulset successfully updated", "reason", reason)
		}
		return true, err
	}
	return false, nil
}

func (r *AIStoreReconciler) syncTargetPVCRetentionPolicy(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (updated bool, err error) {
	desiredPolicy := ais.Spec.TargetSpec.PVCRetentionPolicy
	if !shouldUpdatePVCRetentionPolicy(desiredPolicy, ss.Spec.PersistentVolumeClaimRetentionPolicy) {
		return false, nil
	}
	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name)
	updatedSS := ss.DeepCopy()
	updatedSS.Spec.PersistentVolumeClaimRetentionPolicy = desiredPolicy
	patch := client.MergeFrom(ss)
	if err = r.k8sClient.Patch(ctx, updatedSS, patch); err != nil {
		return false, err
	}
	logger.Info("Updated target PVC retention policy")
	return true, nil
}

func isPodActive(pod *corev1.Pod) bool {
	return pod != nil && pod.DeletionTimestamp == nil
}

func isPodReady(pod *corev1.Pod) bool {
	if !isPodActive(pod) {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *AIStoreReconciler) findPodNeedingUpdate(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) string {
	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name)
	podList, err := r.k8sClient.ListPods(ctx, ais, target.RequiredPodLabels(ais))
	if err != nil {
		logger.Error(err, "Failed to list target pods")
		return ""
	}
	podMap := make(map[string]*corev1.Pod, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		podMap[pod.Name] = pod
	}

	for i := range int(*ss.Spec.Replicas) {
		idx := int32(i)
		// For HA, previous pod MUST be ready before proceeding
		if idx > 0 {
			prevName := target.PodName(ais, idx-1)
			if !isPodReady(podMap[prevName]) {
				logger.Info("Waiting for previous pod to be ready before proceeding with rollout", "pod", prevName)
				return ""
			}
		}

		podName := target.PodName(ais, idx)
		pod := podMap[podName]
		// Do not block on current pod being ready (need to be able to rollback/fix a bad upgrade)
		if !isPodActive(pod) {
			logger.Info("Pod doesn't exist or is being deleted, waiting", "pod", podName)
			return ""
		}

		// Proceed to checking next pod if current pod is up-to-date
		podRevision := pod.Labels["controller-revision-hash"]
		if podRevision == ss.Status.UpdateRevision {
			continue
		}

		logger.Info(
			"Found pod needing update",
			"pod", podName,
			"currentRevision", podRevision,
			"targetRevision", ss.Status.UpdateRevision,
		)

		return podName
	}

	return ""
}

func (r *AIStoreReconciler) handleTargetRollout(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Only handle rollouts if there's a revision mismatch
	if ss.Status.UpdateRevision == "" || ss.Status.CurrentRevision == ss.Status.UpdateRevision {
		return ctrl.Result{}, nil
	}

	// If all pods are updated and ready, rollout is complete
	if ss.Status.UpdatedReplicas >= *ss.Spec.Replicas && ss.Status.ReadyReplicas >= *ss.Spec.Replicas {
		return ctrl.Result{}, nil
	}

	podName := r.findPodNeedingUpdate(ctx, ais, ss)
	if podName == "" {
		return ctrl.Result{RequeueAfter: targetShortRequeueDelay}, nil // No pod needs update or waiting for current pod
	}

	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get API client: %w", err)
	}

	smap, err := apiClient.GetClusterMap()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get cluster map: %w", err)
	}

	node, err := findAISNodeByPodName(smap.Tmap, podName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find node for pod %s: %w", podName, err)
	}

	if !smap.InMaint(node) {
		logger.Info("Setting maintenance mode for pod", "pod", podName, "node", node.ID())
		_, err = apiClient.StartMaintenance(&aisapc.ActValRmNode{DaemonID: node.ID(), SkipRebalance: true})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to start maintenance for pod %s: %w", podName, err)
		}
		return ctrl.Result{RequeueAfter: targetShortRequeueDelay}, nil
	}

	_, err = r.k8sClient.DeletePodIfExists(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: ais.Namespace,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}

	logger.Info("Deleted pod for rollout", "pod", podName)

	// Requeue to handle next pod
	return ctrl.Result{RequeueAfter: targetShortRequeueDelay}, nil
}

func (r *AIStoreReconciler) scaleUpLB(ctx context.Context, ais *aisv1.AIStore) error {
	if !ais.Spec.EnableExternalLB {
		return nil
	}
	return r.enableTargetExternalService(ctx, ais)
}

func (r *AIStoreReconciler) scaleDownLB(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) error {
	if !ais.Spec.EnableExternalLB {
		return nil
	}
	for idx := *ss.Spec.Replicas; idx > ais.GetTargetSize(); idx-- {
		svcName := target.LoadBalancerSVCNSName(ais, idx-1)
		_, err := r.k8sClient.DeleteServiceIfExists(ctx, svcName)
		if err != nil {
			return err
		}
	}
	return nil
}

// enableTargetExternalService, creates a loadbalancer service per target and checks if all the services are assigned an external IP.
func (r *AIStoreReconciler) enableTargetExternalService(ctx context.Context,
	ais *aisv1.AIStore,
) error {
	targetSVCList := target.NewLoadBalancerSVCList(ais)
	// 1. Try creating a LoadBalancer service for each target pod
	for _, svc := range targetSVCList {
		_, err := r.k8sClient.CreateOrUpdateResource(ctx, ais, svc)
		if err != nil {
			return err
		}
	}

	// 2. Ensure every service has an ingress IP assigned to it.
	svcList := &corev1.ServiceList{}
	err := r.k8sClient.List(ctx, svcList, client.MatchingLabels(cmn.NewServiceLabels(ais.Name, target.ServiceLabelLB)))
	if err != nil {
		return err
	}

	for i := range svcList.Items {
		for _, ing := range svcList.Items[i].Status.LoadBalancer.Ingress {
			if ing.IP == "" {
				return fmt.Errorf("ingress IP not set for Load Balancer")
			}
		}
	}
	return nil
}
