/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// authProfileNamespaceLabel stores the namespace of the AIStoreAuth that manages an AIStoreAuthProfile.
const authProfileNamespaceLabel = "auth.ais.nvidia.com/aistoreauth-namespace"

// AuthProfileName returns the admin AIStoreAuthProfile name ({namespace}.{cr-name}-admin).
func AuthProfileName(authn *authv1alpha1.AIStoreAuth) string {
	return fmt.Sprintf("%s.%s-admin", authn.Namespace, authn.Name)
}

// AuthProfileOwner returns the AIStoreAuth that manages the profile or false if the operator does not manage it.
func AuthProfileOwner(profile *authv1alpha1.AIStoreAuthProfile) (types.NamespacedName, bool) {
	labels := profile.GetLabels()
	owner := types.NamespacedName{Namespace: labels[authProfileNamespaceLabel], Name: labels[appInstanceLabel]}
	managed := labels[managedByLabel] == managedByValue && owner.Namespace != "" && owner.Name != ""
	return owner, managed
}

// NewAuthProfile builds the apply configuration for the admin AIStoreAuthProfile.
func NewAuthProfile(authn *authv1alpha1.AIStoreAuth) (runtime.ApplyConfiguration, error) {
	adminSecret := secretName(authn.Spec.AdminSecret)
	if adminSecret == "" {
		return nil, nil
	}
	labels := resourceLabels(authn)
	labels[authProfileNamespaceLabel] = authn.Namespace
	profile := &authv1alpha1.AIStoreAuthProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: authv1alpha1.GroupVersion.String(),
			Kind:       "AIStoreAuthProfile",
		},
		ObjectMeta: metav1.ObjectMeta{Name: AuthProfileName(authn), Labels: labels},
		Spec: authv1alpha1.AIStoreAuthProfileSpec{
			ServiceURL: ServiceURL(authn),
			UsernamePassword: &authv1alpha1.AuthProfileUsernamePassword{
				Secret: authv1alpha1.AuthProfileSecret{
					Name:      adminSecret,
					Namespace: authn.Namespace,
					UserKey:   adminNameKey,
					PassKey:   adminPassKey,
				},
			},
		},
	}
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(profile)
	if err != nil {
		return nil, fmt.Errorf("convert AIStoreAuthProfile %q to unstructured: %w", profile.Name, err)
	}
	return client.ApplyConfigurationFromUnstructured(&unstructured.Unstructured{Object: content}), nil
}
