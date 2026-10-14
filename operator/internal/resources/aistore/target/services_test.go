/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package target

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Target NewTargetLoadBalancerSVC", func() {
	It("should publish the public port", func() {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ais", Namespace: "test-ns"},
			Spec: aisv1.AIStoreSpec{TargetSpec: aisv1.TargetSpec{DaemonSpec: aisv1.DaemonSpec{
				ServiceSpec:    aisv1.ServiceSpec{PublicPort: aisapc.Ptr(intstr.FromInt32(9081))},
				ExternalAccess: &aisv1.ExternalAccessSpec{},
			}}},
		}

		ports := NewTargetLoadBalancerSVC(ais, 0).Spec.Ports
		Expect(ports).To(HaveLen(1))
		Expect(*ports[0].Port).To(Equal(int32(9081)))
		Expect(*ports[0].TargetPort).To(Equal(intstr.FromInt32(9081)))
	})

	DescribeTable("external-dns hostname annotation", func(externalDNSAnnotation bool, want types.GomegaMatcher) {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ais", Namespace: "test-ns"},
			Spec: aisv1.AIStoreSpec{TargetSpec: aisv1.TargetSpec{DaemonSpec: aisv1.DaemonSpec{
				ExternalAccess: &aisv1.ExternalAccessSpec{DNS: &aisv1.ExternalDNSSpec{
					Domain:                "ais.example.com",
					ExternalDNSAnnotation: externalDNSAnnotation,
				}},
			}}},
		}
		Expect(NewTargetLoadBalancerSVC(ais, 2).Annotations).To(want)
	},
		Entry("set when enabled", true, HaveKeyWithValue(aisv1.ExternalDNSHostnameAnnotation, "test-ais-target-2.ais.example.com")),
		Entry("unset when disabled", false, Not(HaveKey(aisv1.ExternalDNSHostnameAnnotation))),
	)
})
