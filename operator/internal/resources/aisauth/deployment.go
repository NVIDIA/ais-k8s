/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"crypto/sha256"
	"encoding/hex"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	containerName = "authn"
	portName      = "http"

	// ConfigChecksumAnnotation rolls the pod when the startup-only authn.json changes.
	ConfigChecksumAnnotation = "auth.ais.nvidia.com/config-checksum"
)

// DeploymentName returns the AuthN Deployment name (the CR name).
func DeploymentName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name
}

// DeploymentNSName returns the namespaced name of the AuthN Deployment.
func DeploymentNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: DeploymentName(authn), Namespace: authn.Namespace}
}

func selectorLabels(authn *authv1alpha1.AIStoreAuth) map[string]string {
	return map[string]string{
		appNameLabel:     appNameValue,
		appInstanceLabel: authn.Name,
	}
}

// NewDeployment builds the server-side apply configuration for the AuthN Deployment.
func NewDeployment(authn *authv1alpha1.AIStoreAuth) (*appsv1ac.DeploymentApplyConfiguration, error) {
	authnJSON, err := renderAuthnJSON(authn)
	if err != nil {
		return nil, err
	}
	checksum := sha256.Sum256([]byte(authnJSON))
	podSpec, err := newPodSpec(authn)
	if err != nil {
		return nil, err
	}

	return appsv1ac.Deployment(DeploymentName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(1). // AuthN doesn't support multiple replicas
			// AuthN is single-replica, so the rollout strategy is fixed to Recreate.
			WithStrategy(appsv1ac.DeploymentStrategy().WithType(appsv1.RecreateDeploymentStrategyType)).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(selectorLabels(authn))).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(resourceLabels(authn)).
				WithAnnotations(map[string]string{
					ConfigChecksumAnnotation: hex.EncodeToString(checksum[:]),
				}).
				WithSpec(podSpec))), nil
}

func newContainer(
	authn *authv1alpha1.AIStoreAuth,
	spec *authv1alpha1.ContainerSpec,
) (*corev1ac.ContainerApplyConfiguration, error) {
	container := corev1ac.Container().
		WithName(containerName).
		WithImage(spec.Image).
		WithPorts(corev1ac.ContainerPort().
			WithName(portName).
			WithContainerPort(authn.ListenPort()).
			WithProtocol(corev1.ProtocolTCP)).
		WithEnv(secretEnvVars(authn)...).
		WithVolumeMounts(volumeMounts(authn)...)
	if spec.ImagePullPolicy != "" {
		container.WithImagePullPolicy(spec.ImagePullPolicy)
	}
	if spec.Resources != nil {
		resources, err := toApplyConfiguration[*corev1ac.ResourceRequirementsApplyConfiguration](spec.Resources)
		if err != nil {
			return nil, err
		}
		container.WithResources(resources)
	}
	if spec.SecurityContext != nil {
		securityContext, err := toApplyConfiguration[*corev1ac.SecurityContextApplyConfiguration](spec.SecurityContext)
		if err != nil {
			return nil, err
		}
		container.WithSecurityContext(securityContext)
	}
	if spec.LivenessProbe != nil {
		livenessProbe, err := toApplyConfiguration[*corev1ac.ProbeApplyConfiguration](spec.LivenessProbe)
		if err != nil {
			return nil, err
		}
		container.WithLivenessProbe(livenessProbe)
	}
	if spec.ReadinessProbe != nil {
		readinessProbe, err := toApplyConfiguration[*corev1ac.ProbeApplyConfiguration](spec.ReadinessProbe)
		if err != nil {
			return nil, err
		}
		container.WithReadinessProbe(readinessProbe)
	}
	return container, nil
}

func newPodSpec(authn *authv1alpha1.AIStoreAuth) (*corev1ac.PodSpecApplyConfiguration, error) {
	spec := &authn.Spec.Deployment
	container, err := newContainer(authn, &spec.Container)
	if err != nil {
		return nil, err
	}
	pod := corev1ac.PodSpec().
		WithContainers(container).
		WithVolumes(volumes(authn)...)
	podSpec := spec.Pod
	if podSpec == nil {
		return pod, nil
	}
	if podSpec.SecurityContext != nil {
		securityContext, err := toApplyConfiguration[*corev1ac.PodSecurityContextApplyConfiguration](podSpec.SecurityContext)
		if err != nil {
			return nil, err
		}
		pod.WithSecurityContext(securityContext)
	}
	if len(podSpec.NodeSelector) > 0 {
		pod.WithNodeSelector(podSpec.NodeSelector)
	}
	if len(podSpec.Tolerations) > 0 {
		tolerations, err := toApplyConfiguration[[]*corev1ac.TolerationApplyConfiguration](podSpec.Tolerations)
		if err != nil {
			return nil, err
		}
		pod.WithTolerations(tolerations...)
	}
	if podSpec.Affinity != nil {
		affinity, err := toApplyConfiguration[*corev1ac.AffinityApplyConfiguration](podSpec.Affinity)
		if err != nil {
			return nil, err
		}
		pod.WithAffinity(affinity)
	}
	if len(podSpec.ImagePullSecrets) > 0 {
		imagePullSecrets, err := toApplyConfiguration[[]*corev1ac.LocalObjectReferenceApplyConfiguration](podSpec.ImagePullSecrets)
		if err != nil {
			return nil, err
		}
		pod.WithImagePullSecrets(imagePullSecrets...)
	}
	return pod, nil
}
