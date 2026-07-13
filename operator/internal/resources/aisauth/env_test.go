/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Secret environment", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Deployment: authv1alpha1.DeploymentSpec{
					Image: "docker.io/aistorage/authn:v4.8",
				},
			},
		}
	})

	It("omits admin variables when adminSecret is absent for a preinitialized database", func() {
		Expect(newContainer(authn).Env).To(BeEmpty())
	})

	It("ignores explicitly empty Secret references", func() {
		authn.Spec.AdminSecret = &corev1.LocalObjectReference{}
		authn.Spec.HMACSecret = &corev1.LocalObjectReference{}
		authn.Spec.RSAPassphraseSecret = &corev1.LocalObjectReference{}
		Expect(newContainer(authn).Env).To(BeEmpty())
	})

	It("wires superuser credentials and permits AuthN to default a missing username", func() {
		authn.Spec.AdminSecret = &corev1.LocalObjectReference{Name: "su-creds"}
		env := newContainer(authn).Env

		Expect(env).To(HaveLen(2))
		Expect(env[0].Name).To(HaveValue(Equal("AIS_AUTHN_SU_NAME")))
		Expect(env[0].ValueFrom.SecretKeyRef.Name).To(HaveValue(Equal("su-creds")))
		Expect(env[0].ValueFrom.SecretKeyRef.Key).To(HaveValue(Equal("SU-NAME")))
		Expect(env[0].ValueFrom.SecretKeyRef.Optional).To(HaveValue(BeTrue()))
		Expect(env[1].Name).To(HaveValue(Equal("AIS_AUTHN_SU_PASS")))
		Expect(env[1].ValueFrom.SecretKeyRef.Key).To(HaveValue(Equal("SU-PASS")))
		Expect(env[1].ValueFrom.SecretKeyRef.Optional).To(BeNil())
	})

	It("wires HMAC signing from the external Secret", func() {
		authn.Spec.HMACSecret = &corev1.LocalObjectReference{Name: "signing-key"}
		env := newContainer(authn).Env

		Expect(env).To(HaveLen(1))
		Expect(env[0].Name).To(HaveValue(Equal("AIS_AUTHN_SECRET_KEY")))
		Expect(env[0].ValueFrom.SecretKeyRef.Name).To(HaveValue(Equal("signing-key")))
		Expect(env[0].ValueFrom.SecretKeyRef.Key).To(HaveValue(Equal("SIGNING-KEY")))
	})

	It("wires the optional RSA passphrase from the external Secret", func() {
		authn.Spec.RSAPassphraseSecret = &corev1.LocalObjectReference{Name: "rsa-passphrase"}
		env := newContainer(authn).Env

		Expect(env).To(HaveLen(1))
		Expect(env[0].Name).To(HaveValue(Equal("AIS_AUTHN_PRIVATE_KEY_PASS")))
		Expect(env[0].ValueFrom.SecretKeyRef.Name).To(HaveValue(Equal("rsa-passphrase")))
		Expect(env[0].ValueFrom.SecretKeyRef.Key).To(HaveValue(Equal("RSA-PASSPHRASE")))
	})
})
