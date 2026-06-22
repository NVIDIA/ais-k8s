/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ConfigMap", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Deployment: authv1alpha1.DeploymentSpec{
					Image: "docker.io/aistorage/authn:v4.5",
				},
			},
		}
	})

	It("names the ConfigMap after the CR", func() {
		Expect(authnres.ConfigMapName(authn)).To(Equal("ais-authn-config"))
		Expect(authnres.ConfigMapNSName(authn).Name).To(Equal("ais-authn-config"))
		Expect(authnres.ConfigMapNSName(authn).Namespace).To(Equal("ais"))
	})

	It("creates an owned ConfigMap with standard labels", func() {
		cm, err := authnres.NewConfigMap(authn)
		Expect(err).NotTo(HaveOccurred())

		Expect(cm.Labels).To(Equal(map[string]string{
			"app.kubernetes.io/name":       "authn",
			"app.kubernetes.io/instance":   "ais-authn",
			"app.kubernetes.io/managed-by": "ais-operator",
		}))
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(*cm.OwnerReferences[0].Name).To(Equal(authn.Name))
		Expect(*cm.OwnerReferences[0].Controller).To(BeTrue())
		Expect(cm.Data).To(HaveKey(authnres.AuthnJSONKey))
	})

})
