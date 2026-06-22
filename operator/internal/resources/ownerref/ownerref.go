/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package ownerref builds OwnerReference apply configurations for resources
// owned by internal operator CRs.
package ownerref

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const aistoreAuthKind = "AIStoreAuth"

// NewAIStoreAuthControllerRef returns an OwnerReference apply configuration
// naming the AIStoreAuth CR as the controlling owner.
func NewAIStoreAuthControllerRef(authn *authv1alpha1.AIStoreAuth) *metav1ac.OwnerReferenceApplyConfiguration {
	return metav1ac.OwnerReference().
		WithAPIVersion(authv1alpha1.GroupVersion.String()).
		WithKind(aistoreAuthKind).
		WithName(authn.Name).
		WithUID(authn.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
}
