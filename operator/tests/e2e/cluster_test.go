// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
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
		cluArgs = tutils.NewClusterSpecArgs(AISTestContext, WorkerCtx.TestNSName)
	})

	Context("Deploy and Destroy cluster", func() {
		Context("without externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func(ctx context.Context) {
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion)
			})

			It("Should deploy admin client when enabled", func(ctx context.Context) {
				cluArgs.EnableAdminClient = true
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				cc.createAndDestroyCluster(cc.verifyAdminClientExists, cc.verifyAdminClientDeleted)
			})

			It("Should allow toggling admin client on running cluster", func(ctx context.Context) {
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				defer func() {
					cc.printLogs()
					cc.destroyAndCleanup()
				}()
				cc.create()
				cc.waitForResources()

				cc.enableAdminClient()
				cc.verifyAdminClientExists()

				cc.disableAdminClient()
				cc.verifyAdminClientDeleted()
			})

			It("Should allow toggling target PDB on running cluster", func(ctx context.Context) {
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				defer func() {
					cc.printLogs()
					cc.destroyAndCleanup()
				}()
				cc.create()
				cc.waitForResources()

				cc.enableTargetPDB()
				cc.verifyTargetPDBExists()

				cc.disableTargetPDB()
				cc.verifyTargetPDBDeleted()
			})
		})

		Context("with externalLB", func() {
			It("Should successfully create an AIS Cluster with required K8s objects", func(ctx context.Context) {
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion)
			})
			It("Should successfully create a hetero-sized AIS Cluster", func(ctx context.Context) {
				// If we have multiple targets on the same node we need a way to reach each of them
				// Require an LB since we can't specify different host ports for each target in a statefulset
				cluArgs.TargetSize = 2
				cluArgs.ProxySize = 1
				cluArgs.DisableTargetAntiAffinity = true
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				cc.createAndDestroyCluster(cc.waitForResources, cc.waitForResourceDeletion)
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
			_, err := WorkerCtx.K8sClient.CreateResourceIfNotExists(ctx, nil, issuer)
			Expect(err).To(BeNil())

			cluArgs.TLSIssuerName = issuer.Name
			cluArgs.TLSIssuerKind = "Issuer"
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				Expect(cc.printLogs()).To(Succeed())
				cc.destroyAndCleanup()
			}()
			cc.create()
			cc.waitForResources()
		})
	})

	Context("Multiple Deployments", func() {
		// Running multiple clusters in the same cluster
		It("Should allow running two clusters in the same namespace", func(ctx context.Context) {
			cc1 := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			cluArgs2 := tutils.NewClusterSpecArgs(AISTestContext, WorkerCtx.TestNSName)
			cc2 := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs2)
			cc2.applyHostPortOffset(int32(5))
			defer func() {
				Expect(cc1.printLogs()).To(Succeed())
				Expect(cc2.printLogs()).To(Succeed())
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(clusters)
			cc1.waitForReadyCluster()
			cc2.waitForReadyCluster()
		})

		It("Should allow two clusters with same name in different namespaces", func(ctx context.Context) {
			otherCluArgs := tutils.NewClusterSpecArgs(AISTestContext, WorkerCtx.TestNSOtherName)
			newNS, nsExists := tutils.CreateNSIfNotExists(ctx, WorkerCtx.K8sClient, WorkerCtx.TestNSOtherName)
			if !nsExists {
				defer func() {
					_, err := WorkerCtx.K8sClient.DeleteResourceIfExists(ctx, newNS)
					Expect(err).To(BeNil())
				}()
			}
			cc1 := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			cc2 := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, otherCluArgs)
			cc2.applyHostPortOffset(int32(5))
			defer func() {
				Expect(cc1.printLogs()).To(Succeed())
				Expect(cc2.printLogs()).To(Succeed())
				cc2.destroyAndCleanup()
				cc1.destroyAndCleanup()
			}()
			clusters := []*clientCluster{cc1, cc2}
			createClusters(clusters)
			cc1.waitForReadyCluster()
			cc2.waitForReadyCluster()
		})
	})

	Context("Upgrade existing cluster", func() {
		It("Should upgrade cluster (without rebalance) if aisnode image changes", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestContext.PreviousNodeImage
			cluArgs.InitImage = AISTestContext.PreviousInitImage
			cluArgs.Size = 2
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
			cc.create()
			cc.patchImagesToCurrent()

			// Check we didn't rebalance at all (nothing else should trigger it on this test)
			args := aisxact.ArgsMsg{Kind: aisapc.ActRebalance}
			jobs, err := aisapi.GetAllXactionStatus(cc.getBaseParams(), &args)
			Expect(err).To(BeNil())
			Expect(len(jobs)).To(BeZero())
		})

		It("Should successfully upgrade cluster with target PDB enabled", func(ctx context.Context) {
			cluArgs.NodeImage = AISTestContext.PreviousNodeImage
			cluArgs.InitImage = AISTestContext.PreviousInitImage
			cluArgs.Size = 2
			cluArgs.EnableTargetPDB = true
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
			cc.create()
			cc.verifyTargetPDBExists()
			cc.patchImagesToCurrent()
		})
	})

	Context("Scale existing cluster", func() {
		Context("without externalLB", func() {
			It("Should be able to scale-up existing cluster", func(ctx context.Context) {
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleUpCluster := func() {
					cc.scale(false, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil)
			})

			It("Should be able to scale-up targets of existing cluster", func(ctx context.Context) {
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleUpCluster := func() {
					cc.scale(true, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil)
			})

			It("Should be able to scale-down existing cluster", func(ctx context.Context) {
				cluArgs.Size = 2
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleDownCluster := func() {
					cc.scale(false, -1)
				}
				cc.createAndDestroyCluster(scaleDownCluster, nil)
			})
		})

		Context("with externalLB", func() {
			It("Should be able to scale-up existing cluster", func(ctx context.Context) {
				cluArgs.EnableExternalLB = true
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleUpCluster := func() {
					cc.scale(false, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil)
			})

			It("Should be able to scale-down existing cluster", func(ctx context.Context) {
				cluArgs.Size = 2
				cluArgs.EnableExternalLB = true
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleDownCluster := func() {
					cc.scale(false, -1)
				}
				cc.createAndDestroyCluster(scaleDownCluster, nil)
			})
		})

		Context("with target PDB", func() {
			It("Should be able to scale-up existing cluster", func(ctx context.Context) {
				cluArgs.EnableTargetPDB = true
				cluArgs.MaxTargets = 2
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleUpCluster := func() {
					cc.verifyTargetPDBExists()
					cc.scale(false, 1)
				}
				cc.createAndDestroyCluster(scaleUpCluster, nil)
			})

			It("Should be able to scale-down existing cluster", func(ctx context.Context) {
				cluArgs.Size = 2
				cluArgs.EnableTargetPDB = true
				cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
				scaleDownCluster := func() {
					cc.verifyTargetPDBExists()
					cc.scale(false, -1)
				}
				cc.createAndDestroyCluster(scaleDownCluster, nil)
			})
		})
	})

	Describe("Data-safety tests", func() {
		It("Restarting cluster must retain data", func(ctx context.Context) {
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
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
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
			// Restart cluster
			cc.restart()
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
		})

		It("Cluster scale down should ensure data safety", func(ctx context.Context) {
			By("Deploy new cluster of size 2")
			cluArgs.Size = 2
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
			cc.create()
			By("Create a bucket and put objects")
			var (
				bck        = aiscmn.Bck{Name: "TEST_BCK_SCALE_DOWN", Provider: aisapc.AIS}
				objPrefix  = "test-opr/"
				baseParams = cc.getBaseParams()
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
			cc.scale(false, -1)
			By("Validate objects exist after scaling")
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
		})

		It("Re-deploying with CleanupData should wipe out all data", func(ctx context.Context) {
			// Default sets CleanupData to true -- wipe when we destroy the cluster
			By("Deploy with CleanupData true")
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
			cc.create()
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
				CksumType: cos.ChecksumOneXxh,
				IgnoreErr: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failCnt).To(Equal(0))
			By("Destroy cluster and delete PVs")
			cc.destroyAndCleanup()
			cc.waitForResourceDeletion()
			By("Create new cluster with new PVs on the same host mount")
			cluArgs.CleanupMetadata = true
			cc = newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			cc.create()
			// All data including metadata should be deleted -- bucket should not exist in new cluster
			By("Expect error getting bucket -- all data deleted")
			_, err = aisapi.HeadBucket(cc.getBaseParams(), bck, true)
			Expect(aiscmn.IsStatusNotFound(err)).To(BeTrue())
		})

		It("Re-deploying with CleanupMetadata disabled should recover cluster", func(ctx context.Context) {
			cluArgs.CleanupMetadata = false
			By("Deploy with cleanupMetadata false")
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				cc.destroyAndCleanup()
			}()
			cc.create()
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
			cc.recreate(cluArgs)
			By("Validate objects from previous cluster still exist")
			tutils.ObjectsShouldExist(cc.getBaseParams(), bck, names...)
		})

		It("Should detect port change when cluster is redeployed with different port", func(ctx context.Context) {
			cluArgs.CleanupMetadata = false
			By("Deploy initial cluster with default ports")
			cc := newClientCluster(ctx, AISTestContext, WorkerCtx.K8sClient, cluArgs)
			defer func() {
				_ = cc.printLogs()
				// Ensure final cleanup has CleanupMetadata enabled
				cluArgs.CleanupMetadata = true
				cc.destroyAndCleanup()
			}()
			cc.create()
			initialURL := cc.getProxyURL()

			By("Re-deploy cluster with different port")
			cc.destroyClusterOnly()
			cc.cluster = tutils.NewAISClusterNoPV(cluArgs)
			cc.applyDefaultHostPortOffset(cluArgs)
			cc.applyHostPortOffset(int32(5))
			cc.createCluster(cc.getTimeout(), clusterCreateInterval)
			cc.waitForReadyCluster()

			newURL := cc.getProxyURL()
			Expect(newURL).NotTo(Equal(initialURL))
			cc.initClientAccess()
		})
	})
})
