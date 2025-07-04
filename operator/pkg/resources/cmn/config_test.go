// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
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
			cfg := &aiscmn.ClusterConfig{}
			err = cfg.Apply(toSet, aisapc.Cluster)
			Expect(err).ToNot(HaveOccurred())

			Expect(cfg.Space.CleanupWM).To(BeEquivalentTo(10))
			Expect(cfg.Space.LowWM).To(BeEquivalentTo(20))
			Expect(cfg.Space.HighWM).To(BeEquivalentTo(30))
			Expect(cfg.Space.OOS).To(BeEquivalentTo(40))

			Expect(cfg.LRU.Enabled).To(BeEquivalentTo(true))
			Expect(cfg.LRU.DontEvictTime).To(BeEquivalentTo(10))

			Expect(cfg.Features).To(BeEquivalentTo(2568))

			Expect(cfg.Tracing.Enabled).To(BeTrue())
			Expect(cfg.Tracing.ExporterAuth.TokenHeader).To(Equal("token-header"))
			Expect(cfg.Tracing.ExporterAuth.TokenFile).To(Equal("token-file"))
		})
	})
	Describe("Generate config override", func() {
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
