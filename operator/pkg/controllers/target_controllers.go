// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"strings"

	"github.com/ais-operator/pkg/resources/cmn"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aisapi "github.com/NVIDIA/aistore/api"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/target"
)

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore) (changed bool, err error) {
	var cm *corev1.ConfigMap
	// 1. Deploy required ConfigMap
	cm, err = target.NewTargetCM(ais)
	if err != nil {
		r.recordError(ais, err, "Failed to generate valid target ConfigMap")
		return
	}

	if _, err = r.client.CreateResourceIfNotExists(context.TODO(), ais, cm); err != nil {
		r.recordError(ais, err, "Failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, svc); err != nil {
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
	// If the target statefulset it not present, we can return immediately.
	targetSS := target.StatefulSetNSName(ais)
	if exists, err := r.client.StatefulSetExists(ctx, targetSS); err != nil || !exists {
		return false, err
	}

	// If we reach here implies, we didn't attempt to shutdown the cluster yet.
	// Attempt graceful cluster shutdown followed by deleting target statefulset.
	r.attemptGracefulShutdown(ctx, ais)
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
	if *ss.Spec.Replicas != ais.Spec.Size {
		ready, err = r.handleTargetScaling(ctx, ais, ss, targetSSName)
		if !ready || err != nil {
			return false, err
		}
	}
	// For now, state of target is considered ready if the number of target pods ready matches the size provided in AIS cluster spec.
	ready = ss.Status.ReadyReplicas == ais.Spec.Size
	return
}

func (r *AIStoreReconciler) handleTargetScaling(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet, targetSS types.NamespacedName) (ready bool, err error) {
	if *ss.Spec.Replicas < ais.Spec.Size {
		// Current SS has fewer replicas than expected size - scale up.
		return r.handleTargetScaleUp(ctx, ais, targetSS)
	}

	// Otherwise - scale down.
	return r.handleTargetScaleDown(ctx, ais, ss, targetSS)
}

func (r *AIStoreReconciler) handleTargetScaleDown(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet, targetSS types.NamespacedName) (ready bool, err error) {
	if ais.Spec.EnableExternalLB {
		ready = true
		for idx := *ss.Spec.Replicas; idx > ais.Spec.Size; idx-- {
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
	decomissioning, err := r.decommissionTargets(ctx, ais, *ss.Spec.Replicas)
	if decomissioning || err != nil {
		return false, err
	}

	// If anything was updated, we consider it not immediately ready.
	updated, err := r.client.UpdateStatefulSetReplicas(ctx, targetSS, ais.Spec.Size)
	return !updated, err
}

func (r *AIStoreReconciler) decommissionTargets(ctx context.Context, ais *aisv1.AIStore, actualSize int32) (decommissioning bool, err error) {
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		return false, err
	}

	smap, err := aisapi.GetClusterMap(*params)
	if err != nil {
		return false, err
	}

	toDecommission := 0
	for idx := actualSize; idx > ais.Spec.Size; idx-- {
		podName := target.PodName(ais, idx-1)
		for _, node := range smap.Tmap {
			if !strings.HasPrefix(node.IntraControlNet.NodeHostname, podName) {
				continue
			}
			toDecommission++
			if !smap.PresentInMaint(node) {
				r.log.Info("decommissioning node - " + node.String())
				_, err = aisapi.DecommissionNode(*params, &aiscmn.ActValRmNode{DaemonID: node.ID(), RmUserData: true})
				if err != nil {
					return
				}
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

	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList, client.InNamespace(ais.Namespace), client.MatchingLabels(target.PodLabels(ais)))
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
	updated, err := r.client.UpdateStatefulSetReplicas(ctx, targetSS, ais.Spec.Size)
	return !updated, err
}

// enableTargetExternalService, creates a loadbalancer service per target and checks if all the services are assigned an external IP.
func (r *AIStoreReconciler) enableTargetExternalService(ctx context.Context,
	ais *aisv1.AIStore) (ready bool, err error) {
	var (
		targetSVCList = target.NewLoadBalancerSVCList(ais)
		exists        bool
		allExist      = true
	)
	// 1. Try creating a LoadBalancer for each target pod, if the SVC are already created (`allExists` == true), then proceed to checking their status.
	for _, svc := range targetSVCList {
		exists, err = r.client.CreateResourceIfNotExists(ctx, ais, svc)
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
