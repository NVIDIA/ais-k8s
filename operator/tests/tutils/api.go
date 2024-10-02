// Package tutils provides utilities for running AIS operator tests
/*
* Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	K8sProviderGKE           = "gke"
	K8sProviderMinikube      = "minikube"
	K8sProviderUnknown       = "unknown"
	K8sProviderUninitialized = "uninitialized"

	GKEDefaultStorageClass = "standard"
)

var k8sProvider = K8sProviderUninitialized

type PVData struct {
	storageClass string
	ns           string
	cluster      string
	mpath        string
	node         string
	target       string
	size         resource.Quantity
}

func checkCRExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetAIStoreCR(ctx, name)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

// DestroyCluster - Deletes the AISCluster resource, and waits for the resource to be cleaned up.
// `intervals` refer - `gomega.Eventually`
func DestroyCluster(ctx context.Context, client *aisclient.K8sClient,
	cluster *aisv1.AIStore, intervals ...interface{},
) {
	if len(intervals) == 0 {
		intervals = []interface{}{time.Minute, time.Second}
	}

	_, err := client.DeleteResourceIfExists(ctx, cluster)
	Expect(err).Should(Succeed())
	EventuallyCRNotExists(ctx, client, cluster, intervals...)
}

func EventuallyCRNotExists(ctx context.Context, client *aisclient.K8sClient,
	cluster *aisv1.AIStore, intervals ...interface{},
) {
	Eventually(func() bool {
		return checkCRExists(ctx, client, cluster.NamespacedName())
	}, intervals...).Should(BeFalse())
}

func DestroyPV(ctx context.Context, client *aisclient.K8sClient, pvs []*corev1.PersistentVolume) {
	const pvDeletionGracePeriodSeconds = int64(20)
	const pvExistenceInterval = 40 * time.Second
	for _, pv := range pvs {
		err := deleteAssociatedPVCs(ctx, pv, client)
		Expect(err).Should(Succeed())
		existed, err := client.DeleteResIfExistsWithGracePeriod(ctx, pv, pvDeletionGracePeriodSeconds)
		if existed {
			fmt.Fprintf(os.Stdout, "Deleted PV : %s \n", pv.Name)
		} else {
			fmt.Fprintf(os.Stdout, "Attempted to delete PV '%s', not found", pv.Name)
		}
		Expect(err).Should(Succeed())
	}
	Eventually(func() bool {
		return checkPVsExist(ctx, client, pvs)
	}, pvExistenceInterval).Should(BeFalse())
}

func checkPVsExist(ctx context.Context, c *aisclient.K8sClient, pvs []*corev1.PersistentVolume) bool {
	allPVs, err := GetAllPVs(ctx, c)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	// create map of all PV names
	pvMap := make(map[string]bool)
	for i := range allPVs.Items {
		pvMap[allPVs.Items[i].Name] = true
	}
	// check if any of the pvs provided still exist
	for _, pv := range pvs {
		if _, found := pvMap[pv.Name]; found {
			return true
		}
	}
	return false
}

func deleteAssociatedPVCs(ctx context.Context, pv *corev1.PersistentVolume, client *aisclient.K8sClient) error {
	if pv.Spec.ClaimRef == nil {
		return nil
	}
	// Create a PVC reference from the PV's ClaimRef
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pv.Spec.ClaimRef.Name,
			Namespace: pv.Spec.ClaimRef.Namespace,
			UID:       pv.Spec.ClaimRef.UID,
		},
	}
	_, err := client.DeleteResourceIfExists(ctx, pvc)
	if err == nil {
		fmt.Printf("Deleted PVC %s in namespace %s\n", pvc.Name, pvc.Namespace)
	} else {
		fmt.Fprintf(os.Stderr, "Error deleting PVC %s: %v", pvc.Name, err)
	}
	return err
}

func checkCMExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetConfigMap(ctx, name)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyCMExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName,
	be OmegaMatcher, intervals ...interface{},
) {
	Eventually(func() bool {
		return checkCMExists(ctx, client, name)
	}, intervals...).Should(be)
}

func checkServiceExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetService(ctx, name)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyServiceExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName,
	be OmegaMatcher, intervals ...interface{},
) {
	Eventually(func() bool {
		return checkServiceExists(ctx, client, name)
	}, intervals...).Should(be)
}

func checkSSExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetStatefulSet(ctx, name)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyProxyIsSize(
	ctx context.Context,
	client *aisclient.K8sClient,
	cluster *aisv1.AIStore,
	size int,
	intervals ...interface{},
) {
	Eventually(func() int {
		podList, err := client.ListProxyPods(ctx, cluster)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list proxy pods; err %v\n", err)
		}
		return len(podList.Items)
	}, intervals...).Should(Equal(size))
}

func EventuallyTargetIsSize(
	ctx context.Context,
	client *aisclient.K8sClient,
	cluster *aisv1.AIStore,
	size int,
	intervals ...interface{},
) {
	Eventually(func() int {
		podList, err := client.ListTargetPods(ctx, cluster)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list target pods; err %v\n", err)
		}
		return len(podList.Items)
	}, intervals...).Should(Equal(size))
}

func EventuallySSExists(
	ctx context.Context,
	client *aisclient.K8sClient,
	name types.NamespacedName,
	be OmegaMatcher,
	intervals ...interface{},
) {
	Eventually(func() bool {
		return checkSSExists(ctx, client, name)
	}, intervals...).Should(be)
}

func EventuallyCRBExists(ctx context.Context, client *aisclient.K8sClient, name string,
	be OmegaMatcher, intervals ...interface{},
) {
	Eventually(func() bool {
		return checkCRBExists(ctx, client, name)
	}, intervals...).Should(be)
}

func checkCRBExists(ctx context.Context, client *aisclient.K8sClient, name string) bool {
	// NOTE: Here we skip the Namespace, as querying CRB with Namespace always returns
	// `NotFound` error leading to test failure.
	err := client.Get(ctx, types.NamespacedName{Name: name}, &rbacv1.ClusterRoleBinding{})
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyResourceExists(ctx context.Context, client *aisclient.K8sClient, obj k8sclient.Object,
	be OmegaMatcher, intervals ...interface{},
) {
	Eventually(func() bool {
		return checkResourceExists(ctx, client, obj)
	}, intervals...).Should(be)
}

func checkResourceExists(ctx context.Context, client *aisclient.K8sClient, obj k8sclient.Object) bool {
	objTemp := &unstructured.Unstructured{}
	objTemp.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	err := client.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, obj)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func CreateNSIfNotExists(ctx context.Context, client *aisclient.K8sClient,
	name string,
) (ns *corev1.Namespace, exists bool) {
	ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := client.Create(ctx, ns)
	if err != nil && errors.IsAlreadyExists(err) {
		exists = true
		return
	}
	Expect(err).To(BeNil())
	return
}

func CreateAISStorageClass(ctx context.Context, client *aisclient.K8sClient, scName string) {
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: scName,
		},
		Provisioner:       "kubernetes.io/no-provisioner",
		VolumeBindingMode: new(storagev1.VolumeBindingMode),
	}
	*storageClass.VolumeBindingMode = storagev1.VolumeBindingImmediate

	client.CreateResourceIfNotExists(ctx, nil, storageClass)
}

func CreatePV(ctx context.Context, client *aisclient.K8sClient, pvData *PVData) (*corev1.PersistentVolume, error) {
	trimmedMpath := strings.TrimPrefix(strings.ReplaceAll(pvData.mpath, "/", "-"), "-")
	// Target name must be included because node name doesn't change and this needs to be unique
	pvName := fmt.Sprintf("%s-%s-%s-%s-pv", pvData.ns, pvData.cluster, trimmedMpath, pvData.target)

	claimRefName := fmt.Sprintf("%s-%s-%s-%s", pvData.cluster, trimmedMpath, pvData.cluster, pvData.target)
	fmt.Fprintf(os.Stdout, "Creating PV '%s' with claim ref '%s' on node '%s'\n", pvName, claimRefName, pvData.node)

	pvSpec := &corev1.PersistentVolumeSpec{
		Capacity: corev1.ResourceList{
			corev1.ResourceStorage: pvData.size,
		},
		PersistentVolumeSource: corev1.PersistentVolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: pvData.mpath},
		},
		AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		ClaimRef:                      &corev1.ObjectReference{Namespace: pvData.ns, Name: claimRefName},
		StorageClassName:              pvData.storageClass,
		PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		NodeAffinity:                  createVolumeNodeAffinity("kubernetes.io/hostname", pvData.node),
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName},
		Spec:       *pvSpec,
	}
	exists, err := client.CreateResourceIfNotExists(ctx, nil, pv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new PV: %s", err)
		return pv, err
	}
	if exists {
		fmt.Fprintf(os.Stdout, "PV %s already exists\n", pvName)
	}
	return pv, nil
}

func createVolumeNodeAffinity(key, value string) *corev1.VolumeNodeAffinity {
	return &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{value}},
					},
				},
			},
		},
	}
}

func WaitForClusterToBeReady(ctx context.Context, client *aisclient.K8sClient, cluster *aisv1.AIStore, intervals ...interface{}) {
	Eventually(func() bool {
		ais, err := client.GetAIStoreCR(ctx, cluster.NamespacedName())
		if err != nil {
			return false
		}
		return ais.Status.State == aisv1.ClusterReady && ais.Spec.NodeImage == cluster.Spec.NodeImage
	}, intervals...).Should(BeTrue())
}

func InitK8sClusterProvider(ctx context.Context, client *aisclient.K8sClient) {
	if k8sProvider != K8sProviderUninitialized {
		return
	}

	nodes := &corev1.NodeList{}
	err := client.List(ctx, nodes)
	Expect(err).NotTo(HaveOccurred())
	for i := range nodes.Items {
		if strings.Contains(nodes.Items[i].Name, "gke") {
			k8sProvider = K8sProviderGKE
			return
		}
		if strings.Contains(nodes.Items[i].Name, "minikube") {
			k8sProvider = K8sProviderMinikube
			return
		}
	}
	k8sProvider = K8sProviderUnknown
}

func GetAllPVs(ctx context.Context, c *aisclient.K8sClient) (*corev1.PersistentVolumeList, error) {
	pvList := &corev1.PersistentVolumeList{}
	err := c.List(ctx, pvList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch existing PVs; err %v\n", err)
		return nil, err
	}
	return pvList, nil
}

func GetK8sClusterProvider() string {
	Expect(k8sProvider).ToNot(Equal(K8sProviderUninitialized))
	return k8sProvider
}

func GetLoadBalancerIP(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) (ip string) {
	svc, err := client.GetService(ctx, name)
	Expect(err).NotTo(HaveOccurred())

	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			return ing.IP
		}
	}
	Expect(ip).NotTo(Equal(""))
	return
}

func GetRandomProxyIP(ctx context.Context, client *aisclient.K8sClient, cluster *aisv1.AIStore) string {
	proxyIndex := rand.IntN(int(cluster.GetProxySize()))
	proxySSName := proxy.StatefulSetNSName(cluster)
	proxySSName.Name = fmt.Sprintf("%s-%d", proxySSName.Name, proxyIndex)
	pod, err := client.GetPod(ctx, proxySSName)
	Expect(err).NotTo(HaveOccurred())
	Expect(pod.Status.PodIP).NotTo(Equal(""))
	return pod.Status.PodIP
}

func GetAllProxyIPs(ctx context.Context, client *aisclient.K8sClient, cluster *aisv1.AIStore) []string {
	proxySize := int(cluster.GetProxySize())
	proxyIPs := make([]string, proxySize)
	proxySSName := proxy.StatefulSetNSName(cluster)

	for i := range proxySize {
		podName := types.NamespacedName{Name: fmt.Sprintf("%s-%d", proxySSName.Name, i), Namespace: proxySSName.Namespace}
		pod, err := client.GetPod(ctx, podName)
		Expect(err).NotTo(HaveOccurred())
		Expect(pod.Status.PodIP).NotTo(Equal(""))
		proxyIPs[i] = pod.Status.PodIP
	}

	return proxyIPs
}

func CreateCleanupJob(nodeName, namespace, hostPath string) *batchv1.Job {
	hostVolumeName := "host-volume"
	ttl := int32(0)
	parentDir := filepath.Dir(hostPath)
	pipelineDir := filepath.Base(hostPath)

	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{nodeName},
							},
						},
					},
				},
			},
		},
	}
	hostVolume := corev1.Volume{
		Name: hostVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: parentDir,
			},
		},
	}

	deletionContainer := corev1.Container{
		Name:  "delete-files",
		Image: "busybox",
		Command: []string{
			"sh", "-c", fmt.Sprintf("rm -rf %s", hostPath),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      hostVolumeName,
				MountPath: parentDir,
			},
		},
	}

	jobSpec := batchv1.JobSpec{
		TTLSecondsAfterFinished: &ttl,
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Affinity: affinity,
				Containers: []corev1.Container{
					deletionContainer,
				},
				RestartPolicy: corev1.RestartPolicyNever,
				Volumes: []corev1.Volume{
					hostVolume,
				},
			},
		},
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("test-cleanup-%s-%s", nodeName, pipelineDir),
			Namespace: namespace,
		},
		Spec: jobSpec,
	}
}

func checkJobExists(ctx context.Context, client *aisclient.K8sClient, job *batchv1.Job) (bool, error) {
	jobList, err := client.ListJobsInNamespace(ctx, job.Namespace)
	if err != nil {
		fmt.Printf("Error listing jobs: %v", err)
		return false, err
	}
	for i := range jobList.Items {
		if job.Name == jobList.Items[i].Name {
			return true, nil
		}
	}
	return false, nil
}

func EventuallyJobNotExists(ctx context.Context, client *aisclient.K8sClient,
	job *batchv1.Job, intervals ...interface{},
) {
	Eventually(func() bool {
		exists, err := checkJobExists(ctx, client, job)
		if err != nil {
			fmt.Printf("Error checking job existence: %v", err)
			// Return true to keep checking
			return true
		}
		return exists
	}, intervals...).Should(BeFalse())
}

func ObjectsShouldExist(params aisapi.BaseParams, bck aiscmn.Bck, objectsNames ...string) {
	for _, objName := range objectsNames {
		_, err := aisapi.GetObject(params, bck, objName, nil)
		Expect(err).NotTo(HaveOccurred())
	}
}
