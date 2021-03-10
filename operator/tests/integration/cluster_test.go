// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package integration

import (
	"context"
	"strings"
	"time"

	"github.com/NVIDIA/aistore/cmn/cos"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
)

const (
	clusterReadyRetryInterval = 10 * time.Second
	clusterReadyTimeout       = 3 * time.Minute
)

var _ = Describe("Run Controller", func() {
	Context("Deploy and Destroy cluster", func() {
		Context("without externalLB", func() {
			It("Should successfully create an AIS Cluster", func() {
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, false)
				createAndDestroyCluster(cluster, nil, nil, false)
			})

			It("Should create all required K8s objects, when AIS Cluster is created", func() {
				tutils.CheckSkip(&tutils.SkipArgs{OnlyLong: true})
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, false)
				createAndDestroyCluster(cluster, checkResExists, checkResShouldNotExist, false)
			})
		})

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, true)
				createAndDestroyCluster(cluster, nil, nil, true)
			})

			It("Should create all required K8s objects, when AIS Cluster is created", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, true)
				createAndDestroyCluster(cluster, checkResExists, checkResShouldNotExist, true)
			})
		})
	})

	Context("Multiple Deployments", func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func() {
			ctx := context.Background()
			cluster1 := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, false)
			cluster2 := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, false, false)
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
			name := clusterName()
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, k8sClient, testNSAnotherName)
			if !nsExists {
				defer func() {
					_, err := k8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cluster1 := tutils.NewAISClusterCR(name, testNSName, storageClass, 1, false, false)
			cluster2 := tutils.NewAISClusterCR(name, testNSAnotherName, storageClass, 1, false, false)
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
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, true, false)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, 1)
				}
				createAndDestroyCluster(cluster, scaleUpCluster, nil, false)
			})

			It("Should be able to scale-down existing cluster", func() {
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 2, true, false)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, -1)
				}
				createAndDestroyCluster(cluster, scaleDownCluster, nil, false)
			})
		})

		Context("with externalLB", func() {
			It("Should be able to scale-up existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, true, true)
				scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, 1)
				}
				createAndDestroyCluster(cluster, scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func() {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true, OnlyLong: true})
				cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 2, true, true)
				scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
					scaleCluster(ctx, cluster, -1)
				}
				createAndDestroyCluster(cluster, scaleDownCluster, nil, true)
			})
		})
	})

	Describe("Client tests", func() {
		var (
			cluster *aisv1.AIStore
			count   = 0
			tout    time.Duration
		)
		// NOTE: the `BeforeEach`/`AfterEach` code intends to imitate non-existing `BeforeAll`/`AfterAll` functionalities.
		BeforeEach(func() {
			tout = tutils.GetClusterCreateTimeout()
			count++
			if count == 1 {
				cluster = tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1, true, true)
				cluster.Spec.EnableExternalLB = testAsExternalClient
				if testAsExternalClient {
					tutils.InitK8sClusterProvider(context.Background(), k8sClient)
					tutils.SkipIfLoadBalancerNotSupported()
					// For a cluster with external LB, allocating external-IP could be time consuming.
					// Allow longer timeout for cluster creation.
					tout = tutils.GetClusterCreateLongTimeout()
				}
				Expect(count).To(Equal(1))
				createCluster(cluster, tout, tutils.ClusterCreateInterval)
				tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster, clusterReadyTimeout, clusterReadyRetryInterval)
				initAISCluster(context.Background(), cluster)
			}
		})
		AfterEach(func() {
			if count == len(tests) {
				tutils.DestroyCluster(context.Background(), k8sClient, cluster, tout, tutils.ClusterCreateInterval)
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
	return "aistore-test-cluster-" + strings.ToLower(cos.RandString(4))
}

func checkResExists(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistance(ctx, cluster, true /*exists*/)
}

func checkResShouldNotExist(ctx context.Context, cluster *aisv1.AIStore) {
	checkResExistance(ctx, cluster, false /*exists*/)
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
	postDestroy func(context.Context, *aisv1.AIStore), long bool) {
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
