/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AIS client manager token handling", func() {
	const (
		profileName = "prod-auth"
		profileUID  = "3f0a8b0e-2c1d-4f7a-9a51-1b6d0c4e77aa"
		secretName  = "auth-creds" //nolint:gosec // name of a Secret, not a credential
		namespace   = "tenant"
	)

	var (
		ctx      context.Context
		requests atomic.Int32
		server   *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
		requests.Store(0)
		// Every login gets its own token, so a test can tell one fetch from the next
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			issued := requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":3600}`, issued)
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	newProfile := func(serviceURL string, generation int64) *authv1alpha1.AIStoreAuthProfile {
		return &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: profileName, UID: profileUID, Generation: generation},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: serviceURL,
				UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
					Secret:    authv1alpha1.AuthProfileSecret{Name: secretName, Namespace: namespace},
					LoginConf: &authv1alpha1.AuthProfileLoginConf{ClientID: "AIStore", Endpoint: "/token"},
				},
			},
		}
	}

	newSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{
				authv1alpha1.DefaultAuthProfileUserKey: []byte("admin"),
				authv1alpha1.DefaultAuthProfilePassKey: []byte("secret"),
			},
		}
	}

	newCluster := func(withAuth bool) *aisv1.AIStore {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: namespace},
		}
		if withAuth {
			ais.Spec.Auth = &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: profileName}}
		}
		return ais
	}

	newManager := func(objs ...client.Object) *AISClientManager {
		return NewAISClientManager(NewFakeK8sClient(objs...), AISClientTLSOpts{})
	}

	// cacheClient puts a client reaching url and holding tokenInfo in the manager's cache, as an
	// earlier reconcile would
	cacheClient := func(m *AISClientManager, ais *aisv1.AIStore, url string, tokenInfo *TokenInfo) *AIStoreClient {
		cached := NewAIStoreClient(ctx, url, tokenInfo, ais.GetAPIMode(), nil)
		m.clientMap[ais.NamespacedName().String()] = cached
		return cached
	}

	// profileGen is the value the operator stamps onto a token that this generation of the profile issued
	profileGen := func(generation int64) string {
		return fmt.Sprintf("%s/%s@%d", profileName, profileUID, generation)
	}

	// currentToken describes a token that the first generation of the profile issued
	currentToken := func() *TokenInfo {
		obtainedAt := time.Now()
		return &TokenInfo{
			Token:      "token-0",
			ObtainedAt: obtainedAt,
			ExpiresAt:  obtainedAt.Add(time.Hour),
			ProfileGen: profileGen(1),
		}
	}

	It("should refetch the token after the profile generation advances", func() {
		ais := newCluster(true)
		m := newManager(newProfile(server.URL, 2), newSecret())
		cached := cacheClient(m, ais, cmn.IntraClusterURL(ais), currentToken())

		got, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeIdenticalTo(cached))
		Expect(cached.params.Token).To(Equal("token-1"))
		Expect(cached.tokenInfo.ProfileGen).To(Equal(profileGen(2)))
		Expect(requests.Load()).To(BeEquivalentTo(1))
	})

	It("should replace a rejected token on the cached client", func() {
		ais := newCluster(true)
		m := newManager(newProfile(server.URL, 1), newSecret())
		cached := cacheClient(m, ais, cmn.IntraClusterURL(ais), currentToken())
		cached.tokenRejected.Store(true)

		got, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeIdenticalTo(cached))
		Expect(cached.params.Token).To(Equal("token-1"))
		Expect(cached.tokenRejected.Load()).To(BeFalse())
		Expect(requests.Load()).To(BeEquivalentTo(1))
	})

	It("should drop the token when the cluster stops requesting auth", func() {
		ais := newCluster(false)
		m := newManager()
		cached := cacheClient(m, ais, cmn.IntraClusterURL(ais), currentToken())

		got, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeIdenticalTo(cached))
		Expect(cached.params.Token).To(BeEmpty())
		Expect(requests.Load()).To(BeZero())
	})

	It("should keep the current token when the auth service rejects the login", func() {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failing.Close()
		ais := newCluster(true)
		m := newManager(newProfile(failing.URL, 2), newSecret())
		cached := cacheClient(m, ais, cmn.IntraClusterURL(ais), currentToken())

		_, err := m.GetClient(ctx, ais)
		Expect(err).To(HaveOccurred())
		Expect(cached.params.Token).To(Equal("token-0"))
	})

	It("should fail without a login attempt when the referenced profile is gone", func() {
		ais := newCluster(true)
		m := newManager(newSecret())
		cached := cacheClient(m, ais, cmn.IntraClusterURL(ais), currentToken())

		_, err := m.GetClient(ctx, ais)
		Expect(err).To(MatchError(ContainSubstring(`failed to get AIStoreAuthProfile "prod-auth"`)))
		Expect(cached.params.Token).To(Equal("token-0"))
		Expect(requests.Load()).To(BeZero())
	})

	It("should fetch a single token when it replaces a client the spec invalidated", func() {
		ais := newCluster(true)
		m := newManager(newProfile(server.URL, 1), newSecret())
		obtainedAt := time.Now().Add(-time.Hour)
		expired := &TokenInfo{
			Token:      "token-0",
			ObtainedAt: obtainedAt,
			ExpiresAt:  time.Now().Add(-time.Minute),
			ProfileGen: profileGen(1),
		}
		// An endpoint the spec no longer resolves to forces a replacement rather than a refresh
		stale := cacheClient(m, ais, "http://stale.example:51080", expired)

		got, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).NotTo(BeIdenticalTo(stale))
		Expect(m.clientMap[ais.NamespacedName().String()].params.Token).To(Equal("token-1"))
		Expect(requests.Load()).To(BeEquivalentTo(1))
	})

	It("should create and cache a client stamped with the profile generation", func() {
		ais := newCluster(true)
		m := newManager(newProfile(server.URL, 3), newSecret())

		created, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		cached := m.clientMap[ais.NamespacedName().String()]
		Expect(created).To(BeIdenticalTo(cached))
		Expect(cached.params.Token).To(Equal("token-1"))
		Expect(cached.tokenInfo.ProfileGen).To(Equal(profileGen(3)))
		Expect(requests.Load()).To(BeEquivalentTo(1))

		reused, err := m.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(reused).To(BeIdenticalTo(cached))
		Expect(requests.Load()).To(BeEquivalentTo(1))
	})
})
