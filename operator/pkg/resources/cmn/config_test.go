// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024-2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Config", Label("short"), func() {
	Describe("Convert", func() {
		It("should convert without an error", func() {
			toUpdate := &aisv1.ConfigToUpdate{
				Space: &aisv1.SpaceConfToUpdate{
					CleanupWM: aisapc.Ptr[int64](10),
					LowWM:     aisapc.Ptr[int64](20),
					HighWM:    aisapc.Ptr[int64](30),
					OOS:       aisapc.Ptr[int64](40),
				},
				LRU: &aisv1.LRUConfToUpdate{
					Enabled:       aisapc.Ptr(true),
					DontEvictTime: (*aisv1.Duration)(aisapc.Ptr[int64](10)),
				},
				Tracing: &aisv1.TracingConfToUpdate{
					Enabled: aisapc.Ptr(true),
					ExporterAuth: &aisv1.TraceExporterAuthConfToUpdate{
						TokenHeader: aisapc.Ptr("token-header"),
						TokenFile:   aisapc.Ptr("token-file"),
					},
				},
				Features: aisapc.Ptr("2568"),
			}

			toSet, err := toUpdate.Convert()
			Expect(err).ToNot(HaveOccurred())
			var clusterCfg aiscmn.ClusterConfig
			err = aiscmn.CopyProps(toSet, &clusterCfg, aisapc.Cluster)
			Expect(err).ToNot(HaveOccurred())

			Expect(clusterCfg.Space.CleanupWM).To(BeEquivalentTo(10))
			Expect(clusterCfg.Space.LowWM).To(BeEquivalentTo(20))
			Expect(clusterCfg.Space.HighWM).To(BeEquivalentTo(30))
			Expect(clusterCfg.Space.OOS).To(BeEquivalentTo(40))

			Expect(clusterCfg.LRU.Enabled).To(BeEquivalentTo(true))
			Expect(clusterCfg.LRU.DontEvictTime).To(BeEquivalentTo(10))

			Expect(clusterCfg.Features).To(BeEquivalentTo(2568))

			Expect(clusterCfg.Tracing.Enabled).To(BeTrue())
			Expect(clusterCfg.Tracing.ExporterAuth.TokenHeader).To(Equal("token-header"))
			Expect(clusterCfg.Tracing.ExporterAuth.TokenFile).To(Equal("token-file"))
		})
	})
	Describe("Generate config override", func() {
		DescribeTable("should auto-configure TLS paths",
			func(spec aisv1.AIStoreSpec) {
				ais := &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
					Spec:       spec,
				}
				conf, err := GenerateConfigToSet(ais)
				Expect(err).ToNot(HaveOccurred())
				Expect(conf.Net).ToNot(BeNil())
				Expect(conf.Net.HTTP).ToNot(BeNil())
				Expect(*conf.Net.HTTP.Certificate).To(Equal("/var/certs/tls.crt"))
				Expect(*conf.Net.HTTP.CertKey).To(Equal("/var/certs/tls.key"))
				Expect(*conf.Net.HTTP.ClientCA).To(Equal("/var/certs/ca.crt"))
			},
			// Deprecated: Use spec.tls.certificate with mode secret instead
			Entry("spec.tlsCertificate", aisv1.AIStoreSpec{
				TLSCertificate: &aisv1.TLSCertificateConfig{
					IssuerRef: aisv1.CertIssuerRef{Name: "test-issuer"},
				},
			}),
			// Deprecated: Use spec.tls.secretName instead
			Entry("spec.tlsSecretName", aisv1.AIStoreSpec{
				TLSSecretName: aisapc.Ptr("my-tls-secret"),
			}),
			// Deprecated: Use spec.tls.certificate with mode csi instead
			Entry("spec.tlsCertManagerIssuerName", aisv1.AIStoreSpec{
				TLSCertManagerIssuerName: aisapc.Ptr("my-issuer"),
			}),
			Entry("spec.tls.secretName", aisv1.AIStoreSpec{
				TLS: &aisv1.TLSSpec{
					SecretName: aisapc.Ptr("my-tls-secret"),
				},
			}),
			Entry("spec.tls.certificate (secret mode)", aisv1.AIStoreSpec{
				TLS: &aisv1.TLSSpec{
					Certificate: &aisv1.TLSCertificateConfig{
						IssuerRef: aisv1.CertIssuerRef{Name: "test-issuer"},
						Mode:      aisv1.TLSCertificateModeSecret,
					},
				},
			}),
			Entry("spec.tls.certificate (csi mode)", aisv1.AIStoreSpec{
				TLS: &aisv1.TLSSpec{
					Certificate: &aisv1.TLSCertificateConfig{
						IssuerRef: aisv1.CertIssuerRef{Name: "test-issuer"},
						Mode:      aisv1.TLSCertificateModeCSI,
					},
				},
			}),
		)

		It("should not set TLS paths when no TLS option is configured", func() {
			ais := &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Spec:       aisv1.AIStoreSpec{},
			}
			conf, err := GenerateConfigToSet(ais)
			Expect(err).ToNot(HaveOccurred())
			Expect(conf.Net).To(BeNil())
		})

		It("should generate initial config without an error", func() {
			const (
				clusterName = "ais-cluster"
				clusterNS   = "ais-ns"
			)
			ais := &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: clusterNS,
				},
				Spec: aisv1.AIStoreSpec{
					ProxySpec: aisv1.DaemonSpec{
						ServiceSpec: aisv1.ServiceSpec{
							PublicPort:       intstr.FromString("51080"),
							IntraControlPort: intstr.FromString("51081"),
							IntraDataPort:    intstr.FromString("51082"),
						},
					},
					AWSSecretName: aisapc.Ptr("any-secret"),
					GCPSecretName: aisapc.Ptr("any-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Backend: &map[string]aisv1.Empty{
							aisapc.OCI: {},
						},
					},
				},
			}
			expected := aiscmn.ConfigToSet{
				Backend: &aiscmn.BackendConf{
					Conf: map[string]interface{}{
						"aws": map[string]any{},
						"gcp": map[string]any{},
						"oci": map[string]any{},
					},
				},
				Rebalance: &aiscmn.RebalanceConfToSet{Enabled: aisapc.Ptr(false)},
				Proxy: &aiscmn.ProxyConfToSet{
					PrimaryURL:   aisapc.Ptr(ais.GetDefaultProxyURL()),
					OriginalURL:  aisapc.Ptr(ais.GetDefaultProxyURL()),
					DiscoveryURL: aisapc.Ptr(ais.GetDiscoveryProxyURL()),
				},
			}
			conf, err := GenerateGlobalConfig(ais)
			Expect(err).ToNot(HaveOccurred())
			Expect(*conf).To(Equal(expected))
		})
	})
})
