// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aisapi "github.com/NVIDIA/aistore/api"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
)

const primaryStartTimeout = time.Minute * 3

func (r *AIStoreReconciler) initProxies(ctx context.Context, ais *aisv1.AIStore) (changed bool, err error) {
	var (
		cm     *corev1.ConfigMap
		exists bool
	)

	// 1. Deploy required ConfigMap
	cm, err = proxy.NewProxyCM(ais)
	if err != nil {
		r.recordError(ais, err, "Failed to generate valid proxy ConfigMap")
		return
	}

	if _, err = r.client.CreateResourceIfNotExists(context.TODO(), ais, cm); err != nil {
		r.recordError(ais, err, "Failed to deploy ConfigMap")
		return
	}

	// 2. Deploy services
	svc := proxy.NewProxyHeadlessSvc(ais)
	if _, err = r.client.CreateResourceIfNotExists(ctx, ais, svc); err != nil {
		r.recordError(ais, err, "Failed to deploy SVC")
		return
	}

	// 3. Create a proxy statefulset with single replica as primary
	pod := proxy.NewProxyStatefulSet(ais, 1)
	if exists, err = r.client.CreateResourceIfNotExists(ctx, ais, pod); err != nil {
		r.recordError(ais, err, "Failed to deploy Primary proxy")
		return
	} else if !exists {
		changed = true
		return
	}

	// Wait for primary to start-up.
	if err = r.client.WaitForPodReady(ctx, proxy.DefaultPrimaryNSName(ais), primaryStartTimeout); err != nil {
		return
	}

	// 4. Start all the proxy daemons
	changed, err = r.client.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), ais.Spec.Size)
	if err != nil {
		r.recordError(ais, err, "Failed to deploy StatefulSet")
		return
	}
	if changed {
		msg := "Successfully initialized proxy nodes"
		r.log.Info(msg)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, msg)
	}
	return
}

func (r *AIStoreReconciler) cleanupProxy(ctx context.Context, ais *aisv1.AIStore) (anyExisted bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) { return r.client.DeleteStatefulSetIfExists(ctx, proxy.StatefulSetNSName(ais)) },
		func() (bool, error) { return r.client.DeleteServiceIfExists(ctx, proxy.HeadlessSVCNSName(ais)) },
		func() (bool, error) { return r.client.DeleteServiceIfExists(ctx, proxy.LoadBalancerSVCNSName(ais)) },
		func() (bool, error) { return r.client.DeleteConfigMapIfExists(ctx, proxy.ConfigMapNSName(ais)) },
	)
}

func (r *AIStoreReconciler) handleProxyState(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	if hasLatest, err := r.handleProxyImage(ctx, ais); !hasLatest || err != nil {
		return false, err
	}

	proxySSName := proxy.StatefulSetNSName(ais)
	// Fetch the latest statefulset for proxies and check if it's spec (for now just replicas), matches the AIS cluster spec.
	ss, err := r.client.GetStatefulSet(ctx, proxySSName)
	if err != nil {
		return ready, err
	}

	if *ss.Spec.Replicas != ais.Spec.Size {
		if *ss.Spec.Replicas > ais.Spec.Size {
			// If the cluster is scaling down, ensure the pod being delete is not primary.
			r.handleProxyScaledown(ctx, ais, *ss.Spec.Replicas)
		}

		// If anything was updated, we consider it not immediately ready.
		updated, err := r.client.UpdateStatefulSetReplicas(ctx, proxySSName, ais.Spec.Size)
		if updated || err != nil {
			return false, err
		}
	}

	// For now, state of proxy is considered ready if the number of proxy pods ready matches the size provided in AIS cluster spec.
	return ss.Status.ReadyReplicas == ais.Spec.Size, nil
}

func (r *AIStoreReconciler) handleProxyImage(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	ss, err := r.client.GetStatefulSet(ctx, proxy.StatefulSetNSName(ais))
	if err != nil {
		return
	}
	firstPodName := proxy.PodName(ais, 0)
	updated := ss.Spec.Template.Spec.Containers[0].Image != ais.Spec.NodeImage
	if updated {
		if err := r.setPrimaryTo(ctx, ais, 0); err != nil {
			r.log.Error(err, "failed to set primary proxy")
			return false, err
		}
		r.log.Info("updated primary to pod " + firstPodName)
		ss.Spec.Template.Spec.Containers[0].Image = ais.Spec.NodeImage
		ss.Spec.UpdateStrategy = apiv1.StatefulSetUpdateStrategy{
			Type: apiv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &apiv1.RollingUpdateStatefulSetStrategy{
				Partition: func(v int32) *int32 { return &v }(1),
			},
		}
		return false, r.client.Update(ctx, ss)
	}

	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList, client.InNamespace(ais.Namespace), client.MatchingLabels(proxy.PodLabels(ais)))
	if err != nil {
		return
	}
	var (
		toUpdate         int
		firstYetToUpdate bool
	)
	for idx := range podList.Items {
		pod := podList.Items[idx]
		if pod.Spec.Containers[0].Image == ais.Spec.NodeImage {
			continue
		}
		toUpdate++
		firstYetToUpdate = firstYetToUpdate || pod.Name == firstPodName
	}

	// NOTE: In case of statefulset rolling update strategy,
	// pod are updated in decending of their pod index.
	// This implies, pod with largest index is oldest proxy,
	// and we set it as a primary.
	if toUpdate == 1 && firstYetToUpdate {
		if err := r.setPrimaryTo(ctx, ais, *ss.Spec.Replicas-1); err != nil {
			return false, err
		}
		// Revert statefulset partition spec
		ss.Spec.UpdateStrategy = apiv1.StatefulSetUpdateStrategy{
			Type: apiv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &apiv1.RollingUpdateStatefulSetStrategy{
				Partition: func(v int32) *int32 { return &v }(0),
			},
		}

		if err := r.client.Update(ctx, ss); err != nil {
			r.log.Error(err, "failed to update proxy statefulset update policy")
			return false, err
		}

		// Delete the first pod to update it's docker image.
		return false, r.client.DeletePodIfExists(ctx, types.NamespacedName{
			Namespace: ais.Namespace,
			Name:      firstPodName,
		})
	}
	return toUpdate == 0, nil
}

func (r *AIStoreReconciler) setPrimaryTo(ctx context.Context, ais *aisv1.AIStore, podIdx int32) error {
	podName := proxy.PodName(ais, podIdx)
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		err = fmt.Errorf("failed to obtain BaseAPIParams, err: %v", err)
		return err
	}

	smap, err := aisapi.GetClusterMap(*params)
	if err != nil {
		err = fmt.Errorf("failed to obtain smap, err: %v", err)
		return err
	}

	if strings.HasPrefix(smap.Primary.IntraControlNet.NodeHostname, podName) {
		return nil
	}

	for _, node := range smap.Pmap {
		if !strings.HasPrefix(node.IntraControlNet.NodeHostname, podName) {
			continue
		}
		return aisapi.SetPrimaryProxy(*params, node.ID(), true /*force*/)
	}
	return fmt.Errorf("couldn't find a proxy node for pod %q", podName)
}

// handleProxyScaledown decommissions all the proxy nodes that will be deleted due to scale down.
// If the node being deleted is a primary, a new primary is designated before decommissioning.
func (r *AIStoreReconciler) handleProxyScaledown(ctx context.Context, ais *aisv1.AIStore, actualSize int32) {
	params, err := r.getAPIParams(ctx, ais)
	if err != nil {
		r.log.Error(err, "failed to obtain BaseAPIParams")
		return
	}

	smap, err := aisapi.GetClusterMap(*params)
	if err != nil {
		r.log.Error(err, "failed to obtain smap")
		return
	}

	decommissionNode := func(daemonID string) {
		_, err := aisapi.DecommissionNode(*params, &aiscmn.ActValRmNode{
			DaemonID: daemonID,
		})
		if err != nil {
			r.log.Error(err, "failed to decommission node - "+daemonID)
		}
	}

	var oldPrimaryID string
	for idx := actualSize; idx > ais.Spec.Size; idx-- {
		podName := proxy.PodName(ais, idx-1)
		for daeID, node := range smap.Pmap {
			if !strings.HasPrefix(node.IntraControlNet.NodeHostname, podName) {
				continue
			}
			delete(smap.Pmap, daeID)
			if smap.IsPrimary(node) {
				oldPrimaryID = daeID
				continue
			}
			decommissionNode(daeID)
		}
	}
	if oldPrimaryID == "" {
		return
	}

	// Set new primary before decommissioning old primary
	for _, node := range smap.Pmap {
		if smap.PresentInMaint(node) {
			continue
		}
		err := aisapi.SetPrimaryProxy(*params, node.DaemonID, true /*force*/)
		if err != nil {
			r.log.Error(err, "failed to set primary as "+node.DaemonID)
			continue
		}
		decommissionNode(oldPrimaryID)
	}
}

// enableProxyExternalService, creates a LoadBalancer service for proxy statefulset.
// NOTE: As opposed to `target` external services, where we have a separate LoadBalancer service per pod,
// `proxies` have a single LoadBalancer service across all the proxy pods.
func (r *AIStoreReconciler) enableProxyExternalService(ctx context.Context,
	ais *aisv1.AIStore) (ready bool, err error) {
	proxyLBSVC := proxy.NewProxyLoadBalancerSVC(ais)
	exists, err := r.client.CreateResourceIfNotExists(ctx, ais, proxyLBSVC)
	if err != nil || !exists {
		return
	}

	// If SVC already exists, check if external IP is allocated
	proxyLBSVC, err = r.client.GetServiceByName(ctx, proxy.LoadBalancerSVCNSName(ais))
	if err != nil {
		return
	}

	for _, ing := range proxyLBSVC.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			ready = true
			return
		}
	}
	return
}
