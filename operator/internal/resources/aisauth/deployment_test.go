/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	"crypto/sha256"
	"encoding/hex"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

var _ = Describe("Deployment", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		storageClass := "openebs-hostpath"
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Persistence: authv1alpha1.PersistenceSpec{StorageClass: &storageClass},
				Deployment: authv1alpha1.DeploymentSpec{
					Image:           "docker.io/aistorage/authn:v4.8",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		}
	})

	It("uses the CR name and namespace", func() {
		Expect(authnres.DeploymentName(authn)).To(Equal("ais-authn"))
		Expect(authnres.DeploymentNSName(authn)).To(Equal(types.NamespacedName{
			Name: "ais-authn", Namespace: "ais",
		}))
	})

	It("builds an owned, single-replica Recreate Deployment with stable selectors", func() {
		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())

		Expect(deployment.Labels).To(Equal(map[string]string{
			"app.kubernetes.io/name":       "authn",
			"app.kubernetes.io/instance":   "ais-authn",
			"app.kubernetes.io/managed-by": "ais-operator",
		}))
		Expect(deployment.OwnerReferences).To(HaveLen(1))
		Expect(deployment.OwnerReferences[0].Name).To(HaveValue(Equal(authn.Name)))
		Expect(deployment.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
		Expect(deployment.Spec.Replicas).To(HaveValue(Equal(int32(1))))
		Expect(deployment.Spec.Strategy.Type).To(HaveValue(Equal(appsv1.RecreateDeploymentStrategyType)))
		Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{
			"app.kubernetes.io/name":     "authn",
			"app.kubernetes.io/instance": "ais-authn",
		}))
		Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "ais-authn"))
	})

	It("runs the configured image and pull policy on the AuthN listen port", func() {
		port := int32(53001)
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{HTTP: &authv1alpha1.HTTPConfSpec{Port: &port}},
		}
		container := newContainer(authn)

		Expect(container.Name).To(HaveValue(Equal("authn")))
		Expect(container.Image).To(HaveValue(Equal("docker.io/aistorage/authn:v4.8")))
		Expect(container.ImagePullPolicy).To(HaveValue(Equal(corev1.PullIfNotPresent)))
		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].Name).To(HaveValue(Equal("http")))
		Expect(container.Ports[0].ContainerPort).To(HaveValue(Equal(port)))
		Expect(container.Ports[0].Protocol).To(HaveValue(Equal(corev1.ProtocolTCP)))
	})

	It("checksums the exact rendered config and changes the checksum when config changes", func() {
		configMap, err := authnres.NewConfigMap(authn)
		Expect(err).NotTo(HaveOccurred())
		expected := sha256.Sum256([]byte(configMap.Data[authnres.AuthnJSONKey]))

		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		checksum := deployment.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]
		Expect(checksum).To(Equal(hex.EncodeToString(expected[:])))

		level := int32(4)
		authn.Spec.Config = &authv1alpha1.ConfigSpec{Log: &authv1alpha1.LogSpec{Level: &level}}
		updated, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]).NotTo(Equal(checksum))
	})

})

func newContainer(authn *authv1alpha1.AIStoreAuth) corev1ac.ContainerApplyConfiguration {
	GinkgoHelper()
	deployment, err := authnres.NewDeployment(authn)
	Expect(err).NotTo(HaveOccurred())
	spec := deployment.Spec.Template.Spec
	Expect(spec.Containers).To(HaveLen(1))
	return spec.Containers[0]
}
