// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package integration

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/statsd"
	"github.com/ais-operator/pkg/resources/target"
	"github.com/ais-operator/tests/tutils"
)

const (
	clusterReadyTimeout       = 2 * time.Minute
	clusterReadyRetryInterval = 10 * time.Second
)

var _ = Describe("Run Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = time.Second
	)
	Context("Deploy and Destroy cluster", func() {
		It("Should successfully create an AIS Cluster", func() {
			cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			createAndDestroyCluster(cluster, nil, nil, timeout, interval)
		})

		It("Should create all required K8s objects, when AIS Cluster is created", func() {
			cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			createAndDestroyCluster(cluster, checkResExists, checkResShouldNotExist, timeout, interval)
		})
	})

	Context("Multiple Deployments", func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func() {
			ctx := context.Background()
			cluster1 := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			cluster2 := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
			}()
			createCluster(cluster1, timeout, interval)
			createCluster(cluster2, timeout, interval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster1, clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2, clusterReadyTimeout, clusterReadyRetryInterval)
		})

		It("Should allow two cluster with same name in different namespaces", func() {
			ctx := context.Background()
			name := clusterName()
			otherNS := testNSName + "-another"
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, k8sClient, otherNS)
			if !nsExists {
				defer func() {
					err := k8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cluster1 := tutils.NewAISClusterCR(name, testNSName, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			cluster2 := tutils.NewAISClusterCR(name, otherNS, storageClass, 1 /*size*/, false /*disableAntiAffinity*/)
			defer func() {
				tutils.DestroyCluster(ctx, k8sClient, cluster2)
				tutils.DestroyCluster(ctx, k8sClient, cluster1)
			}()
			createCluster(cluster1, timeout, interval)
			createCluster(cluster2, timeout, interval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2, clusterReadyTimeout, clusterReadyRetryInterval)
			tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster2, clusterReadyTimeout, clusterReadyRetryInterval)
		})
	})

	Context("Scale existing cluster", func() {
		It("Should be able to scale-up existing cluster", func() {
			cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 1 /*size*/, true /*disableAntiAffinity*/)
			scaleUpCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
				scaleCluster(ctx, cluster, 1)
			}
			createAndDestroyCluster(cluster, scaleUpCluster, nil, timeout, interval)
		})

		It("Should be able to scale-down existing cluster", func() {
			cluster := tutils.NewAISClusterCR(clusterName(), testNSName, storageClass, 2 /*size*/, true /*disableAntiAffinity*/)
			scaleDownCluster := func(ctx context.Context, cluster *aisv1.AIStore) {
				scaleCluster(ctx, cluster, -1)
			}
			createAndDestroyCluster(cluster, scaleDownCluster, nil, timeout, interval)
		})
	})
})

func clusterName() string {
	return "aistore-test-cluster-" + strings.ToLower(aiscmn.RandString(4))
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

	// 1. Check for statsD config
	tutils.EventuallyCMExists(ctx, k8sClient, statsd.ConfigMapNSName(cluster), condition, intervals...)

	// 2. Proxy resources
	// 2.1 config
	tutils.EventuallyCMExists(ctx, k8sClient, proxy.ConfigMapNSName(cluster), condition, intervals...)
	// 2.2 Service
	tutils.EventuallyServiceExists(ctx, k8sClient, proxy.HeadlessSVCNSName(cluster), condition, intervals...)
	// 2.3 StatefulSet
	tutils.EventuallySSExists(ctx, k8sClient, proxy.StatefulSetNSName(cluster), condition, intervals...)

	// 3. Target resources
	// 3.1 config
	tutils.EventuallyCMExists(ctx, k8sClient, target.ConfigMapNSName(cluster), condition, intervals...)
	// 3.2 Service
	tutils.EventuallyServiceExists(ctx, k8sClient, target.HeadlessSVCNSName(cluster), condition, intervals...)
	// 3.3 StatefulSet
	tutils.EventuallySSExists(ctx, k8sClient, target.StatefulSetNSName(cluster), condition, intervals...)
}

func createAndDestroyCluster(cluster *aisv1.AIStore, postCreate func(context.Context, *aisv1.AIStore), postDestroy func(context.Context, *aisv1.AIStore), intervals ...interface{}) {
	var ctx = context.Background()

	// Delete cluster.
	defer func() {
		tutils.DestroyCluster(ctx, k8sClient, cluster, intervals...)
		if postDestroy != nil {
			postDestroy(ctx, cluster)
		}
	}()

	createCluster(cluster, intervals...)
	tutils.WaitForClusterToBeReady(context.Background(), k8sClient, cluster, clusterReadyTimeout, clusterReadyRetryInterval)
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
