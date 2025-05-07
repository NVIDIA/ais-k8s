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
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/controllers"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

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

func setStorageClass() {
	storageClass = os.Getenv(EnvTestStorageClass)
	if storageClass == "" && tutils.GetK8sClusterProvider() == tutils.K8sProviderGKE {
		storageClass = tutils.GKEDefaultStorageClass
	} else if storageClass == "" {
		storageClass = "ais-operator-test-storage"
		tutils.CreateAISStorageClass(context.Background(), k8sClient, storageClass)
	}
}

func setStorageHostPath() {
	storageHostPath = os.Getenv(EnvTestStorageHostPath)
	if storageHostPath == "" {
		storageHostPath = "/etc/ais/" + strings.ToLower(cos.CryptoRandS(6))
	}
}

func cleanupPV(c *aisclient.K8sClient, namespace string) {
	pvList := &corev1.PersistentVolumeList{}
	err := c.List(context.Background(), pvList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list PVs; err %v\n", err)
		return
	}
	pvToDelete := make([]*corev1.PersistentVolume, 0, len(pvList.Items))
	// Delete old PVs within the test namespace
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		old := time.Since(pv.CreationTimestamp.Time).Hours() > 1
		if strings.HasPrefix(pv.Name, namespace) && old {
			fmt.Fprintf(os.Stdout, "Deleting old PV '%s' with creation time '%s'\n", pv.Name, pv.CreationTimestamp.Time)
			pvToDelete = append(pvToDelete, pv)
		}
	}
	tutils.DestroyPV(context.Background(), c, pvToDelete)
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
			fmt.Fprintf(os.Stdout, "Destroying old cluster '%s'", clusters.Items[i].Name)
			tutils.DestroyCluster(context.Background(), c, &clusters.Items[i])
		}
		cleanupPV(c, namespace)
	}
}

var _ = SynchronizedBeforeSuite(
	// --- Run only once ---
	func() []byte {
		defer GinkgoRecover()
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
		time.Sleep(5 * time.Second)

		k8sClient = aisclient.NewClientFromMgr(mgr)
		Expect(k8sClient).NotTo(BeNil())

		cleanupOldTestClusters(k8sClient)

		// Create Namespace if not exists
		testNS, nsExists = tutils.CreateNSIfNotExists(context.Background(), k8sClient, testNSName)
		tutils.InitK8sClusterProvider(context.Background(), k8sClient)
		setStorageClass()
		setStorageHostPath()

		return nil
	},
	// --- Run in every worker ---
	func(_ []byte) {
		defer GinkgoRecover()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping per-process test environment")
		Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aisv1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfg := ctrl.GetConfigOrDie()
		Expect(cfg).NotTo(BeNil())

		client, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		k8sClient = aisclient.NewClient(client, scheme.Scheme)

		// Reinitialize cluster provider for the worker
		tutils.InitK8sClusterProvider(context.Background(), k8sClient)
	},
)

// Statically created hostPath volumes have no reclaim policy to clean up the actual files on host, so this creates a
// job to mount the host path and delete any files created by the test suite
func CleanPVHostPath() {
	if storageHostPath == "" {
		return
	}

	selector := map[string]string{"ais-node": "true"}
	nodes, err := k8sClient.ListNodesMatchingSelector(context.Background(), selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list nodes to run cleanup; err %v\n", err)
		return
	}
	jobs := make([]*batchv1.Job, len(nodes.Items))
	for i := range nodes.Items {
		nodeName := nodes.Items[i].Name
		fmt.Fprintf(os.Stdout, "Starting job to clean up host path %s on node %s\n", storageHostPath, nodeName)
		jobs[i] = tutils.CreateCleanupJob(nodeName, testNSName, storageHostPath)
		if err = k8sClient.Create(context.Background(), jobs[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create cleanup job %s; err %v\n", jobs[i].Name, err)
		}
	}
	jobFinishTimeout := 2 * time.Minute
	jobFinishInterval := 10 * time.Second
	for _, job := range jobs {
		tutils.EventuallyJobNotExists(context.Background(), k8sClient, job, jobFinishTimeout, jobFinishInterval)
	}
}

var _ = SynchronizedAfterSuite(
	// --- Run in every worker ---
	func() {},
	// --- Run only once ---
	func() {
		By("tearing down the test environment")
		cleanupOldTestClusters(k8sClient)
		CleanPVHostPath()
		if !nsExists && testNS != nil {
			_, err := k8sClient.DeleteResourceIfExists(context.Background(), testNS)
			Expect(err).NotTo(HaveOccurred())
			// Wait for namespace to be deleted
			Eventually(func() bool {
				exists, err := k8sClient.CheckIfNamespaceExists(context.Background(), testNS.Name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to check namespace %s existence; err %v\n", testNS.Name, err)
					return false
				}
				return exists
			}, AfterSuiteTimeout).Should(BeFalse())
		}
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	},
)
