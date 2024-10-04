// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"strings"
	"sync"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisxact "github.com/NVIDIA/aistore/xact"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterReadyRetryInterval = 5 * time.Second
	clusterReadyTimeout       = 3 * time.Minute
	clusterDestroyTimeout     = 2 * time.Minute
)

// clientCluster - used for managing cluster used by AIS API tests
type clientCluster struct {
	cluster          *aisv1.AIStore
	tout             time.Duration
	ctx              context.Context
	cancelLogsStream context.CancelFunc
	proxyURL         string
}

func newClientCluster(cluArgs *tutils.ClusterSpecArgs) (*clientCluster, []*corev1.PersistentVolume) {
	cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
	cc := &clientCluster{
		cluster: cluster,
		tout:    tutils.GetClusterCreateTimeout(),
	}

	if cluArgs.EnableExternalLB {
		tutils.InitK8sClusterProvider(context.Background(), k8sClient)
		tutils.SkipIfLoadBalancerNotSupported()
		// For a cluster with external LB, allocating external-IP could be time-consuming.
		// Allow longer timeout for cluster creation.
		cc.tout = tutils.GetClusterCreateLongTimeout()
	}
	return cc, pvs
}

func (cc *clientCluster) create() {
	cc.ctx, cc.cancelLogsStream = context.WithCancel(context.Background())
	createCluster(cc.ctx, cc.cluster, cc.tout, tutils.ClusterCreateInterval)
	tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cc.cluster, clusterReadyTimeout, clusterReadyRetryInterval)
	initAISCluster(cc.ctx, cc.cluster)
	Expect(tutils.StreamLogs(cc.ctx, testNSName)).To(BeNil())
}

func (cc *clientCluster) updateImage(img string) {
	ais, err := k8sClient.GetAIStoreCR(cc.ctx, cc.cluster.NamespacedName())
	Expect(err).To(BeNil())
	cc.cluster = ais
	cc.cluster.Spec.NodeImage = img
	Expect(k8sClient.Update(cc.ctx, cc.cluster)).Should(Succeed())
	By("Update cluster spec and wait for it to be 'Ready'")
	tutils.WaitForClusterToBeReady(cc.ctx, k8sClient, cc.cluster, clusterReadyTimeout, clusterReadyRetryInterval)
}

func (cc *clientCluster) getBaseParams() aisapi.BaseParams {
	proxyURL := tutils.GetProxyURL(cc.ctx, k8sClient, cc.cluster)
	return aistutils.BaseAPIParams(proxyURL)
}

// Initialize AIS tutils to use the deployed cluster
func initAISCluster(ctx context.Context, cluster *aisv1.AIStore) {
	// Wait for all proxies
	proxyURLs := tutils.GetAllProxyURLs(ctx, k8sClient, cluster)
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

func (cc *clientCluster) cleanup(pvs []*corev1.PersistentVolume) {
	cc.cancelLogsStream()
	tutils.DestroyCluster(context.Background(), k8sClient, cc.cluster, cc.tout, tutils.ClusterCreateInterval)
	if pvs != nil {
		tutils.DestroyPV(context.Background(), k8sClient, pvs)
	}
}

func (cc *clientCluster) restart() {
	restartCluster(cc.ctx, cc.cluster)
	initAISCluster(cc.ctx, cc.cluster)
}

var _ = Describe("Run Controller", func() {
	Context("Deploy and Destroy cluster", Label("short"), func() {
		Context("without externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func() {
				cluster, pvs := tutils.NewAISCluster(defaultCluArgs(), k8sClient)
				createAndDestroyCluster(cluster, pvs, checkResExists, checkResShouldNotExist, false)
			})

			It("Should successfully create an hetero-sized AIS Cluster", func() {
				cluArgs := defaultCluArgs()
				cluArgs.TargetSize = 2
				cluArgs.ProxySize = 1
				cluArgs.TargetSharedNode = true
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				createAndDestroyCluster(cluster, pvs, nil, nil, true)
			})
		})

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.EnableExternalLB = true
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				createAndDestroyCluster(cluster, pvs, checkResExists, checkResShouldNotExist, true)
			})
		})
	})

	Context("Backwards Compatibility", Label("long"), func() {
		It("Should successfully create an AIS Cluster with AllowSharedOrNoDisks on > v3.23 image", func() {
			args := defaultCluArgs()
			args.AllowSharedOrNoDisks = true
			cluster, pvs := tutils.NewAISCluster(args, k8sClient)
			createAndDestroyCluster(cluster, pvs, nil, nil, false)
		})
	})

	Context("Multiple Deployments", Label("short"), func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func() {
			ctx := context.Background()
			cluster1, c1pvs := tutils.NewAISCluster(defaultCluArgs(), k8sClient)
			cluster2, c2pvs := tutils.NewAISCluster(defaultCluArgs(), k8sClient)
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyPV(ctx, k8sClient, c2pvs)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
				tutils.DestroyPV(ctx, k8sClient, c1pvs)
			}()
			clusters := []*aisv1.AIStore{cluster1, cluster2}
			createClusters(ctx, clusters, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster1,
				clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2,
				clusterReadyTimeout, clusterReadyRetryInterval)
		})

		It("Should allow two clusters with same name in different namespaces", func() {
			ctx := context.Background()
			cluArgs := defaultCluArgs()
			otherCluArgs := defaultCluArgs()
			otherCluArgs.Namespace = testNSAnotherName
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, k8sClient, testNSAnotherName)
			if !nsExists {
				defer func() {
					_, err := k8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cluster1, c1PVs := tutils.NewAISCluster(cluArgs, k8sClient)
			cluster2, c2PVs := tutils.NewAISCluster(otherCluArgs, k8sClient)
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyPV(ctx, k8sClient, c2PVs)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
				tutils.DestroyPV(ctx, k8sClient, c1PVs)
			}()
			clusters := []*aisv1.AIStore{cluster1, cluster2}
			createClusters(ctx, clusters, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster1,
				clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2,
				clusterReadyTimeout, clusterReadyRetryInterval)
		})
	})

	Context("Upgrade existing cluster", Label("long"), func() {
		It("Should upgrade cluster (without rebalance) if aisnode image changes", func() {
			cluArgs := defaultCluArgs()
			cluArgs.NodeImage = tutils.PreviousNodeImage
			cc, pvs := newClientCluster(cluArgs)
			cc.create()
			cc.updateImage(tutils.DefaultNodeImage)

			// Check we didn't rebalance at all (nothing else should trigger it on this test)
			args := aisxact.ArgsMsg{Kind: aisapc.ActRebalance}
			jobs, err := aisapi.GetAllXactionStatus(cc.getBaseParams(), &args)
			Expect(err).To(BeNil())
			Expect(len(jobs)).To(BeZero())
			cc.cleanup(pvs)
		})
	})

	Context("Scale existing cluster", Label("long"), func() {
		Context("without externalLB", func() {
			It("Should be able to scale-up existing cluster", func() {
				cluArgs := defaultCluArgs()
				cluArgs.MaxTargets = 2
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, false, 1)
				}
				createAndDestroyCluster(cluster, pvs, scaleUpCluster, nil, false)
			})

			It("Should be able to scale-up targets of existing cluster", func() {
				cluArgs := defaultCluArgs()
				cluArgs.MaxTargets = 2
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, true, 1)
				}
				createAndDestroyCluster(cluster, pvs, scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func() {
				cluArgs := defaultCluArgs()
				cluArgs.Size = 2
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, false, -1)
				}
				createAndDestroyCluster(cluster, pvs, scaleDownCluster, nil, true)
			})
		})

		Context("with externalLB", func() {
			It("Should be able to scale-up existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.EnableExternalLB = true
				cluArgs.MaxTargets = 2
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, false, 1)
				}
				createAndDestroyCluster(cluster, pvs, scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.Size = 2
				cluArgs.EnableExternalLB = true
				cluster, pvs := tutils.NewAISCluster(cluArgs, k8sClient)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, false, -1)
				}
				createAndDestroyCluster(cluster, pvs, scaleDownCluster, nil, true)
			})
		})
	})

	Describe("Data-safety tests", Label("long"), func() {
		It("Restarting cluster must retain data", func() {
			cluArgs := defaultCluArgs()
			cc, pvs := newClientCluster(cluArgs)
			cc.create()
			// put objects
			var (
				bck       = aiscmn.Bck{Name: "TEST_BCK_DATA_SAFETY", Provider: aisapc.AIS}
				objPrefix = "test-opr/"
				baseParam = cc.getBaseParams()
			)
			err := aisapi.CreateBucket(baseParam, bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   objPrefix,
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumXXHash,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			// Restart cluster
			cc.restart()
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			cc.cleanup(pvs)
		})

		It("Cluster scale down should ensure data safety", func() {
			cluArgs := defaultCluArgs()
			cluArgs.Size = 2
			cc, pvs := newClientCluster(cluArgs)
			cc.create()
			// put objects
			var (
				bck        = aiscmn.Bck{Name: "TEST_BCK_SCALE_DOWN", Provider: aisapc.AIS}
				objPrefix  = "test-opr/"
				baseParams = cc.getBaseParams()
			)
			err := aisapi.CreateBucket(baseParams, bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   objPrefix,
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumXXHash,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			tutils.ObjectsShouldExist(baseParams, bck, names...)

			// Scale down cluster
			scaleCluster(cc.ctx, cc.cluster, false, -1)

			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			cc.cleanup(pvs)
		})

		It("Re-deploying with CleanupData should wipe out all data", func() {
			// Default sets CleanupData to true -- wipe when we destroy the cluster
			cluArgs := defaultCluArgs()
			cc, pvs := newClientCluster(cluArgs)
			cc.create()
			// Create bucket
			bck := aiscmn.Bck{Name: "TEST_BCK_CLEANUP", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			_, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   "test-opr/",
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumXXHash,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			Expect(err).ShouldNot(HaveOccurred())
			// destroy cluster and pvs (operator should clean up on shutdown before pvs are removed)
			cc.cleanup(pvs)

			checkResShouldNotExist(context.TODO(), cc.cluster)
			// Re-deployed cluster will use the same mounts, but all data should be removed
			cc, pvs = newClientCluster(cluArgs)
			cc.create()
			// All data including metadata should be deleted -- bucket should not exist in new cluster
			_, err = aisapi.HeadBucket(cc.getBaseParams(), bck, true)
			Expect(aiscmn.IsStatusNotFound(err)).To(BeTrue())
			cc.cleanup(pvs)
		})

		It("Re-deploying with CleanupMetadata disabled should recover cluster", func() {
			cluArgs := defaultCluArgs()
			cluArgs.CleanupMetadata = false
			cc, pvs := newClientCluster(cluArgs)
			cc.create()
			// Create bucket
			bck := aiscmn.Bck{Name: "TEST_BCK_DECOMM", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   "test-opr/",
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumXXHash,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			Expect(err).ShouldNot(HaveOccurred())
			// destroy cluster, leave pvs
			cc.cleanup(nil)
			// Cleanup metadata to remove PVCs so we can cleanup PVs at the end
			cluArgs.CleanupMetadata = true
			// Re-deployed cluster should recover all the same data and metadata
			cc, _ = newClientCluster(cluArgs)
			cc.create()
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			cc.cleanup(pvs)
		})
	})
})

func clusterName() string {
	return "aistore-test-cluster-" + strings.ToLower(cos.CryptoRandS(4))
}

func defaultCluArgs() *tutils.ClusterSpecArgs {
	return &tutils.ClusterSpecArgs{
		Name:             clusterName(),
		Namespace:        testNSName,
		StorageClass:     storageClass,
		StorageHostPath:  storageHostPath,
		Size:             1,
		NodeImage:        tutils.DefaultNodeImage,
		InitImage:        tutils.DefaultInitImage,
		CleanupMetadata:  true,
		CleanupData:      true,
		TargetSharedNode: false,
	}
}

func checkResExists(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistence(ctx, cluster, true /*exists*/)
}

func checkResShouldNotExist(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistence(ctx, cluster, false /*exists*/)
	checkPVCDoesNotExist(ctx, cluster)
}

func checkPVCDoesNotExist(ctx context.Context, cluster *aisv1.AIStore) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	err := k8sClient.List(ctx, pvcs, client.InNamespace(cluster.Namespace), client.MatchingLabels(target.PodLabels(cluster)))
	if apierrors.IsNotFound(err) {
		err = nil
	}
	Expect(err).ShouldNot(HaveOccurred())
	Expect(len(pvcs.Items)).To(Equal(0))
}

func checkResExistence(ctx context.Context, cluster *aisv1.AIStore, exists bool, intervals ...interface{}) {
	condition := BeTrue()
	if !exists {
		condition = BeFalse()
	}

	// 1. Check rbac exists
	// 1.1 ServiceAccount
	tutils.EventuallyResourceExists(ctx, k8sClient, cmn.NewAISServiceAccount(cluster), condition, intervals...)
	// 1.2 ClusterRole
	tutils.EventuallyResourceExists(ctx, k8sClient, cmn.NewAISRBACClusterRole(cluster), condition, intervals...)
	// 1.3 ClusterRoleBinding
	tutils.EventuallyCRBExists(ctx, k8sClient, cmn.ClusterRoleBindingName(cluster), condition, intervals...)
	// 1.4 Role
	tutils.EventuallyResourceExists(ctx, k8sClient, cmn.NewAISRBACRole(cluster), condition, intervals...)
	// 1.5 RoleBinding
	tutils.EventuallyResourceExists(ctx, k8sClient, cmn.NewAISRBACRoleBinding(cluster), condition, intervals...)

	// 2. Check for statsD config
	tutils.EventuallyCMExists(ctx, k8sClient, statsd.ConfigMapNSName(cluster), condition, intervals...)

	// 3. Proxy resources
	// 3.1 config
	tutils.EventuallyCMExists(ctx, k8sClient, proxy.ConfigMapNSName(cluster), condition, intervals...)
	// 3.2 Service
	tutils.EventuallyServiceExists(ctx, k8sClient, proxy.HeadlessSVCNSName(cluster), condition, intervals...)
	// 3.3 StatefulSet
	tutils.EventuallySSExists(ctx, k8sClient, proxy.StatefulSetNSName(cluster), condition, intervals...)
	// 3.4 ExternalLB Service (optional)
	if cluster.Spec.EnableExternalLB {
		tutils.EventuallyServiceExists(ctx, k8sClient, proxy.LoadBalancerSVCNSName(cluster), condition, intervals...)
	}

	// 4. Target resources
	// 4.1 config
	tutils.EventuallyCMExists(ctx, k8sClient, target.ConfigMapNSName(cluster), condition, intervals...)
	// 4.2 Service
	tutils.EventuallyServiceExists(ctx, k8sClient, target.HeadlessSVCNSName(cluster), condition, intervals...)
	// 4.3 StatefulSet
	tutils.EventuallySSExists(ctx, k8sClient, target.StatefulSetNSName(cluster), condition, intervals...)
	// 4.4 ExternalLB Service (optional)
	if cluster.Spec.EnableExternalLB {
		timeout, interval := tutils.GetLBExistenceTimeout()
		for i := range cluster.GetTargetSize() {
			tutils.EventuallyServiceExists(ctx, k8sClient, target.LoadBalancerSVCNSName(cluster, i),
				condition, timeout, interval)
		}
	}
}

func createAndDestroyCluster(cluster *aisv1.AIStore, pvs []*corev1.PersistentVolume, postCreate func(context.Context, *aisv1.AIStore),
	postDestroy func(context.Context, *aisv1.AIStore), long bool,
) {
	var (
		ctx       = context.Background()
		intervals []interface{}
	)

	if long {
		intervals = []interface{}{tutils.GetClusterCreateLongTimeout(), tutils.ClusterCreateInterval}
	} else {
		intervals = []interface{}{tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval}
	}

	// Delete cluster.
	defer func() {
		tutils.DestroyCluster(ctx, k8sClient, cluster, intervals...)
		tutils.DestroyPV(ctx, k8sClient, pvs)
		if postDestroy != nil {
			postDestroy(ctx, cluster)
		}
	}()

	createCluster(ctx, cluster, intervals...)
	tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster,
		clusterReadyTimeout, clusterReadyRetryInterval)
	if postCreate != nil {
		postCreate(ctx, cluster)
	}
}

func createCluster(ctx context.Context, cluster *aisv1.AIStore, intervals ...interface{}) {
	Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())
	By("Create cluster and wait for it to be 'Ready'")
	Eventually(func() bool {
		ais := &aisv1.AIStore{}
		_ = k8sClient.Get(ctx, cluster.NamespacedName(), ais)
		return ais.HasState(aisv1.ClusterReady)
	}, intervals...).Should(BeTrue())
}

func createClusters(ctx context.Context, clusters []*aisv1.AIStore, intervals ...interface{}) {
	var wg sync.WaitGroup
	wg.Add(len(clusters))

	for _, cluster := range clusters {
		go func(cluster *aisv1.AIStore) {
			defer GinkgoRecover()
			defer wg.Done()
			createCluster(ctx, cluster, intervals...)
		}(cluster)
	}

	wg.Wait()
}

func restartCluster(ctx context.Context, cluster *aisv1.AIStore) {
	// Shutdown, ensure statefulsets exist and are size 0
	setClusterShutdown(ctx, cluster, true)
	tutils.EventuallyProxyIsSize(ctx, k8sClient, cluster, 0, clusterDestroyTimeout)
	tutils.EventuallyTargetIsSize(ctx, k8sClient, cluster, 0, clusterDestroyTimeout)
	// Resume shutdown cluster, should become fully ready
	setClusterShutdown(ctx, cluster, false)
	tutils.WaitForClusterToBeReady(ctx, k8sClient, cluster,
		clusterReadyTimeout, clusterReadyRetryInterval)
}

func setClusterShutdown(ctx context.Context, cluster *aisv1.AIStore, shutdown bool) {
	cr, err := k8sClient.GetAIStoreCR(ctx, cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	patch := client.MergeFrom(cr.DeepCopy())
	cr.Spec.ShutdownCluster = aisapc.Ptr(shutdown)
	err = k8sClient.Patch(ctx, cr, patch)
	Expect(err).ShouldNot(HaveOccurred())
}

func scaleCluster(ctx context.Context, cluster *aisv1.AIStore, targetOnly bool, factor int32) {
	cr, err := k8sClient.GetAIStoreCR(ctx, cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	if targetOnly {
		cr.Spec.TargetSpec.Size = aisapc.Ptr(cr.GetTargetSize() + factor)
	} else {
		cr.Spec.Size = aisapc.Ptr(*cr.Spec.Size + factor)
	}
	Expect(err).ShouldNot(HaveOccurred())
	err = k8sClient.Update(ctx, cr)
	Expect(err).ShouldNot(HaveOccurred())
	tutils.WaitForClusterToBeReady(ctx, k8sClient, cr, clusterReadyTimeout, clusterReadyRetryInterval)
}
