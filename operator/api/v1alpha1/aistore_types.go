// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ClusterCondition string

const (
	ConiditionInitialized          ClusterCondition = "Initialized"
	ConditionInitializingLBService ClusterCondition = "InitializingLoadBalancerService"
	ConditionFailed                ClusterCondition = "Failed"
	ConditionCreated               ClusterCondition = "Created"
	ConditionReady                 ClusterCondition = "Ready"
	// TODO: Add more states, eg. Terminating etc.
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// IMPORTANT: Run "make" to regenerate code after modifying this file

// AIStoreSpec defines the desired state of AIStore
type AIStoreSpec struct {
	Size           int32   `json:"size"`
	NodeImage      string  `json:"nodeImage"` // docker image of aisnode
	InitImage      string  `json:"initImage"` // init image for nodes
	HostpathPrefix string  `json:"hostpathPrefix"`
	ConfigCRName   *string `json:"configCRName,omitempty"`

	ProxySpec  DaemonSpec `json:"proxySpec"`  // spec for proxy
	TargetSpec TargetSpec `json:"targetSpec"` // spec for target

	// ImagePullScerets is an optional list of references to secrets in the same namespace to pull container images of AIS Daemons
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// DisablePodAntiAffinity, if set allows more than one target/proxy daemon pods to be scheduled on same K8s node.
	// +optional
	DisablePodAntiAffinity *bool `json:"disablePodAntiAffinity,omitempty"`
	// EnableExternalLB, if set, enables external access to AIS cluster using LoadBalancer service
	EnableExternalLB bool `json:"enableExternalLB"`
}

// AIStoreStatus defines the observed state of AIStore
type AIStoreStatus struct {
	State                 ClusterCondition `json:"condition"`
	ConfigResourceVersion string           `json:"config_version"`
}

// ServiceSpec defines the specs of AIS Gateways
type ServiceSpec struct {
	ServicePort      intstr.IntOrString `json:"servicePort"`
	PublicPort       intstr.IntOrString `json:"portPublic"` // port of PublicNet
	IntraControlPort intstr.IntOrString `json:"portIntraControl"`
	IntraDataPort    intstr.IntOrString `json:"portIntraData"`
}

// NodeSpec defines the specs for AIS Daemon pods/containers
type DaemonSpec struct {
	ServiceSpec `json:",inline"`
	// SecurityContext holds pod-level security attributes and common container settings for AIS Daemon (proxy/target) object.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// ContainerSecurity holds the secrity context for AIS Daemon containers.
	// +optional
	ContainerSecurity *corev1.SecurityContext `json:"capabilities,omitempty"`
	// Affinity  - AIS Daemon pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// NodeSelector -  which must match a node's labels for the AIS Daemon pod to be scheduled on that node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations - list of tolerations for AIS Daemon pod
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

type TargetSpec struct {
	DaemonSpec `json:",inline"`
	NoDiskIO   NoDiskIO `json:"nodiskio"`
	Mounts     []Mount  `json:"mounts"`
}

type NoDiskIO struct {
	DryObjSize resource.Quantity `json:"dryobjsize"`
	Enabled    bool              `json:"enabled"`
}

type Mount struct {
	Path         string            `json:"path"`
	Size         resource.Quantity `json:"size"`
	StorageClass *string           `json:"storageClass,omitempty"` // storage class for volume resource
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AIStore is the Schema for the aistores API
type AIStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIStoreSpec   `json:"spec,omitempty"`
	Status AIStoreStatus `json:"status,omitempty"`
}

func (ais *AIStore) SetState(state ClusterCondition) {
	ais.Status.State = state
}

func (ais *AIStore) HasState(state ClusterCondition) bool {
	return ais.Status.State == state
}

func (ais *AIStore) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.Name,
		Namespace: ais.Namespace,
	}
}

// +kubebuilder:object:root=true

// AIStoreList contains a list of AIStore
type AIStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIStore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIStore{}, &AIStoreList{})
}
