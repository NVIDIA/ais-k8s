/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// PVCName returns the AuthN data PVC name ({cr-name}-storage).
func PVCName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name + "-storage"
}

// PVCNSName returns the namespaced name of the AuthN data PVC.
func PVCNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{
		Name:      PVCName(authn),
		Namespace: authn.Namespace,
	}
}

// NewPVC builds the apply configuration for the AuthN data PVC.
//
// The operator supports two persistence modes via spec.persistence:
//   - storageClass: dynamic provisioning via the named StorageClass (provisioner creates the PV).
//   - volumeName:   bind to a pre-provisioned PV by name (PV must exist before reconcile).
func NewPVC(authn *authv1alpha1.AIStoreAuth) (*corev1ac.PersistentVolumeClaimApplyConfiguration, error) {
	persistence := &authn.Spec.Persistence

	spec := corev1ac.PersistentVolumeClaimSpec().
		WithAccessModes(corev1.ReadWriteOnce).
		WithResources(corev1ac.VolumeResourceRequirements().
			WithRequests(corev1.ResourceList{
				corev1.ResourceStorage: persistence.StorageSize(),
			}))

	switch {
	case persistence.UsesStorageClass():
		spec.WithStorageClassName(*persistence.StorageClass)
	case persistence.UsesExistingVolume():
		// Bind by name ("" opts out of the default StorageClass).
		spec.WithVolumeName(*persistence.VolumeName).
			WithStorageClassName("")
	default:
		return nil, fmt.Errorf("spec.persistence must set exactly one of storageClass or volumeName")
	}

	return corev1ac.PersistentVolumeClaim(PVCName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithSpec(spec), nil
}
