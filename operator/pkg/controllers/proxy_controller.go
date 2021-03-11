// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"time"

	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	corev1 "k8s.io/api/core/v1"
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
	proxySSName := proxy.StatefulSetNSName(ais)

	// Fetch the latest statefulset for proxies and check if it's spec (for now just replicas), matches the AIS cluster spec.
	ss, err := r.client.GetStatefulSet(ctx, proxySSName)
	if err != nil {
		return ready, err
	}
	if *ss.Spec.Replicas != ais.Spec.Size {
		// If anything was updated, we consider it not immediately ready.
		updated, err := r.client.UpdateStatefulSetReplicas(ctx, proxySSName, ais.Spec.Size)
		if updated || err != nil {
			return false, err
		}
	}

	// For now, state of proxy is considered ready if the number of proxy pods ready matches the size provided in AIS cluster spec.
	return ss.Status.ReadyReplicas == ais.Spec.Size, nil
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
