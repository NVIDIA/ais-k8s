/*
 * Copyright (c) 2025-2026, NVIDIA CORPORATION. All rights reserved.
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
			authClient *AuthClient
			request    *authenticationv1.TokenRequest
			minted     client.ObjectKey
		)

		BeforeEach(func() {
			request = nil
			minted = client.ObjectKey{}
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: operatorSA, Namespace: operatorNamespace}}
			authClient = NewAuthClient(NewFakeK8sClientWithInterceptors(&interceptor.Funcs{
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
			token, err := authClient.mintSubjectToken(context.Background(), "ais-auth")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(minted).To(Equal(client.ObjectKey{Namespace: operatorNamespace, Name: operatorSA}))
		})

		It("should mint with the audience the provider requires", func() {
			token, err := authClient.mintSubjectToken(context.Background(), "ais-auth")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
			Expect(request).NotTo(BeNil())
			Expect(request.Spec.Audiences).To(Equal([]string{"ais-auth"}))
		})

		It("should refuse to mint without an audience", func() {
			_, err := authClient.mintSubjectToken(context.Background(), "")
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

			tokenInfo, err := authClient.getTokenViaExchange(context.Background(), params, &aisv1.AIStore{}, conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenInfo.Token).To(Equal("exchanged"))
			Expect(request).NotTo(BeNil())
			Expect(request.Spec.Audiences).To(Equal([]string{DefaultSubjectTokenAudience}))
		})

	})

	It("should fail when the operator ServiceAccount does not exist", func() {
		authClient := NewAuthClient(NewFakeK8sClient())
		_, err := authClient.mintSubjectToken(context.Background(), "ais-auth")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to mint token"))
	})
})

var _ = Describe("Token exchange scope", func() {
	DescribeTable("should send the configured scope",
		func(configured, expected string) {
			var exchangedScope string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				exchangedScope = r.FormValue("scope")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"exchanged","token_type":"Bearer",` +
					`"issued_token_type":"urn:ietf:params:oauth:token-type:jwt"}`))
			}))
			defer server.Close()

			params := &api.BaseParams{Client: server.Client(), URL: server.URL}
			_, err := exchangeTokenWithAuthSvc(context.Background(), params, "subject-token", "/token", configured, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(exchangedScope).To(Equal(expected))
		},
		Entry("when omitted", "", ""),
		Entry("when provided", "k8sSA:Admin", "k8sSA:Admin"),
	)
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

var _ = Describe("Auth profile generation", func() {
	const profileName = "prod-auth"

	newProfile := func() *authv1alpha1.AIStoreAuthProfile {
		return &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: profileName, UID: "8c1d1f2e-5a44-4c9b-8f1e-6d2a0b3c9d10", Generation: 1},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL:    "https://prod-auth.ais.svc:52001",
				TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
			},
		}
	}

	newCluster := func(ref string) *aisv1.AIStore {
		ais := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"}}
		if ref != "" {
			ais.Spec.Auth = &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: ref}}
		}
		return ais
	}

	It("should be empty for a cluster with no auth", func() {
		authClient := NewAuthClient(NewFakeK8sClient())

		gen, err := authClient.profileGeneration(context.Background(), newCluster(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(gen).To(BeEmpty())
	})

	It("should change when the profile is recreated under the same name", func() {
		ctx := context.Background()
		ais := newCluster(profileName)

		before, err := NewAuthClient(NewFakeK8sClient(newProfile())).profileGeneration(ctx, ais)
		Expect(err).NotTo(HaveOccurred())

		recreated := newProfile()
		recreated.UID = "f4b0a6c7-9e83-4d52-bb17-2c7f5a1e4408"
		after, err := NewAuthClient(NewFakeK8sClient(recreated)).profileGeneration(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).NotTo(Equal(before))
	})
})
