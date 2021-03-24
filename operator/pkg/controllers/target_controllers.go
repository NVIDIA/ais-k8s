// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"

	"github.com/ais-operator/pkg/resources/cmn"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aisv1 "github.com/ais-operator/api/v1alpha1"
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
	// Check status of all target pods, if any target pod is in non-termination
	// state implies the statefulset is not yet deleted. Attempt to gracefully shutdown cluster.
	targetPods := &corev1.PodList{}
	err = r.client.List(ctx, targetPods, client.InNamespace(ais.Namespace), client.MatchingLabels(target.PodLabels(ais)))
	if err != nil {
		r.log.Error(err, "failed to list target pods")
	}

	if err == nil {
		var anyRunning bool
		for idx := range targetPods.Items {
			pod := targetPods.Items[idx]
			if pod.Status.Reason != "Terminating" {
				anyRunning = true
				break
			}
		}
		if anyRunning {
			r.attemptGracefulShutdown(ctx, ais)
		}
	}
	return r.client.DeleteStatefulSetIfExists(ctx, target.StatefulSetNSName(ais))
}

func (r *AIStoreReconciler) handleTargetState(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
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

// TODO: Decommission a target first to avoid data loss.
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

	// If anything was updated, we consider it not immediately ready.
	updated, err := r.client.UpdateStatefulSetReplicas(ctx, targetSS, ais.Spec.Size)
	return !updated, err
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
