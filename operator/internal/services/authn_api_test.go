/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/NVIDIA/aistore/api"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/opinfo"
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
		Expect(opinfo.ResolveServiceAccount(context.Background(),
			reviewedAs("system:serviceaccount:"+operatorNamespace+":"+operatorSA))).To(Succeed())
	})

	When("the operator ServiceAccount exists", func() {
		var (
			authN   *AuthNClient
			request *authenticationv1.TokenRequest
			minted  client.ObjectKey
		)

		BeforeEach(func() {
			request = nil
			minted = client.ObjectKey{}
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: operatorSA, Namespace: operatorNamespace}}
			authN = NewAuthNClient(NewFakeK8sClientWithInterceptors(&interceptor.Funcs{
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
			token, err := authN.mintSubjectToken(context.Background(), "ais-authn")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(minted).To(Equal(client.ObjectKey{Namespace: operatorNamespace, Name: operatorSA}))
		})

		It("should mint with the audience the provider requires", func() {
			token, err := authN.mintSubjectToken(context.Background(), "ais-authn")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(request).NotTo(BeNil())
			Expect(request.Spec.Audiences).To(Equal([]string{"ais-authn"}))
		})

		It("should refuse to mint without an audience", func() {
			_, err := authN.mintSubjectToken(context.Background(), "")
			Expect(err).To(MatchError(ContainSubstring("audience is required")))
			Expect(request).To(BeNil())
		})

		It("should exchange with the default audience when the profile sets none", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"exchanged","token_type":"Bearer",` +
					`"issued_token_type":"urn:ietf:params:oauth:token-type:jwt"}`))
			}))
			defer server.Close()

			conf := &authProfileConfig{profile: &authv1alpha1.AIStoreAuthProfile{
				Spec: authv1alpha1.AIStoreAuthProfileSpec{
					TokenExchange: &authv1alpha1.AuthProfileTokenExchange{},
				},
			}}
			params := &api.BaseParams{Client: server.Client(), URL: server.URL}

			tokenInfo, err := authN.getTokenViaExchange(context.Background(), params, &aisv1.AIStore{}, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenInfo.Token).To(Equal("exchanged"))
			Expect(request).NotTo(BeNil())
			Expect(request.Spec.Audiences).To(Equal([]string{DefaultSubjectTokenAudience}))
		})
	})

	It("should fail when the operator ServiceAccount does not exist", func() {
		authN := NewAuthNClient(NewFakeK8sClient())
		_, err := authN.mintSubjectToken(context.Background(), "ais-authn")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to mint token"))
	})
})

var _ = Describe("OAuth Password Login", func() {
	var (
		server       *httptest.Server
		requestPath  string
		responseBody string
	)

	BeforeEach(func() {
		requestPath = ""
		responseBody = `{"access_token":"test-token","token_type":"Bearer","expires_in":300}`
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responseBody))
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

	It("should set the expiration from expires_in", func() {
		token, err := login(&OAuthLoginConf{ClientID: "AIStore"})
		Expect(err).NotTo(HaveOccurred())
		Expect(token.ObtainedAt).To(BeTemporally("~", time.Now(), time.Minute))
		Expect(token.ExpiresAt.Sub(token.ObtainedAt)).To(Equal(300 * time.Second))
	})

	It("should leave the expiration unset when expires_in is omitted", func() {
		responseBody = `{"access_token":"test-token","token_type":"Bearer"}`
		token, err := login(&OAuthLoginConf{ClientID: "AIStore"})
		Expect(err).NotTo(HaveOccurred())
		Expect(token.ExpiresAt.IsZero()).To(BeTrue())
	})
})

var _ = Describe("RequiredAudiences", func() {
	It("should return nil when ConfigToUpdate is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{},
		}

		audiences := ais.RequiredAudiences()
		Expect(audiences).To(BeNil())
	})

	It("should return nil when Auth is nil", func() {
		ais := &aisv1.AIStore{
			Spec: aisv1.AIStoreSpec{
				ConfigToUpdate: &aisv1.ConfigToUpdate{},
			},
		}

		audiences := ais.RequiredAudiences()
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

		audiences := ais.RequiredAudiences()
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

		audiences := ais.RequiredAudiences()
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

		audiences := ais.RequiredAudiences()
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

		audiences := ais.RequiredAudiences()
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

		audiences := ais.RequiredAudiences()
		Expect(audiences).To(HaveLen(3))
		Expect(audiences).To(Equal(expectedAudiences))
	})
})
