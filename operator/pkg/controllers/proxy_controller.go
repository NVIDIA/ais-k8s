// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	primaryStartTimeout = time.Minute
	// Default coreDNS cache time is 30 seconds -- should be patched for faster runs on test runners
	dnsEntryWaitTimeout = 40 * time.Second
	dnsEntryInterval    = 2 * time.Second
)

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
	changed, err = r.client.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), ais.GetProxySize())
	if err != nil {
		r.recordError(ais, err, "Failed to deploy StatefulSet")
		return
	}
	if changed {
		msg := "Successfully initialized proxy nodes"
		r.log.Info(msg)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, msg)
	}

	// Wait for proxy service to have a registered DNS entry
	if err = r.waitForDNSEntry(ctx, ais.GetClusterDomain(), svc, dnsEntryInterval, dnsEntryWaitTimeout); err != nil {
		r.recordError(ais, err, "Failed while waiting for DNS entry for proxy service")
	}
	return
}

func (r *AIStoreReconciler) waitForDNSEntry(ctx context.Context, clusterDomain string, svc *corev1.Service, retryInterval, timeout time.Duration) error {
	hostname := fmt.Sprintf("%s.%s.svc.%s", svc.Name, svc.Namespace, clusterDomain)
	return wait.PollUntilContextTimeout(ctx, retryInterval, timeout, true /*immediate*/, func(_ context.Context) (done bool, err error) {
		if _, err = net.LookupIP(hostname); err != nil {
			r.log.Info("Waiting for proxy service DNS entry...", "hostname", hostname)
			return false, nil
		}
		return true, nil // DNS entry found
	})
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

	if *ss.Spec.Replicas != ais.GetProxySize() {
		if *ss.Spec.Replicas > ais.GetProxySize() {
			// If the cluster is scaling down, ensure the pod being delete is not primary.
			r.handleProxyScaledown(ctx, ais, *ss.Spec.Replicas)
		}
		err = r.verifyNodesAvailable(ctx, ais, aisapc.Proxy)
		if err != nil {
			return false, err
		}
		// If anything was updated, we consider it not immediately ready.
		updated, err := r.client.UpdateStatefulSetReplicas(ctx, proxySSName, ais.GetProxySize())
		if updated || err != nil {
			return false, err
		}
	}

	// For now, state of proxy is considered ready if the number of proxy pods ready matches the size provided in AIS cluster spec.
	return ss.Status.ReadyReplicas == ais.GetProxySize(), nil
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

	podList, err := r.client.ListProxyPods(ctx, ais)
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

	if strings.HasPrefix(smap.Primary.ControlNet.Hostname, podName) {
		return nil
	}

	for _, node := range smap.Pmap {
		if !strings.HasPrefix(node.ControlNet.Hostname, podName) {
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
		_, err := aisapi.DecommissionNode(*params, &aisapc.ActValRmNode{
			DaemonID: daemonID,
		})
		if err != nil {
			r.log.Error(err, "failed to decommission node - "+daemonID)
		}
	}

	var oldPrimaryID string
	for idx := actualSize; idx > ais.GetProxySize(); idx-- {
		podName := proxy.PodName(ais, idx-1)
		for daeID, node := range smap.Pmap {
			if !strings.HasPrefix(node.ControlNet.Hostname, podName) {
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
		if smap.InMaintOrDecomm(node) {
			continue
		}
		err := aisapi.SetPrimaryProxy(*params, node.DaeID, true /*force*/)
		if err != nil {
			r.log.Error(err, "failed to set primary as "+node.DaeID)
			continue
		}
		decommissionNode(oldPrimaryID)
	}
}

// Scale down the statefulset without decommissioning or resetting primary
func (r *AIStoreReconciler) scaleProxiesToZero(ctx context.Context, ais *aisv1.AIStore) error {
	r.log.Info("Scaling proxies to zero", "clusterName", ais.Name)
	changed, err := r.client.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), 0)
	if err != nil {
		r.log.Error(err, "Failed to scale proxies to zero", "clusterName", ais.Name)
	} else if changed {
		r.log.Info("Proxy StatefulSet set to size 0", "name", ais.Name)
	} else {
		r.log.Info("Proxy StatefulSet already at size 0", "name", ais.Name)
	}
	return err
}

// enableProxyExternalService, creates a LoadBalancer service for proxy statefulset.
// NOTE: As opposed to `target` external services, where we have a separate LoadBalancer service per pod,
// `proxies` have a single LoadBalancer service across all the proxy pods.
func (r *AIStoreReconciler) enableProxyExternalService(ctx context.Context,
	ais *aisv1.AIStore,
) (ready bool, err error) {
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
