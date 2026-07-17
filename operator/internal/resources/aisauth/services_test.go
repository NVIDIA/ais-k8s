/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

var _ = Describe("Services", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
		}
	})

	It("uses the CR name for the in-cluster Service", func() {
		Expect(authnres.ServiceNSName(authn)).To(Equal(types.NamespacedName{Name: "ais-authn", Namespace: "ais"}))
	})

	It("builds an owned ClusterIP Service for in-cluster access", func() {
		port := int32(53001)
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{HTTP: &authv1alpha1.HTTPConfSpec{Port: &port}},
		}

		service := authnres.NewService(authn)

		Expect(service.OwnerReferences).To(HaveLen(1))
		Expect(service.OwnerReferences[0].Name).To(HaveValue(Equal(authn.Name)))
		Expect(service.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
		Expect(service.Labels).To(Equal(standardLabels()))
		Expect(service.Spec.Type).To(HaveValue(Equal(corev1.ServiceTypeClusterIP)))
		Expect(service.Spec.ClusterIP).To(BeNil())
		expectServicePort(service.Spec.Ports, port)
		Expect(service.Spec.Selector).To(Equal(selectorLabels()))
	})

	DescribeTable("publishes the canonical in-cluster URL",
		func(tls *authv1alpha1.TLSSpec, expected string) {
			authn.Spec.TLS = tls
			Expect(authnres.ServiceURL(authn)).To(Equal(expected))
		},
		Entry("over HTTP", nil, "http://ais-authn.ais.svc:52001"),
		Entry("over HTTPS", &authv1alpha1.TLSSpec{SecretName: stringPtr("authn-tls")}, "https://ais-authn.ais.svc:52001"),
	)
})

func expectServicePort(ports []corev1ac.ServicePortApplyConfiguration, port int32) {
	GinkgoHelper()
	Expect(ports).To(HaveLen(1))
	Expect(ports[0].Name).To(HaveValue(Equal("http")))
	Expect(ports[0].Protocol).To(HaveValue(Equal(corev1.ProtocolTCP)))
	Expect(ports[0].Port).To(HaveValue(Equal(port)))
	Expect(ports[0].TargetPort).To(HaveValue(Equal(intstr.FromString("http"))))
	Expect(ports[0].NodePort).To(BeNil())
}

func selectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "authn",
		"app.kubernetes.io/instance": "ais-authn",
	}
}

func standardLabels() map[string]string {
	labels := selectorLabels()
	labels["app.kubernetes.io/managed-by"] = "ais-operator"
	return labels
}

func stringPtr(value string) *string {
	return &value
}
