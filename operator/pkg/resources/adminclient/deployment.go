// Package adminclient contains resources for the AIS admin client deployment
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package adminclient

import (
	"maps"
	"path/filepath"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisenv "github.com/NVIDIA/aistore/api/env"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// DefaultClientImage is the default container image for the client pod
	DefaultClientImage = "docker.io/aistorage/ais-util:latest"
	// DefaultCABundleKey is the default key name for the CA bundle in the ConfigMap
	DefaultCABundleKey = "trust-bundle.pem"
	// ClientCAMountPath is the path where CA certificates are mounted
	ClientCAMountPath = "/etc/ais-ca"
	// ComponentLabelValue is the value for the component labels
	ComponentLabelValue = "client"
	// CAVolumeName is the name of the volume and volume mount for CA certificates
	CAVolumeName = "ais-ca"

	// DefaultAuthNServiceURL is the default URL for the AuthN service
	DefaultAuthNServiceURL = "https://ais-authn.ais:52001"
	// AuthN secret keys
	authnSecretKeyUsername = "SU-NAME"
	authnSecretKeyPassword = "SU-PASS"
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
				// Set explicitly to avoid causing rollout on diff
				DefaultMode: aisapc.Ptr(corev1.ConfigMapVolumeSourceDefaultMode),
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

// authnEnvVars returns environment variables for AuthN configuration.
func authnEnvVars(auth *aisv1.AuthSpec) []corev1.EnvVar {
	if auth == nil || auth.UsernamePassword == nil {
		return nil
	}
	serviceURL := DefaultAuthNServiceURL
	if auth.ServiceURL != nil {
		serviceURL = *auth.ServiceURL
	}
	return []corev1.EnvVar{
		{Name: "AIS_AUTHN_URL", Value: serviceURL},
		{
			Name: "AIS_AUTHN_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: auth.UsernamePassword.SecretName,
					},
					Key: authnSecretKeyUsername,
				},
			},
		},
		{
			Name: "AIS_AUTHN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: auth.UsernamePassword.SecretName,
					},
					Key: authnSecretKeyPassword,
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
	authn := authnEnvVars(ais.Spec.Auth)

	env := make([]corev1.EnvVar, 0, len(base)+len(clientSpec.Env)+len(ca)+len(authn))
	env = append(env, base...)
	env = append(env, clientSpec.Env...)
	env = append(env, ca...)
	env = append(env, authn...)
	return env
}

// SyncDeployment takes a desired and current deployment and modifies the current to match, returning a list of reasons
func SyncDeployment(desired, modified *appsv1.Deployment) (bool, string) {
	reasons := syncContainerSpec(&desired.Spec.Template.Spec.Containers[0], &modified.Spec.Template.Spec.Containers[0])
	reasons = append(reasons, syncPodTemplateSpec(&desired.Spec.Template, &modified.Spec.Template)...)
	if !equality.Semantic.DeepEqual(desired.Labels, modified.Labels) {
		modified.Labels = desired.Labels
		reasons = append(reasons, "deployment labels")
	}
	if len(reasons) == 0 {
		return false, ""
	}
	return true, strings.Join(reasons, ", ")
}

func syncContainerSpec(desired, current *corev1.Container) (reasons []string) {
	if desired.Image != current.Image {
		current.Image = desired.Image
		reasons = append(reasons, "image")
	}
	// K8s will set default, only reconcile this if set
	if desired.ImagePullPolicy != "" && desired.ImagePullPolicy != current.ImagePullPolicy {
		current.ImagePullPolicy = desired.ImagePullPolicy
		reasons = append(reasons, "imagePullPolicy")
	}
	if !equality.Semantic.DeepEqual(desired.Env, current.Env) {
		current.Env = desired.Env
		reasons = append(reasons, "env")
	}
	if !equality.Semantic.DeepEqual(desired.Resources, current.Resources) {
		current.Resources = desired.Resources
		reasons = append(reasons, "resources")
	}
	if !equality.Semantic.DeepEqual(desired.VolumeMounts, current.VolumeMounts) {
		current.VolumeMounts = desired.VolumeMounts
		reasons = append(reasons, "volumeMounts")
	}
	return
}

func syncPodTemplateSpec(desired, modified *corev1.PodTemplateSpec) (reasons []string) {
	dSpec := &desired.Spec
	mSpec := &modified.Spec
	if !equality.Semantic.DeepEqual(dSpec.Volumes, mSpec.Volumes) {
		mSpec.Volumes = dSpec.Volumes
		reasons = append(reasons, "volumes")
	}
	if !equality.Semantic.DeepEqual(dSpec.NodeSelector, mSpec.NodeSelector) {
		mSpec.NodeSelector = dSpec.NodeSelector
		reasons = append(reasons, "nodeSelector")
	}
	if !equality.Semantic.DeepEqual(dSpec.Affinity, mSpec.Affinity) {
		mSpec.Affinity = dSpec.Affinity
		reasons = append(reasons, "affinity")
	}
	if !equality.Semantic.DeepEqual(dSpec.Tolerations, mSpec.Tolerations) {
		mSpec.Tolerations = dSpec.Tolerations
		reasons = append(reasons, "tolerations")
	}
	if !equality.Semantic.DeepEqual(desired.Labels, modified.Labels) {
		modified.Labels = desired.Labels
		reasons = append(reasons, "pod labels")
	}
	if !equality.Semantic.DeepEqual(desired.Annotations, modified.Annotations) {
		modified.Annotations = desired.Annotations
		reasons = append(reasons, "annotations")
	}
	return
}
