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

	container := corev1ac.Container().
		WithName(containerName).
		WithImage(authn.Spec.Deployment.Image).
		WithPorts(corev1ac.ContainerPort().
			WithName(portName).
			WithContainerPort(authn.ListenPort()).
			WithProtocol(corev1.ProtocolTCP)).
		WithEnv(secretEnvVars(authn)...).
		WithVolumeMounts(volumeMounts(authn)...)
	if authn.Spec.Deployment.ImagePullPolicy != "" {
		container.WithImagePullPolicy(authn.Spec.Deployment.ImagePullPolicy)
	}

	podSpec := corev1ac.PodSpec().
		WithContainers(container).
		WithVolumes(volumes(authn)...)

	return appsv1ac.Deployment(DeploymentName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(1).
			WithStrategy(appsv1ac.DeploymentStrategy().WithType(appsv1.RecreateDeploymentStrategyType)).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(selectorLabels(authn))).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(resourceLabels(authn)).
				WithAnnotations(map[string]string{
					ConfigChecksumAnnotation: hex.EncodeToString(checksum[:]),
				}).
				WithSpec(podSpec))), nil
}
