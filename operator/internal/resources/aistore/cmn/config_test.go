/*
 * Copyright (c) 2024-2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"strings"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	jsoniter "github.com/json-iterator/go"
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

		It("should convert the config options added in AIS v5.0", func() {
			toUpdate := &aisv1.ConfigToUpdate{
				Lso: &aisv1.LsoConfToUpdate{
					WalkBuffer:  aisapc.Ptr(2048),
					IdleTime:    (*aisv1.Duration)(aisapc.Ptr[int64](10)),
					QuiesceTime: (*aisv1.Duration)(aisapc.Ptr[int64](20)),
				},
				Net: &aisv1.NetConfToUpdate{
					UseIPv6: aisapc.Ptr(true),
					HTTP: &aisv1.HTTPConfToUpdate{
						BackendIdleConnTimeout: (*aisv1.Duration)(aisapc.Ptr[int64](30)),
						Pub: &aisv1.TLSConfToUpdate{
							Certificate: aisapc.Ptr("/var/certs/pub.crt"),
							CertKey:     aisapc.Ptr("/var/certs/pub.key"),
						},
					},
				},
				Auth: &aisv1.AuthConfToUpdate{
					ClientAuthRequired: aisapc.Ptr(true),
					OIDC: &aisv1.OIDCConfToUpdate{
						JWKSCacheConf: &aisv1.JWKSCacheConfToUpdate{
							MinRotationRefresh:   (*aisv1.Duration)(aisapc.Ptr[int64](40)),
							MinBackgroundRefresh: (*aisv1.Duration)(aisapc.Ptr[int64](50)),
						},
					},
					IntraCluster: &aisv1.IntraClusterConfToUpdate{
						NodeJoinSecretPath: aisapc.Ptr("/var/secrets/node-join"),
						RequestAuth:        aisapc.Ptr(true),
					},
				},
				Transport: &aisv1.TransportConfToUpdate{
					LZ4BlockMaxSize: (*aisv1.SizeIEC)(aisapc.Ptr[int64](262144)),
				},
				Space:  &aisv1.SpaceConfToUpdate{BatchSize: aisapc.Ptr[int64](1024)},
				Chunks: &aisv1.ChunksConfToUpdate{CheckpointEvery: aisapc.Ptr(8), Flags: aisapc.Ptr[uint64](1)},
			}

			toSet, err := toUpdate.Convert()
			Expect(err).ToNot(HaveOccurred())

			Expect(*toSet.Lso.WalkBuffer).To(BeEquivalentTo(2048))
			Expect(*toSet.Lso.IdleTime).To(BeEquivalentTo(10))
			Expect(*toSet.Lso.QuiesceTime).To(BeEquivalentTo(20))
			Expect(*toSet.Net.UseIPv6).To(BeTrue())
			Expect(*toSet.Net.HTTP.BackendIdleConnTimeout).To(BeEquivalentTo(30))
			Expect(*toSet.Net.HTTP.Pub.Certificate).To(Equal("/var/certs/pub.crt"))
			Expect(*toSet.Net.HTTP.Pub.CertKey).To(Equal("/var/certs/pub.key"))
			Expect(*toSet.Auth.ClientAuthRequired).To(BeTrue())
			Expect(*toSet.Auth.OIDC.JWKSCacheConf.MinRotationRefresh).To(BeEquivalentTo(40))
			Expect(*toSet.Auth.OIDC.JWKSCacheConf.MinBackgroundRefresh).To(BeEquivalentTo(50))
			Expect(*toSet.Auth.IntraCluster.NodeJoinSecretPath).To(Equal("/var/secrets/node-join"))
			Expect(*toSet.Auth.IntraCluster.RequestAuth).To(BeTrue())
			Expect(*toSet.Transport.LZ4BlockMaxSize).To(BeEquivalentTo(262144))
			Expect(*toSet.Space.BatchSize).To(BeEquivalentTo(1024))
			Expect(*toSet.Chunks.CheckpointEvery).To(BeEquivalentTo(8))
			Expect(*toSet.Chunks.Flags).To(BeEquivalentTo(1))
		})

		It("should map the deprecated auth options onto their replacements", func() {
			toUpdate := &aisv1.ConfigToUpdate{
				Auth: &aisv1.AuthConfToUpdate{
					Enabled: aisapc.Ptr(true),
					ClusterKey: &aisv1.ClusterKeyConfToUpdate{ //nolint:staticcheck // exercising the deprecated option
						Enabled: aisapc.Ptr(true),
						TTL:     (*aisv1.Duration)(aisapc.Ptr[int64](60)),
					},
				},
			}

			toSet, err := toUpdate.Convert()
			Expect(err).ToNot(HaveOccurred())
			Expect(*toSet.Auth.ClientAuthRequired).To(BeTrue())
			Expect(*toSet.Auth.IntraCluster.RequestAuth).To(BeTrue())
			Expect(*toSet.Auth.IntraCluster.TTL).To(BeEquivalentTo(60))
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

		DescribeTable("should build the auth config",
			func(spec aisv1.AIStoreSpec, want *aiscmn.AuthConfToSet) {
				ais := &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
					Spec:       spec,
				}
				conf, err := GenerateConfigToSet(ais)
				Expect(err).ToNot(HaveOccurred())
				Expect(conf.Auth).To(Equal(want))
			},
			Entry("no auth options, including the issuer CA",
				aisv1.AIStoreSpec{IssuerCAConfigMap: aisapc.Ptr("issuer-ca")},
				nil,
			),
			Entry("auth enabled with an issuer CA",
				aisv1.AIStoreSpec{
					IssuerCAConfigMap: aisapc.Ptr("issuer-ca"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)},
					},
				},
				&aiscmn.AuthConfToSet{
					ClientAuthRequired: aisapc.Ptr(true),
					OIDC:               &aiscmn.OIDCConfToSet{IssuerCA: aisapc.Ptr("/etc/ais/oidc-ca/ca.crt")},
				},
			),
			Entry("hmac secret alone",
				aisv1.AIStoreSpec{AuthNSecretName: aisapc.Ptr("hmac-secret")},
				&aiscmn.AuthConfToSet{
					Signature: &aiscmn.AuthSignatureConfToSet{Method: aisapc.Ptr(aisv1.SigningKeyMethodHMAC)},
				},
			),
			Entry("hmac secret with auth enabled",
				aisv1.AIStoreSpec{
					AuthNSecretName: aisapc.Ptr("hmac-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)},
					},
				},
				&aiscmn.AuthConfToSet{
					ClientAuthRequired: aisapc.Ptr(true),
					Signature:          &aiscmn.AuthSignatureConfToSet{Method: aisapc.Ptr(aisv1.SigningKeyMethodHMAC)},
				},
			),
			Entry("hmac secret with auth disabled",
				aisv1.AIStoreSpec{
					AuthNSecretName: aisapc.Ptr("hmac-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(false)},
					},
				},
				&aiscmn.AuthConfToSet{
					ClientAuthRequired: aisapc.Ptr(false),
					Signature:          &aiscmn.AuthSignatureConfToSet{Method: aisapc.Ptr(aisv1.SigningKeyMethodHMAC)},
				},
			),
			// The signing key is expected to reach AIS through the authN secret, never through the spec,
			// but AIS rejects a key with no method so the operator must still fill the method in.
			Entry("hmac secret with a signature key in the config",
				aisv1.AIStoreSpec{
					AuthNSecretName: aisapc.Ptr("hmac-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{
							Signature: &aisv1.AuthSignatureConfToUpdate{Key: aisapc.Ptr("unexpected-key")},
						},
					},
				},
				&aiscmn.AuthConfToSet{
					Signature: &aiscmn.AuthSignatureConfToSet{
						Key:    aisapc.Ptr("unexpected-key"),
						Method: aisapc.Ptr(aisv1.SigningKeyMethodHMAC),
					},
				},
			),
			Entry("hmac secret with an explicit signature method",
				aisv1.AIStoreSpec{
					AuthNSecretName: aisapc.Ptr("hmac-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{
							Signature: &aisv1.AuthSignatureConfToUpdate{Method: aisapc.Ptr("RSA")},
						},
					},
				},
				&aiscmn.AuthConfToSet{
					Signature: &aiscmn.AuthSignatureConfToSet{Method: aisapc.Ptr("RSA")},
				},
			),
			Entry("node join secret alone",
				aisv1.AIStoreSpec{NodeJoin: &aisv1.NodeJoinSpec{SecretName: aisapc.Ptr("node-join-creds")}},
				&aiscmn.AuthConfToSet{
					IntraCluster: &aiscmn.IntraClusterConfToSet{
						NodeJoinSecretPath: aisapc.Ptr("/var/node_join_secret/node_join_secret"),
					},
				},
			),
			Entry("hmac secret with OIDC issuers",
				aisv1.AIStoreSpec{
					AuthNSecretName: aisapc.Ptr("hmac-secret"),
					ConfigToUpdate: &aisv1.ConfigToUpdate{
						Auth: &aisv1.AuthConfToUpdate{
							OIDC: &aisv1.OIDCConfToUpdate{
								AllowedIssuers: aisapc.Ptr([]string{"https://issuer.example.com"}),
							},
						},
					},
				},
				&aiscmn.AuthConfToSet{
					OIDC: &aiscmn.OIDCConfToSet{AllowedIssuers: aisapc.Ptr([]string{"https://issuer.example.com"})},
				},
			),
		)

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
							PublicPort:       aisapc.Ptr(intstr.FromString("51080")),
							IntraControlPort: aisapc.Ptr(intstr.FromString("51081")),
							IntraDataPort:    aisapc.Ptr(intstr.FromString("51082")),
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
					PrimaryURL:   aisapc.Ptr(DefaultProxyURL(ais)),
					OriginalURL:  aisapc.Ptr(DefaultProxyURL(ais)),
					DiscoveryURL: aisapc.Ptr(DiscoveryProxyURL(ais)),
				},
			}
			conf, err := GenerateGlobalConfig(ais)
			Expect(err).ToNot(HaveOccurred())
			Expect(*conf).To(Equal(expected))
		})
	})
	Describe("Marshal global config", func() {
		marshal := func(conf *aisv1.ConfigToUpdate) string {
			ais := &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Spec:       aisv1.AIStoreSpec{ConfigToUpdate: conf},
			}
			toSet, err := GenerateConfigToSet(ais)
			Expect(err).ToNot(HaveOccurred())
			data, err := MarshalGlobalConfig(ais, toSet)
			Expect(err).ToNot(HaveOccurred())
			return string(data)
		}

		marshalRoot := func(conf *aisv1.ConfigToUpdate) map[string]jsoniter.RawMessage {
			var root map[string]jsoniter.RawMessage
			Expect(jsoniter.Unmarshal([]byte(marshal(conf)), &root)).To(Succeed())
			return root
		}

		// Isolates the auth section so assertions cannot collide with same-named options elsewhere.
		marshalAuth := func(conf *aisv1.ConfigToUpdate) string {
			root := marshalRoot(conf)
			Expect(root).To(HaveKey("auth"))
			return string(root["auth"])
		}

		DescribeTable("should write the auth option named in spec",
			func(conf *aisv1.ConfigToUpdate, expected string) {
				Expect(marshalAuth(conf)).To(Equal(expected))
			},
			Entry("deprecated auth.enabled",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)}},
				`{"enabled":true}`,
			),
			Entry("deprecated auth.enabled set to false",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(false)}},
				`{"enabled":false}`,
			),
			Entry("auth.client_auth_required",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{ClientAuthRequired: aisapc.Ptr(true)}},
				`{"client_auth_required":true}`,
			),
			Entry("deprecated auth.cluster_key",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
					ClusterKey: &aisv1.ClusterKeyConfToUpdate{ //nolint:staticcheck // exercising the deprecated option
						Enabled:       aisapc.Ptr(true),
						TTL:           (*aisv1.Duration)(aisapc.Ptr(int64(time.Hour))),
						NonceWindow:   (*aisv1.Duration)(aisapc.Ptr(int64(time.Minute))),
						RotationGrace: (*aisv1.Duration)(aisapc.Ptr(int64(time.Second))),
					},
				}},
				`{"cluster_key":{"enabled":true,"ttl":"1h0m","nonce_window":"1m","rotation_grace":"1s"}}`,
			),
			Entry("auth.intra_cluster",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
					IntraCluster: &aisv1.IntraClusterConfToUpdate{
						RequestAuth:        aisapc.Ptr(true),
						NodeJoinSecretPath: aisapc.Ptr("/var/secrets/node-join"),
					},
				}},
				`{"intra_cluster":{"node_join_secret_path":"/var/secrets/node-join","request_auth":true}}`,
			),
			Entry("both auth options, each in its own form",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
					Enabled:      aisapc.Ptr(true),
					IntraCluster: &aisv1.IntraClusterConfToUpdate{RequestAuth: aisapc.Ptr(true)},
				}},
				`{"enabled":true,"intra_cluster":{"request_auth":true}}`,
			),
			Entry("no renameable option",
				&aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
					Signature: &aisv1.AuthSignatureConfToUpdate{Method: aisapc.Ptr("hmac")},
				}},
				`{"signature":{"method":"hmac"}}`,
			),
		)

		It("should not let spec.auth influence the emitted config", func() {
			ais := &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Spec: aisv1.AIStoreSpec{
					Auth: &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: "provider"}},
				},
			}
			toSet, err := GenerateConfigToSet(ais)
			Expect(err).ToNot(HaveOccurred())
			Expect(toSet.Auth).To(BeNil())

			data, err := MarshalGlobalConfig(ais, toSet)
			Expect(err).ToNot(HaveOccurred())
			var root map[string]jsoniter.RawMessage
			Expect(jsoniter.Unmarshal(data, &root)).To(Succeed())
			Expect(root).ToNot(HaveKey("auth"))
		})

		It("should emit a single auth section when the rewritten one shadows the original", func() {
			out := marshal(&aisv1.ConfigToUpdate{
				Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)},
			})
			Expect(strings.Count(out, `"auth":`)).To(Equal(1))
		})

		It("should leave other sections untouched", func() {
			out := marshal(&aisv1.ConfigToUpdate{
				Auth:      &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)},
				Rebalance: &aisv1.RebalanceConfToUpdate{Enabled: aisapc.Ptr(true)},
				Timeout: &aisv1.TimeoutConfToUpdate{
					MaxKeepalive: (*aisv1.Duration)(aisapc.Ptr[int64](4000000000)),
				},
			})
			Expect(out).To(ContainSubstring(`"max_keepalive":"4s"`))
			Expect(out).To(ContainSubstring(`"rebalance":{"enabled":false}`))
		})

		It("should not emit an auth section when spec sets no auth options", func() {
			root := marshalRoot(&aisv1.ConfigToUpdate{
				Rebalance: &aisv1.RebalanceConfToUpdate{Enabled: aisapc.Ptr(true)},
			})
			Expect(root).ToNot(HaveKey("auth"))
		})

		It("should render byte-identical output for the same spec", func() {
			conf := &aisv1.ConfigToUpdate{
				Backend: &map[string]aisv1.Empty{
					aisapc.AWS: {}, aisapc.GCP: {}, aisapc.OCI: {},
				},
				Auth: &aisv1.AuthConfToUpdate{Enabled: aisapc.Ptr(true)},
			}
			first := marshal(conf)
			for range 20 {
				Expect(marshal(conf)).To(Equal(first))
			}
		})

		DescribeTable("should write auth options AIS still accepts",
			func(conf *aisv1.ConfigToUpdate) {
				var asMap map[string]any
				Expect(jsoniter.Unmarshal([]byte(marshal(conf)), &asMap)).To(Succeed())
				// The proxy decodes the set-config message this way, rejecting unknown fields.
				decoded := &aiscmn.ConfigToSet{}
				Expect(aiscos.MorphMarshal(asMap, decoded)).To(Succeed())
				Expect(*decoded.Auth.ClientAuthRequired).To(BeTrue())
				Expect(*decoded.Auth.IntraCluster.RequestAuth).To(BeTrue())
				Expect(*decoded.Auth.Signature.Method).To(Equal("HS256"))
			},
			Entry("deprecated names", &aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
				Enabled:    aisapc.Ptr(true),
				ClusterKey: &aisv1.ClusterKeyConfToUpdate{Enabled: aisapc.Ptr(true)}, //nolint:staticcheck // exercising the deprecated option
				Signature:  &aisv1.AuthSignatureConfToUpdate{Method: aisapc.Ptr("HS256")},
			}}),
			Entry("current names", &aisv1.ConfigToUpdate{Auth: &aisv1.AuthConfToUpdate{
				ClientAuthRequired: aisapc.Ptr(true),
				IntraCluster:       &aisv1.IntraClusterConfToUpdate{RequestAuth: aisapc.Ptr(true)},
				Signature:          &aisv1.AuthSignatureConfToUpdate{Method: aisapc.Ptr("HS256")},
			}}),
		)
	})
})
