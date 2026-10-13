/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthProfileName returns the admin AIStoreAuthProfile name ({namespace}.{cr-name}-admin).
func AuthProfileName(authn *authv1alpha1.AIStoreAuth) string {
	return fmt.Sprintf("%s.%s-admin", authn.Namespace, authn.Name)
}

// NewAuthProfile builds the admin AIStoreAuthProfile.
func NewAuthProfile(authn *authv1alpha1.AIStoreAuth) *authv1alpha1.AIStoreAuthProfile {
	adminSecret := secretName(authn.Spec.AdminSecret)
	if adminSecret == "" {
		return nil
	}
	profile := &authv1alpha1.AIStoreAuthProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: authv1alpha1.GroupVersion.String(),
			Kind:       "AIStoreAuthProfile",
		},
		ObjectMeta: metav1.ObjectMeta{Name: AuthProfileName(authn), Labels: resourceLabels(authn)},
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
	ownerref.SetLabeledOwner(profile, ownerref.AIStoreAuthOwner(authn))
	return profile
}
