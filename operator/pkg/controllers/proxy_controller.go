// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	proxyStartupInterval = 5 * time.Second
	proxyDNSInterval     = 5 * time.Second
)

func (r *AIStoreReconciler) ensureProxyPrereqs(ctx context.Context, ais *aisv1.AIStore) (err error) {
	var cm *corev1.ConfigMap

	// 1. Deploy required ConfigMap
	cm, err = proxy.NewProxyCM(ais)
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to generate valid proxy ConfigMap")
		return
	}

	if err = r.k8sClient.CreateOrUpdateResource(context.TODO(), ais, cm); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy ConfigMap")
		return
	}

	svc := proxy.NewProxyHeadlessSvc(ais)
	if err = r.k8sClient.CreateOrUpdateResource(ctx, ais, svc); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy SVC")
		return
	}
	return
}

func (r *AIStoreReconciler) initProxies(ctx context.Context, ais *aisv1.AIStore) (ctrl.Result, error) {
	var (
		err     error
		exists  bool
		changed bool
		logger  = logf.FromContext(ctx)
	)

	// 1. Create a proxy statefulset with single replica as primary
	ss := proxy.NewProxyStatefulSet(ais, 1)
	if exists, err = r.k8sClient.CreateResourceIfNotExists(ctx, ais, ss); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy Primary proxy")
		return ctrl.Result{}, err
	} else if !exists {
		return ctrl.Result{RequeueAfter: proxyStartupInterval}, nil
	}

	// Wait for primary to start-up.
	_, err = r.k8sClient.GetReadyPod(ctx, proxy.DefaultPrimaryNSName(ais))
	if err != nil {
		logger.Info("Waiting for primary proxy to come up", "err", err.Error())
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonWaiting, "Waiting for primary proxy to come up")
		return ctrl.Result{RequeueAfter: proxyStartupInterval}, nil
	}

	// 2. Start all the proxy daemons
	changed, err = r.k8sClient.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), ais.GetProxySize())
	if err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy StatefulSet")
		return ctrl.Result{}, err
	}
	if changed {
		msg := "Successfully initialized proxy nodes"
		logger.Info(msg)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonInitialized, msg)
	}

	// Check whether proxy service has resolvable endpoints.
	return r.checkProxySvcEndpoints(ctx, ais)
}

func (r *AIStoreReconciler) checkProxySvcEndpoints(ctx context.Context, ais *aisv1.AIStore) (ctrl.Result, error) {
	svcName := proxy.HeadlessSVCNSName(ais)
	logger := logf.FromContext(ctx).WithValues("service", svcName.Name)
	endpoints, err := r.k8sClient.GetServiceEndpoints(ctx, svcName)
	if err != nil {
		logger.Error(err, "Failed to get service endpoints")
		return ctrl.Result{}, err
	}
	for i := range endpoints.Items {
		slice := &endpoints.Items[i]
		// Found a ready endpoint in an endpoint slice for the proxy SVC
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				return ctrl.Result{}, nil
			}
		}
	}
	logger.Info("No ready endpoints available")
	r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonWaiting, "Waiting for proxy service to have registered endpoints")
	return ctrl.Result{RequeueAfter: proxyDNSInterval}, nil
}

func (r *AIStoreReconciler) cleanupProxy(ctx context.Context, ais *aisv1.AIStore) (anyExisted bool, err error) {
	return cmn.AnyFunc(
		func() (bool, error) { return r.k8sClient.DeleteStatefulSetIfExists(ctx, proxy.StatefulSetNSName(ais)) },
		func() (bool, error) { return r.k8sClient.DeleteServiceIfExists(ctx, proxy.HeadlessSVCNSName(ais)) },
		func() (bool, error) { return r.k8sClient.DeleteServiceIfExists(ctx, proxy.LoadBalancerSVCNSName(ais)) },
		func() (bool, error) { return r.k8sClient.DeleteConfigMapIfExists(ctx, proxy.ConfigMapNSName(ais)) },
	)
}

func (r *AIStoreReconciler) handleProxyState(ctx context.Context, ais *aisv1.AIStore) (result ctrl.Result, err error) {
	proxySSName := proxy.StatefulSetNSName(ais)
	ss, err := r.k8sClient.GetStatefulSet(ctx, proxySSName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return r.initProxies(ctx, ais)
		}
		return
	}

	updated, err := r.syncProxyPodSpec(ctx, ais, ss)
	if err != nil {
		return
	}
	// Fetch the latest statefulset with any spec updates
	if updated {
		ss, err = r.k8sClient.GetStatefulSet(ctx, proxySSName)
		if err != nil {
			return
		}
	}

	err = r.handleProxyRollout(ctx, ais, ss)
	if err != nil {
		return
	}
	err = r.handleProxyScale(ctx, ais, ss)
	if err != nil {
		return
	}

	return r.waitForProxyReplicasReady(ctx, ais, ss), nil
}

func (r *AIStoreReconciler) handleProxyScale(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) error {
	currentReplicas := *ss.Spec.Replicas
	desiredReplicas := ais.GetProxySize()

	// If the current replicas match the desired replicas, no scaling is needed
	if currentReplicas == desiredReplicas {
		return nil
	}

	// If the current replicas are greater than the desired replicas, decommission
	// proxies to be scaled down and move primary to the lowest ready pod if needed
	if currentReplicas > desiredReplicas {
		if err := r.handleProxyScaledown(ctx, ais, currentReplicas); err != nil {
			return err
		}
	}

	_, err := r.k8sClient.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), desiredReplicas)
	return err
}

func (*AIStoreReconciler) waitForProxyReplicasReady(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) ctrl.Result {
	desired := ais.GetProxySize()
	if ss.Status.ReadyReplicas == desired && ss.Status.Replicas == desired {
		return ctrl.Result{}
	}

	logf.FromContext(ctx).Info("Waiting for proxy StatefulSet to reach desired replicas",
		"ready", ss.Status.ReadyReplicas,
		"replicas", ss.Status.Replicas,
		"desired", desired)

	return ctrl.Result{RequeueAfter: proxyStartupInterval}
}

func (r *AIStoreReconciler) syncProxyPodSpec(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (updated bool, err error) {
	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name)
	desiredTemplate := &proxy.NewProxyStatefulSet(ais, ais.GetProxySize()).Spec.Template

	needsUpdate, reason := shouldUpdatePodTemplate(desiredTemplate, &ss.Spec.Template)
	if !needsUpdate {
		return
	}

	updatedSS := ss.DeepCopy()
	// Any change to pod template will trigger a new rollout, so any changes to the SS should happen here
	// If we have an active cluster, set primary to 0 before triggering rollout
	if updatedSS.Status.ReadyReplicas > 0 {
		err = r.setPrimaryTo(ctx, ais, 0)
		if err != nil {
			logger.Error(err, "failed to set primary proxy", "podIndex", 0)
			return
		}
		logger.Info("Updated primary to pod", "pod", proxy.PodName(ais, 0), "reason", reason)
		// Block updating the primary
		updatedSS.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
				Partition: aisapc.Ptr(int32(1)),
			},
		}
	}
	syncPodTemplate(desiredTemplate, &updatedSS.Spec.Template)
	updated = true
	logger.Info("Proxy pod template spec modified", "reason", reason)
	patch := client.MergeFrom(ss)
	err = r.k8sClient.Patch(ctx, updatedSS, patch)
	if err != nil {
		return
	}
	logger.Info("Statefulset successfully updated", "reason", reason)
	return
}

func (r *AIStoreReconciler) handleProxyRollout(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) error {
	// If rollout is complete, current revision will match update revision
	if ss.Status.UpdateRevision == ss.Status.CurrentRevision {
		return nil
	}

	// Reset partition to update last pod
	if shouldResetPartition(ss) {
		err := r.setHighestPodAsPrimary(ctx, ais, ss)
		if err != nil {
			return err
		}
		err = r.resetSSPartition(ctx, ss)
		if err != nil {
			return err
		}
	}
	return nil
}

// With statefulset rolling update strategy, pods are updated in descending order of their pod index.
// This implies the pod with the largest index is the oldest proxy, and we set it as primary.
func (r *AIStoreReconciler) setHighestPodAsPrimary(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (err error) {
	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name)
	podIndex := *ss.Spec.Replicas - 1
	logger.Info("Setting highest pod as primary", "podIndex", podIndex)
	err = r.setPrimaryTo(ctx, ais, podIndex)
	if err != nil {
		logger.Error(err, "failed to set primary proxy", "podIndex", podIndex)
	}
	return
}

func shouldResetPartition(ss *appsv1.StatefulSet) bool {
	// Not using rolling update
	if ss.Spec.UpdateStrategy.RollingUpdate == nil {
		return false
	}
	// Already reset
	if *ss.Spec.UpdateStrategy.RollingUpdate.Partition == int32(0) {
		return false
	}
	// Reset to allow updating the last pod (lowest ordinal)
	return ss.Status.CurrentReplicas == 1
}

func (r *AIStoreReconciler) resetSSPartition(ctx context.Context, ss *appsv1.StatefulSet) (err error) {
	logger := logf.FromContext(ctx).WithValues("statefulset", ss.Name)
	logger.Info("Removing partition from rolling update strategy")
	// Revert statefulset partition spec
	updatedSS := ss.DeepCopy()
	updatedSS.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: aisapc.Ptr(int32(0)),
		},
	}
	patch := client.MergeFrom(ss)
	err = r.k8sClient.Patch(ctx, updatedSS, patch)
	if err != nil {
		logger.Error(err, "failed to patch statefulset update strategy")
	}
	return
}

func (r *AIStoreReconciler) setPrimaryTo(ctx context.Context, ais *aisv1.AIStore, podIdx int32) error {
	podName := proxy.PodName(ais, podIdx)
	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return err
	}
	smap, err := apiClient.GetClusterMap()
	if err != nil {
		return err
	}
	// Primary already set to pod at given pod index
	if strings.HasPrefix(smap.Primary.ControlNet.Hostname, podName) {
		return nil
	}

	node, err := findAISNodeByPodName(smap.Pmap, podName)
	if err != nil {
		return err
	}
	logf.FromContext(ctx).Info("Setting primary proxy", "pod", podName)
	return apiClient.SetPrimaryProxy(node.ID(), node.PubNet.URL, true /*force*/)
}

// handleProxyScaledown decommissions all the proxy nodes that will be deleted due to scale down.
// If the node being deleted is a primary, a new primary is designated before decommissioning.
func (r *AIStoreReconciler) handleProxyScaledown(ctx context.Context, ais *aisv1.AIStore, currentSize int32) (err error) {
	logger := logf.FromContext(ctx)
	desiredSize := ais.GetProxySize()

	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		logger.Error(err, "failed to get API client")
		return
	}
	smap, err := apiClient.GetClusterMap()
	if err != nil {
		logger.Error(err, "failed to get cluster map")
		return
	}

	// Find the current primary pod index
	currentPrimaryPodIdx := int32(-1)
	for idx := range currentSize {
		if strings.HasPrefix(smap.Primary.ControlNet.Hostname, proxy.PodName(ais, idx)) {
			currentPrimaryPodIdx = idx
			break
		}
	}

	// If current primary will be removed, reassign it first
	if currentPrimaryPodIdx >= desiredSize {
		if err = r.reassignPrimaryForScaledown(ctx, ais, smap); err != nil {
			logger.Error(err, "failed to reassign primary for scaledown")
			return
		}
	}

	// Decommission nodes from highest index down (best-effort)
	for idx := currentSize - 1; idx >= desiredSize; idx-- {
		podName := proxy.PodName(ais, idx)
		node, err := findAISNodeByPodName(smap.Pmap, podName)
		if err != nil {
			logger.Info("Proxy node not found in cluster map", "podName", podName)
			continue
		}
		logger.Info("Decommissioning proxy node", "nodeID", node.ID(), "podName", podName)
		rmAction := &aisapc.ActValRmNode{DaemonID: node.ID()}
		if _, err := apiClient.DecommissionNode(rmAction); err != nil {
			logger.Error(err, "failed to decommission node", "nodeID", node.ID())
		}
	}
	return
}

func (r *AIStoreReconciler) reassignPrimaryForScaledown(ctx context.Context, ais *aisv1.AIStore, smap *aismeta.Smap) (err error) {
	logger := logf.FromContext(ctx)
	for idx := range ais.GetProxySize() {
		var node *aismeta.Snode
		podName := proxy.PodName(ais, idx)
		node, err = findAISNodeByPodName(smap.Pmap, podName)
		if err != nil {
			logger.Error(err, "failed to find node by pod name, trying next pod", "podName", podName)
			continue
		}
		if !smap.InMaintOrDecomm(node.ID()) {
			_, err = r.k8sClient.GetReadyPod(ctx, types.NamespacedName{Name: podName, Namespace: ais.Namespace})
			if err != nil {
				logger.Error(err, "failed to get ready pod, trying next pod", "podIndex", idx)
				continue
			}
			err = r.setPrimaryTo(ctx, ais, idx)
			if err != nil {
				logger.Error(err, "failed to set primary, trying next pod", "podIndex", idx)
				continue
			}
			logger.Info("Set new primary before scale down", "podIndex", idx)
			return
		}
	}
	return fmt.Errorf("no pod found to set as primary")
}

// enableProxyExternalService, creates a LoadBalancer service for proxy statefulset.
// NOTE: As opposed to `target` external services, where we have a separate LoadBalancer service per pod,
// `proxies` have a single LoadBalancer service across all the proxy pods.
func (r *AIStoreReconciler) enableProxyExternalService(ctx context.Context, ais *aisv1.AIStore) (ready bool, err error) {
	proxyLBSVC := proxy.NewProxyLoadBalancerSVC(ais)
	err = r.k8sClient.CreateOrUpdateResource(ctx, ais, proxyLBSVC)
	if err != nil {
		return
	}

	// If SVC already exists, check if external IP is allocated
	proxyLBSVC, err = r.k8sClient.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
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
