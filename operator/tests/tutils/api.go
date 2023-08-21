// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"math/rand"
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
	"k8s.io/apimachinery/pkg/api/errors"
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

func checkClusterExists(ctx context.Context, client *aisclient.K8sClient, name types.NamespacedName) bool {
	_, err := client.GetAIStoreCR(ctx, name)
	if errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

// DestroyCluster - Deletes the AISCluster resource, and waits for the resource to be cleaned up.
// `intervals` refer - `gomega.Eventually`
func DestroyCluster(_ context.Context, client *aisclient.K8sClient,
	cluster *aisv1.AIStore, intervals ...interface{},
) {
	if len(intervals) == 0 {
		intervals = []interface{}{time.Minute, time.Second}
	}

	_, err := client.DeleteResourceIfExists(context.Background(), cluster)
	Expect(err).Should(Succeed())
	Eventually(func() bool {
		return checkClusterExists(context.Background(), client, cluster.NamespacedName())
	}, intervals...).Should(BeFalse())
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

func WaitForClusterToBeReady(ctx context.Context, client *aisclient.K8sClient, cluster *aisv1.AIStore,
	intervals ...interface{},
) {
	Eventually(func() bool {
		proxySS, err := client.GetStatefulSet(ctx, proxy.StatefulSetNSName(cluster))
		replicasReady := cluster.Spec.Size == *proxySS.Spec.Replicas && proxySS.Status.ReadyReplicas == *proxySS.Spec.Replicas
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
	proxyIndex := rand.Intn(int(cluster.Spec.Size))
	proxySSName := proxy.StatefulSetNSName(cluster)
	proxySSName.Name = fmt.Sprintf("%s-%d", proxySSName.Name, proxyIndex)
	pod, err := client.GetPodByName(ctx, proxySSName)
	Expect(err).NotTo(HaveOccurred())
	Expect(pod.Status.PodIP).NotTo(Equal(""))
	return pod.Status.PodIP
}
