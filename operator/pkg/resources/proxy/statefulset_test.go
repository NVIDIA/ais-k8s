/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func envByName(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for i := range env {
		if env[i].Name == name {
			return env[i], true
		}
	}
	return corev1.EnvVar{}, false
}

var _ = Describe("Proxy NewInitContainerEnv", func() {
	newAIS := func(spec aisv1.AIStoreSpec) *aisv1.AIStore {
		return &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ais", Namespace: "test-ns"},
			Spec:       spec,
		}
	}

	Describe("public hostname", func() {
		It("should set AIS_PUBLIC_HOSTNAME from host IP when hostPort is set", func() {
			ais := newAIS(aisv1.AIStoreSpec{
				ProxySpec: aisv1.DaemonSpec{HostPort: aisapc.Ptr(int32(51080))},
			})
			env := NewInitContainerEnv(ais)
			ev, ok := envByName(env, cmn.EnvPublicHostname)
			Expect(ok).To(BeTrue())
			Expect(ev.ValueFrom).ToNot(BeNil())
			Expect(ev.ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))
		})

		It("should set AIS_PUBLIC_HOSTNAME from node name in Node DNS mode", func() {
			ais := newAIS(aisv1.AIStoreSpec{
				ProxySpec:        aisv1.DaemonSpec{HostPort: aisapc.Ptr(int32(51080))},
				PublicNetDNSMode: aisapc.Ptr(aisv1.PubNetDNSModeNode),
			})
			env := NewInitContainerEnv(ais)
			ev, ok := envByName(env, cmn.EnvPublicHostname)
			Expect(ok).To(BeTrue())
			Expect(ev.ValueFrom).ToNot(BeNil())
			Expect(ev.ValueFrom.FieldRef.FieldPath).To(Equal("spec.nodeName"))
		})

		// In Pod DNS mode aisinit ignores AIS_PUBLIC_HOSTNAME, so the operator still sets host IP.
		It("should set AIS_PUBLIC_HOSTNAME from host IP in Pod DNS mode", func() {
			ais := newAIS(aisv1.AIStoreSpec{
				ProxySpec:        aisv1.DaemonSpec{HostPort: aisapc.Ptr(int32(51080))},
				PublicNetDNSMode: aisapc.Ptr(aisv1.PubNetDNSModePod),
			})
			ev, ok := envByName(NewInitContainerEnv(ais), cmn.EnvPublicHostname)
			Expect(ok).To(BeTrue())
			Expect(ev.ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))
		})

		It("should not set AIS_PUBLIC_HOSTNAME when hostPort is unset", func() {
			ais := newAIS(aisv1.AIStoreSpec{ProxySpec: aisv1.DaemonSpec{}})
			env := NewInitContainerEnv(ais)
			_, ok := envByName(env, cmn.EnvPublicHostname)
			Expect(ok).To(BeFalse())
		})

		// Proxies use a single shared LoadBalancer and aisinit does not rewrite their public
		// hostname from a per-pod external IP, so external access must not suppress it (otherwise
		// proxies fall back to advertising pod IPs in the cluster map).
		It("should still set AIS_PUBLIC_HOSTNAME when external access is enabled", func() {
			ais := newAIS(aisv1.AIStoreSpec{
				ProxySpec: aisv1.DaemonSpec{
					HostPort:       aisapc.Ptr(int32(51080)),
					ExternalAccess: &aisv1.ExternalAccessSpec{},
				},
			})
			env := NewInitContainerEnv(ais)
			ev, ok := envByName(env, cmn.EnvPublicHostname)
			Expect(ok).To(BeTrue())
			Expect(ev.ValueFrom).ToNot(BeNil())
			Expect(ev.ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))

			extEnv, ok := envByName(env, cmn.EnvEnableExternalAccess)
			Expect(ok).To(BeTrue())
			Expect(extEnv.Value).To(Equal("true"))
		})

		It("should still set AIS_PUBLIC_HOSTNAME with the legacy enableExternalLB", func() {
			ais := newAIS(aisv1.AIStoreSpec{
				ProxySpec:        aisv1.DaemonSpec{HostPort: aisapc.Ptr(int32(51080))},
				EnableExternalLB: true,
			})
			env := NewInitContainerEnv(ais)
			ev, ok := envByName(env, cmn.EnvPublicHostname)
			Expect(ok).To(BeTrue())
			Expect(ev.ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))
		})
	})

	Describe("service name", func() {
		It("should set MY_SERVICE to the proxy headless service", func() {
			ais := newAIS(aisv1.AIStoreSpec{ProxySpec: aisv1.DaemonSpec{}})
			env := NewInitContainerEnv(ais)
			ev, ok := envByName(env, cmn.EnvServiceName)
			Expect(ok).To(BeTrue())
			Expect(ev.Value).To(Equal(ais.Name + "-" + aisapc.Proxy))
		})
	})
})
