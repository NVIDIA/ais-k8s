// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/adminclient"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterCreateInterval     = time.Second
	clusterReadyRetryInterval = 5 * time.Second
	clusterReadyTimeout       = 5 * time.Minute
	clusterDestroyInterval    = 2 * time.Second
	clusterDestroyTimeout     = 4 * time.Minute
	clusterUpdateTimeout      = 2 * time.Minute
	clusterUpdateInterval     = 2 * time.Second

	urlTemplate = "http://%s:%s"
)

// clientCluster - This struct contains an AIS custom resource, references to required persistent volumes,
// and utility methods for managing clusters used by operator tests
type clientCluster struct {
	aisCfg    *tutils.AISTestCfg
	k8sClient *aisclient.K8sClient
	cluster   *aisv1.AIStore
	pvs       []*corev1.PersistentVolume
	proxyURL  string
}

func (cc *clientCluster) applyDefaultHostPortOffset(args *tutils.ClusterSpecArgs) {
	if args.EnableExternalLB {
		return
	}
	// Apply host port offset of 10 per parallel Ginkgo process to give each process a unique host port
	// and allow for further in-test offsets (e.g. multiple clusters in the same test)
	gid := int32(GinkgoParallelProcess())
	cc.applyHostPortOffset(gid * 10)
}

func newClientCluster(ctx context.Context, aisCfg *tutils.AISTestCfg, k8sClient *aisclient.K8sClient, cluArgs *tutils.ClusterSpecArgs) *clientCluster {
	cluster, pvs := tutils.NewAISCluster(ctx, cluArgs, k8sClient)
	cc := &clientCluster{
		aisCfg:    aisCfg,
		k8sClient: k8sClient,
		cluster:   cluster,
		pvs:       pvs,
	}
	cc.applyDefaultHostPortOffset(cluArgs)
	return cc
}

func (cc *clientCluster) getTimeout() time.Duration {
	// For a cluster with external LB, allocating external-IP could be time-consuming.
	// Force longer timeout for cluster creation.
	if cc.cluster.Spec.EnableExternalLB {
		return cc.aisCfg.GetClusterCreateLongTimeout()
	}
	return cc.aisCfg.GetClusterCreateTimeout()
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
func (cc *clientCluster) recreate(ctx context.Context, cluArgs *tutils.ClusterSpecArgs) {
	cc.cluster = tutils.NewAISClusterNoPV(cluArgs)
	cc.applyDefaultHostPortOffset(cluArgs)
	cc.create(ctx)
}

func (cc *clientCluster) create(ctx context.Context) {
	cc.createCluster(ctx, cc.getTimeout(), clusterCreateInterval)
	cc.waitForReadyCluster(ctx)
	cc.initClientAccess(ctx)
}

func createClusters(ctx context.Context, clusters []*clientCluster) {
	var wg sync.WaitGroup
	wg.Add(len(clusters))

	for _, cluster := range clusters {
		go func(cc *clientCluster) {
			defer GinkgoRecover()
			defer wg.Done()
			cc.create(ctx)
		}(cluster)
	}
	wg.Wait()
}

func (cc *clientCluster) createWithCallback(ctx context.Context, postCreate func()) {
	cc.create(ctx)
	if postCreate != nil {
		By("Running post-create callback")
		postCreate()
	}
}

func (cc *clientCluster) createAndDestroyCluster(ctx context.Context, postCreate, postDestroy func()) {
	defer func() {
		cc.printLogs(ctx)
		cc.destroyCleanupWithCallback(postDestroy)
	}()
	cc.createWithCallback(ctx, postCreate)
}

func (cc *clientCluster) createAndDestroyWithWait(ctx context.Context) {
	cc.createAndDestroyCluster(ctx, func() { cc.waitForResources(ctx) }, func() { cc.waitForResourceDeletion(ctx) })
}

func (cc *clientCluster) createCluster(ctx context.Context, intervals ...interface{}) {
	Expect(cc.k8sClient.Create(ctx, cc.cluster)).Should(Succeed())
	By("Create cluster and wait for it to be 'Ready'")
	Eventually(func(ctx context.Context) bool {
		ais := &aisv1.AIStore{}
		_ = cc.k8sClient.Get(ctx, cc.cluster.NamespacedName(), ais)
		return ais.HasState(aisv1.ClusterReady)
	}, intervals...).WithContext(ctx).Should(BeTrue())
}

func (cc *clientCluster) waitForReadyCluster(ctx context.Context) {
	tutils.WaitForClusterToBeReady(ctx, cc.k8sClient, cc.cluster.NamespacedName(), clusterReadyTimeout, clusterReadyRetryInterval)

	By("Verifying ClusterID status matches smap UUID")
	baseParams := cc.getBaseParams(ctx)
	smap, err := aisapi.GetClusterMap(baseParams)
	Expect(err).NotTo(HaveOccurred())
	Expect(smap.UUID).NotTo(BeEmpty(), "smap UUID should not be empty")

	cc.fetchLatestCluster(ctx)
	Expect(cc.cluster.Status.ClusterID).To(Equal(smap.UUID),
		"ClusterID in status should match smap UUID")
}

// patchClusterSpec applies the given spec to the cluster and waits for it to return to Ready state.
func (cc *clientCluster) patchClusterSpec(ctx context.Context, newSpec *aisv1.AIStoreSpec) {
	readyGen := cc.getReadyObservedGen()
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	cc.cluster.Spec = *newSpec
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).Should(Succeed())
	tutils.WaitForReadyConditionChange(ctx, cc.k8sClient, cc.cluster, readyGen, clusterUpdateTimeout, clusterUpdateInterval)
	cc.waitForReadyCluster(ctx)
}

// patchClusterSpecNoWait applies the given spec to the cluster without waiting for Ready state.
func (cc *clientCluster) patchClusterSpecNoWait(ctx context.Context, newSpec *aisv1.AIStoreSpec) {
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	cc.cluster.Spec = *newSpec
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).Should(Succeed())
}

func (cc *clientCluster) patchImagesToCurrent(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	newSpec := cc.cluster.Spec.DeepCopy()
	newSpec.NodeImage = cc.aisCfg.NodeImage
	newSpec.InitImage = cc.aisCfg.InitImage
	cc.patchClusterSpec(ctx, newSpec)
}

func (cc *clientCluster) patchImagesToBroken(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	newSpec := cc.cluster.Spec.DeepCopy()
	newSpec.NodeImage = "docker.io/aistorage/aisnode:non-existent-tag"
	newSpec.InitImage = "docker.io/aistorage/ais-init:non-existent-tag"
	cc.patchClusterSpecNoWait(ctx, newSpec)
}

func (cc *clientCluster) patchImagesAndScale(ctx context.Context, factor int32) {
	cc.fetchLatestCluster(ctx)
	newSpec := cc.cluster.Spec.DeepCopy()
	newSpec.NodeImage = cc.aisCfg.NodeImage
	newSpec.InitImage = cc.aisCfg.InitImage
	newSpec.Size = aisapc.Ptr(*newSpec.Size + factor)
	cc.patchClusterSpec(ctx, newSpec)
}

// podHasImagePullError checks if a pod has an ImagePullBackOff or ErrImagePull status.
func (cc *clientCluster) podHasImagePullError(ctx context.Context, podName string) bool {
	pod, err := cc.k8sClient.GetPod(ctx, types.NamespacedName{
		Namespace: cc.cluster.Namespace,
		Name:      podName,
	})
	if err != nil {
		return false
	}
	if pod.Status.InitContainerStatuses != nil {
		for i := range pod.Status.InitContainerStatuses {
			status := &pod.Status.InitContainerStatuses[i]
			if status.State.Waiting != nil {
				reason := status.State.Waiting.Reason
				if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					return true
				}
			}
		}
	}
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if status.State.Waiting != nil {
			reason := status.State.Waiting.Reason
			if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
				return true
			}
		}
	}
	return false
}

func (cc *clientCluster) getBaseParams(ctx context.Context) aisapi.BaseParams {
	cc.fetchLatestCluster(ctx)
	proxyURL := cc.getProxyURL(ctx)
	return aistutils.BaseAPIParams(proxyURL)
}

func (cc *clientCluster) fetchLatestCluster(ctx context.Context) {
	ais, err := cc.k8sClient.GetAIStoreCR(ctx, cc.cluster.NamespacedName())
	Expect(err).To(BeNil())
	cc.cluster = ais
}

func (cc *clientCluster) getReadyObservedGen() int64 {
	cond := tutils.GetClusterReadyCondition(cc.cluster)
	if cond == nil {
		return 0
	}
	return cond.ObservedGeneration
}

// Initialize AIS tutils to use the deployed cluster
func (cc *clientCluster) initClientAccess(ctx context.Context) {
	// Refresh CR to avoid using stale proxy size
	cc.fetchLatestCluster(ctx)
	// Wait for all proxies
	proxyURLs := cc.getAllProxyURLs(ctx)
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

func (cc *clientCluster) getProxyURL(ctx context.Context) (proxyURL string) {
	var ip string
	if cc.cluster.Spec.EnableExternalLB {
		ip = tutils.GetLoadBalancerIP(ctx, cc.k8sClient, proxy.LoadBalancerSVCNSName(cc.cluster))
	} else {
		ip = tutils.GetRandomProxyIP(ctx, cc.k8sClient, cc.cluster)
	}
	Expect(ip).NotTo(Equal(""))
	return fmt.Sprintf(urlTemplate, ip, cc.cluster.Spec.ProxySpec.ServicePort.String())
}

func (cc *clientCluster) getAllProxyURLs(ctx context.Context) (proxyURLs []*string) {
	var proxyIPs []string
	if cc.cluster.Spec.EnableExternalLB {
		proxyIPs = []string{tutils.GetLoadBalancerIP(ctx, cc.k8sClient, proxy.LoadBalancerSVCNSName(cc.cluster))}
	} else {
		proxyIPs = tutils.GetAllProxyIPs(ctx, cc.k8sClient, cc.cluster)
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
	cc.destroyClusterOnly()
	if cc.pvs != nil {
		tutils.DestroyPV(context.Background(), cc.k8sClient, cc.pvs)
	}
}

func (cc *clientCluster) destroyClusterOnly() {
	tutils.DestroyCluster(context.Background(), cc.k8sClient, cc.cluster, clusterDestroyTimeout, clusterDestroyInterval)
}

func (cc *clientCluster) scaleSpec(ctx context.Context, targetOnly bool, factor int32) *aisv1.AIStoreSpec {
	cc.fetchLatestCluster(ctx)
	newSpec := cc.cluster.Spec.DeepCopy()
	if targetOnly {
		newSpec.TargetSpec.Size = aisapc.Ptr(cc.cluster.GetTargetSize() + factor)
	} else {
		newSpec.Size = aisapc.Ptr(*newSpec.Size + factor)
	}
	return newSpec
}

func (cc *clientCluster) scale(ctx context.Context, targetOnly bool, factor int32) {
	By(fmt.Sprintf("Scaling cluster %q by %d", cc.cluster.Name, factor))
	cc.patchClusterSpec(ctx, cc.scaleSpec(ctx, targetOnly, factor))
	cc.verifyPodCounts(ctx)
	cc.initClientAccess(ctx)
}

func (cc *clientCluster) attemptScale(ctx context.Context, targetOnly bool, factor int32) {
	By(fmt.Sprintf("Attempting to scale cluster %q by %d", cc.cluster.Name, factor))
	cc.patchClusterSpecNoWait(ctx, cc.scaleSpec(ctx, targetOnly, factor))
}

func (cc *clientCluster) restart(ctx context.Context) {
	// Shutdown, ensure statefulsets exist and are size 0
	cc.setShutdownStatus(ctx, true)
	tutils.EventuallyPodsIsSize(ctx, cc.k8sClient, cc.cluster, proxy.BasicLabels(cc.cluster), 0, clusterDestroyTimeout)
	tutils.EventuallyPodsIsSize(ctx, cc.k8sClient, cc.cluster, target.BasicLabels(cc.cluster), 0, clusterDestroyTimeout)
	// Resume shutdown cluster, should become fully ready
	cc.setShutdownStatus(ctx, false)
	cc.waitForReadyCluster(ctx)
	cc.initClientAccess(ctx)
}

func (cc *clientCluster) setShutdownStatus(ctx context.Context, shutdown bool) {
	cr, err := cc.k8sClient.GetAIStoreCR(ctx, cc.cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	patch := clientpkg.MergeFrom(cr.DeepCopy())
	cr.Spec.ShutdownCluster = aisapc.Ptr(shutdown)
	err = cc.k8sClient.Patch(ctx, cr, patch)
	Expect(err).ShouldNot(HaveOccurred())
}

func (cc *clientCluster) waitForResources(ctx context.Context) {
	tutils.CheckResExistence(ctx, cc.cluster, cc.aisCfg, cc.k8sClient, true /*exists*/)
}

func (cc *clientCluster) waitForResourceDeletion(ctx context.Context) {
	tutils.CheckResExistence(ctx, cc.cluster, cc.aisCfg, cc.k8sClient, false /*exists*/)
	tutils.CheckPVCDoesNotExist(ctx, cc.cluster, cc.aisCfg, cc.k8sClient)
}

func (cc *clientCluster) enableAdminClient(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	if cc.cluster.Spec.AdminClient != nil {
		cc.cluster.Spec.AdminClient.Enabled = aisapc.Ptr(true)
	} else {
		cc.cluster.Spec.AdminClient = &aisv1.AdminClientSpec{Enabled: aisapc.Ptr(true)}
	}
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).To(Succeed())
}

func (cc *clientCluster) disableAdminClient(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	if cc.cluster.Spec.AdminClient == nil {
		return
	}
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	cc.cluster.Spec.AdminClient.Enabled = aisapc.Ptr(false)
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).To(Succeed())
}

func (cc *clientCluster) verifyAdminClientExists(ctx context.Context) {
	tutils.EventuallyDeploymentExists(ctx, cc.k8sClient, adminclient.DeploymentNSName(cc.cluster), BeTrue(), clusterReadyTimeout, clusterReadyRetryInterval)
}

func (cc *clientCluster) verifyAdminClientDeleted(ctx context.Context) {
	tutils.EventuallyDeploymentExists(ctx, cc.k8sClient, adminclient.DeploymentNSName(cc.cluster), BeFalse(), clusterDestroyTimeout, clusterDestroyInterval)
}

func (cc *clientCluster) enableTargetPDB(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	if cc.cluster.Spec.TargetSpec.PodDisruptionBudget != nil {
		cc.cluster.Spec.TargetSpec.PodDisruptionBudget.Enabled = true
	} else {
		cc.cluster.Spec.TargetSpec.PodDisruptionBudget = &aisv1.PDBSpec{Enabled: true}
	}
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).To(Succeed())
}

func (cc *clientCluster) verifyTargetPDBExists(ctx context.Context) {
	tutils.EventuallyPDBExists(ctx, cc.k8sClient, target.PDBNSName(cc.cluster), BeTrue(), clusterReadyTimeout, clusterReadyRetryInterval)
}

func (cc *clientCluster) disableTargetPDB(ctx context.Context) {
	cc.fetchLatestCluster(ctx)
	patch := clientpkg.MergeFrom(cc.cluster.DeepCopy())
	cc.cluster.Spec.TargetSpec.PodDisruptionBudget.Enabled = false
	Expect(cc.k8sClient.Patch(ctx, cc.cluster, patch)).To(Succeed())
}

func (cc *clientCluster) verifyTargetPDBDeleted(ctx context.Context) {
	tutils.EventuallyPDBExists(ctx, cc.k8sClient, target.PDBNSName(cc.cluster), BeFalse(), clusterDestroyTimeout, clusterDestroyInterval)
}

func (cc *clientCluster) verifyPodImages(ctx context.Context) {
	By("Verifying pod images match cluster spec")
	cc.fetchLatestCluster(ctx)
	proxies, err := cc.k8sClient.ListPods(ctx, cc.cluster, proxy.BasicLabels(cc.cluster))
	Expect(err).To(BeNil())
	for i := range proxies.Items {
		Expect(proxies.Items[i].Spec.Containers[0].Image).To(Equal(cc.cluster.Spec.NodeImage))
	}
	targets, err := cc.k8sClient.ListPods(ctx, cc.cluster, target.BasicLabels(cc.cluster))
	Expect(err).To(BeNil())
	for i := range targets.Items {
		Expect(targets.Items[i].Spec.Containers[0].Image).To(Equal(cc.cluster.Spec.NodeImage))
	}
}

func (cc *clientCluster) verifyPodCounts(ctx context.Context) {
	By("Verifying pod counts match cluster spec")
	cc.fetchLatestCluster(ctx)
	proxies, err := cc.k8sClient.ListPods(ctx, cc.cluster, proxy.BasicLabels(cc.cluster))
	Expect(err).To(BeNil())
	Expect(len(proxies.Items)).To(Equal(int(cc.cluster.GetProxySize())))
	targets, err := cc.k8sClient.ListPods(ctx, cc.cluster, target.BasicLabels(cc.cluster))
	Expect(err).To(BeNil())
	Expect(len(targets.Items)).To(Equal(int(cc.cluster.GetTargetSize())))
}

func (cc *clientCluster) hasPendingTargetPods(ctx context.Context) bool {
	podList, err := cc.k8sClient.ListPods(ctx, cc.cluster, target.BasicLabels(cc.cluster))
	Expect(err).To(BeNil())
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodPending {
			return true
		}
	}
	return false
}

// Print logs from all pods in this cluster
// On error, make a best-effort attempt to log and continue with other pods
func (cc *clientCluster) printLogs(ctx context.Context) {
	clusterName := cc.cluster.Name
	cs, err := tutils.NewClientset()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error creating clientset: %v", err)
		return
	}

	clusterSelector := map[string]string{"app.kubernetes.io/name": clusterName}
	podList, err := cc.k8sClient.ListPods(ctx, cc.cluster, clusterSelector)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error listing pods for cluster %s: %v", clusterName, err)
		return
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		fmt.Printf("Logs for pod %s in cluster %s:\n", pod.Name, clusterName)
		err = printPodLogs(ctx, clusterName, cs, pod)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr,
				"error printing logs for pod %s in cluster %s: %v\n",
				pod.Name, clusterName, err)
		}
		// Spacer for better visualization in CI logs
		fmt.Println("---------------------------------------------------")
	}
}

func printPodLogs(ctx context.Context, clusterName string, cs *kubernetes.Clientset, pod *corev1.Pod) error {
	opts := &corev1.PodLogOptions{Container: "ais-logs"}
	req := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)
	stream, streamErr := req.Stream(ctx)
	if streamErr != nil {
		return fmt.Errorf("error opening log stream: %w", streamErr)
	}
	// Ensure this stream is closed before moving to the next pod.
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			// Log close failure; do not change the function’s return value.
			_, _ = fmt.Fprintf(os.Stderr,
				"error closing log stream for pod %s in cluster %s: %v\n",
				pod.Name, clusterName, cerr)
		}
	}()

	if _, err := io.Copy(os.Stdout, stream); err != nil {
		return err
	}
	return nil
}
