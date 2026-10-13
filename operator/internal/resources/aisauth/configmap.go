/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"github.com/NVIDIA/aistore/cmn/cos"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnconfig "github.com/ais-operator/internal/resources/aisauth/config"
	"github.com/ais-operator/internal/resources/ownerref"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// Constants for Kubernetes labels
const (
	appNameLabel     = "app.kubernetes.io/name"
	appInstanceLabel = "app.kubernetes.io/instance"
	appNameValue     = "authn"
)

// AuthnJSONKey is the ConfigMap data key for the rendered AuthN config.
const AuthnJSONKey = "authn.json"

// ConfigMapNSName returns the namespaced name of the AuthN configuration ConfigMap.
func ConfigMapNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{
		Name:      ConfigMapName(authn),
		Namespace: authn.Namespace,
	}
}

// ConfigMapName returns the AuthN configuration ConfigMap name ({cr-name}-config).
func ConfigMapName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name + "-config"
}

func resourceLabels(authn *authv1alpha1.AIStoreAuth) map[string]string {
	return map[string]string{
		appNameLabel:            appNameValue,
		appInstanceLabel:        authn.Name,
		ownerref.ManagedByLabel: ownerref.ManagedByValue,
	}
}

// NewConfigMap creates the apply configuration for the AuthN configmap mounted by AuthN pods.
func NewConfigMap(authn *authv1alpha1.AIStoreAuth) (*corev1ac.ConfigMapApplyConfiguration, error) {
	conf, err := renderAuthnJSON(authn)
	if err != nil {
		return nil, err
	}
	return corev1ac.ConfigMap(ConfigMapName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithData(map[string]string{
			AuthnJSONKey: conf,
		}), nil
}

// ValidateConfig reports whether AuthN accepts the configuration rendered from authn.
func ValidateConfig(authn *authv1alpha1.AIStoreAuth) error {
	return authnconfig.GenerateConfig(authn, configPaths).Validate()
}

func renderAuthnJSON(authn *authv1alpha1.AIStoreAuth) (string, error) {
	return cos.JSON.MarshalToString(authnconfig.GenerateConfig(authn, configPaths))
}
