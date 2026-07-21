/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package certificates

import (
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewSpec(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		g := NewWithT(t)
		spec := NewSpec(&SpecConfig{
			SecretName: "test" + "-tls",
			IssuerName: "test-issuer",
			Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
		}, []string{"auth.example.com"}, []string{"192.0.2.1"})

		g.Expect(spec.SecretName).To(HaveValue(Equal("test-tls")))
		g.Expect(spec.Duration).To(HaveValue(Equal(metav1.Duration{Duration: 8760 * time.Hour})))
		g.Expect(spec.RenewBefore).To(HaveValue(Equal(metav1.Duration{Duration: 720 * time.Hour})))
		g.Expect(spec.IssuerRef.Name).To(HaveValue(Equal("test-issuer")))
		g.Expect(spec.IssuerRef.Kind).To(HaveValue(Equal("ClusterIssuer")))
		g.Expect(spec.IssuerRef.Group).To(HaveValue(Equal("cert-manager.io")))
		g.Expect(spec.Usages).To(Equal([]certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth}))
		g.Expect(spec.DNSNames).To(Equal([]string{"auth.example.com"}))
		g.Expect(spec.IPAddresses).To(Equal([]string{"192.0.2.1"}))
	})

	t.Run("overrides", func(t *testing.T) {
		g := NewWithT(t)
		duration := metav1.Duration{Duration: 90 * 24 * time.Hour}
		renewBefore := metav1.Duration{Duration: 15 * 24 * time.Hour}
		spec := NewSpec(&SpecConfig{
			SecretName:  "test" + "-tls",
			IssuerName:  "test-issuer",
			IssuerKind:  "Issuer",
			Duration:    &duration,
			RenewBefore: &renewBefore,
		}, nil, nil)

		g.Expect(spec.Duration).To(HaveValue(Equal(duration)))
		g.Expect(spec.RenewBefore).To(HaveValue(Equal(renewBefore)))
		g.Expect(spec.IssuerRef.Kind).To(HaveValue(Equal("Issuer")))
	})
}

func TestSANHelpers(t *testing.T) {
	g := NewWithT(t)
	dnsNames, ipAddresses := AppendHosts(
		[]string{"z.example.com", "a.example.com"},
		[]string{"192.0.2.2"},
		"a.example.com", "192.0.2.1", "",
	)
	dnsNames, ipAddresses = NormalizeSANs(dnsNames, ipAddresses)

	g.Expect(dnsNames).To(Equal([]string{"a.example.com", "z.example.com"}))
	g.Expect(ipAddresses).To(Equal([]string{"192.0.2.1", "192.0.2.2"}))
}

func TestLoadBalancerEndpoints(t *testing.T) {
	g := NewWithT(t)
	services := []corev1.Service{
		{Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{
			{IP: "192.0.2.1"},
			{Hostname: "lb.example.com"},
			{},
		}}}},
		{Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{
			{IP: "192.0.2.2", Hostname: "dual.example.com"},
		}}}},
	}

	g.Expect(LoadBalancerEndpoints(services...)).To(Equal([]string{
		"192.0.2.1",
		"lb.example.com",
		"192.0.2.2",
		"dual.example.com",
	}))
}
