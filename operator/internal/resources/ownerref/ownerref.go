/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package ownerref records the operator CR that owns a resource, through an
// OwnerReference or, for a cluster-scoped resource with a namespaced owner,
// through labels.
package ownerref

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	aisKind = "AIStore"
	// AIStoreAuthKind is the kind of the AIStoreAuth CR.
	AIStoreAuthKind = "AIStoreAuth"
)

// ManagedByLabel and ManagedByValue mark a resource that the operator manages.
const (
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "ais-operator"
)

// Labels that record the namespaced owner of a cluster-scoped resource.
const (
	ownerKindLabel      = "ais.nvidia.com/owner-kind"
	ownerNamespaceLabel = "ais.nvidia.com/owner-namespace"
	ownerNameLabel      = "ais.nvidia.com/owner-name"
)

// LabeledOwner identifies the owner that a resource records in its labels.
type LabeledOwner struct {
	Kind string
	types.NamespacedName
}

// AIStoreAuthOwner returns the LabeledOwner for the AIStoreAuth CR.
func AIStoreAuthOwner(authn *authv1alpha1.AIStoreAuth) LabeledOwner {
	return LabeledOwner{Kind: AIStoreAuthKind, NamespacedName: types.NamespacedName{Namespace: authn.Namespace, Name: authn.Name}}
}

// SetLabeledOwner records owner in the labels of obj and marks obj as managed by the operator.
func SetLabeledOwner(obj metav1.Object, owner LabeledOwner) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 4)
	}
	labels[ManagedByLabel] = ManagedByValue
	labels[ownerKindLabel] = owner.Kind
	labels[ownerNamespaceLabel] = owner.Namespace
	labels[ownerNameLabel] = owner.Name
	obj.SetLabels(labels)
}

// GetLabeledOwner returns the owner that obj records in its labels. It returns false if the operator
// does not manage obj or if obj does not record a complete owner.
func GetLabeledOwner(obj metav1.Object) (LabeledOwner, bool) {
	labels := obj.GetLabels()
	owner := LabeledOwner{
		Kind:      labels[ownerKindLabel],
		Namespace: labels[ownerNamespaceLabel], Name: labels[ownerNameLabel],
	}
	ok := labels[ManagedByLabel] == ManagedByValue && owner.Kind != "" && owner.Namespace != "" && owner.Name != ""
	return owner, ok
}

// NewAIStoreAuthControllerRef returns an OwnerReference apply configuration
// naming the AIStoreAuth CR as the controlling owner.
func NewAIStoreAuthControllerRef(authn *authv1alpha1.AIStoreAuth) *metav1ac.OwnerReferenceApplyConfiguration {
	return metav1ac.OwnerReference().
		WithAPIVersion(authv1alpha1.GroupVersion.String()).
		WithKind(AIStoreAuthKind).
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
