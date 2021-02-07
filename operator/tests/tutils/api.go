// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package tutils

import (
	"context"
	"strings"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aisv1 "github.com/ais-operator/api/v1alpha1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/ais-operator/pkg/resources/target"
)

const (
	K8SProviderGKE      = "gke"
	K8SProviderMinikube = "minikube"
	K8SProviderUnknown  = "unknown"

	GKEDefaultStorageClass = "standard"
)

func checkClusterExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName) bool {
	_, err := client.GetAIStoreCR(ctx, name)
	if err != nil && errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

// DestroyCluster - Deletes the AISCluster resource, and waits for the resource to be cleaned up.
// `intervals` refer - `gomega.Eventually`
func DestroyCluster(ctx context.Context, client *aisclient.K8SClient, cluster *aisv1.AIStore, intervals ...interface{}) {
	Expect(client.DeleteResourceIfExists(context.Background(), cluster)).Should(Succeed())
	Eventually(func() bool {
		return checkClusterExists(context.Background(), client, cluster.NamespacedName())
	}, intervals...).Should(BeFalse())
}

func checkCMExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName) bool {
	_, err := client.GetCMByName(ctx, name)
	if err != nil && errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyCMExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName, be OmegaMatcher, intervals ...interface{}) {
	Eventually(func() bool {
		return checkCMExists(context.Background(), client, name)
	}, intervals...).Should(be)
}

func checkServiceExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName) bool {
	_, err := client.GetServiceByName(ctx, name)
	if err != nil && errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallyServiceExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName, be OmegaMatcher, intervals ...interface{}) {
	Eventually(func() bool {
		return checkServiceExists(context.Background(), client, name)
	}, intervals...).Should(be)
}

func checkSSExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName) bool {
	_, err := client.GetStatefulSet(ctx, name)
	if err != nil && errors.IsNotFound(err) {
		return false
	}
	Expect(err).To(BeNil())
	return true
}

func EventuallySSExists(ctx context.Context, client *aisclient.K8SClient, name types.NamespacedName, be OmegaMatcher, intervals ...interface{}) {
	Eventually(func() bool {
		return checkSSExists(context.Background(), client, name)
	}, intervals...).Should(be)
}

func CreateNSIfNotExists(ctx context.Context, client *aisclient.K8SClient, name string) (ns *corev1.Namespace, exists bool) {
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := client.Create(ctx, ns)
	if err != nil && errors.IsAlreadyExists(err) {
		exists = true
		return
	}
	Expect(err).To(BeNil())
	return
}

func WaitForClusterToBeReady(ctx context.Context, client *aisclient.K8SClient, cluster *aisv1.AIStore, intervals ...interface{}) {
	Eventually(func() bool {
		proxySS, err := client.GetStatefulSet(ctx, proxy.StatefulSetNSName(cluster))
		if err != nil {
			return false
		}
		targetSS, err := client.GetStatefulSet(ctx, target.StatefulSetNSName(cluster))
		if err != nil {
			return false
		}
		return proxySS.Status.ReadyReplicas == *proxySS.Spec.Replicas && targetSS.Status.ReadyReplicas == *targetSS.Spec.Replicas
	}, intervals...).Should(BeTrue())
}

func GetK8SClusterProvider(ctx context.Context, client *aisclient.K8SClient) string {
	nodes := &corev1.NodeList{}
	err := client.List(ctx, nodes)
	Expect(err).NotTo(HaveOccurred())
	for _, node := range nodes.Items {
		if strings.Contains(node.Name, "gke") {
			return K8SProviderGKE
		}
		if strings.Contains(node.Name, "minikube") {
			return K8SProviderMinikube
		}
	}
	return K8SProviderUnknown
}
