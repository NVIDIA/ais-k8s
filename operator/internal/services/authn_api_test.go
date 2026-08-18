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
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/NVIDIA/aistore/api"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/opinfo"
	"github.com/ais-operator/internal/truststore"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("AuthN Base Params", func() {
	Describe("HTTP vs HTTPS", func() {
		It("should not configure TLS for HTTP URLs", func() {
			conf := &mockAuthConfig{
				serviceURL: "http://ais-authn.ais:52001",
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
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

			baseParams, err := newAuthBaseParams(context.Background(), conf)
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

			baseParams, err := newAuthBaseParams(context.Background(), conf)
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

			baseParams, err := newAuthBaseParams(context.Background(), conf)
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

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should gracefully handle missing CA files", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
				caCertPath: "/nonexistent/ca.crt",
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should use system CA certs when no custom CAs provided", func() {
			conf := &mockAuthConfig{
				serviceURL: "https://ais-authn.ais:52001",
				caCertPath: "",
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
		})

		It("should ignore CA certs for HTTP URLs", func() {
			conf := &mockAuthConfig{
				serviceURL: "http://ais-authn.ais:52001",
				caCertPath: caCertPath,
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
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

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("https://ais-authn.ais:52001"))
		})

		It("should disable certificate verification when skip verify is true", func() {
			conf := &mockAuthConfig{
				serviceURL:         "https://ais-authn.ais:52001",
				insecureSkipVerify: true,
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("https://ais-authn.ais:52001"))
		})

		It("should ignore skip verify for HTTP", func() {
			conf := &mockAuthConfig{
				serviceURL:         "http://ais-authn.ais:52001",
				insecureSkipVerify: true,
			}

			baseParams, err := newAuthBaseParams(context.Background(), conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseParams).NotTo(BeNil())
			Expect(baseParams.URL).To(Equal("http://ais-authn.ais:52001"))
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

var _ = Describe("Subject token", func() {
	const (
		operatorNamespace = "ais-operator-system"
		operatorSA        = "ais-operator-controller-manager"
	)

	// reviewedAs answers every SelfSubjectReview with the given username.
	reviewedAs := func(username string) client.Client {
		return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				review, ok := obj.(*authenticationv1.SelfSubjectReview)
				if !ok {
					return c.Create(ctx, obj, opts...)
				}
				review.Status.UserInfo = authenticationv1.UserInfo{Username: username}
				return nil
			},
		}).Build()
	}

	BeforeEach(func() {
		GinkgoT().Setenv("KUBERNETES_CLUSTER_DOMAIN", "cluster.local")
		Expect(opinfo.Resolve(context.Background(),
			reviewedAs("system:serviceaccount:"+operatorNamespace+":"+operatorSA))).To(Succeed())
	})

	It("should read the projected token when a token path is configured", func() {
		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("projected-token"), 0o600)).To(Succeed())

		authN := NewAuthNClient(newFakeK8sClient())
		token, err := authN.getSubjectToken(context.Background(), &mockAuthConfig{tokenPath: tokenPath})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("projected-token"))
	})

	When("no token path is configured", func() {
		var (
			authN   *AuthNClient
			request *authenticationv1.TokenRequest
			minted  client.ObjectKey
		)

		BeforeEach(func() {
			request = nil
			minted = client.ObjectKey{}
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: operatorSA, Namespace: operatorNamespace}}
			authN = NewAuthNClient(newFakeK8sClientWithInterceptors(&interceptor.Funcs{
				SubResourceCreate: func(ctx context.Context, c client.Client, subResource string,
					obj, body client.Object, opts ...client.SubResourceCreateOption,
				) error {
					minted = client.ObjectKeyFromObject(obj)
					request = body.(*authenticationv1.TokenRequest).DeepCopy()
					return c.SubResource(subResource).Create(ctx, obj, body, opts...)
				},
			}, sa))
		})

		It("should mint a token for the operator ServiceAccount", func() {
			token, err := authN.getSubjectToken(context.Background(), &mockAuthConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(minted).To(Equal(client.ObjectKey{Namespace: operatorNamespace, Name: operatorSA}))
		})

		It("should mint with the audience the provider requires", func() {
			token, err := authN.getSubjectToken(context.Background(), &mockAuthConfig{subjectTokenAud: "ais-authn"})
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(request).NotTo(BeNil())
			Expect(request.Spec.Audiences).To(Equal([]string{"ais-authn"}))
		})
	})

	It("should fail when the operator ServiceAccount does not exist", func() {
		authN := NewAuthNClient(newFakeK8sClient())
		_, err := authN.getSubjectToken(context.Background(), &mockAuthConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to mint token"))
	})
})

var _ = Describe("OAuth Password Login", func() {
	var (
		server      *httptest.Server
		requestPath string
	)

	BeforeEach(func() {
		requestPath = ""
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":300}`))
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	login := func(conf *OAuthLoginConf) (*TokenInfo, error) {
		params := &api.BaseParams{Client: server.Client(), URL: server.URL}
		return getTokenFromOAuth(context.Background(), params, credentials{user: "admin", pass: "secret"}, conf)
	}

	It("should post to the token endpoint under the service URL", func() {
		token, err := login(&OAuthLoginConf{ClientID: "AIStore", Endpoint: "/realms/aistore/protocol/openid-connect/token"})
		Expect(err).NotTo(HaveOccurred())
		Expect(token.Token).To(Equal("test-token"))
		Expect(requestPath).To(Equal("/realms/aistore/protocol/openid-connect/token"))
	})

	It("should post to the service URL when no endpoint is configured", func() {
		token, err := login(&OAuthLoginConf{ClientID: "AIStore"})
		Expect(err).NotTo(HaveOccurred())
		Expect(token.Token).To(Equal("test-token"))
		Expect(requestPath).To(Equal("/"))
	})
})

// Helper: mockAuthConfig implements AuthConfig interface for testing
type mockAuthConfig struct {
	serviceURL         string
	isTokenExchange    bool
	tokenPath          string
	subjectTokenAud    string
	tokenExchangeEP    string
	secretName         string
	secretNamespace    string
	caCertPath         string
	insecureSkipVerify bool
	tls                tlsCache
}

func (m *mockAuthConfig) GetServiceURL() string {
	return m.serviceURL
}

func (m *mockAuthConfig) IsTokenExchange() bool {
	return m.isTokenExchange
}

func (m *mockAuthConfig) GetTokenPath() string {
	return m.tokenPath
}

func (m *mockAuthConfig) GetSubjectTokenAudience() string {
	return m.subjectTokenAud
}

func (m *mockAuthConfig) GetTokenExchangeEndpoint() string {
	if m.tokenExchangeEP == "" {
		return DefaultTokenExchangeEndpoint
	}
	return m.tokenExchangeEP
}

func (*mockAuthConfig) GetOAuthLoginConf() *OAuthLoginConf {
	return nil
}

func (*mockAuthConfig) GetUserKey() string { return AuthNSecretRefName }

func (*mockAuthConfig) GetPassKey() string { return AuthNSecretRefPass }

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
	return m.tls.get(ctx, func(context.Context) (truststore.Config, error) {
		var caCertPaths []string
		if m.caCertPath != "" {
			caCertPaths = []string{m.caCertPath}
		}
		return truststore.Config{CACertPaths: caCertPaths}, nil
	}, m.insecureSkipVerify)
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
