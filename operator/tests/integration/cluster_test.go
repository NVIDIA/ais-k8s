// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package integration

import (
	"context"
	"strings"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterReadyRetryInterval = 10 * time.Second
	clusterReadyTimeout       = 3 * time.Minute
)

// clientCluster - used for managing cluster used AIS API tests
type clientCluster struct {
	cluster          *aisv1.AIStore
	tout             time.Duration
	ctx              context.Context
	cancelLogsStream context.CancelFunc
}

func newClientCluster(cluArgs tutils.ClusterSpecArgs) *clientCluster {
	cc := &clientCluster{
		cluster: tutils.NewAISClusterCR(cluArgs),
		tout:    tutils.GetClusterCreateTimeout(),
	}

	if cluArgs.EnableExternalLB {
		tutils.InitK8sClusterProvider(context.Background(), k8sClient)
		tutils.SkipIfLoadBalancerNotSupported()
		// For a cluster with external LB, allocating external-IP could be time consuming.
		// Allow longer timeout for cluster creation.
		cc.tout = tutils.GetClusterCreateLongTimeout()
	}
	return cc
}

func (cc *clientCluster) create() {
	cc.ctx, cc.cancelLogsStream = context.WithCancel(context.Background())
	createCluster(cc.cluster, cc.tout, tutils.ClusterCreateInterval)
	tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cc.cluster, clusterReadyTimeout, clusterReadyRetryInterval)
	initAISCluster(context.Background(), cc.cluster)
	Expect(tutils.StreamLogs(cc.ctx, testNSName)).To(BeNil())
}

func (cc *clientCluster) cleanup() {
	cc.cancelLogsStream()
	tutils.DestroyCluster(context.Background(), k8sClient, cc.cluster, cc.tout, tutils.ClusterCreateInterval)
}

var _ = Describe("Run Controller", func() {
	Context("Deploy and Destroy cluster", func() {
		Context("without externalLB", func() {
			It("Should successfully create an AIS Cluster", func() {
				cluster := tutils.NewAISClusterCR(defaultCluArgs())
				createAndDestroyCluster(cluster, nil, nil, false)
			})

			It("Should create all required K8s objects, when AIS Cluster is created", func() {
				tutils.CheckSkip(&tutils.SkipArgs{OnlyLong: true})
				cluster := tutils.NewAISClusterCR(defaultCluArgs())
				createAndDestroyCluster(cluster, checkResExists, checkResShouldNotExist, false)
			})
		})

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 1,
					EnableExternalLB:     true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				createAndDestroyCluster(cluster, nil, nil, true)
			})

			It("Should create all required K8s objects, when AIS Cluster is created", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 1,
					EnableExternalLB:     true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				createAndDestroyCluster(cluster, checkResExists, checkResShouldNotExist, true)
			})
		})
	})

	Context("Multiple Deployments", func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func() {
			ctx := context.Background()
			cluster1 := tutils.NewAISClusterCR(defaultCluArgs())
			cluster2 := tutils.NewAISClusterCR(defaultCluArgs())
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
			}()
			createCluster(cluster1, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			createCluster(cluster2, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster1,
				clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2,
				clusterReadyTimeout, clusterReadyRetryInterval)
		})

		It("Should allow two cluster with same name in different namespaces", func() {
			ctx := context.Background()
			cluArgs := defaultCluArgs()
			otherCluArgs := cluArgs
			otherCluArgs.Namespace = testNSAnotherName
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, k8sClient, testNSAnotherName)
			if !nsExists {
				defer func() {
					_, err := k8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cluster1 := tutils.NewAISClusterCR(cluArgs)
			cluster2 := tutils.NewAISClusterCR(otherCluArgs)
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
			}()
			createCluster(cluster1, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			createCluster(cluster2, tutils.GetClusterCreateTimeout(), tutils.ClusterCreateInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2,
				clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2,
				clusterReadyTimeout, clusterReadyRetryInterval)
		})
	})

	Context("Scale existing cluster", func() {
		Context("without externalLB", func() {
			It("Should be able to scale-up existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{SkipInternal: testAsExternalClient})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 1,
					DisableAntiAffinity:  true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, 1)
				}
				createAndDestroyCluster(cluster, scaleUpCluster, nil, false)
			})

			It("Should be able to scale-down existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{SkipInternal: testAsExternalClient})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 2,
					DisableAntiAffinity:  true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, -1)
				}
				createAndDestroyCluster(cluster, scaleDownCluster, nil, false)
			})
		})

		Context("with externalLB", func() {
			It("Should be able to scale-up existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 1,
					DisableAntiAffinity:  true,
					EnableExternalLB:     true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, 1)
				}
				createAndDestroyCluster(cluster, scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 2,
					DisableAntiAffinity:  true,
					EnableExternalLB:     true,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cluster := tutils.NewAISClusterCR(cluArgs)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, -1)
				}
				createAndDestroyCluster(cluster, scaleDownCluster, nil, true)
			})
		})
	})

	Describe("Data-safety tests", func() {
		It("Re-deploying same cluster must retain data", func() {
			tutils.CheckSkip(&tutils.SkipArgs{OnlyLong: true})
			cluArgs := tutils.ClusterSpecArgs{
				Name:                 clusterName(),
				Namespace:            testNSName,
				StorageClass:         storageClass,
				Size:                 1,
				EnableExternalLB:     testAsExternalClient,
				PreservePVCs:         true,
				AllowSharedOrNoDisks: testAllowSharedNoDisks,
			}
			cc := newClientCluster(cluArgs)
			cc.create()
			// put objects
			var (
				bck       = aiscmn.Bck{Name: "TEST_BUCKET", Provider: aisapc.AIS}
				objPrefix = "test-opr/"
				baseParam = aistutils.BaseAPIParams(proxyURL)
			)
			aisapi.DestroyBucket(baseParam, bck)
			err := aisapi.CreateBucket(baseParam, bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  proxyURL,
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
			aistutils.EnsureObjectsExist(testCtx, baseParam, bck, names...)
			cc.cleanup()

			// Re-deploy cluster and check if the data exists.
			// Don't preserve the PVCs for re-deploy cluster (cleanup).
			cluArgs.PreservePVCs = false
			cc = newClientCluster(cluArgs)
			cc.create()
			aistutils.EnsureObjectsExist(testCtx, aistutils.BaseAPIParams(proxyURL), bck, names...)
			cc.cleanup()
		})

		It("Cluster scale down should ensure data safety", func() {
			tutils.CheckSkip(&tutils.SkipArgs{OnlyLong: true})
			cluArgs := tutils.ClusterSpecArgs{
				Name:                 clusterName(),
				Namespace:            testNSName,
				StorageClass:         storageClass,
				Size:                 2,
				DisableAntiAffinity:  true,
				EnableExternalLB:     testAsExternalClient,
				AllowSharedOrNoDisks: testAllowSharedNoDisks,
			}
			cc := newClientCluster(cluArgs)
			cc.create()
			// put objects
			var (
				bck       = aiscmn.Bck{Name: "TEST_BUCKET", Provider: aisapc.AIS}
				objPrefix = "test-opr/"
				baseParam = aistutils.BaseAPIParams(proxyURL)
			)
			aisapi.DestroyBucket(baseParam, bck)
			err := aisapi.CreateBucket(baseParam, bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  proxyURL,
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
			aistutils.EnsureObjectsExist(testCtx, baseParam, bck, names...)

			// Scale down cluster
			scaleCluster(context.TODO(), cc.cluster, -1)

			aistutils.EnsureObjectsExist(testCtx, aistutils.BaseAPIParams(proxyURL), bck, names...)
			cc.cleanup()
		})

		It("Re-deploying without preserving PVCs should wipeout all data", func() {
			tutils.CheckSkip(&tutils.SkipArgs{OnlyLong: true})
			cluArgs := tutils.ClusterSpecArgs{
				Name:                 clusterName(),
				Namespace:            testNSName,
				StorageClass:         storageClass,
				Size:                 1,
				EnableExternalLB:     testAsExternalClient,
				AllowSharedOrNoDisks: testAllowSharedNoDisks,
			}
			cc := newClientCluster(cluArgs)
			cc.create()
			// Create bucket
			bck := aiscmn.Bck{Name: "TEST_BUCKET", Provider: aisapc.AIS}
			baseParams := aistutils.BaseAPIParams(proxyURL)
			aisapi.DestroyBucket(baseParams, bck)
			err := aisapi.CreateBucket(baseParams, bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			_, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  proxyURL,
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
			cc.cleanup()

			checkResShouldNotExist(context.TODO(), cc.cluster)
			// Re-deploy cluster and check if data is wipedoff
			cc = newClientCluster(cluArgs)
			cc.create()
			baseParams = aistutils.BaseAPIParams(proxyURL)
			_, err = aisapi.HeadBucket(baseParams, bck, true)
			if err == nil {
				// NOTE: When we redeploy a cluster in same namespace, proxy metadata from previous deployment is not
				// deleted due to the usage of `hostPath` volume for storing proxy metadata.
				// New primary proxy finds the old BMD from previous deployment and metasyncs it,
				// leading to creation of all the buckets. However, the bucket data doesn't exist anymore
				// as the targets PVs/PVCs are deleted.
				objList, err := aisapi.ListObjects(baseParams, bck, nil, 0)
				Expect(err).ShouldNot(HaveOccurred())
				if objList != nil {
					Expect(len(objList.Entries)).To(Equal(0))
				}
			} else {
				Expect(aiscmn.IsStatusNotFound(err)).To(BeTrue())
			}
			cc.cleanup()
		})
	})

	Describe("Client tests", func() {
		var (
			count = 0
			cc    *clientCluster
		)
		// NOTE: the `BeforeEach`/`AfterEach` code intends to imitate non-existing `BeforeAll`/`AfterAll` functionalities.
		BeforeEach(func() {
			count++
			if count == 1 {
				Expect(count).To(Equal(1))
				cluArgs := tutils.ClusterSpecArgs{
					Name:                 clusterName(),
					Namespace:            testNSName,
					StorageClass:         storageClass,
					Size:                 1,
					DisableAntiAffinity:  true,
					EnableExternalLB:     testAsExternalClient,
					AllowSharedOrNoDisks: testAllowSharedNoDisks,
				}
				cc = newClientCluster(cluArgs)
				cc.create()
			}
		})
		AfterEach(func() {
			if count == len(tests) && cc != nil {
				cc.cleanup()
			}
		})

		DescribeTable(
			"AIS cluster tests",
			runCustom,
			tests...,
		)
	})
})

func clusterName() string {
	return "aistore-test-cluster-" + strings.ToLower(cos.RandStringStrong(4))
}

func defaultCluArgs() tutils.ClusterSpecArgs {
	return tutils.ClusterSpecArgs{
		Name:                 clusterName(),
		Namespace:            testNSName,
		StorageClass:         storageClass,
		Size:                 1,
		AllowSharedOrNoDisks: testAllowSharedNoDisks,
	}
}

func checkResExists(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistance(ctx, cluster, true /*exists*/)
}

func checkResShouldNotExist(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistance(ctx, cluster, false /*exists*/)

	// PVCs should be deleted if Delete strategy is set.
	if cluster.Spec.CleanupData != nil && *cluster.Spec.CleanupData {
		pvcs := &corev1.PersistentVolumeClaimList{}
		err := k8sClient.List(ctx, pvcs, client.InNamespace(cluster.Namespace), client.MatchingLabels(target.PodLabels(cluster)))
		if apierrors.IsNotFound(err) {
			err = nil
		}
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(pvcs.Items)).To(Equal(0))
	}
}

func checkResExistance(ctx context.Context, cluster *aisv1.AIStore, exists bool, intervals ...interface{}) {
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
		for i := int32(0); i < cluster.Spec.Size; i++ {
			tutils.EventuallyServiceExists(ctx, k8sClient, target.LoadBalancerSVCNSName(cluster, i),
				condition, timeout, interval)
		}
	}
}

func createAndDestroyCluster(cluster *aisv1.AIStore, postCreate func(context.Context, *aisv1.AIStore),
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
		if postDestroy != nil {
			postDestroy(ctx, cluster)
		}
	}()

	createCluster(cluster, intervals...)
	tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster,
		clusterReadyTimeout, clusterReadyRetryInterval)
	if postCreate != nil {
		postCreate(ctx, cluster)
	}
}

func createCluster(cluster *aisv1.AIStore, intervals ...interface{}) {
	Expect(k8sClient.Create(context.Background(), cluster)).Should(Succeed())
	By("Create cluster and mark status as 'Created'")
	Eventually(func() bool {
		r := &aisv1.AIStore{}
		_ = k8sClient.Get(context.Background(), cluster.NamespacedName(), r)
		return r.Status.State == aisv1.ConditionCreated
	}, intervals...).Should(BeTrue())
}

func scaleCluster(ctx context.Context, cluster *aisv1.AIStore, factor int32) {
	cr, err := k8sClient.GetAIStoreCR(ctx, cluster.NamespacedName())
	Expect(err).ShouldNot(HaveOccurred())
	cr.Spec.Size += factor
	err = k8sClient.Update(ctx, cr)
	Expect(err).ShouldNot(HaveOccurred())
	tutils.WaitForClusterToBeReady(ctx, k8sClient, cr, clusterReadyTimeout, clusterReadyRetryInterval)
}
