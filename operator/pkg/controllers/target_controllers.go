// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"strings"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/target"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *AIStoreReconciler) ensureTargetPrereqs(ctx context.Context, ais *aisv1.AIStore) (err error) {
	// 1. Deploy required ConfigMap
	cm, err := target.NewTargetCM(ctx, ais)
	if err != nil {
		r.recordError(ais, err, "Failed to generate valid target ConfigMap")
		return
	}

	if err = r.client.CreateOrUpdateResource(context.TODO(), ais, cm); err != nil {
		r.recordError(ais, err, "Failed to deploy target ConfigMap")
		return
	}

	// 2. Deploy services
	svc := target.NewTargetHeadlessSvc(ais)
	if err = r.client.CreateOrUpdateResource(ctx, ais, svc); err != nil {
		r.recordError(ais, err, "Failed to deploy target SVC")
		return
	}
	return
}

func (r *AIStoreReconciler) initTargets(ctx context.Context, ais *aisv1.AIStore) (changed bool, err error) {
	// Deploy statefulset
	ss := target.NewTargetSS(ais)
	if exists, err := r.client.CreateResourceIfNotExists(ctx, ais, ss); err != nil {
		r.recordError(ais, err, "Failed to deploy target statefulset")
		return true, err
	} else if !exists {
		msg := "Successfully initialized target nodes"
		logf.FromContext(ctx).Info(msg)
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
	logf.FromContext(ctx).Info("Cleaning up target statefulset")
	targetSS := target.StatefulSetNSName(ais)
	return r.client.DeleteStatefulSetIfExists(ctx, targetSS)
}

func (r *AIStoreReconciler) handleTargetState(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	logger := logf.FromContext(ctx)
	if hasLatest, err := r.handleTargetImage(ctx, ais); !hasLatest || err != nil {
		return false, err
	}
	// Fetch the latest target StatefulSet
	ss, err := r.client.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
	if err != nil {
		return
	}
	if ais.HasState(aisv1.ConditionScaling) {
		// If desired does not match AIS, update the statefulset
		if *ss.Spec.Replicas != ais.GetTargetSize() {
			err = r.resolveStatefulSetScaling(ctx, ais)
			if err != nil {
				return false, err
			}
		} else if *ss.Spec.Replicas != ss.Status.ReadyReplicas {
			logger.Info("Waiting for statefulset replicas to match desired count")
			return false, nil
		}
	}
	// Start the target scaling process by updating services and contacting the AIS API
	if *ss.Spec.Replicas != ais.GetTargetSize() {
		err = r.startTargetScaling(ctx, ais, ss)
		if err != nil {
			return false, err
		}
		// If successful, mark as scaling so future reconciliations will update the SS
		_, err = r.setStatus(ctx, ais, aisv1.AIStoreStatus{State: aisv1.ConditionScaling})
		return false, err
	}
	// For now, state of target is considered ready if the number of target pods ready matches the size provided in AIS cluster spec.
	ready = ss.Status.ReadyReplicas == ais.GetTargetSize()
	return
}

func (r *AIStoreReconciler) resolveStatefulSetScaling(ctx context.Context, ais *aisv1.AIStore) error {
	logger := logf.FromContext(ctx)
	desiredSize := ais.GetTargetSize()
	current, err := r.client.GetStatefulSet(ctx, target.StatefulSetNSName(ais))
	if err != nil {
		return err
	}
	currentSize := *current.Spec.Replicas
	// Scaling up
	if desiredSize > currentSize {
		logger.Info("Scaling up target statefulset to match AIS cluster spec size", "desiredSize", desiredSize)
	} else if desiredSize < currentSize {
		// Don't proceed to update state to ready until we can proceed to statefulset scale-down
		if ready, scaleErr := r.isReadyToScaleDown(ctx, ais, currentSize); scaleErr != nil || !ready {
			return scaleErr
		}
		logger.Info("Scaling down target statefulset to match AIS cluster spec size", "desiredSize", desiredSize)
	}
	_, err = r.client.UpdateStatefulSetReplicas(ctx, target.StatefulSetNSName(ais), desiredSize)
	if err != nil {
		return err
	}
	logger.Info("Finished scaling target statefulset")
	ais.SetState(aisv1.ConditionReady)
	return nil
}

func (r *AIStoreReconciler) isReadyToScaleDown(ctx context.Context, ais *aisv1.AIStore, currentSize int32) (bool, error) {
	logger := logf.FromContext(ctx)
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get API params")
		return false, err
	}

	smap, err := r.GetSmap(ctx, params)
	if err != nil {
		return false, err
	}
	// If any targets are still in the smap as decommissioning, delay scaling
	for _, targetNode := range smap.Tmap {
		if smap.InMaintOrDecomm(targetNode) && !smap.InMaint(targetNode) {
			logger.Info("Delaying scaling. Target still in decommissioning state", "target", targetNode.ID())
			return false, nil
		}
	}
	// If we have the same number of target nodes as current replicas and none showed as decommissioned, don't scale
	if int32(len(smap.Tmap)) == currentSize {
		logger.Info("Delaying scaling. All target nodes are still listed as active")
		return false, nil
	}
	return true, nil
}

func (r *AIStoreReconciler) startTargetScaling(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet) error {
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
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get API params")
		return err
	}

	smap, err := r.GetSmap(ctx, params)
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
			if !smap.InMaintOrDecomm(node) {
				logger.Info("Decommissioning target", "nodeID", node.ID())
				_, err = aisapi.DecommissionNode(*params, &aisapc.ActValRmNode{DaemonID: node.ID(), RmUserData: true})
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

func (r *AIStoreReconciler) handleTargetImage(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	updated, err := r.client.UpdateStatefulSetImage(ctx,
		target.StatefulSetNSName(ais), 0 /*idx*/, ais.Spec.NodeImage)
	if updated || err != nil {
		logf.FromContext(ctx).Info("target image updated")
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

func (r *AIStoreReconciler) scaleUpLB(ctx context.Context, ais *aisv1.AIStore) error {
	if !ais.Spec.EnableExternalLB {
		return nil
	}
	return r.enableTargetExternalService(ctx, ais)
}

func (r *AIStoreReconciler) scaleDownLB(ctx context.Context, ais *aisv1.AIStore, ss *v1.StatefulSet) error {
	if !ais.Spec.EnableExternalLB {
		return nil
	}
	for idx := *ss.Spec.Replicas; idx > ais.GetTargetSize(); idx-- {
		svcName := target.LoadBalancerSVCNSName(ais, idx-1)
		_, err := r.client.DeleteServiceIfExists(ctx, svcName)
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
		err := r.client.CreateOrUpdateResource(ctx, ais, svc)
		if err != nil {
			return err
		}
	}

	// 2. Ensure every service has an ingress IP assigned to it.
	svcList := &corev1.ServiceList{}
	err := r.client.List(ctx, svcList, client.MatchingLabels(target.ExternalServiceLabels(ais)))
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
