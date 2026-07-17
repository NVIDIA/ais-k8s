/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"context"
	"testing"

	authv1 "github.com/ais-operator/api/aisauth/v1alpha1"
	. "github.com/onsi/gomega"
)

func TestAIStoreAuthProfileWebhook(t *testing.T) {
	webhook := &AIStoreAuthProfileWebhook{}
	ctx := context.Background()

	t.Run("accepts https without warnings", func(t *testing.T) {
		g := NewWithT(t)
		warnings, err := webhook.ValidateCreate(ctx, &authv1.AIStoreAuthProfile{
			Spec: authv1.AIStoreAuthProfileSpec{ServiceURL: "https://auth.example.com"},
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})

	t.Run("rejects invalid spec", func(t *testing.T) {
		g := NewWithT(t)
		_, err := webhook.ValidateCreate(ctx, &authv1.AIStoreAuthProfile{})
		g.Expect(err).To(HaveOccurred())
	})

	for _, tt := range []struct {
		name    string
		profile *authv1.AIStoreAuthProfile
	}{
		{
			name: "http serviceURL",
			profile: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{ServiceURL: "http://auth.example.com"},
			},
		},
		{
			name: "insecureSkipVerify",
			profile: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL: "https://auth.example.com",
					TLS:        &authv1.AuthProfileTLSConfig{InsecureSkipVerify: true},
				},
			},
		},
	} {
		t.Run("warns on "+tt.name+" at create only", func(t *testing.T) {
			g := NewWithT(t)
			warnings, err := webhook.ValidateCreate(ctx, tt.profile)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(warnings).NotTo(BeEmpty())

			warnings, err = webhook.ValidateUpdate(ctx, tt.profile, tt.profile)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
