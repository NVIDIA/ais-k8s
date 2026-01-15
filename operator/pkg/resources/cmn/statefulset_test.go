// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
	"path"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Statefulset", Label("short"), func() {
	Describe("Log Sidecar", func() {
		DescribeTable("should create log container spec",
			func(daeType string) {
				sidecarImage := "testImage"
				cSpec := NewLogSidecar(sidecarImage, daeType)

				Expect(cSpec.Name).To(BeEquivalentTo("ais-logs"))
				Expect(cSpec.ImagePullPolicy).To(BeEquivalentTo(corev1.PullIfNotPresent))
				Expect(cSpec.Args).To(BeEquivalentTo([]string{fmt.Sprintf(LogsDir+"/ais%s.INFO", daeType)}))

				Expect(cSpec.VolumeMounts).To(HaveLen(1))
				Expect(cSpec.VolumeMounts[0]).To(BeEquivalentTo(newLogsVolumeMount(daeType)))
			},
			Entry("for proxy", aisapc.Proxy),
			Entry("for target", aisapc.Target),
		)
	})

	Describe("PrepareAnnotations", func() {
		It("should handle nil network attachment", func() {
			annotations := map[string]string{"key1": "value1"}
			result := PrepareAnnotations(annotations, nil, nil)

			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
		})

		It("should add network attachment when provided", func() {
			annotations := map[string]string{"key1": "value1"}
			netAttachment := "test-network"
			result := PrepareAnnotations(annotations, &netAttachment, nil)

			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
			Expect(result).To(HaveKeyWithValue(nadv1.NetworkAttachmentAnnot, "test-network"))
		})

		It("should add restart hash when provided", func() {
			annotations := map[string]string{"key1": "value1"}
			restartHash := "restart-hash"
			result := PrepareAnnotations(annotations, nil, &restartHash)

			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
			Expect(result).To(HaveKeyWithValue(RestartConfigHashAnnotation, "restart-hash"))
		})

		It("should handle empty input annotations", func() {
			netAttachment := "test-network"
			result := PrepareAnnotations(nil, &netAttachment, nil)

			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue(nadv1.NetworkAttachmentAnnot, "test-network"))
		})

		It("should not modify original annotations", func() {
			original := map[string]string{"key1": "value1"}
			originalCopy := map[string]string{"key1": "value1"}
			netAttachment := "test-network"

			result := PrepareAnnotations(original, &netAttachment, nil)

			Expect(original).To(Equal(originalCopy))
			Expect(result).NotTo(BeIdenticalTo(original))
		})
	})

	Describe("NewInitContainerArgs", func() {
		Describe("when creating container arguments", func() {
			Context("with empty hostname map", func() {
				It("should return basic arguments for any daemon type", func() {
					args := NewInitContainerArgs("daeType", map[string]string{})
					Expect(args).To(Equal([]string{
						"-role=daeType",
						"-local_config_template=" + path.Join(InitConfTemplateDir, AISLocalConfigName),
						"-output_local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
						"-cluster_config_override=" + path.Join(InitGlobalConfDir, AISGlobalConfigName),
						"-output_cluster_config=" + path.Join(AisConfigDir, AISGlobalConfigName),
					}))
				})
			})

			Context("with non-empty hostname map", func() {
				It("should include hostname map file argument", func() {
					hostnameMap := map[string]string{
						"host1": "ip1",
						"host2": "ip2",
					}
					args := NewInitContainerArgs("daeType", hostnameMap)
					Expect(args).To(Equal([]string{
						"-role=daeType",
						"-local_config_template=" + path.Join(InitConfTemplateDir, AISLocalConfigName),
						"-output_local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
						"-cluster_config_override=" + path.Join(InitGlobalConfDir, AISGlobalConfigName),
						"-output_cluster_config=" + path.Join(AisConfigDir, AISGlobalConfigName),
						"-hostname_map_file=" + path.Join(InitGlobalConfDir, hostnameMapFileName),
					}))
				})
			})
		})
	})
	DescribeTable("NewAISContainerArgs",
		func(role string, expectedArgs []string) {
			targetSize := int32(3)
			args := NewAISContainerArgs(targetSize, role)
			Expect(args).To(Equal(expectedArgs))
		},
		Entry("should return basic arguments for target",
			aisapc.Target,
			[]string{
				"-config=" + path.Join(AisConfigDir, AISGlobalConfigName),
				"-local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
				"-role=" + aisapc.Target,
			},
		),
		Entry("should return arguments with ntargets for proxy",
			aisapc.Proxy,
			[]string{
				"-config=" + path.Join(AisConfigDir, AISGlobalConfigName),
				"-local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
				"-role=" + aisapc.Proxy,
				"-ntargets=3",
			},
		),
	)

	Describe("getTLSVolume", func() {
		DescribeTable("should return correct volume type",
			func(spec aisv1.AIStoreSpec, expectCSI, expectSecret bool) {
				ais := &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
					Spec:       spec,
				}
				vol := getTLSVolume(ais, aisapc.Proxy)

				if !expectCSI && !expectSecret {
					Expect(vol).To(BeNil())
					return
				}

				Expect(vol).ToNot(BeNil())
				Expect(vol.Name).To(Equal(tlsSecretVolume))

				if expectCSI {
					Expect(vol.CSI).ToNot(BeNil())
					Expect(vol.CSI.Driver).To(Equal("csi.cert-manager.io"))
				}
				if expectSecret {
					Expect(vol.Secret).ToNot(BeNil())
				}
			},
			Entry("no TLS configured", aisv1.AIStoreSpec{}, false, false),
			Entry("TLSCertificate", aisv1.AIStoreSpec{
				TLSCertificate: &aisv1.TLSCertificateSpec{
					IssuerRef: aisv1.CertIssuerRef{Name: "test-issuer"},
				},
			}, false, true),
			Entry("TLSSecretName", aisv1.AIStoreSpec{
				TLSSecretName: aisapc.Ptr("my-tls-secret"),
			}, false, true),
			Entry("TLSCertManagerIssuerName", aisv1.AIStoreSpec{
				TLSCertManagerIssuerName: aisapc.Ptr("my-issuer"),
			}, true, false),
		)
	})
})
