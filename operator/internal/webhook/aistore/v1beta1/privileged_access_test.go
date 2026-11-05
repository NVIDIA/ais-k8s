/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"strings"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
)

// podAIS returns a cluster whose pods meet the baseline policy.
func podAIS() *aisv1.AIStore {
	ais := profileAIS()
	ais.Spec.Size = aisapc.Ptr[int32](1)
	ais.Spec.StateStorage = &aisv1.StateStorage{EmptyDir: &aisv1.StateEmptyDirConfig{}}
	return ais
}

func TestValidatePrivilegedFields(t *testing.T) {
	unsafeSysctl := func() *corev1.PodSecurityContext {
		return &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{{Name: "net.core.somaxconn", Value: "4096"}}}
	}

	for _, tt := range []struct {
		name string
		set  func(spec *aisv1.AIStoreSpec)
		want []string
	}{
		{
			name: "proxy pod unsafe sysctl",
			set:  func(s *aisv1.AIStoreSpec) { s.ProxySpec.SecurityContext = unsafeSysctl() },
			want: []string{"proxy pods:", "net.core.somaxconn"},
		},
		{
			name: "target privileged container",
			set: func(s *aisv1.AIStoreSpec) {
				s.TargetSpec.AISContainerSecurityContext = &corev1.SecurityContext{Privileged: aisapc.Ptr(true)}
			},
			want: []string{"target pods:", "privileged"},
		},
		{
			name: "hostNetwork",
			set:  func(s *aisv1.AIStoreSpec) { s.TargetSpec.HostNetwork = aisapc.Ptr(true) },
			want: []string{"target pods:", "host namespaces"},
		},
		{
			name: "hostPath mount",
			set: func(s *aisv1.AIStoreSpec) {
				s.TargetSpec.Mounts = []aisv1.Mount{{Path: "/ais/a"}, {Path: "/ais/b", UseHostPath: aisapc.Ptr(true)}}
			},
			want: []string{"target pods:", "hostPath volumes"},
		},
		{
			name: "hostPath state storage",
			set: func(s *aisv1.AIStoreSpec) {
				s.StateStorage = &aisv1.StateStorage{HostPath: &aisv1.StateHostPathConfig{Prefix: "/etc/ais"}}
			},
			want: []string{"proxy pods:", "target pods:", "hostPath volumes"},
		},
		{
			name: "logsDir",
			set:  func(s *aisv1.AIStoreSpec) { s.LogsDirectory = "/var/log/ais" },
			want: []string{"proxy pods:", "target pods:", "hostPath volumes"},
		},
	} {
		t.Run(tt.name+" is reviewed and warns when unauthorized", func(t *testing.T) {
			g := NewWithT(t)
			ais := podAIS()
			tt.set(&ais.Spec)

			webhook, reviews := newSARWebhook(t, false)
			warnings, err := webhook.validatePrivilegedAccess(admissionCtx(), ais)

			g.Expect(err).To(Succeed())
			g.Expect(warnings).To(HaveLen(1))
			for _, want := range tt.want {
				g.Expect(warnings[0]).To(ContainSubstring(want))
			}
			g.Expect(warnings[0]).To(ContainSubstring(
				`is not authorized to use aistores/privileged resource "cluster" in namespace "tenant"`))
			g.Expect(*reviews).To(HaveLen(1))
			g.Expect((*reviews)[0].Spec.ResourceAttributes).To(Equal(&authorizationv1.ResourceAttributes{
				Verb:        "use",
				Group:       "ais.nvidia.com",
				Version:     "v1beta1",
				Resource:    "aistores",
				Subresource: "privileged",
				Namespace:   tenantNS,
				Name:        "cluster",
			}))

			allowed, _ := newSARWebhook(t, true)
			warnings, err = allowed.validatePrivilegedAccess(admissionCtx(), ais)
			g.Expect(err).To(Succeed())
			g.Expect(warnings).To(BeEmpty())
		})
	}

	t.Run("one review covers every violation and the warning names each", func(t *testing.T) {
		g := NewWithT(t)
		ais := podAIS()
		ais.Spec.ProxySpec.SecurityContext = unsafeSysctl()
		ais.Spec.TargetSpec.HostNetwork = aisapc.Ptr(true)
		ais.Spec.LogsDirectory = "/var/log/ais"

		webhook, reviews := newSARWebhook(t, false)
		warnings, err := webhook.validatePrivilegedAccess(admissionCtx(), ais)

		g.Expect(err).To(Succeed())
		g.Expect(*reviews).To(HaveLen(1))
		g.Expect(warnings).To(HaveLen(1))
		g.Expect(warnings[0]).To(ContainSubstring("net.core.somaxconn"))
		g.Expect(warnings[0]).To(ContainSubstring("host namespaces"))
		g.Expect(warnings[0]).To(ContainSubstring("hostPath volumes"))
		g.Expect(strings.Count(warnings[0], privilegedAccessDocURL)).To(Equal(1))
	})

	t.Run("pods that meet the baseline policy are not reviewed", func(t *testing.T) {
		g := NewWithT(t)
		ais := podAIS()
		ais.Spec.ProxySpec.HostPort = aisapc.Ptr[int32](51080)
		ais.Spec.TargetSpec.HostPort = aisapc.Ptr[int32](51081)
		ais.Spec.TargetSpec.HostNetwork = aisapc.Ptr(false)
		ais.Spec.TargetSpec.Mounts = []aisv1.Mount{{Path: "/ais/a", UseHostPath: aisapc.Ptr(false)}}
		ais.Spec.TargetSpec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser: aisapc.Ptr[int64](1000),
			Sysctls:   []corev1.Sysctl{{Name: "net.ipv4.tcp_rmem", Value: "4096 87380 6291456"}},
		}

		webhook, reviews := newSARWebhook(t, false)
		warnings, err := webhook.validatePrivilegedAccess(admissionCtx(), ais)
		g.Expect(err).To(Succeed())
		g.Expect(warnings).To(BeEmpty())
		g.Expect(*reviews).To(BeEmpty())
	})
}

// The review must run on admission, not only when validatePrivilegedFields is called directly.
func TestValidateSpecReviewsPrivilegedFields(t *testing.T) {
	newAIS := func() *aisv1.AIStore {
		ais := podAIS()
		ais.Spec.Auth = nil
		ais.Spec.TargetSpec.HostNetwork = aisapc.Ptr(true)
		return ais
	}

	t.Run("create", func(t *testing.T) {
		g := NewWithT(t)
		webhook, reviews := newSARWebhook(t, false)
		warnings, err := webhook.validateSpec(admissionCtx(), nil, newAIS())

		g.Expect(err).To(Succeed())
		g.Expect(warnings).To(ContainElement(ContainSubstring("host namespaces")))
		g.Expect(*reviews).To(HaveLen(1))
	})

	t.Run("an update of another field is reviewed", func(t *testing.T) {
		g := NewWithT(t)
		prev, ais := newAIS(), newAIS()
		ais.Spec.Size = aisapc.Ptr[int32](2)
		webhook, reviews := newSARWebhook(t, false)
		warnings, err := webhook.validateSpec(admissionCtx(), prev, ais)

		g.Expect(err).To(Succeed())
		g.Expect(warnings).To(ContainElement(ContainSubstring("host namespaces")))
		g.Expect(*reviews).To(HaveLen(1))
	})

	t.Run("a write that leaves the spec unchanged is not reviewed", func(t *testing.T) {
		g := NewWithT(t)
		webhook, reviews := newSARWebhook(t, false)
		warnings, err := webhook.validateSpec(admissionCtx(), newAIS(), newAIS())

		g.Expect(err).To(Succeed())
		g.Expect(warnings).NotTo(ContainElement(ContainSubstring("privileged")))
		g.Expect(*reviews).To(BeEmpty())
	})
}
