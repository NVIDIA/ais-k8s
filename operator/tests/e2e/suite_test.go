// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/controllers"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testCtx *testing.T
	testEnv *envtest.Environment

	testNS *corev1.Namespace
	// Do not remove test namespace if it already existed
	preexistingNS  bool
	AISTestContext *tutils.AISTestContext
)

const AfterSuiteTimeout = 60 * time.Second

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
	// --- Run only once ---
	func() []byte {
		defer GinkgoRecover()
		ctx := context.Background()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping test environment")
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aisv1.AddToScheme(scheme.Scheme)).To(Succeed())

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(controllers.NewAISReconcilerFromMgr(
			mgr,
			ctrl.Log.WithName("controllers").WithName("AIStore"),
		).SetupWithManager(mgr)).To(Succeed())

		go func() {
			err = mgr.Start(ctrl.SetupSignalHandler())
			Expect(err).ToNot(HaveOccurred())
		}()

		// Give some time for client cache to start before creating instances.
		mgr.GetCache().WaitForCacheSync(ctx)

		k8sClient := aisclient.NewClientFromMgr(mgr)
		Expect(k8sClient).NotTo(BeNil())

		cleanupOldTestClusters(ctx, k8sClient)

		// Create Namespace if not exists
		testNS, preexistingNS = tutils.CreateNSIfNotExists(ctx, k8sClient, tutils.TestNSName)
		AISTestContext, err = tutils.NewAISTestContext(ctx, k8sClient)
		return nil
	},
	// --- Run in every worker ---
	func(_ []byte) {
		defer GinkgoRecover()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping per-process test environment")
		Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aisv1.AddToScheme(scheme.Scheme)).To(Succeed())

		// Update client in context for each worker
		var err error
		AISTestContext, err = tutils.NewAISTestContext(context.Background(), getK8sClient())
		Expect(err).To(Not(HaveOccurred()))
	},
)

var _ = SynchronizedAfterSuite(
	// --- Run in every worker ---
	func() {},
	// --- Run only once ---
	func() {
		ctx := context.Background()
		By("tearing down the test environment")
		cleanupOldTestClusters(ctx, AISTestContext.K8sClient)
		cleanPVHostPath(ctx)
		cleanNamespace(ctx, AfterSuiteTimeout)

		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	},
)
