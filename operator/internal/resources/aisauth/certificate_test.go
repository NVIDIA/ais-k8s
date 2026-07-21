/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	"context"
	"time"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Certificate", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				TLS: &authv1alpha1.TLSSpec{
					Certificate: &authv1alpha1.TLSCertificateConfig{
						IssuerRef: authv1alpha1.CertIssuerRef{Name: "ca-issuer"},
					},
				},
			},
		}
	})

	It("builds an owned Certificate with defaults and in-cluster SANs", func() {
		certificate := authnres.NewCertificate(context.Background(), authn, nil)

		Expect(authnres.CertificateNSName(authn)).To(Equal(types.NamespacedName{
			Name: "ais-authn-authn-tls-cert", Namespace: "ais",
		}))
		Expect(certificate.OwnerReferences).To(HaveLen(1))
		Expect(certificate.OwnerReferences[0].Name).To(HaveValue(Equal(authn.Name)))
		Expect(certificate.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
		Expect(certificate.Labels).To(Equal(standardLabels()))
		Expect(certificate.Spec.SecretName).To(HaveValue(Equal("ais-authn-authn-tls")))
		Expect(certificate.Spec.Duration).To(HaveValue(Equal(metav1.Duration{Duration: 8760 * time.Hour})))
		Expect(certificate.Spec.RenewBefore).To(HaveValue(Equal(metav1.Duration{Duration: 720 * time.Hour})))
		Expect(certificate.Spec.Usages).To(Equal([]certmanagerv1.KeyUsage{
			certmanagerv1.UsageDigitalSignature,
			certmanagerv1.UsageKeyEncipherment,
			certmanagerv1.UsageServerAuth,
		}))
		Expect(certificate.Spec.IssuerRef.Name).To(HaveValue(Equal("ca-issuer")))
		Expect(certificate.Spec.IssuerRef.Kind).To(HaveValue(Equal("ClusterIssuer")))
		Expect(certificate.Spec.IssuerRef.Group).To(HaveValue(Equal("cert-manager.io")))
		Expect(certificate.Spec.DNSNames).To(Equal([]string{
			"ais-authn",
			"ais-authn.ais",
			"ais-authn.ais.svc",
			"ais-authn.ais.svc.cluster.local",
			"localhost",
		}))
		Expect(certificate.Spec.IPAddresses).To(Equal([]string{"127.0.0.1"}))
	})

	It("applies configured lifetime and resolved external SANs without duplicates", func() {
		duration := metav1.Duration{Duration: 90 * 24 * time.Hour}
		renewBefore := metav1.Duration{Duration: 15 * 24 * time.Hour}
		additionalIP := "192.0.2.10"
		externalURL := "https://oidc.authn.example.com:5443"
		authn.Spec.TLS.Certificate = &authv1alpha1.TLSCertificateConfig{
			IssuerRef: authv1alpha1.CertIssuerRef{Name: "namespace-issuer", Kind: "Issuer"},
			AdditionalDNSNames: []string{
				"authn.example.com",
				"ais-authn.ais.svc.cluster.local",
			},
			AdditionalIPAddresses: []string{additionalIP, additionalIP},
			Duration:              &duration,
			RenewBefore:           &renewBefore,
		}
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{ExternalURL: &externalURL},
		}

		certificate := authnres.NewCertificate(context.Background(), authn, []string{"lb.authn.example.com", "192.0.2.20", "lb.authn.example.com"})

		Expect(certificate.Spec.Duration).To(HaveValue(Equal(duration)))
		Expect(certificate.Spec.RenewBefore).To(HaveValue(Equal(renewBefore)))
		Expect(certificate.Spec.IssuerRef.Kind).To(HaveValue(Equal("Issuer")))
		Expect(certificate.Spec.DNSNames).To(Equal([]string{
			"ais-authn",
			"ais-authn.ais",
			"ais-authn.ais.svc",
			"ais-authn.ais.svc.cluster.local",
			"authn.example.com",
			"lb.authn.example.com",
			"localhost",
			"oidc.authn.example.com",
		}))
		Expect(certificate.Spec.IPAddresses).To(Equal([]string{"127.0.0.1", additionalIP, "192.0.2.20"}))
	})

	It("does not build a Certificate for disabled or existing-Secret TLS", func() {
		authn.Spec.TLS = nil
		Expect(authnres.NewCertificate(context.Background(), authn, nil)).To(BeNil())

		secretName := "existing-tls"
		authn.Spec.TLS = &authv1alpha1.TLSSpec{SecretName: &secretName}
		Expect(authnres.NewCertificate(context.Background(), authn, nil)).To(BeNil())
	})
})
