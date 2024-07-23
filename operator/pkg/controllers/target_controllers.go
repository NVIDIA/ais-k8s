// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"strings"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/target"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore) (changed bool, err error) {
	var cm *corev1.ConfigMap
	// 1. Deploy required ConfigMap
	cm, err = target.NewTargetCM(ctx, ais)
	if err != nil {
		r.recordError(ais, err, "Failed to generate valid target ConfigMap")
		return
	}

	if _, err = r.client.CreateOrUpdateResource(context.TODO(), ais, cm); err != nil {
		r.recordError(ais, err, "Failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if _, err = r.client.CreateOrUpdateResource(ctx, ais, svc); err != nil {
		r.recordError(ais, err, "Failed to deploy target SVC")
		return
	}

	// 3. Deploy statefulset
	ss := target.NewTargetSS(ais)
	if exists, err := r.client.CreateResourceIfNotExists(ctx, ais, ss); err != nil {
		r.recordError(ais, err, "Failed to deploy target statefulset")
		return false, err
	} else if !exists {
		msg := "Successfully initialized target nodes"
		r.log.Info(msg)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, msg)
		changed = true
	}
	return
}

func (r *AIStoreReconciler) cleanupTarget(ctx context.Context, ais *aisv1.AIStore) (updated bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) { return r.cleanupTargetSS(ctx, ais) },
		func() (bool, error) { return r.client.DeleteServiceIfExists(ctx, target.HeadlessSVCNSName(ais)) },
		func() (bool, error) {
			return r.client.DeleteAllServicesIfExist(ctx, ais.Namespace, target.ExternalServiceLabels(ais))
		},
		func() (bool, error) { return r.client.DeleteConfigMapIfExists(ctx, target.ConfigMapNSName(ais)) },
	)
}

func (r *AIStoreReconciler) cleanupTargetSS(ctx context.Context, ais *aisv1.AIStore) (anyUpdated bool, err error) {
	r.log.Info("Cleaning up target statefulset")
	targetSS := target.StatefulSetNSName(ais)

	// If the target statefulset is not present, we can return immediately.
	if exists, err := r.client.StatefulSetExists(ctx, targetSS); err != nil || !exists {
		return false, err
	}

	var baseParams *aisapi.BaseParams
	if r.isExternal {
		baseParams, err = r.getAPIParams(ctx, ais)
	} else {
		baseParams, err = r.primaryBaseParams(ctx, ais)
	}
	if err != nil {
		r.log.Error(err, "Failed to get API parameters", "clusterName", ais.Name)
		// If we cannot get API parameters, we may have a broken statefulset with no ready replicas so delete it
		currentSS, ssErr := r.client.GetStatefulSet(ctx, targetSS)
		if ssErr != nil && !k8serrors.IsNotFound(ssErr) {
			return false, ssErr
		}
		if k8serrors.IsNotFound(ssErr) || currentSS.Status.ReadyReplicas == 0 {
			r.log.Info("Deleting target statefulset", "clusterName", ais.Name)
			return r.client.DeleteStatefulSetIfExists(ctx, targetSS)
		}
		// Somehow we have ready replicas but cannot get parameters to properly decommission, so return the error
		return false, err
	}

	// Attempt graceful cluster decommission via API call before deleting statefulset
	cleanupData := ais.Spec.CleanupData != nil && *ais.Spec.CleanupData
	r.attemptGracefulDecommission(baseParams, cleanupData)

	// TODO: if the environment is slow the statefulset controller might create new pods to compensate for the old ones being
	// deleted in the shutdown/decommission operation. Find a way to stop the statefulset controller from creating new pods
	return r.client.DeleteStatefulSetIfExists(ctx, targetSS)
}

func (r *AIStoreReconciler) handleTargetState(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	if hasLatest, err := r.handleTargetImage(ctx, ais); !hasLatest || err != nil {
		return false, err
	}

	targetSSName := target.StatefulSetNSName(ais)
	// Fetch the latest StatefulSet for targets and check if it's spec (for now just replicas), matches the AIS cluster spec.
	ss, err := r.client.GetStatefulSet(ctx, targetSSName)
	if err != nil {
		return ready, err
	}
	if *ss.Spec.Replicas != ais.GetTargetSize() {
		err = r.verifyNodesAvailable(ctx, ais, aisapc.Target)
		if err != nil {
			return false, err
		}
		ready, err = r.handleTargetScaling(ctx, ais, ss, targetSSName)
		if !ready || err != nil {
			return false, err
		}
	}
	// For now, state of target is considered ready if the number of target pods ready matches the size provided in AIS cluster spec.
	ready = ss.Status.ReadyReplicas == ais.GetTargetSize()
	return
}

func (r *AIStoreReconciler) handleTargetScaling(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet, targetSS types.NamespacedName) (ready bool, err error) {
	if *ss.Spec.Replicas < ais.GetTargetSize() {
		// Current SS has fewer replicas than expected size - scale up.
		return r.handleTargetScaleUp(ctx, ais, targetSS)
	}

	// Otherwise - scale down.
	return r.handleTargetScaleDown(ctx, ais, ss, targetSS)
}

func (r *AIStoreReconciler) handleTargetScaleDown(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet, targetSS types.NamespacedName) (ready bool, err error) {
	if ais.Spec.EnableExternalLB {
		ready = true
		for idx := *ss.Spec.Replicas; idx > ais.GetTargetSize(); idx-- {
			svcName := target.LoadBalancerSVCNSName(ais, idx-1)
			singleExisted, err := r.client.DeleteServiceIfExists(ctx, svcName)
			if err != nil {
				return false, err
			}
			ready = ready && !singleExisted
		}
		if !ready {
			return
		}
	}

	// Decommission target scaling down statefulset
	decommissioning, err := r.decommissionTargets(ctx, ais, *ss.Spec.Replicas)
	if decommissioning || err != nil {
		return false, err
	}

	r.log.Info("Targets decommissioned, scaling down statefulset to match AIS cluster spec size")
	// If anything was updated, we consider it not immediately ready.
	updated, err := r.client.UpdateStatefulSetReplicas(ctx, targetSS, ais.GetTargetSize())
	return !updated, err
}

// Scale down the statefulset without decommissioning
func (r *AIStoreReconciler) scaleTargetsToZero(ctx context.Context, ais *aisv1.AIStore) error {
	r.log.Info("Scaling targets to zero", "clusterName", ais.Name)
	changed, err := r.client.UpdateStatefulSetReplicas(ctx, target.StatefulSetNSName(ais), 0)
	if err != nil {
		r.log.Error(err, "Failed to scale targets to zero", "clusterName", ais.Name)
	} else if changed {
		r.log.Info("Target StatefulSet set to size 0", "name", ais.Name)
	} else {
		r.log.Info("Target StatefulSet already at size 0", "name", ais.Name)
	}
	return err
}

func (r *AIStoreReconciler) decommissionTargets(ctx context.Context, ais *aisv1.AIStore, actualSize int32) (decommissioning bool, err error) {
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		r.log.Error(err, "Failed to get API params")
		return false, err
	}

	smap, err := aisapi.GetClusterMap(*params)
	if err != nil {
		r.log.Error(err, "Failed to get cluster map")
		return false, err
	}
	r.log.Info("Decommissioning targets", "Smap version", smap)
	toDecommission := 0
	for idx := actualSize; idx > ais.GetTargetSize(); idx-- {
		podName := target.PodName(ais, idx-1)
		r.log.Info("Attempting to decommission target", "podName", podName)
		for _, node := range smap.Tmap {
			if !strings.HasPrefix(node.ControlNet.Hostname, podName) {
				continue
			}
			toDecommission++
			if !smap.InMaintOrDecomm(node) {
				r.log.Info("Decommissioning node", "nodeID", node.ID())
				_, err = aisapi.DecommissionNode(*params, &aisapc.ActValRmNode{DaemonID: node.ID(), RmUserData: true})
				if err != nil {
					r.log.Error(err, "Failed to decommission node", "nodeID", node.ID())
					return
				}
			} else {
				r.log.Info("Node is already in decommissioning state", "nodeID", node.ID())
			}
		}
	}
	decommissioning = toDecommission != 0
	return
}

func (r *AIStoreReconciler) handleTargetImage(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	updated, err := r.client.UpdateStatefulSetImage(ctx,
		target.StatefulSetNSName(ais), 0 /*idx*/, ais.Spec.NodeImage)
	if updated || err != nil {
		r.log.Info("target image updated")
		return false, err
	}

	podList, err := r.client.ListTargetPods(ctx, ais)
	if err != nil {
		return
	}
	for idx := range podList.Items {
		pod := podList.Items[idx]
		if pod.Spec.Containers[0].Image != ais.Spec.NodeImage {
			return
		}
	}
	return true, nil
}

func (r *AIStoreReconciler) handleTargetScaleUp(ctx context.Context, ais *aisv1.AIStore, targetSS types.NamespacedName) (ready bool, err error) {
	if ais.Spec.EnableExternalLB {
		ready, err = r.enableTargetExternalService(ctx, ais)
		// External services not fully ready yet, end here and wait for another retry.
		// Do not proceed to updating targets SS until all external services are ready.
		if !ready || err != nil {
			return
		}
	}

	// If anything was updated, we consider it not immediately ready.
	updated, err := r.client.UpdateStatefulSetReplicas(ctx, targetSS, ais.GetTargetSize())
	return !updated, err
}

// enableTargetExternalService, creates a loadbalancer service per target and checks if all the services are assigned an external IP.
func (r *AIStoreReconciler) enableTargetExternalService(ctx context.Context,
	ais *aisv1.AIStore,
) (ready bool, err error) {
	var (
		targetSVCList = target.NewLoadBalancerSVCList(ais)
		exists        bool
		allExist      = true
	)
	// 1. Try creating a LoadBalancer for each target pod, if the SVC are already created (`allExists` == true), then proceed to checking their status.
	for _, svc := range targetSVCList {
		exists, err = r.client.CreateOrUpdateResource(ctx, ais, svc)
		if err != nil {
			return
		}
		allExist = allExist && exists
	}
	if !allExist {
		return
	}

	// 2. If all the SVC already exist, ensure every `service` has an external IP assigned to it.
	//    If not, `ready` will be set to false.
	svcList := &corev1.ServiceList{}
	err = r.client.List(ctx, svcList, client.MatchingLabels(target.ExternalServiceLabels(ais)))
	if err != nil {
		return
	}

	for i := range svcList.Items {
		for _, ing := range svcList.Items[i].Status.LoadBalancer.Ingress {
			if ing.IP == "" {
				return
			}
		}
	}
	ready = true
	return
}
