// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"strconv"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisxact "github.com/NVIDIA/aistore/xact"
	"github.com/ais-operator/tests/tutils"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Run Controller", func() {
	var cluArgs *tutils.ClusterSpecArgs

	BeforeEach(func() {
		cluArgs = tutils.NewClusterSpecArgs(AISTestCfg, WorkerCfg.TestNSName)
	})

	Context("Deploy and feature toggles", Ordered, func() {
		var cc *clientCluster

		BeforeAll(func(ctx context.Context) {
			cc = newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			cc.printLogs(ctx)
			cc.destroyAndCleanup()
			cc.waitForResourceDeletion(ctx)
		})

		It("Should have all required K8s resources", func(ctx context.Context) {
			cc.waitForResources(ctx)
		})

		It("Should deploy admin client when enabled", func(ctx context.Context) {
			cc.enableAdminClient(ctx)
			cc.verifyAdminClientExists(ctx)
		})

		It("Should remove admin client when disabled", func(ctx context.Context) {
			cc.disableAdminClient(ctx)
			cc.verifyAdminClientDeleted(ctx)
		})

		It("Should deploy target PDB when enabled", func(ctx context.Context) {
			cc.enableTargetPDB(ctx)
			cc.verifyTargetPDBExists(ctx)
		})

		It("Should remove target PDB when disabled", func(ctx context.Context) {
			cc.disableTargetPDB(ctx)
			cc.verifyTargetPDBDeleted(ctx)
		})
	})

	Context("Deploy and Destroy cluster", func() {

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func(ctx context.Context) {
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
				cc.createAndDestroyWithWait(ctx)
			})
			It("Should successfully create a hetero-sized AIS Cluster", func(ctx context.Context) {
				// If we have multiple targets on the same node we need a way to reach each of them
				// Require an LB since we can't specify different host ports for each target in a statefulset
				cluArgs.TargetSize = 2
				cluArgs.ProxySize = 1
				cluArgs.DisableTargetAntiAffinity = true
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
				cc.createAndDestroyWithWait(ctx)
			})
		})
	})

	Context("TLS Certificate", func() {
		It("Should create Certificate and Secret via cert-manager", func(ctx context.Context) {
			issuer := &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-selfsigned-issuer",
					Namespace: cluArgs.Namespace,
				},
				Spec: certmanagerv1.IssuerSpec{
					IssuerConfig: certmanagerv1.IssuerConfig{
						SelfSigned: &certmanagerv1.SelfSignedIssuer{},
					},
				},
			}
			_, err := WorkerCfg.K8sClient.CreateResourceIfNotExists(ctx, nil, issuer)
			Expect(err).To(BeNil())

			cluArgs.TLS = &tutils.TLSArgs{
				IssuerName: issuer.Name,
				IssuerKind: "Issuer",
			}
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
				_, err := WorkerCfg.K8sClient.DeleteResourceIfExists(ctx, issuer)
				Expect(err).To(BeNil())
			}()
			cc.create(ctx)
			cc.waitForResources(ctx)
		})
	})

	Context("Multiple Deployments", func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func(ctx context.Context) {
			cc1 := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cluArgs2 := tutils.NewClusterSpecArgs(AISTestCfg, WorkerCfg.TestNSName)
			cc2 := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs2)
			cc2.applyHostPortOffset(int32(5))
			defer func() {
				cc1.printLogs(ctx)
				cc2.printLogs(ctx)
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(ctx, clusters)
			cc1.waitForReadyCluster(ctx)
			cc2.waitForReadyCluster(ctx)
		})

		It("Should allow two clusters with same name in different namespaces", func(ctx context.Context) {
			otherCluArgs := tutils.NewClusterSpecArgs(AISTestCfg, WorkerCfg.TestNSOtherName)
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, WorkerCfg.K8sClient, WorkerCfg.TestNSOtherName)
			if !nsExists {
				defer func() {
					_, err := WorkerCfg.K8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cc1 := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc2 := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, otherCluArgs)
			cc2.applyHostPortOffset(int32(5))
			defer func() {
				cc1.printLogs(ctx)
				cc2.printLogs(ctx)
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(ctx, clusters)
			cc1.waitForReadyCluster(ctx)
			cc2.waitForReadyCluster(ctx)
		})
	})

	Context("Upgrade existing cluster", func() {
		It("Should upgrade cluster (without rebalance) if aisnode image changes", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestCfg.PreviousNodeImage
			cluArgs.InitImage = AISTestCfg.PreviousInitImage
			cluArgs.Size = 2
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			cc.patchImagesToCurrent(ctx)
			cc.verifyPodImages(ctx)

			// Check we didn't rebalance at all (nothing else should trigger it on this test)
			args := aisxact.ArgsMsg{Kind: aisapc.ActRebalance}
			jobs, err := aisapi.GetAllXactionStatus(cc.getBaseParams(ctx), &args)
			Expect(err).To(BeNil())
			Expect(len(jobs)).To(BeZero())
		})

		It("Should successfully upgrade cluster with target PDB enabled", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestCfg.PreviousNodeImage
			cluArgs.InitImage = AISTestCfg.PreviousInitImage
			cluArgs.Size = 2
			cluArgs.EnableTargetPDB = true
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			cc.verifyTargetPDBExists(ctx)
			cc.patchImagesToCurrent(ctx)
			cc.verifyPodImages(ctx)
		})

		It("Should allow reverting a broken upgrade", func(ctx context.Context) {
			cluArgs.Size = 3
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)

			By("Upgrade w/ non-existent images")
			cc.patchImagesToBroken(ctx)

			By("Wait for highest index proxy pod to be stuck in ImagePullBackOff")
			stuckPodName := cc.cluster.ProxyStatefulSetName() + "-" + strconv.Itoa(int(cc.cluster.GetProxySize()-1))
			Eventually(func(ctx context.Context) bool {
				return cc.podHasImagePullError(ctx, stuckPodName)
			}, 60*time.Second, 2*time.Second).WithContext(ctx).Should(BeTrue(), "Pod %s should be stuck in ImagePullBackOff", stuckPodName)

			By("Revert and verify cluster recovers")
			cc.patchImagesToCurrent(ctx)
			cc.verifyPodImages(ctx)
		})
	})

	Context("Scale without LB", Ordered, func() {
		var cc *clientCluster

		BeforeAll(func(ctx context.Context) {
			cluArgs.MaxTargets = 2
			cc = newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			cc.printLogs(ctx)
			cc.destroyAndCleanup()
		})

		It("Should be able to scale-up existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, 1)
		})

		It("Should be able to scale-down existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, -1)
		})

		It("Should be able to scale-up targets only", func(ctx context.Context) {
			cc.scale(ctx, true, 1)
		})

		It("Should be able to scale-down targets only", func(ctx context.Context) {
			cc.scale(ctx, true, -1)
		})
	})

	Context("Scale with LB", Ordered, func() {
		var cc *clientCluster

		BeforeAll(func(ctx context.Context) {
			cluArgs.EnableExternalLB = true
			cluArgs.MaxTargets = 2
			cc = newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			cc.printLogs(ctx)
			cc.destroyAndCleanup()
		})

		It("Should be able to scale-up existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, 1)
		})

		It("Should be able to scale-down existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, -1)
		})
	})

	Context("Scale with PDB", Ordered, func() {
		var cc *clientCluster

		BeforeAll(func(ctx context.Context) {
			cluArgs.EnableTargetPDB = true
			cluArgs.MaxTargets = 2
			cc = newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			cc.printLogs(ctx)
			cc.destroyAndCleanup()
		})

		It("Should have target PDB", func(ctx context.Context) {
			cc.verifyTargetPDBExists(ctx)
		})

		It("Should be able to scale-up existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, 1)
		})

		It("Should be able to scale-down existing cluster", func(ctx context.Context) {
			cc.scale(ctx, false, -1)
		})
	})

	Context("Scale error recovery", func() {
		It("Should allow reverting a broken scale-up", func(ctx context.Context) {
			// MaxTargets=1 means PVs only created for 1 target
			// Scaling to 2 targets means target-1 has no PV and will be stuck Pending
			cluArgs.MaxTargets = 1
			cluArgs.DisableTargetAntiAffinity = true
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)

			By("Scale up targets beyond available PVs")
			cc.attemptScale(ctx, true, 1)

			By("Wait for target pod to be stuck in Pending")
			Eventually(func(ctx context.Context) bool {
				return cc.hasPendingTargetPods(ctx)
			}, 60*time.Second, 2*time.Second).WithContext(ctx).Should(BeTrue(), "Should have target pod stuck in Pending")

			By("Verify target pod stays stuck in Pending state")
			Consistently(func(ctx context.Context) bool {
				return cc.hasPendingTargetPods(ctx)
			}, 10*time.Second, 2*time.Second).WithContext(ctx).Should(BeTrue(), "Target pod should remain stuck in Pending")

			By("Revert scale back to original size")
			cc.scale(ctx, true, -1)
		})
	})

	Describe("Data-safety tests", func() {
		It("Restarting cluster must retain data", func(ctx context.Context) {
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			// put objects
			var (
				bck       = aiscmn.Bck{Name: "TEST_BCK_DATA_SAFETY", Provider: aisapc.AIS}
				objPrefix = "test-opr/"
				baseParam = cc.getBaseParams(ctx)
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
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)
			// Restart cluster
			cc.restart(ctx)
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)
		})

		It("Cluster scale down should ensure data safety", func(ctx context.Context) {
			By("Deploy new cluster of size 2")
			cluArgs.Size = 2
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			By("Create a bucket and put objects")
			var (
				bck        = aiscmn.Bck{Name: "TEST_BCK_SCALE_DOWN", Provider: aisapc.AIS}
				objPrefix  = "test-opr/"
				baseParams = cc.getBaseParams(ctx)
			)
			// TODO: Remove once K8s cluster readiness is tightened to ensure operational readiness.
			Eventually(func() error { return aisapi.CreateBucket(baseParams, bck, nil) }, 5*time.Second).Should(Succeed())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   objPrefix,
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			By("Validate the objects exist")
			tutils.ObjectsShouldExist(baseParams, bck, names...)
			By("Scale down cluster to size 1")
			cc.scale(ctx, false, -1)
			By("Validate objects exist after scaling")
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)
		})

		It("Upgrade and scale-down in same patch should succeed and retain data", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestCfg.PreviousNodeImage
			cluArgs.InitImage = AISTestCfg.PreviousInitImage
			cluArgs.Size = 3
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)

			By("Create a bucket and put objects")
			var (
				bck       = aiscmn.Bck{Name: "TEST_BCK_UPGRADE_SCALE_DOWN", Provider: aisapc.AIS}
				objPrefix = "test-opr/"
			)
			Eventually(func(ctx context.Context) error { return aisapi.CreateBucket(cc.getBaseParams(ctx), bck, nil) }, 5*time.Second).WithContext(ctx).Should(Succeed())
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   objPrefix,
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)

			By("Simultaneously upgrade images and scale down")
			cc.patchImagesAndScale(ctx, -1)
			cc.verifyPodImages(ctx)
			cc.verifyPodCounts(ctx)

			By("Validate objects exist after upgrade and scale down")
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)
		})

		It("Upgrade and scale-up in same patch should succeed", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestCfg.PreviousNodeImage
			cluArgs.InitImage = AISTestCfg.PreviousInitImage
			cluArgs.MaxTargets = 2
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)

			By("Simultaneously upgrade images and scale up")
			cc.patchImagesAndScale(ctx, 1)
			cc.verifyPodImages(ctx)
			cc.verifyPodCounts(ctx)
		})

		It("Re-deploying with CleanupData should wipe out all data", func(ctx context.Context) {
			// Default sets CleanupData to true -- wipe when we destroy the cluster
			By("Deploy with CleanupData true")
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			By("Create AIS bucket")
			bck := aiscmn.Bck{Name: "TEST_BCK_CLEANUP", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(ctx), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			By("Test putting objects")
			_, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   "test-opr/",
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			By("Destroy cluster and delete PVs")
			cc.destroyAndCleanup()
			cc.waitForResourceDeletion(ctx)
			By("Create new cluster with new PVs on the same host mount")
			cluArgs.CleanupMetadata = true
			cc = newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			cc.create(ctx)
			// All data including metadata should be deleted -- bucket should not exist in new cluster
			By("Expect error getting bucket -- all data deleted")
			_, err = aisapi.HeadBucket(cc.getBaseParams(ctx), bck, true)
			Expect(aiscmn.IsStatusNotFound(err)).To(BeTrue())
		})

		It("Re-deploying with CleanupMetadata disabled should recover cluster", func(ctx context.Context) {
			cluArgs.CleanupMetadata = false
			By("Deploy with cleanupMetadata false")
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			By("Create AIS bucket")
			bck := aiscmn.Bck{Name: "TEST_BCK_DECOMM", Provider: aisapc.AIS}
			err := aisapi.CreateBucket(cc.getBaseParams(ctx), bck, nil)
			Expect(err).ShouldNot(HaveOccurred())
			By("Test putting objects")
			names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
				ProxyURL:  cc.proxyURL,
				Bck:       bck,
				ObjPath:   "test-opr/",
				ObjCnt:    10,
				ObjSize:   10 * cos.KiB,
				FixedSize: true,
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			By("Destroy initial cluster but leave PVs")
			cc.destroyClusterOnly()
			// Cleanup metadata to remove PVCs so we can destroyAndCleanup PVs at the end
			cluArgs.CleanupMetadata = true
			// Same cluster should recover all the same data and metadata
			By("Redeploy with cleanupMetadata true")
			cc.recreate(ctx, cluArgs)
			By("Validate objects from previous cluster still exist")
			tutils.ObjectsShouldExist(cc.getBaseParams(ctx), bck, names...)
		})

		It("Should detect port change when cluster is redeployed with different port", func(ctx context.Context) {
			cluArgs.CleanupMetadata = false
			By("Deploy initial cluster with default ports")
			cc := newClientCluster(ctx, AISTestCfg, WorkerCfg.K8sClient, cluArgs)
			defer func() {
				cc.printLogs(ctx)
				// Ensure final cleanup has CleanupMetadata enabled
				cluArgs.CleanupMetadata = true
				cc.destroyAndCleanup()
			}()
			cc.create(ctx)
			initialURL := cc.getProxyURL(ctx)

			By("Re-deploy cluster with different port")
			cc.destroyClusterOnly()
			cc.cluster = tutils.NewAISClusterNoPV(cluArgs)
			cc.applyDefaultHostPortOffset(cluArgs)
			cc.applyHostPortOffset(int32(5))
			cc.createCluster(ctx, cc.getTimeout(), clusterCreateInterval)
			cc.waitForReadyCluster(ctx)

			newURL := cc.getProxyURL(ctx)
			Expect(newURL).NotTo(Equal(initialURL))
			cc.initClientAccess(ctx)
		})
	})

})
