// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func newTestAIS() *aisv1.AIStore {
	return &aisv1.AIStore{
		Spec: aisv1.AIStoreSpec{
			ProxySpec: aisv1.DaemonSpec{
				ServiceSpec: aisv1.ServiceSpec{
					PublicPort: intstr.FromInt32(51080),
				},
			},
			TargetSpec: aisv1.TargetSpec{
				DaemonSpec: aisv1.DaemonSpec{
					ServiceSpec: aisv1.ServiceSpec{
						PublicPort: intstr.FromInt32(51081),
					},
				},
			},
		},
	}
}

func int32Ptr(v int32) *int32 { return &v }

var _ = Describe("Health Probes", Label("short"), func() {
	Describe("default values (no overrides)", func() {
		var ais *aisv1.AIStore

		BeforeEach(func() {
			ais = newTestAIS()
		})

		DescribeTable("NewLivenessProbe",
			func(role string, expectedPort int32) {
				probe := NewLivenessProbe(ais, role)
				Expect(probe.InitialDelaySeconds).To(BeEquivalentTo(defaultLivenessInitialDelaySeconds))
				Expect(probe.PeriodSeconds).To(BeEquivalentTo(defaultProbePeriodSeconds))
				Expect(probe.FailureThreshold).To(BeEquivalentTo(defaultLivenessFailureThreshold))
				Expect(probe.TimeoutSeconds).To(BeEquivalentTo(defaultProbeTimeoutSeconds))
				Expect(probe.SuccessThreshold).To(BeEquivalentTo(defaultProbeSuccessThreshold))
				Expect(probe.HTTPGet.Port.IntValue()).To(Equal(int(expectedPort)))
				Expect(probe.HTTPGet.Path).To(Equal(probeLivenessEndpoint))
				Expect(probe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTP))
			},
			Entry("proxy", aisapc.Proxy, int32(51080)),
			Entry("target", aisapc.Target, int32(51081)),
		)

		DescribeTable("NewReadinessProbe",
			func(role string) {
				probe := NewReadinessProbe(ais, role)
				Expect(probe.InitialDelaySeconds).To(BeZero())
				Expect(probe.PeriodSeconds).To(BeEquivalentTo(defaultProbePeriodSeconds))
				Expect(probe.FailureThreshold).To(BeEquivalentTo(defaultReadinessFailureThreshold))
				Expect(probe.TimeoutSeconds).To(BeEquivalentTo(defaultProbeTimeoutSeconds))
				Expect(probe.SuccessThreshold).To(BeEquivalentTo(defaultProbeSuccessThreshold))
				Expect(probe.HTTPGet.Path).To(Equal(probeReadinessEndpoint))
			},
			Entry("proxy", aisapc.Proxy),
			Entry("target", aisapc.Target),
		)

		DescribeTable("NewStartupProbe",
			func(role string) {
				probe := NewStartupProbe(ais, role)
				Expect(probe.InitialDelaySeconds).To(BeZero())
				Expect(probe.PeriodSeconds).To(BeEquivalentTo(defaultStartupPeriodSeconds))
				Expect(probe.FailureThreshold).To(BeEquivalentTo(defaultStartupFailureThreshold))
				Expect(probe.TimeoutSeconds).To(BeEquivalentTo(defaultProbeTimeoutSeconds))
				Expect(probe.SuccessThreshold).To(BeEquivalentTo(defaultProbeSuccessThreshold))
				Expect(probe.HTTPGet.Path).To(Equal(probeReadinessEndpoint))
			},
			Entry("proxy", aisapc.Proxy),
			Entry("target", aisapc.Target),
		)
	})

	Describe("with overrides", func() {
		It("should use CR values for proxy liveness probe", func() {
			ais := newTestAIS()
			ais.Spec.ProxySpec.Probes = &aisv1.ProbeConfSpec{
				Liveness: &aisv1.ProbeSpec{
					InitialDelaySeconds: int32Ptr(120),
					FailureThreshold:    int32Ptr(20),
				},
			}
			probe := NewLivenessProbe(ais, aisapc.Proxy)
			Expect(probe.InitialDelaySeconds).To(BeEquivalentTo(120))
			Expect(probe.FailureThreshold).To(BeEquivalentTo(20))
			// Non-overridden fields use defaults
			Expect(probe.PeriodSeconds).To(BeEquivalentTo(defaultProbePeriodSeconds))
			Expect(probe.TimeoutSeconds).To(BeEquivalentTo(defaultProbeTimeoutSeconds))
		})

		It("should use CR values for target readiness probe", func() {
			ais := newTestAIS()
			ais.Spec.TargetSpec.Probes = &aisv1.ProbeConfSpec{
				Readiness: &aisv1.ProbeSpec{
					PeriodSeconds:    int32Ptr(15),
					TimeoutSeconds:   int32Ptr(10),
					FailureThreshold: int32Ptr(3),
				},
			}
			probe := NewReadinessProbe(ais, aisapc.Target)
			Expect(probe.PeriodSeconds).To(BeEquivalentTo(15))
			Expect(probe.TimeoutSeconds).To(BeEquivalentTo(10))
			Expect(probe.FailureThreshold).To(BeEquivalentTo(3))
		})

		It("should use CR values for startup probe", func() {
			ais := newTestAIS()
			ais.Spec.ProxySpec.Probes = &aisv1.ProbeConfSpec{
				Startup: &aisv1.ProbeSpec{
					FailureThreshold: int32Ptr(60),
					PeriodSeconds:    int32Ptr(10),
				},
			}
			probe := NewStartupProbe(ais, aisapc.Proxy)
			Expect(probe.FailureThreshold).To(BeEquivalentTo(60))
			Expect(probe.PeriodSeconds).To(BeEquivalentTo(10))
			Expect(probe.TimeoutSeconds).To(BeEquivalentTo(defaultProbeTimeoutSeconds))
		})

		It("should allow overriding initialDelaySeconds on startup probe", func() {
			ais := newTestAIS()
			ais.Spec.TargetSpec.Probes = &aisv1.ProbeConfSpec{
				Startup: &aisv1.ProbeSpec{
					InitialDelaySeconds: int32Ptr(30),
				},
			}
			probe := NewStartupProbe(ais, aisapc.Target)
			Expect(probe.InitialDelaySeconds).To(BeEquivalentTo(30))
			// Other fields use defaults
			Expect(probe.PeriodSeconds).To(BeEquivalentTo(defaultStartupPeriodSeconds))
			Expect(probe.FailureThreshold).To(BeEquivalentTo(defaultStartupFailureThreshold))
		})

		It("should not affect other daemon role", func() {
			ais := newTestAIS()
			ais.Spec.ProxySpec.Probes = &aisv1.ProbeConfSpec{
				Liveness: &aisv1.ProbeSpec{
					InitialDelaySeconds: int32Ptr(999),
				},
			}
			// Target should still use defaults
			probe := NewLivenessProbe(ais, aisapc.Target)
			Expect(probe.InitialDelaySeconds).To(BeEquivalentTo(defaultLivenessInitialDelaySeconds))
		})

		It("should handle ProbeConfSpec with nil probe type", func() {
			ais := newTestAIS()
			ais.Spec.ProxySpec.Probes = &aisv1.ProbeConfSpec{
				Liveness: &aisv1.ProbeSpec{
					InitialDelaySeconds: int32Ptr(120),
				},
				// Readiness and Startup are nil
			}
			readiness := NewReadinessProbe(ais, aisapc.Proxy)
			Expect(readiness.PeriodSeconds).To(BeEquivalentTo(defaultProbePeriodSeconds))

			startup := NewStartupProbe(ais, aisapc.Proxy)
			Expect(startup.FailureThreshold).To(BeEquivalentTo(defaultStartupFailureThreshold))
		})
	})
})
