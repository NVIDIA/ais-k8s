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

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	proxyStartupInterval = 5 * time.Second
	proxyDNSInterval     = 5 * time.Second
	proxyDNSTimeout      = 10 * time.Second
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

	// 1. Create a proxy StatefulSet with single replica as primary
	ss := proxy.NewProxyStatefulSet(ais, 1)
	if exists, err = r.k8sClient.CreateResourceIfNotExists(ctx, ais, ss); err != nil {
		r.recordError(ctx, ais, err, "Failed to deploy Primary proxy")
		return ctrl.Result{}, err
	} else if !exists {
		return ctrl.Result{Requeue: true}, nil
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

	// If an upgrade or scaling operation is in progress, handle it
	switch {
	case ais.HasState(aisv1.ClusterProxyScaling):
		if res := handleProxyScale(ctx, ais, ss); !res.IsZero() {
			return res, nil
		}
	case ais.HasState(aisv1.ClusterProxyUpgrading):
		if res, err := r.handleProxyUpgrade(ctx, ais, ss); err != nil || !res.IsZero() {
			return res, err
		}
	}

	// Determine if we need to start a scale or upgrade operation
	if needsProxyScale(ais, ss) {
		return r.startProxyScale(ctx, ais, ss)
	}
	return r.checkProxyUpgrade(ctx, ais, ss)
}

// With StatefulSet rolling update strategy, pods are updated in descending order of their pod index.
// This implies the pod with the largest index is the oldest proxy, and we set it as primary.
func (r *AIStoreReconciler) setHighestPodAsPrimary(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (err error) {
	podIndex := *ss.Spec.Replicas - 1
	err = r.setPrimaryTo(ctx, ais, podIndex)
	if err != nil {
		logger := logf.FromContext(ctx).WithValues("StatefulSet", ss.Name)
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
	if ss.Spec.UpdateStrategy.RollingUpdate.Partition == aisapc.Ptr(int32(0)) {
		return false
	}
	// Reset to allow updating the last pod (lowest ordinal)
	return ss.Status.CurrentReplicas == 1
}

func (r *AIStoreReconciler) resetSSPartition(ctx context.Context, ss *appsv1.StatefulSet) (err error) {
	logger := logf.FromContext(ctx).WithValues("StatefulSet", ss.Name)
	logger.Info("Removing partition from rolling update strategy")
	// Revert StatefulSet partition spec
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
		logger.Error(err, "failed to patch StatefulSet update strategy")
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

	node, err := findNodeByPodName(smap.Pmap, podName)
	if err != nil {
		return err
	}
	logf.FromContext(ctx).Info("Setting primary proxy", "pod", podName)
	return apiClient.SetPrimaryProxy(node.ID(), node.PubNet.URL, true /*force*/)
}

func findNodeByPodName(pmap aismeta.NodeMap, podName string) (*aismeta.Snode, error) {
	for _, node := range pmap {
		if strings.HasPrefix(node.ControlNet.Hostname, podName) {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no matching AIS node found for pod %q", podName)
}

// prepProxyScaleDown decommissions all the proxy nodes that will be deleted due to scale down.
// If the node being deleted is a primary, a new primary is designated before decommissioning.
func (r *AIStoreReconciler) prepProxyScaleDown(ctx context.Context, ais *aisv1.AIStore, actualSize int32) {
	logger := logf.FromContext(ctx)

	apiClient, err := r.clientManager.GetClient(ctx, ais)
	if err != nil {
		return
	}
	smap, err := apiClient.GetClusterMap()
	if err != nil {
		return
	}

	decommissionNode := func(daemonID string) {
		rmAction := &aisapc.ActValRmNode{
			DaemonID: daemonID,
		}
		_, decommErr := apiClient.DecommissionNode(rmAction)
		if decommErr != nil {
			logger.Error(err, "failed to decommission node - "+daemonID)
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
		if smap.InMaintOrDecomm(node.ID()) {
			continue
		}
		err = apiClient.SetPrimaryProxy(node.DaeID, node.PubNet.URL, true /*force*/)
		if err != nil {
			logger.Error(err, "failed to set primary as "+node.DaeID)
			continue
		}
		decommissionNode(oldPrimaryID)
	}
}

// enableProxyExternalService, creates a LoadBalancer service for proxy StatefulSet.
// NOTE: As opposed to `target` external services, where we have a separate LoadBalancer service per pod,
// `proxies` have a single LoadBalancer service across all the proxy pods.
func (r *AIStoreReconciler) enableProxyExternalService(ctx context.Context,
	ais *aisv1.AIStore,
) (ready bool, err error) {
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

func (r *AIStoreReconciler) checkProxyUpgrade(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (result ctrl.Result, err error) {
	logger := logf.FromContext(ctx).WithValues("StatefulSet", ss.Name)
	desiredTemplate := &proxy.NewProxyStatefulSet(ais, ais.GetProxySize()).Spec.Template
	// Any change to pod template will trigger a new rollout, so any changes to the SS should happen here.
	needsUpdate, reason := shouldUpdatePodTemplate(desiredTemplate, &ss.Spec.Template)
	if !needsUpdate {
		return
	}

	updatedSS := ss.DeepCopy()

	// Update status and condition to indicate that the cluster is upgrading.
	ais.SetConditionFalse(aisv1.ConditionReady, aisv1.ReasonUpgrading, "Upgrading proxy StatefulSet")
	if err = r.updateStatusWithState(ctx, ais, aisv1.ClusterProxyUpgrading); err != nil {
		return
	}
	// If we have an active cluster, set primary to 0 before triggering rollout.
	if updatedSS.Status.ReadyReplicas > 0 {
		if err = r.setPrimaryTo(ctx, ais, 0); err != nil {
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
	// Sync pod template and patch StatefulSet.
	syncPodTemplate(desiredTemplate, &updatedSS.Spec.Template)
	logger.Info("Proxy pod template spec modified", "reason", reason)
	patch := client.MergeFrom(ss)
	err = r.k8sClient.Patch(ctx, updatedSS, patch)
	if err != nil {
		return
	}
	logger.Info("StatefulSet successfully updated", "reason", reason)

	// Requeue to enter handleProxyUpgrade.
	return ctrl.Result{Requeue: true}, nil
}

func (r *AIStoreReconciler) handleProxyUpgrade(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (ctrl.Result, error) {
	// Reset partition to update last pod when at last pod to update.
	if shouldResetPartition(ss) {
		if err := r.setHighestPodAsPrimary(ctx, ais, ss); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.resetSSPartition(ctx, ss); err != nil {
			return ctrl.Result{}, err
		}
	}
	// Requeue until upgrade is complete.
	if !proxy.IsStatefulSetReady(ais, ss) {
		logf.FromContext(ctx).Info("Waiting for proxy StatefulSet upgrade to complete")
		return ctrl.Result{RequeueAfter: proxyStartupInterval}, nil
	}
	return ctrl.Result{}, nil
}

func needsProxyScale(ais *aisv1.AIStore, ss *appsv1.StatefulSet) bool {
	return *ss.Spec.Replicas != ais.GetProxySize()
}

func (r *AIStoreReconciler) startProxyScale(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (ctrl.Result, error) {
	// Update status and condition to indicate that the cluster is scaling.
	ais.SetConditionFalse(aisv1.ConditionReady, aisv1.ReasonScaling, "Scaling proxy StatefulSet")
	if err := r.updateStatusWithState(ctx, ais, aisv1.ClusterProxyScaling); err != nil {
		return ctrl.Result{}, err
	}
	// If scale-in, decommission proxies and move primary if necessary before updating replicas.
	if *ss.Spec.Replicas > ais.GetProxySize() {
		r.prepProxyScaleDown(ctx, ais, *ss.Spec.Replicas)
	}
	// Update replicas to desired size
	if _, err := r.k8sClient.UpdateStatefulSetReplicas(ctx, proxy.StatefulSetNSName(ais), ais.GetProxySize()); err != nil {
		return ctrl.Result{}, err
	}
	// Requeue to enter handleProxyScale.
	return ctrl.Result{RequeueAfter: proxyStartupInterval}, nil
}

func handleProxyScale(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) ctrl.Result {
	// Requeue until scale is complete
	if !proxy.IsStatefulSetReady(ais, ss) {
		logf.FromContext(ctx).Info("Waiting for proxy StatefulSet scale to complete")
		return ctrl.Result{RequeueAfter: proxyStartupInterval}
	}
	return ctrl.Result{}
}
