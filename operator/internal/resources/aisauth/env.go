/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	aisenv "github.com/NVIDIA/aistore/api/env"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	adminNameKey     = "SU-NAME"
	adminPassKey     = "SU-PASS"
	signingKeyKey    = "SIGNING-KEY"
	rsaPassphraseKey = "RSA-PASSPHRASE"
)

func secretEnvVars(authn *authv1alpha1.AIStoreAuth) []*corev1ac.EnvVarApplyConfiguration {
	var envs []*corev1ac.EnvVarApplyConfiguration
	if name := secretName(authn.Spec.AdminSecret); name != "" {
		envs = append(envs,
			secretKeyEnv(aisenv.AisAuthAdminUsername, name, adminNameKey, true),
			secretKeyEnv(aisenv.AisAuthAdminPassword, name, adminPassKey, false),
		)
	}
	if name := secretName(authn.Spec.HMACSecret); name != "" {
		envs = append(envs, secretKeyEnv(aisenv.AisAuthSecretKey, name, signingKeyKey, false))
	}
	if name := secretName(authn.Spec.RSAPassphraseSecret); name != "" {
		envs = append(envs, secretKeyEnv(aisenv.AisAuthPrivateKeyPass, name, rsaPassphraseKey, false))
	}
	return envs
}

func secretName(ref *corev1.LocalObjectReference) string {
	if ref == nil {
		return ""
	}
	return ref.Name
}

func secretKeyEnv(envName, secretName, secretKey string, optional bool) *corev1ac.EnvVarApplyConfiguration {
	selector := corev1ac.SecretKeySelector().WithName(secretName).WithKey(secretKey)
	if optional {
		selector.WithOptional(true)
	}
	return corev1ac.EnvVar().
		WithName(envName).
		WithValueFrom(corev1ac.EnvVarSource().WithSecretKeyRef(selector))
}
