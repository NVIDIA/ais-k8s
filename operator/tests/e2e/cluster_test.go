// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisxact "github.com/NVIDIA/aistore/xact"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Run Controller", func() {
	Context("Deploy and Destroy cluster", Label("short"), func() {
		Context("without externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func(ctx context.Context) {
				cc := newClientCluster(ctx, defaultCluArgs())
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion, false)
			})
		})

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func(ctx context.Context) {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, cluArgs)
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion, true)
			})
			It("Should successfully create a hetero-sized AIS Cluster", func(ctx context.Context) {
				// If we have multiple targets on the same node we need a way to reach each of them
				// Require an LB since we can't specify different host ports for each target in a statefulset
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.TargetSize = 2
				cluArgs.ProxySize = 1
				cluArgs.DisableTargetAntiAffinity = true
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, cluArgs)
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion, true)
			})
		})
	})

	Context("Multiple Deployments", Label("short"), func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func(ctx context.Context) {
			cc1 := newClientCluster(ctx, defaultCluArgs())
			cc2 := newClientCluster(ctx, defaultCluArgs())
			cc2.applyHostPortOffset(int32(10))
			defer func() {
				Expect(tutils.PrintLogs(cc1.ctx, cc1.cluster, k8sClient)).To(Succeed())
				Expect(tutils.PrintLogs(cc2.ctx, cc2.cluster, k8sClient)).To(Succeed())
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(clusters, false)
			cc1.waitForReadyCluster()
			cc2.waitForReadyCluster()
		})

		It("Should allow two clusters with same name in different namespaces", func(ctx context.Context) {
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
			cc1 := newClientCluster(ctx, cluArgs)
			cc2 := newClientCluster(ctx, otherCluArgs)
			cc2.applyHostPortOffset(int32(10))
			defer func() {
				Expect(tutils.PrintLogs(cc1.ctx, cc1.cluster, k8sClient)).To(Succeed())
				Expect(tutils.PrintLogs(cc2.ctx, cc2.cluster, k8sClient)).To(Succeed())
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(clusters, false)
			cc1.waitForReadyCluster()
			cc2.waitForReadyCluster()
		})
	})

	Context("Upgrade existing cluster", Label("long"), func() {
		It("Should upgrade cluster (without rebalance) if aisnode image changes", func(ctx context.Context) {
			cluArgs := defaultCluArgs()
			cluArgs.NodeImage = tutils.PreviousNodeImage
			cc := newClientCluster(ctx, cluArgs)
			cc.create(true)
			cc.patchImage(tutils.DefaultNodeImage)

			// Check we didn't rebalance at all (nothing else should trigger it on this test)
			args := aisxact.ArgsMsg{Kind: aisapc.ActRebalance}
			jobs, err := aisapi.GetAllXactionStatus(cc.getBaseParams(), &args)
			Expect(err).To(BeNil())
			Expect(len(jobs)).To(BeZero())
			Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
			cc.destroyAndCleanup()
		})
	})

	Context("Scale existing cluster", Label("long"), func() {
		Context("without externalLB", func() {
			It("Should be able to scale-up existing cluster", func(ctx context.Context) {
				cluArgs := defaultCluArgs()
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, cluArgs)
				scaleUpCluster := func() {
					cc.scale(false, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil, false)
			})

			It("Should be able to scale-up targets of existing cluster", func(ctx context.Context) {
				cluArgs := defaultCluArgs()
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, cluArgs)
				scaleUpCluster := func() {
					cc.scale(true, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func(ctx context.Context) {
				cluArgs := defaultCluArgs()
				cluArgs.Size = 2
				cc := newClientCluster(ctx, cluArgs)
				scaleDownCluster := func() {
					cc.scale(false, -1)
				}
				cc.createAndDestroyCluster(scaleDownCluster, nil, true)
			})
		})

		Context("with externalLB", func() {
			It("Should be able to scale-up existing cluster", func(ctx context.Context) {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.EnableExternalLB = true
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, cluArgs)
				scaleUpCluster := func() {
					cc.scale(false, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil, true)
			})

			It("Should be able to scale-down existing cluster", func(ctx context.Context) {
				tutils.CheckSkip(&tutils.SkipArgs{RequiresLB: true})
				cluArgs := defaultCluArgs()
				cluArgs.Size = 2
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, cluArgs)
				scaleDownCluster := func() {
					cc.scale(false, -1)
				}
				cc.createAndDestroyCluster(scaleDownCluster, nil, true)
			})
		})
	})

	Describe("Data-safety tests", Label("long"), func() {
		It("Restarting cluster must retain data", func(ctx context.Context) {
			cluArgs := defaultCluArgs()
			cc := newClientCluster(ctx, cluArgs)
			cc.create(true)
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
			Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
			cc.destroyAndCleanup()
		})

		It("Cluster scale down should ensure data safety", func(ctx context.Context) {
			By("Deploy new cluster of size 2")
			cluArgs := defaultCluArgs()
			cluArgs.Size = 2
			cc := newClientCluster(ctx, cluArgs)
			cc.create(true)
			By("Create a bucket and put objects")
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
			By("Validate the objects exist")
			tutils.ObjectsShouldExist(baseParams, bck, names...)
			By("Scale down cluster to size 1")
			cc.scale(false, -1)
			By("Validate objects exist after scaling")
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
			cc.destroyAndCleanup()
		})

		It("Re-deploying with CleanupData should wipe out all data", func(ctx context.Context) {
			// Default sets CleanupData to true -- wipe when we destroy the cluster
			By("Deploy with CleanupData true")
			cluArgs := defaultCluArgs()
			cc := newClientCluster(ctx, cluArgs)
			cc.create(true)
			By("Create AIS bucket")
			bck := aiscmn.Bck{Name: "TEST_BCK_CLEANUP", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			By("Test putting objects")
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
			By("Destroy cluster including PVs")
			// Operator should clean up host data on shutdown before pvs are removed
			cc.destroyAndCleanup()
			cc.waitForResourceDeletion()
			By("Create new cluster with the new PVs on the same host mount")
			cc = newClientCluster(ctx, cluArgs)
			cc.create(true)
			// All data including metadata should be deleted -- bucket should not exist in new cluster
			By("Expect error getting bucket -- all data deleted")
			_, err = aisapi.HeadBucket(cc.getBaseParams(), bck, true)
			Expect(aiscmn.IsStatusNotFound(err)).To(BeTrue())
			Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
			cc.destroyAndCleanup()
		})

		It("Re-deploying with CleanupMetadata disabled should recover cluster", func(ctx context.Context) {
			cluArgs := defaultCluArgs()
			cluArgs.CleanupMetadata = false
			By("Deploy with cleanupMetadata false")
			cc := newClientCluster(ctx, cluArgs)
			cc.create(true)
			By("Create AIS bucket")
			bck := aiscmn.Bck{Name: "TEST_BCK_DECOMM", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			By("Test putting objects")
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
			By("Destroy initial cluster but leave PVs")
			cc.destroyClusterOnly()
			// Cleanup metadata to remove PVCs so we can destroyAndCleanup PVs at the end
			cluArgs.CleanupMetadata = true
			// Same cluster should recover all the same data and metadata
			By("Redeploy with cleanupMetadata true")
			cc.recreate(cluArgs, true)
			By("Validate objects from previous cluster still exist")
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			Expect(tutils.PrintLogs(cc.ctx, cc.cluster, k8sClient)).To(Succeed())
			cc.destroyAndCleanup()
		})
	})
})
