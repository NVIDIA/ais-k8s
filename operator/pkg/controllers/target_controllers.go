// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/target"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const targetRequeueDelay = 10 * time.Second

func (r *AIStoreReconciler) ensureTargetPrereqs(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Deploy required ConfigMap
	cm, err := target.NewTargetCM(ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to generate valid target ConfigMap")
		return
	}

	if err = r.k8sClient.CreateOrUpdateResource(context.TODO(), ais, cm); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if err = r.k8sClient.CreateOrUpdateResource(ctx, ais, svc); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target SVC")
		return
	}
	return
}

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	// Deploy statefulset
	ss := target.NewTargetSS(ais)
	exists, err := r.k8sClient.CreateResourceIfNotExists(ctx, ais, ss)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy target statefulset")
		return
	}
	if !exists {
		msg := "Successfully initialized target nodes"
		logf.FromContext(ctx).Info(msg)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, msg)
		result.Requeue = true
	}
	return
}

func (r *AIStoreReconciler) cleanupTarget(ctx context.Context, ais *aisv1.AIStore) (updated bool, err error) {
	return cmn.AnyFunc(
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
	// Fetch the latest target StatefulSet.
	ss, err := r.k8sClient.GetStatefulSet(ctx, target.StatefulSetNSName(ais))

	if err != nil {
		if k8serrors.IsNotFound(err) {
			result, err = r.initTargets(ctx, ais)
		}
		return
	}

	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name, "status", ss.Status)

	updated, err := r.syncTargetPodSpec(ctx, ais, ss)
	if err != nil {
		return ctrl.Result{}, err
	}
	if updated {
		return ctrl.Result{RequeueAfter: targetRequeueDelay}, nil
	}

	if ais.HasState(aisv1.ClusterScaling) {
		// If desired does not match AIS, update the statefulset
		if *ss.Spec.Replicas != ais.GetTargetSize() {
			err = r.resolveStatefulSetScaling(ctx, ais)
			if err != nil {
				return
			}
		}
	}
	// Start the target scaling process by updating services and contacting the AIS API
	if *ss.Spec.Replicas != ais.GetTargetSize() {
		err = r.startTargetScaling(ctx, ais, ss)
		if err != nil {
			return
		}
		// If successful, mark as scaling so future reconciliations will update the SS
		err = r.updateStatusWithState(ctx, ais, aisv1.ClusterScaling)
		if err != nil {
			return
		}
		return ctrl.Result{RequeueAfter: targetRequeueDelay}, nil
	}
	// Requeue if the number of target pods ready does not match the size provided in AIS cluster spec.
	if !isStatefulSetReady(ais, ss) {
		logger.Info("Waiting for target statefulset to reach desired replicas", "desired", ss.Spec.Replicas)
		return ctrl.Result{RequeueAfter: targetRequeueDelay}, nil
	}
	return
}

func isStatefulSetReady(ais *aisv1.AIStore, ss *appsv1.StatefulSet) bool {
	specReplicas := *ss.Spec.Replicas
	// Must match size provided in AIS cluster spec
	if specReplicas != ais.GetTargetSize() {
		return false
	}

	// If update revision is set, check that it equals current revision indicating the rollout is complete
	if ss.Status.UpdateRevision != "" && ss.Status.CurrentRevision != ss.Status.UpdateRevision {
		return false
	}
	// To be ready, spec must match status.Replicas, status.CurrentReplicas, status.ReadyReplicas
	if specReplicas != ss.Status.Replicas {
		return false
	}
	if specReplicas != ss.Status.CurrentReplicas {
		return false
	}
	return specReplicas == ss.Status.ReadyReplicas
}

func (r *AIStoreReconciler) resolveStatefulSetScaling(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	desiredSize := ais.GetTargetSize()
	current, ssErr := r.k8sClient.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
	if ssErr != nil {
		return ssErr
	}
	currentSize := *current.Spec.Replicas
	// Scaling up
	if desiredSize > currentSize {
		if desiredSize > currentSize+1 {
			ready, err := r.checkAISClusterReady(ctx, ais)
			if err != nil {
				return err
			}
			if !ready {
				logger.Info("Waiting for cluster readiness before scaling")
				return fmt.Errorf("cannot disable rebalance before target scaling, cluster not ready")
			}
			logger.Info("Disabling rebalance before target scale-up of > 1 new nodes")
			err = r.disableRebalance(ctx, ais, aisv1.ReasonScaling, "Disabled due to target scale-up")
			if err != nil {
				logger.Error(err, "Failed to disable rebalance before scaling")
				return err
			}
		}
		logger.Info("Scaling up target statefulset to match AIS cluster spec size", "desiredSize", desiredSize)
	} else if desiredSize < currentSize {
		// Don't proceed to update state to ready until we can proceed to statefulset scale-down
		if ready, scaleErr := r.isReadyToScaleDown(ctx, ais, currentSize); scaleErr != nil || !ready {
			return scaleErr
		}
		logger.Info("Scaling down target statefulset to match AIS cluster spec size", "desiredSize", desiredSize)
	}
	_, ssErr = r.k8sClient.UpdateStatefulSetReplicas(ctx, target.StatefulSetNSName(ais), desiredSize)
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
		for _, node := range smap.Tmap {
			if !strings.HasPrefix(node.ControlNet.Hostname, podName) {
				continue
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
	}
	return nil
}

func (r *AIStoreReconciler) syncTargetPodSpec(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (updated bool, err error) {
	logger := logf.FromContext(ctx)
	updatedSS := ss.DeepCopy()
	desiredTemplate := &target.NewTargetSS(ais).Spec.Template
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
		err := r.k8sClient.CreateOrUpdateResource(ctx, ais, svc)
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
