/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"time"

	"github.com/NVIDIA/aistore/api"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/services"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const testProfileName = "auth-profile"

var _ = Describe("AuthProfileConfig", func() {
	It("should read the provider endpoint from the profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{ServiceURL: "https://auth-provider.ais.svc:52001"})
		Expect(config.GetServiceURL()).To(Equal("https://auth-provider.ais.svc:52001"))
	})

	It("should default the token exchange endpoint when the profile leaves it empty", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{},
		})
		Expect(config.IsTokenExchange()).To(BeTrue())
		Expect(config.GetTokenExchangeEndpoint()).To(Equal(services.DefaultTokenExchangeEndpoint))
	})

	It("should use the token exchange endpoint from the profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
		})
		Expect(config.GetTokenExchangeEndpoint()).To(Equal("/exchange"))
	})

	It("should use the subject token audience from the profile", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{SubjectTokenAudience: "ais-auth"},
		})
		Expect(config.GetSubjectTokenAudience()).To(Equal("ais-auth"))
	})

	It("should report no subject token audience when the profile leaves it empty", func() {
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			TokenExchange: &authv1alpha1.AuthProfileTokenExchange{},
		})
		Expect(config.GetSubjectTokenAudience()).To(BeEmpty())
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
		Expect(config.GetUserKey()).To(Equal(authv1alpha1.DefaultAuthProfileUserKey))
		Expect(config.GetPassKey()).To(Equal(authv1alpha1.DefaultAuthProfilePassKey))
		Expect(config.GetOAuthLoginConf()).To(BeNil())
	})

	It("should use the credential keys and login conf from the profile", func() {
		scope := "read write"
		endpoint := "/realms/aistore/protocol/openid-connect/token"
		config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
			UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
				Secret: authv1alpha1.AuthProfileSecret{
					Name: "admin", Namespace: "auth-config", UserKey: "username", PassKey: "password",
				},
				LoginConf: &authv1alpha1.AuthProfileLoginConf{
					ClientID: "ais-operator", Endpoint: endpoint, Scope: &scope,
				},
			},
		})
		Expect(config.GetUserKey()).To(Equal("username"))
		Expect(config.GetPassKey()).To(Equal("password"))
		Expect(config.GetOAuthLoginConf()).To(Equal(
			&services.OAuthLoginConf{ClientID: "ais-operator", Endpoint: endpoint, Scope: &scope},
		))
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

	Describe("client", func() {
		caConfigMapRef := &authv1alpha1.AuthProfileCAConfigMapRef{
			Namespace: "auth-config", Name: "auth-provider-ca", Key: "ca.crt",
		}

		It("should target the profile service over TLS", func() {
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
			})

			params, err := config.Client(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(params.URL).To(Equal("https://auth-provider.ais.svc:52001"))
			tlsConfig := clientTLSConfig(params)
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.InsecureSkipVerify).To(BeFalse())
		})

		It("should trust the CA certificate held in the referenced ConfigMap", func() {
			caCertPEM := createTestCACertPEM("profile-ca")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-provider-ca", Namespace: "auth-config"},
				Data:       map[string]string{"ca.crt": string(caCertPEM)},
			}
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
				TLS:        &authv1alpha1.AuthProfileTLSConfig{CAConfigMapRef: caConfigMapRef},
			}, configMap)

			params, err := config.Client(context.Background())
			Expect(err).NotTo(HaveOccurred())
			rootCAs := clientTLSConfig(params).RootCAs
			Expect(rootCAs).NotTo(BeNil())

			block, _ := pem.Decode(caCertPEM)
			Expect(block).NotTo(BeNil())
			caCert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())
			_, err = caCert.Verify(x509.VerifyOptions{
				Roots:     rootCAs,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip certificate verification when the profile requests it", func() {
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
				TLS:        &authv1alpha1.AuthProfileTLSConfig{InsecureSkipVerify: true},
			})

			params, err := config.Client(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(clientTLSConfig(params).InsecureSkipVerify).To(BeTrue())
		})

		It("should leave TLS unconfigured for an HTTP service URL", func() {
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "http://auth-provider.ais.svc:52001",
				TLS:        &authv1alpha1.AuthProfileTLSConfig{InsecureSkipVerify: true},
			})

			params, err := config.Client(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(params.URL).To(Equal("http://auth-provider.ais.svc:52001"))
			Expect(clientTLSConfig(params)).To(BeNil())
		})

		It("should fail when the referenced ConfigMap has no CA key", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-provider-ca", Namespace: "auth-config"},
			}
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
				TLS:        &authv1alpha1.AuthProfileTLSConfig{CAConfigMapRef: caConfigMapRef},
			}, configMap)

			_, err := config.Client(context.Background())
			Expect(err).To(MatchError(ContainSubstring(`has no key "ca.crt"`)))
		})

		It("should fail when the referenced ConfigMap is missing", func() {
			config := profileConfig(authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: "https://auth-provider.ais.svc:52001",
				TLS:        &authv1alpha1.AuthProfileTLSConfig{CAConfigMapRef: caConfigMapRef},
			})

			_, err := config.Client(context.Background())
			Expect(err).To(MatchError(ContainSubstring("failed to get TLS config for auth service")))
		})
	})
})

// profileConfig resolves a config from the given profile spec, with objs available to the operator's client.
func profileConfig(spec authv1alpha1.AIStoreAuthProfileSpec, objs ...client.Object) services.AuthConfig {
	GinkgoHelper()
	profile := &authv1alpha1.AIStoreAuthProfile{
		ObjectMeta: metav1.ObjectMeta{Name: testProfileName},
		Spec:       spec,
	}
	authClient := services.NewAuthClient(services.NewFakeK8sClient(append(objs, profile)...))
	config, err := authClient.ResolveAuthConfig(context.Background(), aisWithProfileRef(testProfileName))
	Expect(err).NotTo(HaveOccurred())
	return config
}

func clientTLSConfig(params *api.BaseParams) *tls.Config {
	GinkgoHelper()
	transport, ok := params.Client.Transport.(*http.Transport)
	Expect(ok).To(BeTrue())
	return transport.TLSClientConfig
}

// createTestCACertPEM creates a self-signed CA certificate in PEM format
func createTestCACertPEM(commonName string) []byte {
	GinkgoHelper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}
