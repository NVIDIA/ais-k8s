// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// IMPROTANT: Run "make" to regenerate code after modifying this file

// AISConfigStatus defines the observed state of AISConfig
type AISConfigStatus struct { // INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AISConfig is the Schema for the aisconfigs API
type AISConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigToUpdate  `json:"spec,omitempty"`
	Status AISConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AISConfigList contains a list of AISConfig
type AISConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AISConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AISConfig{}, &AISConfigList{})
}
