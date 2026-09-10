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
)

var _ = Describe("AIS client manager token handling", func() {
	const (
		profileName = "prod-auth"
		secretName  = "auth-creds" //nolint:gosec // name of a Secret, not a credential
		namespace   = "tenant"
	)

	It("should replace a rejected token on the cached client", func() {
		ctx := context.Background()
		var logins atomic.Int32
		// Every login gets its own token, so the test can tell one fetch from the next
		authSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			issued := logins.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":3600}`, issued)
		}))
		defer authSvc.Close()

		profile := &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: profileName},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL: authSvc.URL,
				UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
					Secret:    authv1alpha1.AuthProfileSecret{Name: secretName, Namespace: namespace},
					LoginConf: &authv1alpha1.AuthProfileLoginConf{ClientID: "AIStore", Endpoint: "/token"},
				},
			},
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{
				authv1alpha1.DefaultAuthProfileUserKey: []byte("admin"),
				authv1alpha1.DefaultAuthProfilePassKey: []byte("secret"),
			},
		}
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: namespace},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: profileName}},
			},
		}
		manager := NewAISClientManager(NewFakeK8sClient(profile, secret), AISClientTLSOpts{})

		// Cache a client that AIS has rejected, as an earlier reconcile would leave it
		obtainedAt := time.Now()
		rejected := &TokenInfo{Token: "token-0", ObtainedAt: obtainedAt, ExpiresAt: obtainedAt.Add(time.Hour)}
		cached := NewAIStoreClient(ctx, cmn.IntraClusterURL(ais), rejected, ais.GetAPIMode(), nil)
		manager.clientMap[ais.NamespacedName().String()] = cached
		cached.tokenRejected.Store(true)

		got, err := manager.GetClient(ctx, ais)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeIdenticalTo(cached))
		Expect(cached.params.Token).To(Equal("token-1"))
		Expect(cached.tokenRejected.Load()).To(BeFalse())
		Expect(logins.Load()).To(BeEquivalentTo(1))
	})
})
