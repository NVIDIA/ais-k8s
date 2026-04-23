// Package ownerref builds OwnerReference apply configurations for resources
// owned by an AIStore CR.
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package ownerref

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const aisKind = "AIStore"

// NewControllerRef returns an OwnerReference apply configuration naming the AIStore CR as
// the controlling owner. Used in place of controllerutil.SetControllerReference, which does
// not accept runtime.ApplyConfiguration values.
func NewControllerRef(ais *aisv1.AIStore) *metav1ac.OwnerReferenceApplyConfiguration {
	return metav1ac.OwnerReference().
		WithAPIVersion(aisv1.GroupVersion.String()).
		WithKind(aisKind).
		WithName(ais.Name).
		WithUID(ais.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
}
