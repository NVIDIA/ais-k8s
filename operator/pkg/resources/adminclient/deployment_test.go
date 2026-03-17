// Package adminclient contains resources for the AIS admin client deployment
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package adminclient

import (
	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AuthN Environment Variables", Label("short"), func() {
	baseAIS := func() *aisv1.AIStore {
		return &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ais",
				Namespace: "test-ns",
			},
			Spec: aisv1.AIStoreSpec{
				Size: apc.Ptr(int32(1)),
				AdminClient: &aisv1.AdminClientSpec{
					Enabled: apc.Ptr(true),
				},
			},
		}
	}

	Describe("authnEnvVars", func() {
		It("should return nil when auth is nil", func() {
			Expect(authnEnvVars(nil)).To(BeNil())
		})

		It("should return nil when auth uses tokenExchange only", func() {
			auth := &aisv1.AuthSpec{
				TokenExchange: &aisv1.TokenExchangeAuth{},
			}
			Expect(authnEnvVars(auth)).To(BeNil())
		})

		It("should return env vars when auth uses usernamePassword", func() {
			auth := &aisv1.AuthSpec{
				ServiceURL: apc.Ptr("https://authn.example.com:52001"),
				UsernamePassword: &aisv1.UsernamePasswordAuth{ //nolint:gosec // test credentials
					SecretName: "my-authn-creds",
				},
			}
			envVars := authnEnvVars(auth)
			Expect(envVars).To(HaveLen(3))
			Expect(envVars[0]).To(Equal(corev1.EnvVar{
				Name:  "AIS_AUTHN_URL",
				Value: "https://authn.example.com:52001",
			}))
			Expect(envVars[1].Name).To(Equal("AIS_AUTHN_USERNAME"))
			Expect(envVars[1].ValueFrom.SecretKeyRef.Name).To(Equal("my-authn-creds"))
			Expect(envVars[1].ValueFrom.SecretKeyRef.Key).To(Equal("SU-NAME"))
			Expect(envVars[2].Name).To(Equal("AIS_AUTHN_PASSWORD"))
			Expect(envVars[2].ValueFrom.SecretKeyRef.Name).To(Equal("my-authn-creds"))
			Expect(envVars[2].ValueFrom.SecretKeyRef.Key).To(Equal("SU-PASS"))
		})

		It("should use default URL when serviceURL is nil", func() {
			auth := &aisv1.AuthSpec{
				UsernamePassword: &aisv1.UsernamePasswordAuth{
					SecretName: "creds",
				},
			}
			envVars := authnEnvVars(auth)
			Expect(envVars).To(HaveLen(3))
			Expect(envVars[0].Value).To(Equal(DefaultAuthNServiceURL))
		})
	})

	Describe("buildClientEnv with AuthN", func() {
		It("should include authn env vars when auth is configured", func() {
			ais := baseAIS()
			ais.Spec.Auth = &aisv1.AuthSpec{
				ServiceURL: apc.Ptr("https://authn.test:52001"),
				UsernamePassword: &aisv1.UsernamePasswordAuth{
					SecretName: "test-creds",
				},
			}
			env := buildClientEnv(ais)
			envNames := make([]string, len(env))
			for i, e := range env {
				envNames[i] = e.Name
			}
			Expect(envNames).To(ContainElements("AIS_AUTHN_URL", "AIS_AUTHN_USERNAME", "AIS_AUTHN_PASSWORD"))
		})

		It("should not include authn env vars when auth is nil", func() {
			ais := baseAIS()
			env := buildClientEnv(ais)
			envNames := make([]string, len(env))
			for i, e := range env {
				envNames[i] = e.Name
			}
			Expect(envNames).NotTo(ContainElement("AIS_AUTHN_URL"))
			Expect(envNames).NotTo(ContainElement("AIS_AUTHN_USERNAME"))
			Expect(envNames).NotTo(ContainElement("AIS_AUTHN_PASSWORD"))
		})
	})
})
