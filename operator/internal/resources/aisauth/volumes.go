/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnconfig "github.com/ais-operator/internal/resources/aisauth/config"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	storageVolumeName = "storage"
	configVolumeName  = "config"
	tlsVolumeName     = "tls-certs"

	stateMountPath  = "/etc/ais/authn"
	authnConfigPath = stateMountPath + "/" + AuthnJSONKey
	tlsMountPath    = "/var/certs"
)

var configPaths = authnconfig.Paths{
	Database:       stateMountPath + "/authn.db",
	TLSCertificate: tlsMountPath + "/tls.crt",
	TLSKey:         tlsMountPath + "/tls.key",
}

func volumes(authn *authv1alpha1.AIStoreAuth) []*corev1ac.VolumeApplyConfiguration {
	result := []*corev1ac.VolumeApplyConfiguration{
		corev1ac.Volume().WithName(storageVolumeName).
			WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().
				WithClaimName(PVCName(authn))),
		corev1ac.Volume().WithName(configVolumeName).
			WithConfigMap(corev1ac.ConfigMapVolumeSource().
				WithName(ConfigMapName(authn))),
	}
	if authn.HasTLSEnabled() {
		result = append(result, corev1ac.Volume().WithName(tlsVolumeName).
			WithSecret(corev1ac.SecretVolumeSource().
				WithSecretName(authn.GetTLSSecretName())))
	}
	return result
}

func volumeMounts(authn *authv1alpha1.AIStoreAuth) []*corev1ac.VolumeMountApplyConfiguration {
	result := []*corev1ac.VolumeMountApplyConfiguration{
		corev1ac.VolumeMount().WithName(storageVolumeName).
			WithMountPath(stateMountPath),
		corev1ac.VolumeMount().WithName(configVolumeName).
			WithMountPath(authnConfigPath).
			WithSubPath(AuthnJSONKey).
			WithReadOnly(true),
	}
	if authn.HasTLSEnabled() {
		result = append(result, corev1ac.VolumeMount().WithName(tlsVolumeName).
			WithMountPath(tlsMountPath).
			WithReadOnly(true))
	}
	return result
}
