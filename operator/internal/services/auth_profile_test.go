/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/x509"
	"encoding/pem"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("AuthProfileConfig", func() {
	profileConfig := func(spec authv1alpha1.AIStoreAuthProfileSpec) *AuthProfileConfig {
		return &AuthProfileConfig{profile: &authv1alpha1.AIStoreAuthProfile{Spec: spec}}
	}

	It("should read the provider endpoint from the profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{ServiceURL: "https://auth-provider.ais.svc:52001"})
		Expect(config.GetServiceURL()).To(Equal("https://auth-provider.ais.svc:52001"))
	})

	It("should default the token exchange endpoint when the profile leaves it empty", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{},
		})
		Expect(config.IsTokenExchange()).To(BeTrue())
		Expect(config.GetTokenExchangeEndpoint()).To(Equal(DefaultTokenExchangeEndpoint))
	})

	It("should use the token exchange endpoint from the profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
		})
		Expect(config.GetTokenExchangeEndpoint()).To(Equal("/exchange"))
	})

	It("should default the credential keys to the AuthN admin secret format", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
				Secret: authv1alpha1.AuthProfileSecret{Name: "admin", Namespace: "auth-config"},
			},
		})
		Expect(config.IsTokenExchange()).To(BeFalse())
		Expect(config.GetSecretName()).To(Equal("admin"))
		Expect(config.GetSecretNamespace()).To(Equal("auth-config"))
		Expect(config.GetUserKey()).To(Equal(AuthNSecretRefName))
		Expect(config.GetPassKey()).To(Equal(AuthNSecretRefPass))
		Expect(config.GetOAuthLoginConf()).To(BeNil())
	})

	It("should use the credential keys and login conf from the profile", func() {
		scope := "read write"
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
				Secret: authv1alpha1.AuthProfileSecret{
					Name: "admin", Namespace: "auth-config", UserKey: "username", PassKey: "password",
				},
				LoginConf: &authv1alpha1.AuthProfileLoginConf{ClientID: "ais-operator", Scope: &scope},
			},
		})
		Expect(config.GetUserKey()).To(Equal("username"))
		Expect(config.GetPassKey()).To(Equal("password"))
		Expect(config.GetOAuthLoginConf()).To(Equal(&OAuthLoginConf{ClientID: "ais-operator", Scope: &scope}))
	})

	It("should report no login secret for a token exchange profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{},
		})
		Expect(config.GetSecretName()).To(BeEmpty())
		Expect(config.GetSecretNamespace()).To(BeEmpty())
		Expect(config.GetUserKey()).To(BeEmpty())
		Expect(config.GetPassKey()).To(BeEmpty())
	})

	It("should trust the CA certificate held in the referenced ConfigMap", func() {
		caCertPEM := createTestCACertPEM("profile-ca")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-provider-ca", Namespace: "auth-config"},
			Data:       map[string]string{"ca.crt": string(caCertPEM)},
		}
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TLS: &authv1alpha1.AuthProfileTLSConfig{
				CAConfigMapRef: &authv1alpha1.AuthProfileCAConfigMapRef{
					Namespace: "auth-config", Name: "auth-provider-ca", Key: "ca.crt",
				},
			},
		})
		config.k8sClient = newFakeK8sClient(configMap)

		tlsConfig, err := config.GetTLSConfig(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsConfig.RootCAs).NotTo(BeNil())

		block, _ := pem.Decode(caCertPEM)
		Expect(block).NotTo(BeNil())
		caCert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		_, err = caCert.Verify(x509.VerifyOptions{
			Roots:     tlsConfig.RootCAs,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should fail when the referenced ConfigMap has no CA key", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-provider-ca", Namespace: "auth-config"},
		}
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TLS: &authv1alpha1.AuthProfileTLSConfig{
				CAConfigMapRef: &authv1alpha1.AuthProfileCAConfigMapRef{
					Namespace: "auth-config", Name: "auth-provider-ca", Key: "ca.crt",
				},
			},
		})
		config.k8sClient = newFakeK8sClient(configMap)

		_, err := config.GetTLSConfig(context.Background())
		Expect(err).To(MatchError(ContainSubstring(`has no key "ca.crt"`)))
	})
})

var _ = Describe("getAuthConfig", func() {
	It("should resolve auth from the referenced AIStoreAuthProfile", func() {
		profile := &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-authn"},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL:    "https://auth-provider.ais.svc:52001",
				TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
			},
		}
		client := NewAuthNClient(newFakeK8sClient(profile))
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{
					ProfileRef: &aisv1.AuthProfileRef{Name: "prod-authn"},
				},
			},
		}

		config, err := client.getAuthConfig(context.Background(), ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(BeAssignableToTypeOf(&AuthProfileConfig{}))
		Expect(config.GetServiceURL()).To(Equal("https://auth-provider.ais.svc:52001"))
		Expect(config.IsTokenExchange()).To(BeTrue())
		Expect(config.GetTokenExchangeEndpoint()).To(Equal("/exchange"))
	})

	It("should ignore spec auth fields when profileRef is set", func() {
		specURL := "http://spec-authn.ais:52001"
		profile := &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-authn"},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL:    "https://auth-provider.ais.svc:52001",
				TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
			},
		}
		client := NewAuthNClient(newFakeK8sClient(profile))
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{
					ProfileRef: &aisv1.AuthProfileRef{Name: "prod-authn"},
					ServiceURL: &specURL,
					UsernamePassword: &aisv1.UsernamePasswordAuth{
						SecretName: "spec-admin",
					},
				},
			},
		}

		config, err := client.getAuthConfig(context.Background(), ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(BeAssignableToTypeOf(&AuthProfileConfig{}))
		Expect(config.GetServiceURL()).To(Equal("https://auth-provider.ais.svc:52001"))
		Expect(config.IsTokenExchange()).To(BeTrue())
		Expect(config.GetSecretName()).To(BeEmpty())
	})

	It("should surface an error when the referenced profile does not exist", func() {
		client := NewAuthNClient(newFakeK8sClient())
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{
					ProfileRef: &aisv1.AuthProfileRef{Name: "missing-profile"},
				},
			},
		}

		_, err := client.getAuthConfig(context.Background(), ais)
		Expect(err).To(MatchError(ContainSubstring(`failed to get AIStoreAuthProfile "missing-profile"`)))
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should fall back to the spec auth fields when profileRef is unset", func() {
		serviceURL := "https://spec-authn.ais:8443"
		client := NewAuthNClient(newFakeK8sClient())
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{
					ServiceURL: &serviceURL,
					UsernamePassword: &aisv1.UsernamePasswordAuth{
						SecretName: "admin",
					},
				},
			},
		}

		config, err := client.getAuthConfig(context.Background(), ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(BeAssignableToTypeOf(&AuthSpecConfig{}))
		Expect(config.GetServiceURL()).To(Equal(serviceURL))
		Expect(config.GetSecretName()).To(Equal("admin"))
		Expect(config.GetSecretNamespace()).To(Equal("tenant"))
	})
})

func newFakeK8sClient(objs ...client.Object) *aisclient.K8sClient {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(authv1alpha1.AddToScheme(scheme)).To(Succeed())
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return aisclient.NewClient(c, scheme)
}
