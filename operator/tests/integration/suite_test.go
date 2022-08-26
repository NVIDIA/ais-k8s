// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NVIDIA/aistore/cmn/cos"
	aisk8s "github.com/NVIDIA/aistore/cmn/k8s"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/controllers"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient *aisclient.K8sClient
	testEnv   *envtest.Environment
	testCtx   *testing.T

	storageClass           string // storage-class to use in tests
	testNS                 *corev1.Namespace
	nsExists               bool
	testAsExternalClient   bool
	testAllowSharedNoDisks bool
)

const (
	testNSName        = "ais-test-namespace"
	testNSAnotherName = "ais-test-namespace-other"

	EnvTestEnforceExternal = "TEST_EXTERNAL_CLIENT" // if set, will force the test suite to run as external client to deployed k8s cluster.
	EnvTestStorageClass    = "TEST_STORAGECLASS"
	EnvTestNoFsChecks      = "TEST_ALLOW_SHARED_NO_DISKS" // if set, deploys cluster with Fs ID/Disk check disabled
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	testCtx = t
	if testing.Short() {
		fmt.Fprintf(os.Stdout, "Running tests in short mode")
	}
	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func setStorageClass() {
	storageClass = os.Getenv(EnvTestStorageClass)
	if storageClass == "" && tutils.GetK8sClusterProvider() == tutils.K8sProviderGKE {
		storageClass = tutils.GKEDefaultStorageClass
	}
}

func cleanupOldTestClusters(c *aisclient.K8sClient) {
	for _, namespace := range []string{testNSName, testNSAnotherName} {
		exists, err := c.CheckIfNamespaceExists(context.Background(), namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to check namespaces %q existence; err %v\n", namespace, err)
			continue
		}
		if !exists {
			continue
		}

		clusters, err := c.ListAIStoreCR(context.Background(), namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch existing clusters; err %v\n", err)
			continue
		}
		for i := range clusters.Items {
			tutils.DestroyCluster(context.Background(), c, &clusters.Items[i])
		}
	}
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = aisv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})

	k8sClient = aisclient.NewClientFromMgr(mgr)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	err = controllers.NewAISReconciler(
		mgr,
		ctrl.Log.WithName("controllers").WithName("AIStore"),
		testAsExternalClient,
	).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Give some time for client cache to start before creating instances.
	time.Sleep(5 * time.Second)

	By("Cleaning orphaned test clusters")
	cleanupOldTestClusters(k8sClient)

	tutils.InitK8sClusterProvider(context.Background(), k8sClient)
	// Create Namespace if not exists
	testNS, nsExists = tutils.CreateNSIfNotExists(context.Background(), k8sClient, testNSName)

	// NOTE: On gitlab, tests run in a pod inside minikube cluster. In that case we can run the tests as an internal client, unless enforced to test as external client.
	testAsExternalClient = cos.IsParseBool(os.Getenv(EnvTestEnforceExternal)) || aisk8s.Detect() != nil
	setStorageClass()

	testAllowSharedNoDisks = cos.IsParseBool(os.Getenv(EnvTestNoFsChecks))

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if !nsExists && testNS != nil {
		_, err := k8sClient.DeleteResourceIfExists(context.Background(), testNS)
		Expect(err).NotTo(HaveOccurred())
	}
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
