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

	It("uses chart-compatible names for each Service", func() {
		Expect(authnres.ServiceNSName(authn)).To(Equal(types.NamespacedName{Name: "ais-authn", Namespace: "ais"}))
		Expect(authnres.NodePortServiceNSName(authn)).To(Equal(types.NamespacedName{Name: "ais-authn-nodeport", Namespace: "ais"}))
		Expect(authnres.LoadBalancerServiceNSName(authn)).To(Equal(types.NamespacedName{Name: "ais-authn-lb", Namespace: "ais"}))
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
		expectServicePort(service.Spec.Ports, port, nil)
		Expect(service.Spec.Selector).To(Equal(selectorLabels()))
	})

	It("builds a NodePort Service with an explicit node port", func() {
		nodePort := int32(31001)
		authn.Spec.ExternalAccess = &authv1alpha1.ExternalAccessSpec{
			NodePort: &authv1alpha1.NodePortSpec{Port: nodePort},
		}

		service := authnres.NewNodePortService(authn)

		Expect(service.Spec.Type).To(HaveValue(Equal(corev1.ServiceTypeNodePort)))
		expectServicePort(service.Spec.Ports, int32(52001), &nodePort)
	})

	It("does not build a NodePort Service when it is disabled", func() {
		Expect(authnres.NewNodePortService(authn)).To(BeNil())
	})

	It("builds a LoadBalancer Service with its public port and annotations", func() {
		annotations := map[string]string{
			"external-dns.alpha.kubernetes.io/hostname": "authn.ais.example.com",
		}
		authn.Spec.ExternalAccess = &authv1alpha1.ExternalAccessSpec{
			LoadBalancer: &authv1alpha1.LoadBalancerSpec{
				Port:        5443,
				Annotations: annotations,
			},
		}

		service := authnres.NewLoadBalancerService(authn)

		Expect(service.Spec.Type).To(HaveValue(Equal(corev1.ServiceTypeLoadBalancer)))
		Expect(service.Spec.ClusterIP).To(BeNil())
		Expect(service.Annotations).To(Equal(annotations))
		expectServicePort(service.Spec.Ports, int32(5443), nil)
	})

	It("does not build a LoadBalancer Service when it is disabled", func() {
		Expect(authnres.NewLoadBalancerService(authn)).To(BeNil())
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

func expectServicePort(ports []corev1ac.ServicePortApplyConfiguration, port int32, nodePort *int32) {
	GinkgoHelper()
	Expect(ports).To(HaveLen(1))
	Expect(ports[0].Name).To(HaveValue(Equal("http")))
	Expect(ports[0].Protocol).To(HaveValue(Equal(corev1.ProtocolTCP)))
	Expect(ports[0].Port).To(HaveValue(Equal(port)))
	Expect(ports[0].TargetPort).To(HaveValue(Equal(intstr.FromString("http"))))
	if nodePort == nil {
		Expect(ports[0].NodePort).To(BeNil())
	} else {
		Expect(ports[0].NodePort).To(HaveValue(Equal(*nodePort)))
	}
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
