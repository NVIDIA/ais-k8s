// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
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

	// 1. Create a proxy statefulset with single replica as primary
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

	// Check whether proxy service to have a registered DNS entry.
	if dnsErr := checkDNSEntry(ctx, ais); dnsErr != nil {
		logger.Info("Failed to find any DNS entries for proxy service", "error", dnsErr)
		r.recorder.Event(ais, corev1.EventTypeNormal, EventReasonWaiting, "Waiting for proxy service to have registered DNS entries")
		return ctrl.Result{RequeueAfter: proxyDNSInterval}, nil
	}
	return ctrl.Result{}, nil
}

var checkDNSEntry = checkDNSEntryDefault

func checkDNSEntryDefault(ctx context.Context, ais *aisv1.AIStore) error {
	nsName := proxy.HeadlessSVCNSName(ais)
	clusterDomain := ais.GetClusterDomain()
	hostname := fmt.Sprintf("%s.%s.svc.%s", nsName.Name, nsName.Namespace, clusterDomain)

	ctx, cancel := context.WithTimeout(ctx, proxyDNSTimeout)
	defer cancel()
	_, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	// Log an error if we have an actual error, not just no host found
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && !dnsErr.IsNotFound {
			logf.FromContext(ctx).Error(dnsErr, "Error looking up DNS entry")
		}
	}
	return err
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

	result, err = r.handleProxyImage(ctx, ais, ss)
	if err != nil || !result.IsZero() {
		return
	}

	if *ss.Spec.Replicas != ais.GetProxySize() {
		if *ss.Spec.Replicas > ais.GetProxySize() {
			// If the cluster is scaling down, ensure the pod being delete is not primary.
			r.handleProxyScaledown(ctx, ais, *ss.Spec.Replicas)
		}
		// If anything was updated, we consider it not immediately ready.
		updated, err := r.k8sClient.UpdateStatefulSetReplicas(ctx, proxySSName, ais.GetProxySize())
		if err != nil || updated {
			result.Requeue = true
			return result, err
		}
	}

	// Requeue if the number of proxy pods ready does not match the size provided in AIS cluster spec.
	if ss.Status.ReadyReplicas != ais.GetProxySize() {
		logf.FromContext(ctx).Info("Waiting for proxy statefulset to reach desired replicas")
		return ctrl.Result{RequeueAfter: proxyStartupInterval}, nil
	}
	return
}

// formerly "ready"
func (r *AIStoreReconciler) handleProxyImage(ctx context.Context, ais *aisv1.AIStore, ss *appsv1.StatefulSet) (result ctrl.Result, err error) {
	logger := logf.FromContext(ctx)

	firstPodName := proxy.PodName(ais, 0)
	updated := ss.Spec.Template.Spec.Containers[0].Image != ais.Spec.NodeImage
	if updated {
		if ss.Status.ReadyReplicas > 0 {
			err = r.setPrimaryTo(ctx, ais, 0)
			if err != nil {
				logger.Error(err, "failed to set primary proxy")
				return
			}
			logger.Info("Updated primary to pod " + firstPodName)
			ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: func(v int32) *int32 { return &v }(1),
				},
			}
		}
		ss.Spec.Template.Spec.Containers[0].Image = ais.Spec.NodeImage
		result.Requeue = true
		err = r.k8sClient.Update(ctx, ss)
		return
	}

	podList, err := r.k8sClient.ListPods(ctx, ss)
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
	// pod are updated in descending order of their pod index.
	// This implies the pod with the largest index is the oldest proxy,
	// and we set it as a primary.
	if toUpdate == 1 && firstYetToUpdate {
		err = r.setPrimaryTo(ctx, ais, *ss.Spec.Replicas-1)
		if err != nil {
			return
		}
		// Revert statefulset partition spec
		ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
				Partition: func(v int32) *int32 { return &v }(0),
			},
		}

		err = r.k8sClient.Update(ctx, ss)
		if err != nil {
			logger.Error(err, "failed to update proxy statefulset update policy")
			return
		}

		// Delete the first pod to update its docker image.
		_, err = r.k8sClient.DeletePodIfExists(ctx, types.NamespacedName{
			Namespace: ais.Namespace,
			Name:      firstPodName,
		})
		if err != nil {
			return
		}
		return ctrl.Result{Requeue: true}, nil
	}
	if toUpdate == 0 {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{Requeue: true}, nil
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

	if strings.HasPrefix(smap.Primary.ControlNet.Hostname, podName) {
		return nil
	}

	for _, node := range smap.Pmap {
		if !strings.HasPrefix(node.ControlNet.Hostname, podName) {
			continue
		}
		return apiClient.SetPrimaryProxy(node.ID(), node.PubNet.URL, true /*force*/)
	}
	return fmt.Errorf("couldn't find a proxy node for pod %q", podName)
}

// handleProxyScaledown decommissions all the proxy nodes that will be deleted due to scale down.
// If the node being deleted is a primary, a new primary is designated before decommissioning.
func (r *AIStoreReconciler) handleProxyScaledown(ctx context.Context, ais *aisv1.AIStore, actualSize int32) {
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
		if smap.InMaintOrDecomm(node) {
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

// enableProxyExternalService, creates a LoadBalancer service for proxy statefulset.
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
