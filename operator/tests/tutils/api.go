// Package tutils provides utilities for running AIS operator tests
/*
* Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
	. "github.com/onsi/gomega"
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
	const pvExistenceInterval = 30 * time.Second
	for _, pv := range pvs {
		deleteAssociatedPVCs(ctx, pv, client)
		existed, err := client.DeleteResourceIfExists(ctx, pv)
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

func deleteAssociatedPVCs(ctx context.Context, pv *corev1.PersistentVolume, client *aisclient.K8sClient) {
	if pv.Spec.ClaimRef == nil {
		return
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting PVC %s: %v", pvc.Name, err)
	}
	fmt.Printf("Deleted PVC %s in namespace %s\n", pvc.Name, pvc.Namespace)
}

func checkCMExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetCMByName(ctx, name)
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
	_, err := client.GetServiceByName(ctx, name)
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
	fmt.Fprintf(os.Stdout, "Creating PV '%s' with claim ref '%s'\n", pvName, claimRefName)

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
		NodeAffinity:                  createVolumeNodeAffinity("kubernetes.io/hostname", "minikube"),
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName},
		Spec:       *pvSpec,
	}
	if _, err := client.CreateResourceIfNotExists(ctx, nil, pv); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new PV: %s", err)
		return pv, err
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
		proxySS, err := client.GetStatefulSet(ctx, proxy.StatefulSetNSName(cluster))
		replicasReady := cluster.GetProxySize() == *proxySS.Spec.Replicas && proxySS.Status.ReadyReplicas == *proxySS.Spec.Replicas
		if err != nil || !replicasReady {
			return false
		}

		// Ensure primary is ready (including rebalance)
		proxyURL := GetProxyURL(ctx, client, cluster)
		smap, err := aisapi.GetClusterMap(aistutils.BaseAPIParams(proxyURL))
		if err != nil {
			return false
		}
		err = aisapi.Health(aistutils.BaseAPIParams(smap.Primary.PubNet.URL), true)
		if err != nil {
			return false
		}

		targetSS, err := client.GetStatefulSet(ctx, target.StatefulSetNSName(cluster))
		if err != nil {
			return false
		}
		return targetSS.Status.ReadyReplicas == *targetSS.Spec.Replicas
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
	svc, err := client.GetServiceByName(ctx, name)
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
	proxyIndex := rand.Intn(int(cluster.GetProxySize()))
	proxySSName := proxy.StatefulSetNSName(cluster)
	proxySSName.Name = fmt.Sprintf("%s-%d", proxySSName.Name, proxyIndex)
	pod, err := client.GetPodByName(ctx, proxySSName)
	Expect(err).NotTo(HaveOccurred())
	Expect(pod.Status.PodIP).NotTo(Equal(""))
	return pod.Status.PodIP
}
