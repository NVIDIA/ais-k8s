// Package services contains services for the operator to use when reconciling AIS
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/truststore"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("AuthN Base Params", func() {
	var ctx context.Context

	BeforeEach(func() {
		baseCtx := context.Background() //nolint:fatcontext // Test setup requires context creation in BeforeEach
		ctx = logf.IntoContext(baseCtx, zap.New(zap.UseDevMode(true)))
	})

	Describe("HTTP vs HTTPS", func() {
		It("should not configure TLS for HTTP URLs", func() {
			conf := &mockAuthConfig{
				serviceURL: "http://ais-authn.ais:52001",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())

			transport := baseParams.Client.Transport
			Expect(transport).NotTo(BeNil())

			// Type assert to get TLSClientConfig
			if httpTransport, ok := transport.(interface{ TLSClientConfig() *interface{} }); ok {
				tlsConfig := httpTransport.TLSClientConfig()
				Expect(tlsConfig).To(BeNil())
			}
		})

		It("should configure TLS for HTTPS URLs", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())

			transport := baseParams.Client.Transport
			Expect(transport).NotTo(BeNil())

			// Type assert to get TLSClientConfig
			if httpTransport, ok := transport.(interface{ TLSClientConfig() *interface{} }); ok {
				tlsConfig := httpTransport.TLSClientConfig()
				Expect(tlsConfig).NotTo(BeNil())
			}
		})

		It("should configure TLS for HTTPS URLs without port", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())

			transport := baseParams.Client.Transport
			Expect(transport).NotTo(BeNil())

			// Type assert to get TLSClientConfig
			if httpTransport, ok := transport.(interface{ TLSClientConfig() *interface{} }); ok {
				tlsConfig := httpTransport.TLSClientConfig()
				Expect(tlsConfig).NotTo(BeNil())
			}
		})

		It("should not configure TLS for HTTP URLs without port", func() {
			conf := &mockAuthConfig{
				serviceURL: "http://ais-authn.ais",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())

			transport := baseParams.Client.Transport
			Expect(transport).NotTo(BeNil())

			// Type assert to get TLSClientConfig
			if httpTransport, ok := transport.(interface{ TLSClientConfig() *interface{} }); ok {
				tlsConfig := httpTransport.TLSClientConfig()
				Expect(tlsConfig).To(BeNil())
			}
		})
	})

	Describe("Custom CA Certificates", func() {
		var tmpDir string
		var caCertPath string

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			caCertPath = filepath.Join(tmpDir, "ca.crt")
			caCertPEM := createTestCACertPEM("test-ca")
			err := os.WriteFile(caCertPath, caCertPEM, 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should load CA certificate", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
				caCertPath: caCertPath,
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should gracefully handle missing CA files", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
				caCertPath: "/nonexistent/ca.crt",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should use system CA certs when no custom CAs provided", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
				caCertPath: "",
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should ignore CA certs for HTTP URLs", func() {
			conf := &mockAuthConfig{
				serviceURL: "http://ais-authn.ais:52001",
				caCertPath: caCertPath,
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})
	})

	Describe("InsecureSkipVerify", func() {
		It("should enable certificate verification when skip verify is false", func() {
			conf := &mockAuthConfig{
				serviceURL:         "https://ais-authn.ais:52001",
				insecureSkipVerify: false,
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("https://ais-authn.ais:52001"))
		})

		It("should disable certificate verification when skip verify is true", func() {
			conf := &mockAuthConfig{
				serviceURL:         "https://ais-authn.ais:52001",
				insecureSkipVerify: true,
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("https://ais-authn.ais:52001"))
		})

		It("should ignore skip verify for HTTP", func() {
			conf := &mockAuthConfig{
				serviceURL:         "http://ais-authn.ais:52001",
				insecureSkipVerify: true,
			}

			baseParams, err := newAuthBaseParams(ctx, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("http://ais-authn.ais:52001"))
		})
	})
})

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

var _ = Describe("ReadTokenFromFile", func() {
	var tmpDir string

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
	})

	It("should read valid token file", func() {
		tokenPath := filepath.Join(tmpDir, "token")
		expectedToken := "test-token-12345"

		err := os.WriteFile(tokenPath, []byte(expectedToken), 0o600)
		Expect(err).NotTo(HaveOccurred())

		token, err := readTokenFromFile(tokenPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal(expectedToken))
	})

	It("should trim whitespace from token", func() {
		tokenPath := filepath.Join(tmpDir, "token")
		tokenWithWhitespace := "  test-token-12345  \n"

		err := os.WriteFile(tokenPath, []byte(tokenWithWhitespace), 0o600)
		Expect(err).NotTo(HaveOccurred())

		token, err := readTokenFromFile(tokenPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("test-token-12345"))
	})

	It("should return error for empty token file", func() {
		tokenPath := filepath.Join(tmpDir, "token")

		err := os.WriteFile(tokenPath, []byte(""), 0o600)
		Expect(err).NotTo(HaveOccurred())

		_, err = readTokenFromFile(tokenPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("token file is empty"))
	})

	It("should return error for non-existent token file", func() {
		_, err := readTokenFromFile("/nonexistent/token")
		Expect(err).To(HaveOccurred())
	})
})

// Helper: mockAuthConfig implements AuthConfig interface for testing
type mockAuthConfig struct {
	serviceURL         string
	isTokenExchange    bool
	tokenPath          string
	tokenExchangeEP    string
	secretName         string
	secretNamespace    string
	caCertPath         string
	insecureSkipVerify bool
	// TLS config caching
	tlsConfig  *tls.Config
	tlsCreated time.Time
	tlsMu      sync.RWMutex
}

func (m *mockAuthConfig) GetServiceURL() string {
	return m.serviceURL
}

func (m *mockAuthConfig) IsTokenExchange() bool {
	return m.isTokenExchange
}

func (m *mockAuthConfig) GetTokenPath() string {
	if m.tokenPath == "" {
		return DefaultTokenPath
	}
	return m.tokenPath
}

func (m *mockAuthConfig) GetTokenExchangeEndpoint() string {
	if m.tokenExchangeEP == "" {
		return DefaultTokenExchangeEndpoint
	}
	return m.tokenExchangeEP
}

func (*mockAuthConfig) GetOAuthLoginConf() *aisv1.AuthServerLoginConf {
	return nil
}

func (m *mockAuthConfig) GetSecretName() string {
	return m.secretName
}

func (m *mockAuthConfig) GetSecretNamespace() string {
	return m.secretNamespace
}

func (m *mockAuthConfig) GetCACertPath() string {
	return m.caCertPath
}

func (m *mockAuthConfig) GetInsecureSkipVerify() bool {
	return m.insecureSkipVerify
}

func (m *mockAuthConfig) GetTLSConfig(ctx context.Context) (*tls.Config, error) {
	logger := logf.FromContext(ctx)
	cacheTTL := getTLSConfigCacheTTL(ctx)

	m.tlsMu.RLock()
	// Check if we have a valid cached config
	if m.tlsConfig != nil && time.Since(m.tlsCreated) < cacheTTL {
		tlsConfig := m.tlsConfig
		m.tlsMu.RUnlock()
		logger.V(2).Info("Using cached TLS config", "age", time.Since(m.tlsCreated), "ttl", cacheTTL)
		return tlsConfig, nil
	}
	m.tlsMu.RUnlock()

	// Need to create/refresh TLS config
	m.tlsMu.Lock()
	defer m.tlsMu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if m.tlsConfig != nil && time.Since(m.tlsCreated) < cacheTTL {
		logger.V(2).Info("Using cached TLS config (after lock)", "age", time.Since(m.tlsCreated), "ttl", cacheTTL)
		return m.tlsConfig, nil
	}

	// Create new TLS config
	caCertPath := m.GetCACertPath()
	var caCertPaths []string
	if caCertPath != "" {
		caCertPaths = []string{caCertPath}
	}
	logger.V(1).Info("Creating new TLS config", "caCertPath", caCertPath)
	tlsConfig, err := truststore.NewTLSConfig(logger.WithName("truststore"), truststore.Config{
		CACertPaths: caCertPaths,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	// Apply insecureSkipVerify if configured
	if m.GetInsecureSkipVerify() {
		logger.Info("WARNING: TLS certificate verification disabled (insecureSkipVerify=true)")
		tlsConfig.InsecureSkipVerify = true
	}

	// Cache the new config
	m.tlsConfig = tlsConfig
	m.tlsCreated = time.Now()

	return tlsConfig, nil
}

// Helper: createTestCACertPEM creates a test CA certificate in PEM format
func createTestCACertPEM(commonName string) []byte {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	// Create certificate template
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

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM
}

var _ = Describe("GetRequiredAudiences", func() {
	It("should return nil when ConfigToUpdate is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(BeNil())
	})

	It("should return nil when Auth is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(BeNil())
	})

	It("should return nil when RequiredClaims is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{
					Auth: &aisv1.AuthConfToUpdate{},
				},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(BeNil())
	})

	It("should return nil when Aud slice is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{
					Auth: &aisv1.AuthConfToUpdate{
						RequiredClaims: &aisv1.RequiredClaimsConfToUpdate{
							Aud: nil,
						},
					},
				},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(BeNil())
	})

	It("should return empty slice when Aud slice is empty", func() {
		var emptyAud []string
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{
					Auth: &aisv1.AuthConfToUpdate{
						RequiredClaims: &aisv1.RequiredClaimsConfToUpdate{
							Aud: &emptyAud,
						},
					},
				},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(Equal(emptyAud))
	})

	It("should return single audience when one is configured", func() {
		expectedAudience := "namespace/cluster-name"
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{
					Auth: &aisv1.AuthConfToUpdate{
						RequiredClaims: &aisv1.RequiredClaimsConfToUpdate{
							Aud: &[]string{expectedAudience},
						},
					},
				},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(HaveLen(1))
		Expect(audiences[0]).To(Equal(expectedAudience))
	})

	It("should return all audiences when multiple are configured", func() {
		expectedAudiences := []string{
			"namespace/cluster-name",
			"admin",
			"global-access",
		}
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{
					Auth: &aisv1.AuthConfToUpdate{
						RequiredClaims: &aisv1.RequiredClaimsConfToUpdate{
							Aud: &expectedAudiences,
						},
					},
				},
			},
		}

		audiences := ais.GetRequiredAudiences()
		Expect(audiences).To(HaveLen(3))
		Expect(audiences).To(Equal(expectedAudiences))
	})
})
