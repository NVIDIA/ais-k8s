// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"fmt"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

const urlTemplate = "http://%s:%s"

// clientCluster - This struct contains an AIS custom resource, references to required persistent volumes,
// and utility methods for managing clusters used by operator tests
type clientCluster struct {
	cluster          *aisv1.AIStore
	pvs              []*corev1.PersistentVolume
	ctx              context.Context
	cancelLogsStream context.CancelFunc
	proxyURL         string
}

func newClientCluster(ctx context.Context, cluArgs *tutils.ClusterSpecArgs) *clientCluster {
	cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
	cc := &clientCluster{
		cluster: cluster,
		pvs:     pvs,
	}
	cc.ctx, cc.cancelLogsStream = context.WithCancel(ctx)
	if cluArgs.EnableExternalLB {
		tutils.InitK8sClusterProvider(testCtx.Context(), k8sClient)
		tutils.SkipIfLoadBalancerNotSupported()
	}
	return cc
}

func (cc *clientCluster) getTimeout(long bool) time.Duration {
	// For a cluster with external LB, allocating external-IP could be time-consuming.
	// Force longer timeout for cluster creation.
	if long || cc.cluster.Spec.EnableExternalLB {
		return tutils.GetClusterCreateLongTimeout()
	}
	return tutils.GetClusterCreateTimeout()
}

// Use to avoid a host port collision with an existing host port cluster
func (cc *clientCluster) applyHostPortOffset(offset int32) {
	specs := []*aisv1.DaemonSpec{&cc.cluster.Spec.ProxySpec, &cc.cluster.Spec.TargetSpec.DaemonSpec}
	for i := range specs {
		specs[i].HostPort = aisapc.Ptr(*specs[i].HostPort + offset)
		specs[i].ServicePort = intstr.FromInt32(specs[i].ServicePort.IntVal + offset)
		specs[i].PublicPort = intstr.FromInt32(specs[i].PublicPort.IntVal + offset)
	}
}

// Re-initialize the local cluster CR from the given cluster args and re-create it remotely -- does not create PVs
func (cc *clientCluster) recreate(cluArgs *tutils.ClusterSpecArgs, long bool) {
	cc.cluster = tutils.NewAISClusterNoPV(cluArgs)
	cc.create(long)
}

func (cc *clientCluster) create(long bool) {
	cc.createCluster(cc.getTimeout(long), clusterCreateInterval)
	cc.waitForReadyCluster()
	cc.initClientAccess()
}

func (cc *clientCluster) createWithCallback(long bool, postCreate func()) {
	cc.create(long)
	if postCreate != nil {
		By("Running post-create callback")
		postCreate()
	}
}

func (cc *clientCluster) createAndDestroyCluster(postCreate func(),
	postDestroy func(), long bool) {
	defer func() {
		Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
		cc.destroyCleanupWithCallback(postDestroy)
	}()
	cc.createWithCallback(long, postCreate)
}

func (cc *clientCluster) createCluster(intervals ...interface{}) {
	Expect(k8sClient.Create(cc.ctx, cc.cluster)).Should(Succeed())
	By("Create cluster and wait for it to be 'Ready'")
	Eventually(func() bool {
		ais := &aisv1.AIStore{}
		_ = k8sClient.Get(cc.ctx, cc.cluster.NamespacedName(), ais)
		return ais.HasState(aisv1.ClusterReady)
	}, intervals...).Should(BeTrue())
}

func (cc *clientCluster) refresh() {
	var err error
	cc.cluster, err = k8sClient.GetAIStoreCR(cc.ctx, cc.cluster.NamespacedName())
	Expect(err).NotTo(HaveOccurred())
}

func (cc *clientCluster) waitForReadyCluster() {
	tutils.WaitForClusterToBeReady(cc.ctx, k8sClient, cc.cluster.NamespacedName(), clusterReadyTimeout, clusterReadyRetryInterval)
	// Validate the cluster map -- make sure all AIS nodes have successfully joined cluster
	cc.refresh()
	cc.initClientAccess()
	bp := cc.getBaseParams()
	Eventually(func() bool {
		smap, err := aisapi.GetClusterMap(bp)
		Expect(err).NotTo(HaveOccurred())
		activeProxies := int32(len(smap.Pmap.ActiveNodes()))
		activeTargets := int32(len(smap.Tmap.ActiveNodes()))
		return activeProxies == cc.cluster.GetProxySize() && activeTargets == cc.cluster.GetTargetSize()
	}).Should(BeTrue())
}

func (cc *clientCluster) patchImage(img string) {
	cc.fetchLatestCluster()
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	cc.cluster.Spec.NodeImage = img
	Expect(k8sClient.Patch(cc.ctx, cc.cluster, patch)).Should(Succeed())
	By("Update cluster spec and wait for it to be 'Ready'")
	cc.waitForReadyCluster()
}

func (cc *clientCluster) getBaseParams() aisapi.BaseParams {
	cc.fetchLatestCluster()
	proxyURL := cc.getProxyURL()
	return aistutils.BaseAPIParams(proxyURL)
}

func (cc *clientCluster) fetchLatestCluster() {
	ais, err := k8sClient.GetAIStoreCR(cc.ctx, cc.cluster.NamespacedName())
	Expect(err).To(BeNil())
	cc.cluster = ais
}

// Initialize AIS tutils to use the deployed cluster
func (cc *clientCluster) initClientAccess() {
	// Wait for all proxies
	proxyURLs := cc.getAllProxyURLs()
	for i := range proxyURLs {
		proxyURL := *proxyURLs[i]
		retries := 2
		for retries > 0 {
			err := aistutils.WaitNodeReady(proxyURL, &aistutils.WaitRetryOpts{
				MaxRetries: 12,
				Interval:   10 * time.Second,
			})
			if err == nil {
				break
			}
			retries--
			time.Sleep(5 * time.Second)
		}
		// Wait until the cluster has actually started (targets have registered).
		Expect(aistutils.InitCluster(proxyURL, aistutils.ClusterTypeK8s)).NotTo(HaveOccurred())
	}
}

func (cc *clientCluster) getProxyURL() (proxyURL string) {
	var ip string
	if cc.cluster.Spec.EnableExternalLB {
		ip = tutils.GetLoadBalancerIP(cc.ctx, k8sClient, proxy.LoadBalancerSVCNSName(cc.cluster))
	} else {
		ip = tutils.GetRandomProxyIP(cc.ctx, k8sClient, cc.cluster)
	}
	Expect(ip).NotTo(Equal(""))
	return fmt.Sprintf(urlTemplate, ip, cc.cluster.Spec.ProxySpec.ServicePort.String())
}

func (cc *clientCluster) getAllProxyURLs() (proxyURLs []*string) {
	var proxyIPs []string
	if cc.cluster.Spec.EnableExternalLB {
		proxyIPs = []string{tutils.GetLoadBalancerIP(cc.ctx, k8sClient, proxy.LoadBalancerSVCNSName(cc.cluster))}
	} else {
		proxyIPs = tutils.GetAllProxyIPs(cc.ctx, k8sClient, cc.cluster)
	}
	for _, ip := range proxyIPs {
		proxyURL := fmt.Sprintf(urlTemplate, ip, cc.cluster.Spec.ProxySpec.ServicePort.String())
		proxyURLs = append(proxyURLs, &proxyURL)
	}
	return proxyURLs
}

func (cc *clientCluster) destroyCleanupWithCallback(postDestroy func()) {
	cc.destroyAndCleanup()
	if postDestroy != nil {
		By("Running post-destroy callback")
		postDestroy()
	}
}

func (cc *clientCluster) destroyAndCleanup() {
	By(fmt.Sprintf("Destroying cluster %q", cc.cluster.Name))
	cc.cancelLogsStream()
	cc.destroyClusterOnly()
	if cc.pvs != nil {
		tutils.DestroyPV(context.Background(), k8sClient, cc.pvs)
	}
}

func (cc *clientCluster) destroyClusterOnly() {
	tutils.DestroyCluster(context.Background(), k8sClient, cc.cluster, clusterDestroyTimeout, clusterDestroyInterval)
}

func (cc *clientCluster) scale(targetOnly bool, factor int32) {
	By(fmt.Sprintf("Scaling cluster %q by %d", cc.cluster.Name, factor))
	cr, err := k8sClient.GetAIStoreCR(cc.ctx, cc.cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	patch := clientpkg.MergeFrom(cr.DeepCopy())
	if targetOnly {
		cr.Spec.TargetSpec.Size = aisapc.Ptr(cr.GetTargetSize() + factor)
	} else {
		cr.Spec.Size = aisapc.Ptr(*cr.Spec.Size + factor)
	}
	// Get current ready condition generation
	readyCond := tutils.GetClusterReadyCondition(cc.cluster)
	var readyGen int64
	if readyCond == nil {
		readyGen = 0
	} else {
		readyGen = readyCond.ObservedGeneration
	}
	Expect(k8sClient.Patch(cc.ctx, cr, patch)).Should(Succeed())
	// Wait for the condition's generation to receive some update so we know reconciliation began
	// Otherwise, the cluster may be immediately ready
	tutils.WaitForReadyConditionChange(cc.ctx, k8sClient, cr, readyGen, clusterUpdateTimeout, clusterUpdateInterval)
	cc.waitForReadyCluster()
	cc.initClientAccess()
}

func (cc *clientCluster) restart() {
	// Shutdown, ensure statefulsets exist and are size 0
	cc.setShutdownStatus(true)
	tutils.EventuallyPodsIsSize(cc.ctx, k8sClient, cc.cluster, proxy.PodLabels(cc.cluster), 0, clusterDestroyTimeout)
	tutils.EventuallyPodsIsSize(cc.ctx, k8sClient, cc.cluster, target.PodLabels(cc.cluster), 0, clusterDestroyTimeout)
	// Resume shutdown cluster, should become fully ready
	cc.setShutdownStatus(false)
	cc.waitForReadyCluster()
	cc.initClientAccess()
}

func (cc *clientCluster) setShutdownStatus(shutdown bool) {
	cr, err := k8sClient.GetAIStoreCR(cc.ctx, cc.cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	patch := clientpkg.MergeFrom(cr.DeepCopy())
	cr.Spec.ShutdownCluster = aisapc.Ptr(shutdown)
	err = k8sClient.Patch(cc.ctx, cr, patch)
	Expect(err).ShouldNot(HaveOccurred())
}

func (cc *clientCluster) waitForResources() {
	tutils.CheckResExistence(cc.ctx, cc.cluster, k8sClient, true /*exists*/)
}

func (cc *clientCluster) waitForResourceDeletion() {
	tutils.CheckResExistence(cc.ctx, cc.cluster, k8sClient, false /*exists*/)
	tutils.CheckPVCDoesNotExist(cc.ctx, cc.cluster, k8sClient, storageClass)
}
