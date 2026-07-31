/*
 * Copyright (c) 2025-2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuthSpecConfig", func() {
	Describe("GetServiceURL", func() {
		It("should return custom URL when specified", func() {
			customURL := "https://custom-authn.example.com:8443"
			spec := &aisv1.AuthSpec{
				ServiceURL: &customURL,
			}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.GetServiceURL()).To(Equal(customURL))
		})

		It("should return default URL when not specified", func() {
			spec := &aisv1.AuthSpec{}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.GetServiceURL()).To(Equal(DefaultAuthNServiceURL))
		})
	})

	Describe("GetCACertPath", func() {
		It("should return path when specified", func() {
			path := "/etc/ssl/certs/ca.crt"
			spec := &aisv1.AuthSpec{
				TLS: &aisv1.AuthTLSConfig{
					CACertPath: path,
				},
			}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.GetCACertPath()).To(Equal(path))
		})

		It("should return empty string when no TLS config", func() {
			spec := &aisv1.AuthSpec{}
			config := &AuthSpecConfig{spec: spec}
			Expect(config.GetCACertPath()).To(Equal(""))
		})

		It("should return empty string TLS config exists but CACertPath is empty", func() {
			spec := &aisv1.AuthSpec{
				TLS: &aisv1.AuthTLSConfig{
					InsecureSkipVerify: false,
				},
			}
			config := &AuthSpecConfig{spec: spec}
			Expect(config.GetCACertPath()).To(Equal(""))
		})
	})

	Describe("GetInsecureSkipVerify", func() {
		It("should return true when configured", func() {
			spec := &aisv1.AuthSpec{
				TLS: &aisv1.AuthTLSConfig{
					InsecureSkipVerify: true,
				},
			}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.GetInsecureSkipVerify()).To(BeTrue())
		})

		It("should return false by default", func() {
			spec := &aisv1.AuthSpec{}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.GetInsecureSkipVerify()).To(BeFalse())
		})
	})

	Describe("IsTokenExchange", func() {
		It("should return true when TokenExchange is configured", func() {
			spec := &aisv1.AuthSpec{
				TokenExchange: &aisv1.TokenExchangeAuth{},
			}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.IsTokenExchange()).To(BeTrue())
		})

		It("should return false when UsernamePassword is configured", func() {
			spec := &aisv1.AuthSpec{
				UsernamePassword: &aisv1.UsernamePasswordAuth{
					SecretName: "test-secret",
				},
			}
			config := &AuthSpecConfig{spec: spec}

			Expect(config.IsTokenExchange()).To(BeFalse())
		})
	})
})
