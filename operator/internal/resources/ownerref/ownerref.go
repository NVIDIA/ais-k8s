/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package ownerref builds OwnerReference apply configurations for resources
// owned by internal operator CRs.
package ownerref

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	aisKind         = "AIStore"
	aistoreAuthKind = "AIStoreAuth"
)

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

// NewControllerRef returns an OwnerReference apply configuration naming the AIStore CR as
// the controlling owner.
func NewControllerRef(ais *aisv1.AIStore) *metav1ac.OwnerReferenceApplyConfiguration {
	return metav1ac.OwnerReference().
		WithAPIVersion(aisv1.GroupVersion.String()).
		WithKind(aisKind).
		WithName(ais.Name).
		WithUID(ais.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
}
