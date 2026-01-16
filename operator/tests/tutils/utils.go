package tutils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	aisclient "github.com/ais-operator/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func cleanupPV(ctx context.Context, c *aisclient.K8sClient, namespace string) {
	pvList := &corev1.PersistentVolumeList{}
	err := c.List(ctx, pvList)
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
	DestroyPV(ctx, c, pvToDelete)
}

func CleanupOldTestClusters(ctx context.Context, c *aisclient.K8sClient) {
	allNamespaces := &corev1.NamespaceList{}
	err := c.List(ctx, allNamespaces)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list namespaces; err %v\n", err)
		return
	}
	for i := range allNamespaces.Items {
		// Clean up any resources in namespaces that start with our test namespace base names
		ns := &allNamespaces.Items[i]
		if strings.HasPrefix(ns.Name, TestNSBase) || strings.HasPrefix(ns.Name, TestNSOtherBase) {
			clusters, err := c.ListAIStoreCR(ctx, ns.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to fetch existing clusters in namespace %q; err %v\n", ns.Name, err)
				continue
			}
			for i := range clusters.Items {
				fmt.Fprintf(os.Stdout, "Destroying old cluster '%s' in namespace '%s'\n", clusters.Items[i].Name, ns.Name)
				DestroyCluster(ctx, c, &clusters.Items[i])
			}
			cleanupPV(ctx, c, ns.Name)
		}
	}
}

// Statically created hostPath volumes have no reclaim policy to clean up the actual files on host, so this creates a
// job to mount the host path and delete any files created by the test suite
func CleanPVHostPath(ctx context.Context, k8sClient *aisclient.K8sClient, storageHostPath string) {
	if storageHostPath == "" {
		return
	}
	cleanupNSName := "ais-op-test-cleanup"
	cleanupNS, preexisting := CreateNSIfNotExists(ctx, k8sClient, cleanupNSName)
	if !preexisting {
		defer func() {
			fmt.Fprintf(os.Stdout, "Cleaning up cleanup namespace %s\n", cleanupNSName)
			_, err := k8sClient.DeleteResourceIfExists(ctx, cleanupNS)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete cleanup namespace %s; err %v\n", cleanupNSName, err)
			}
		}()
	}

	selector := map[string]string{"ais-node": "true"}
	nodes, err := k8sClient.ListNodesMatchingSelector(ctx, selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list nodes to run cleanup; err %v\n", err)
		return
	}
	jobs := make([]*batchv1.Job, len(nodes.Items))
	for i := range nodes.Items {
		nodeName := nodes.Items[i].Name
		fmt.Fprintf(os.Stdout, "Starting job to clean up host path %s on node %s\n", storageHostPath, nodeName)
		jobs[i] = CreateCleanupJob(nodeName, storageHostPath, cleanupNSName)
		if err = k8sClient.Create(ctx, jobs[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create cleanup job %s; err %v\n", jobs[i].Name, err)
		}
	}
	jobFinishTimeout := 2 * time.Minute
	jobFinishInterval := 10 * time.Second
	for _, job := range jobs {
		EventuallyJobNotExists(ctx, k8sClient, job, jobFinishTimeout, jobFinishInterval)
	}
}

func checkIfNamespaceExists(ctx context.Context, k8sClient *aisclient.K8sClient, name string) (exists bool, err error) {
	ns := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns)
	if err == nil {
		exists = true
	} else if apierrors.IsNotFound(err) {
		err = nil
	}
	return exists, err
}

func CleanNamespace(ctx context.Context, k8sClient *aisclient.K8sClient, testNS *corev1.Namespace, timeout time.Duration) {
	_, err := k8sClient.DeleteResourceIfExists(ctx, testNS)
	Expect(err).NotTo(HaveOccurred())

	// Wait for namespace to be deleted
	Eventually(func() bool {
		exists, checkErr := checkIfNamespaceExists(ctx, k8sClient, testNS.Name)
		if checkErr != nil {
			Fail(fmt.Sprintf("Failed to check namespace %s existence; err %v\n", testNS.Name, checkErr))
		}
		return exists
	}, timeout).Should(BeFalse())
}

func GetK8sClient() (*aisclient.K8sClient, error) {
	cfg := ctrl.GetConfigOrDie()
	if cfg == nil {
		return nil, fmt.Errorf("failed to get K8s config")
	}

	newClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s client: %w", err)
	}
	return aisclient.NewClient(newClient, scheme.Scheme), nil
}
