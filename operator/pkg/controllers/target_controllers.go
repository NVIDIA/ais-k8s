// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/target"
)

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore, customConfig *aiscmn.ConfigToUpdate) (changed bool, err error) {
	var cm *corev1.ConfigMap
	// 1. Deploy required ConfigMap
	cm, err = target.NewTargetCM(ais, customConfig)
	if err != nil {
		r.log.Error(err, "failed to generate valid target ConfigMap")
		return
	}

	if _, err = r.client.CreateResourceIfNotExists(context.TODO(), ais, cm); err != nil {
		r.log.Error(err, "failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, svc); err != nil {
		r.log.Error(err, "failed to deploy SVC")
		return
	}

	// 3. Deploy statefulset
	ss := target.NewTargetSS(ais)
	if exists, err := r.client.CreateResourceIfNotExists(ctx, ais, ss); err != nil {
		r.log.Error(err, "failed to deploy Primary proxy")
		return false, err
	} else if !exists {
		r.log.Info("successfully initialized target nodes")
		changed = true
	}
	return
}

func (r *AIStoreReconciler) cleanupTarget(ctx context.Context, ais *aisv1.AIStore) error {
	err := r.client.DeleteStatefulSetIfExists(ctx, target.StatefulSetNSName(ais))
	if err != nil {
		return err
	}

	err = r.client.DeleteServiceIfExists(ctx, target.HeadlessSVCNSName(ais))
	if err != nil {
		return err
	}

	err = r.client.DeleteAllServicesIfExists(ctx, ais.Namespace, client.MatchingLabels(target.ExternalServiceLabels(ais)))
	if err != nil {
		return err
	}

	return r.client.DeleteConfigMapIfExists(ctx, target.ConfigMapNSName(ais))
}

func (r *AIStoreReconciler) handleTargetState(ctx context.Context, ais *aisv1.AIStore) (state daemonState, err error) {
	targetSSName := target.StatefulSetNSName(ais)
	// Fetch the latest statefulset for targets and check if it's spec (for now just replicas), matches the AIS cluster spec.
	ss, err := r.client.GetStatefulSet(ctx, targetSSName)
	if err != nil {
		return state, err
	}
	if *ss.Spec.Replicas != ais.Spec.Size {
		state.isUpdated = true
		_, err = r.client.UpdateStatefulSetReplicas(ctx, targetSSName, ais.Spec.Size)
		// TODO: Deal with target scale-down; should decommission to avoid data loss.
		return
	}
	// For now, state of target is considered ready if the number of target pods ready matches the size provided in AIS cluster spec.
	state.isReady = ss.Status.ReadyReplicas == ais.Spec.Size
	return
}

// enableTargetExternalService, creates a loadbalancer service per target and checks if all the services are assigned an external IP.
func (r *AIStoreReconciler) enableTargetExternalService(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	var (
		targetSVCList = target.NewTargetLoadBalancerSVCList(ais)
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

	for _, svc := range svcList.Items {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP == "" {
				return
			}
		}
	}
	ready = true
	return
}
