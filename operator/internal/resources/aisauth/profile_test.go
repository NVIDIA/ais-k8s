/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AuthProfile", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "ais-authn", Namespace: "ais"},
			Spec: authv1alpha1.AIStoreAuthSpec{
				AdminSecret: &corev1.LocalObjectReference{Name: "ais-authn-su-creds"},
			},
		}
	})

	It("builds the admin profile for the in-cluster Service", func() {
		profile := authnres.NewAuthProfile(authn)

		Expect(profile.APIVersion).To(Equal("auth.ais.nvidia.com/v1alpha1"))
		Expect(profile.Kind).To(Equal("AIStoreAuthProfile"))
		Expect(profile.Name).To(Equal("ais.ais-authn-admin"))
		Expect(profile.Labels).To(Equal(map[string]string{
			"app.kubernetes.io/name":         "authn",
			"app.kubernetes.io/instance":     "ais-authn",
			"app.kubernetes.io/managed-by":   "ais-operator",
			"ais.nvidia.com/owner-kind":      "AIStoreAuth",
			"ais.nvidia.com/owner-namespace": "ais",
			"ais.nvidia.com/owner-name":      "ais-authn",
		}))
		Expect(profile.Spec).To(Equal(authv1alpha1.AIStoreAuthProfileSpec{
			ServiceURL: "http://ais-authn.ais.svc:52001",
			UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
				Secret: authv1alpha1.AuthProfileSecret{
					Name:      "ais-authn-su-creds",
					Namespace: "ais",
					UserKey:   "SU-NAME",
					PassKey:   "SU-PASS",
				},
			},
		}))
	})
})
