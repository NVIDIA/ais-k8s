// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	corev1 "k8s.io/api/core/v1"
)

const oidcCAVolumeName = "oidc-ca"

func newVolumes(ais *aisv1.AIStore) []corev1.Volume {
	volumes := cmn.NewAISVolumes(ais, aisapc.Proxy)
	if ais.Spec.IssuerCAConfigMap != nil {
		volumes = append(volumes, newOIDCCAVolume(*ais.Spec.IssuerCAConfigMap))
	}
	return volumes
}

// newOIDCCAVolume creates a volume for OIDC issuer CA certificates
func newOIDCCAVolume(configMapName string) corev1.Volume {
	return corev1.Volume{
		Name: oidcCAVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}

func newVolumeMounts(ais *aisv1.AIStore) []corev1.VolumeMount {
	vm := cmn.NewAISVolumeMounts(ais, aisapc.Proxy)
	if ais.Spec.IssuerCAConfigMap != nil {
		vm = append(vm, newOIDCCAVolumeMount())
	}
	return vm
}

// newOIDCCAVolumeMount creates a volume mount for OIDC issuer CA certificates
func newOIDCCAVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      oidcCAVolumeName,
		MountPath: cmn.OIDCCAMountPath,
		ReadOnly:  true,
	}
}
