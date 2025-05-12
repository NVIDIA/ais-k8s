package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createClusters(clusters []*clientCluster, long bool) {
	var wg sync.WaitGroup
	wg.Add(len(clusters))

	for _, cluster := range clusters {
		go func(cc *clientCluster) {
			defer GinkgoRecover()
			defer wg.Done()
			cc.create(long)
		}(cluster)
	}
	wg.Wait()
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

func cleanupOldTestClusters(ctx context.Context, c *aisclient.K8sClient) {
	for _, namespace := range []string{tutils.TestNSName, tutils.TestNSAnotherName} {
		exists, err := c.CheckIfNamespaceExists(ctx, namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to check namespaces %q existence; err %v\n", namespace, err)
			continue
		}
		if !exists {
			continue
		}

		clusters, err := c.ListAIStoreCR(ctx, namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch existing clusters; err %v\n", err)
			continue
		}
		for i := range clusters.Items {
			fmt.Fprintf(os.Stdout, "Destroying old cluster '%s'", clusters.Items[i].Name)
			tutils.DestroyCluster(ctx, c, &clusters.Items[i])
		}
		cleanupPV(c, namespace)
	}
}

// Statically created hostPath volumes have no reclaim policy to clean up the actual files on host, so this creates a
// job to mount the host path and delete any files created by the test suite
func cleanPVHostPath(ctx context.Context) {
	if AISTestContext.StorageHostPath == "" {
		return
	}

	selector := map[string]string{"ais-node": "true"}
	nodes, err := AISTestContext.K8sClient.ListNodesMatchingSelector(ctx, selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list nodes to run cleanup; err %v\n", err)
		return
	}
	jobs := make([]*batchv1.Job, len(nodes.Items))
	for i := range nodes.Items {
		nodeName := nodes.Items[i].Name
		fmt.Fprintf(os.Stdout, "Starting job to clean up host path %s on node %s\n", AISTestContext.StorageHostPath, nodeName)
		jobs[i] = tutils.CreateCleanupJob(nodeName, AISTestContext.StorageHostPath)
		if err = AISTestContext.K8sClient.Create(ctx, jobs[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create cleanup job %s; err %v\n", jobs[i].Name, err)
		}
	}
	jobFinishTimeout := 2 * time.Minute
	jobFinishInterval := 10 * time.Second
	for _, job := range jobs {
		tutils.EventuallyJobNotExists(ctx, AISTestContext.K8sClient, job, jobFinishTimeout, jobFinishInterval)
	}
}

func cleanNamespace(ctx context.Context, timeout time.Duration) {
	if !preexistingNS && testNS != nil {
		_, err := AISTestContext.K8sClient.DeleteResourceIfExists(ctx, testNS)
		Expect(err).NotTo(HaveOccurred())
		// Wait for namespace to be deleted
		Eventually(func() bool {
			exists, checkErr := AISTestContext.K8sClient.CheckIfNamespaceExists(ctx, testNS.Name)
			if checkErr != nil {
				Fail(fmt.Sprintf("Failed to check namespace %s existence; err %v\n", testNS.Name, checkErr))
			}
			return exists
		}, timeout).Should(BeFalse())
	}
}

func getK8sClient() *aisclient.K8sClient {
	cfg := ctrl.GetConfigOrDie()
	Expect(cfg).NotTo(BeNil())

	newClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	return aisclient.NewClient(newClient, scheme.Scheme)
}
