// Package adminclient contains resources for the AIS admin client deployment
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package adminclient

import (
	"maps"
	"path/filepath"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisenv "github.com/NVIDIA/aistore/api/env"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// DefaultClientImage is the default container image for the client pod
	DefaultClientImage = "aistorage/ais-util:latest"
	// DefaultCABundleKey is the default key name for the CA bundle in the ConfigMap
	DefaultCABundleKey = "trust-bundle.pem"
	// ClientCAMountPath is the path where CA certificates are mounted
	ClientCAMountPath = "/etc/ais-ca"
	// ComponentLabelValue is the value for the component labels
	ComponentLabelValue = "client"
	// CAVolumeName is the name of the volume and volume mount for CA certificates
	CAVolumeName = "ais-ca"
)

func DeploymentNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.AdminClientName(),
		Namespace: ais.Namespace,
	}
}

func caVolumes(caConfigMap *aisv1.CAConfigMapRef) []corev1.Volume {
	if caConfigMap == nil {
		return nil
	}
	caKey := DefaultCABundleKey
	if caConfigMap.Key != nil {
		caKey = *caConfigMap.Key
	}
	return []corev1.Volume{{
		Name: CAVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: caConfigMap.Name,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  caKey,
						Path: caKey,
					},
				},
				Optional: aisapc.Ptr(true),
			},
		},
	}}
}

func caVolumeMounts(caConfigMap *aisv1.CAConfigMapRef) []corev1.VolumeMount {
	if caConfigMap == nil {
		return nil
	}
	return []corev1.VolumeMount{{
		Name:      CAVolumeName,
		MountPath: ClientCAMountPath,
		ReadOnly:  true,
	}}
}

func caEnvVars(caConfigMap *aisv1.CAConfigMapRef) []corev1.EnvVar {
	if caConfigMap == nil {
		return nil
	}
	caKey := DefaultCABundleKey
	if caConfigMap.Key != nil {
		caKey = *caConfigMap.Key
	}
	return []corev1.EnvVar{{
		Name:  aisenv.AisClientCA,
		Value: filepath.Join(ClientCAMountPath, caKey),
	}}
}

// selectorLabels returns the standard labels for the admin client deployment
func selectorLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		cmn.LabelAppPrefixed:       ais.AdminClientName(),
		cmn.LabelComponentPrefixed: ComponentLabelValue,
	}
}

func NewClientDeployment(ais *aisv1.AIStore) *appsv1.Deployment {
	clientSpec := ais.Spec.AdminClient
	if clientSpec == nil {
		return nil
	}

	image := DefaultClientImage
	if clientSpec.Image != nil {
		image = *clientSpec.Image
	}

	matchLabels := selectorLabels(ais)
	podLabels := selectorLabels(ais)
	maps.Copy(podLabels, clientSpec.Labels)

	volumes := caVolumes(clientSpec.CAConfigMap)
	volumeMounts := caVolumeMounts(clientSpec.CAConfigMap)

	container := corev1.Container{
		Name:         "ais-client",
		Image:        image,
		Command:      []string{"sleep", "infinity"},
		Env:          buildClientEnv(ais),
		Resources:    clientSpec.Resources,
		VolumeMounts: volumeMounts,
	}
	if clientSpec.ImagePullPolicy != nil {
		container.ImagePullPolicy = *clientSpec.ImagePullPolicy
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ais.AdminClientName(),
			Namespace:   ais.Namespace,
			Labels:      matchLabels,
			Annotations: clientSpec.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: aisapc.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: clientSpec.Annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: cmn.ServiceAccountName(ais),
					NodeSelector:       clientSpec.NodeSelector,
					Affinity:           clientSpec.Affinity,
					Tolerations:        clientSpec.Tolerations,
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
				},
			},
		},
	}
}

func buildClientEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	clientSpec := ais.Spec.AdminClient
	base := []corev1.EnvVar{
		{Name: aisenv.AisEndpoint, Value: ais.GetIntraClusterURL()},
	}
	ca := caEnvVars(clientSpec.CAConfigMap)

	env := make([]corev1.EnvVar, 0, len(base)+len(clientSpec.Env)+len(ca))
	env = append(env, base...)
	env = append(env, clientSpec.Env...)
	env = append(env, ca...)
	return env
}
