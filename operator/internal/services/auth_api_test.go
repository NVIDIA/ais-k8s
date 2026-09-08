/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services_test

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/services"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ResolveAuthConfig", func() {
	It("should resolve auth from the referenced AIStoreAuthProfile", func() {
		profile := &authv1alpha1.AIStoreAuthProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-auth"},
			Spec: authv1alpha1.AIStoreAuthProfileSpec{
				ServiceURL:    "https://prod-auth.ais.svc:52001",
				TokenExchange: &authv1alpha1.AuthProfileTokenExchange{Endpoint: "/exchange"},
			},
		}
		authClient := services.NewAuthClient(services.NewFakeK8sClient(profile))

		config, err := authClient.ResolveAuthConfig(context.Background(), aisWithProfileRef("prod-auth"))
		Expect(err).NotTo(HaveOccurred())
		Expect(config.GetServiceURL()).To(Equal("https://prod-auth.ais.svc:52001"))
		Expect(config.IsTokenExchange()).To(BeTrue())
		Expect(config.GetTokenExchangeEndpoint()).To(Equal("/exchange"))
	})

	It("should safely return empty auth config if given a nil auth spec", func() {
		authClient := services.NewAuthClient(services.NewFakeK8sClient())
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec:       aisv1.AIStoreSpec{},
		}

		config, err := authClient.ResolveAuthConfig(context.Background(), ais)
		Expect(err).To(BeNil())
		Expect(config).To(BeNil())
	})

	It("should surface an error when no profile is referenced", func() {
		authClient := services.NewAuthClient(services.NewFakeK8sClient())
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
			Spec: aisv1.AIStoreSpec{
				Auth: &aisv1.AuthSpec{},
			},
		}

		config, err := authClient.ResolveAuthConfig(context.Background(), ais)
		Expect(err).To(MatchError(ContainSubstring(`no profileRef specified`)))
		Expect(config).To(BeNil())
	})

	It("should surface an error when the referenced profile does not exist", func() {
		authClient := services.NewAuthClient(services.NewFakeK8sClient())

		_, err := authClient.ResolveAuthConfig(context.Background(), aisWithProfileRef("missing-profile"))
		Expect(err).To(MatchError(ContainSubstring(`failed to get AIStoreAuthProfile "missing-profile"`)))
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})

func aisWithProfileRef(name string) *aisv1.AIStore {
	return &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "tenant"},
		Spec: aisv1.AIStoreSpec{
			Auth: &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: name}},
		},
	}
}
