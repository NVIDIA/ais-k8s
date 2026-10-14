/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("buildCertificateSANs", func() {
	It("generates sorted unique DNS names and IP addresses", func() {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: aisv1.AIStoreSpec{
				TLS: &aisv1.TLSSpec{
					Certificate: &aisv1.TLSCertificateConfig{
						AdditionalDNSNames: []string{"test-additional-dns-name", "test-additional-dns-name"},
					},
				},
				HostnameMap: map[string]string{
					"test-worker-1": "test-worker-1, 127.0.0.1",
					"test-worker-2": "test-worker-2, 127.0.0.2",
				},
			},
		}
		nodeNames := []string{
			"test-target-node-1", "test-target-node-2",
			"test-proxy-node-1", "test-proxy-node-2", "test-proxy-node-1", "127.0.0.1",
		}

		dnsNames, ipAddresses := buildCertificateSANs(ais, nodeNames)

		Expect(dnsNames).To(Equal([]string{
			"*.test-cluster-proxy.test-ns.svc.cluster.local",
			"*.test-cluster-target.test-ns.svc.cluster.local",
			"test-additional-dns-name",
			"test-cluster-proxy",
			"test-cluster-proxy.test-ns",
			"test-cluster-proxy.test-ns.svc.cluster.local",
			"test-cluster-target",
			"test-cluster-target.test-ns",
			"test-cluster-target.test-ns.svc.cluster.local",
			"test-proxy-node-1",
			"test-proxy-node-2",
			"test-target-node-1",
			"test-target-node-2",
			"test-worker-1",
			"test-worker-2",
		}))
		Expect(ipAddresses).To(Equal([]string{"127.0.0.1", "127.0.0.2"}))
	})
})

var _ = Describe("NewPublicCertificate", func() {
	It("requests only the SANs the spec gives", func() {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			Spec: aisv1.AIStoreSpec{
				HostnameMap: map[string]string{"test-worker-1": "test-worker-1, 127.0.0.1"},
				TargetSpec: aisv1.TargetSpec{DaemonSpec: aisv1.DaemonSpec{
					ExternalAccess: &aisv1.ExternalAccessSpec{DNS: &aisv1.ExternalDNSSpec{Domain: "targets.example.com"}},
				}},
				TLS: &aisv1.TLSSpec{
					SecretName: aisapc.Ptr("my-tls-secret"),
					Public: &aisv1.PublicTLSSpec{
						ServerName: "ais.example.com",
						Certificate: &aisv1.TLSCertificateConfig{
							IssuerRef:          aisv1.CertIssuerRef{Name: "test-issuer", Kind: "Issuer"},
							AdditionalDNSNames: []string{"alt.example.com", "ais.example.com"},
						},
					},
				},
			},
		}

		cert := NewPublicCertificate(ais)

		Expect(*cert.Name).To(Equal("test-cluster-tls-pub-cert"))
		Expect(cert.Spec.DNSNames).To(Equal([]string{"*.targets.example.com", "ais.example.com", "alt.example.com"}))
		Expect(cert.Spec.IPAddresses).To(BeEmpty())
		Expect(*cert.Spec.SecretName).To(Equal("test-cluster-tls-pub"))
		Expect(*cert.Spec.IssuerRef.Name).To(Equal("test-issuer"))
		Expect(*cert.Spec.IssuerRef.Kind).To(Equal("Issuer"))
		Expect(cert.Spec.Usages).To(Equal([]certmanagerv1.KeyUsage{
			certmanagerv1.UsageDigitalSignature, certmanagerv1.UsageKeyEncipherment, certmanagerv1.UsageServerAuth,
		}))
	})

	DescribeTable("returns nil when cert-manager writes no secret", func(public *aisv1.PublicTLSSpec) {
		ais := &aisv1.AIStore{Spec: aisv1.AIStoreSpec{TLS: &aisv1.TLSSpec{Public: public}}}
		Expect(NewPublicCertificate(ais)).To(BeNil())
	},
		Entry("existing secret", &aisv1.PublicTLSSpec{SecretName: aisapc.Ptr("my-pub-tls-secret")}),
		Entry("csi mode", &aisv1.PublicTLSSpec{
			ServerName: "ais.example.com",
			Certificate: &aisv1.TLSCertificateConfig{
				IssuerRef: aisv1.CertIssuerRef{Name: "test-issuer"},
				Mode:      aisv1.TLSCertificateModeCSI,
			},
		}),
	)
})
