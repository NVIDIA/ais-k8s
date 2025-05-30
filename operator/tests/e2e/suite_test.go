// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/controllers"
	"github.com/ais-operator/pkg/services"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type WorkerContext struct {
	K8sClient       *aisclient.K8sClient // Worker namespace-scoped client (from manager)
	TestNSName      string
	TestNSOtherName string
	TestNS          *corev1.Namespace
	PreexistingNS   bool
}

var (
	testCtx        *testing.T
	testEnv        *envtest.Environment
	AISTestContext *tutils.AISTestContext // Global test context shared across all workers
	K8sClient      *aisclient.K8sClient   // Non-namespace-scoped client for global cleanup on process #1
	WorkerCtx      *WorkerContext         // Worker-specific context
)

const AfterSuiteTimeout = 90 * time.Second

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	testCtx = t
	if testing.Short() {
		fmt.Fprintf(os.Stdout, "Running tests in short mode")
	}
	RunSpecs(t, "Controller Suite")
}

var _ = SynchronizedBeforeSuite(
	// --- Run only once (process #1) ---
	func() []byte {
		defer GinkgoRecover()
		ctx := context.Background()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping test environment")
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "base", "crd")},
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aisv1.AddToScheme(scheme.Scheme)).To(Succeed())

		// Cache K8s client and AISTestContext for "run only once" cleanup on process #1
		K8sClient, err = tutils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())
		AISTestContext, err = tutils.NewAISTestContext(ctx, K8sClient)
		Expect(err).NotTo(HaveOccurred())

		if AISTestContext.Ephemeral {
			tutils.CleanupOldTestClusters(ctx, K8sClient)
		}

		// Serialize AISTestContext to pass to all workers
		data, err := json.Marshal(AISTestContext)
		Expect(err).NotTo(HaveOccurred())
		return data
	},
	// --- Run in every worker ---
	func(data []byte) {
		defer GinkgoRecover()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping per-process test environment")
		Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aisv1.AddToScheme(scheme.Scheme)).To(Succeed())

		// Deserialize AISTestContext from process #1
		err := json.Unmarshal(data, &AISTestContext)
		Expect(err).NotTo(HaveOccurred())

		cfg := ctrl.GetConfigOrDie()

		// Worker-specific namespace names for manager scoping
		workerTestNS := fmt.Sprintf("%s-%d", tutils.TestNSBase, GinkgoParallelProcess())
		workerTestNSOther := fmt.Sprintf("%s-%d", tutils.TestNSOtherBase, GinkgoParallelProcess())

		// Each worker creates its own manager with namespace-scoped cache
		// This prevents all reconcilers from watching all clusters
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					workerTestNS:      {},
					workerTestNSOther: {},
				},
			},
			Metrics: metricsserver.Options{
				BindAddress: "0", // Disable metrics endpoint
			},
			HealthProbeBindAddress: "0", // Disable health probe endpoint
			WebhookServer: webhook.NewServer(webhook.Options{
				Port: 0, // Disable webhook server
			}),
		})
		Expect(err).NotTo(HaveOccurred())

		tlsOpts := services.AISClientTLSOpts{
			CertPath:       "my/cert/path",
			CertPerCluster: false,
		}

		Expect(controllers.NewAISReconcilerFromMgr(
			mgr,
			tlsOpts,
			ctrl.Log.WithName("controllers").WithName("AIStore"),
		).SetupWithManager(mgr)).To(Succeed())

		go func() {
			err = mgr.Start(ctrl.SetupSignalHandler())
			Expect(err).ToNot(HaveOccurred())
		}()

		// Give some time for client cache to start
		ctx := context.Background()
		mgr.GetCache().WaitForCacheSync(ctx)

		workerK8sClient := aisclient.NewClientFromMgr(mgr)
		testNS, preexistingNS := tutils.CreateNSIfNotExists(ctx, workerK8sClient, workerTestNS)

		WorkerCtx = &WorkerContext{
			K8sClient:       workerK8sClient, // Namespace-scoped client
			TestNSName:      workerTestNS,
			TestNSOtherName: workerTestNSOther,
			TestNS:          testNS,
			PreexistingNS:   preexistingNS,
		}
	},
)

var _ = SynchronizedAfterSuite(
	// --- Run in every worker ---
	func() {
		if !AISTestContext.Ephemeral {
			By("Tearing down worker-specific test namespace")
			ctx := context.Background()
			if !WorkerCtx.PreexistingNS && WorkerCtx.TestNS != nil {
				tutils.CleanNamespace(ctx, WorkerCtx.K8sClient, WorkerCtx.TestNS, AfterSuiteTimeout)
			}
		}
	},
	// --- Run only once ---
	func() {
		if !AISTestContext.Ephemeral {
			By("Tearing down the test environment")
			ctx := context.Background()
			tutils.CleanupOldTestClusters(ctx, K8sClient)
			tutils.CleanPVHostPath(ctx, K8sClient, AISTestContext.StorageHostPath)
			err := testEnv.Stop()
			Expect(err).NotTo(HaveOccurred())
		}
	},
)
