// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type (
	// ClusterState represents the various states a cluster can be in during its
	// lifecycle, such as Created, Ready, or ShuttingDown.
	ClusterState string
	// ClusterConditionType is a valid value for Condition.Type
	ClusterConditionType string
	// ClusterConditionReason is a valid value for Condition.Reason
	ClusterConditionReason string
)

// Cluster state constants represent various stages in the cluster lifecycle.
const (
	// ClusterInitialized indicates the cluster is initialized but not yet provisioned.
	ClusterInitialized ClusterState = "Initialized"
	// ClusterCreated means the cluster is created with basic resources but not yet fully operational.
	ClusterCreated ClusterState = "Created"
	// ClusterReady indicates the cluster is fully operational and ready for workloads.
	ClusterReady ClusterState = "Ready"
	// ClusterInitializingLBService means the cluster is setting up the load-balancer service.
	ClusterInitializingLBService ClusterState = "InitializingLoadBalancerService"
	// ClusterPendingLBService indicates the cluster is waiting for the load-balancer to become operational.
	ClusterPendingLBService ClusterState = "PendingLoadBalancerService"
	// ClusterUpgrading signifies the cluster is undergoing an upgrade process.
	ClusterUpgrading ClusterState = "Upgrading"
	// ClusterScaling indicates the cluster is adjusting its resources (up or down).
	ClusterScaling ClusterState = "Scaling"
	// ClusterShuttingDown means the cluster is in the process of shutting down.
	ClusterShuttingDown ClusterState = "ShuttingDown"
	// ClusterShutdown indicates the cluster is fully shut down and not operational.
	ClusterShutdown ClusterState = "Shutdown"
	// ClusterDecommissioning means the cluster is being dismantled and resources are being reclaimed.
	ClusterDecommissioning ClusterState = "Decommissioning"
	// ClusterCleanup indicates the cluster is cleaning up residual resources.
	ClusterCleanup ClusterState = "CleaningResources"
	// HostCleanup indicates jobs are running to clean up the hosts, e.g. hostpath state mounts.
	HostCleanup ClusterState = "HostCleanup"
	// ClusterFinalized indicates the cluster is fully decommissioned and cleaned up
	ClusterFinalized ClusterState = "Finalized"
)

// These are built-in cluster conditions.
// Applications can define custom conditions as needed.
const (
	// ConditionInitialized indicates the cluster has been initialized.
	ConditionInitialized ClusterConditionType = "Initialized"
	// ConditionCreated means the cluster has been successfully created.
	ConditionCreated ClusterConditionType = "Created"
	// ConditionReady indicates the cluster is fully operational and ready for use.
	ConditionReady ClusterConditionType = "Ready"
	// ConditionReadyRebalance indicates whether the cluster should allow rebalance as determined by spec or default config.
	ConditionReadyRebalance ClusterConditionType = "ReadyRebalance"
)

// These are reasons for a AIStore's transition to a condition.
const (
	ReasonUpgrading ClusterConditionReason = "Upgrading"
	ReasonScaling   ClusterConditionReason = "Scaling"
	ReasonShutdown  ClusterConditionReason = "Shutdown"
)

// Helper constants.
const (
	defaultClusterDomain = "cluster.local"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// IMPORTANT: Run "make" to regenerate code after modifying this file

// AIStoreSpec defines the desired state of AIStore
// +kubebuilder:validation:XValidation:rule="(has(self.targetSpec.size) && has(self.proxySpec.size)) || (has(self.size) && self.size > 0)",message="Invalid cluster size, it is either not specified or value is not valid"
type AIStoreSpec struct {
	// Size of the cluster i.e. number of proxies and number of targets.
	// This can be changed by specifying size in either `proxySpec` or `targetSpec`.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Size *int32 `json:"size,omitempty"`
	// Container image used for `aisnode` container.
	// +kubebuilder:validation:MinLength=1
	NodeImage string `json:"nodeImage"`
	// Container image used for `ais-init` container.
	// +kubebuilder:validation:MinLength=1
	InitImage string `json:"initImage"`
	// Deprecated: use StateStorageClass
	// See docs/state_storage.md
	// Path on host used for state
	// +optional
	HostpathPrefix *string `json:"hostpathPrefix,omitempty"`
	// Used for creating dynamic volumes for storing state
	// +optional
	StateStorageClass *string         `json:"stateStorageClass,omitempty"`
	ConfigToUpdate    *ConfigToUpdate `json:"configToUpdate,omitempty"`
	// Map of primary host to comma-separated string of all hosts for multi-home
	// +optional
	HostnameMap map[string]string `json:"hostnameMap,omitempty"`
	// Commma-separated list of names of additional network attachment definitions to attach to each pod
	// +optional
	NetAttachment *string `json:"networkAttachment,omitempty"`

	// Proxy deployment specification.
	ProxySpec DaemonSpec `json:"proxySpec"`
	// Target deployment specification.
	TargetSpec TargetSpec `json:"targetSpec"`

	// ShutdownCluster can be set true if the desired state of the cluster is shutdown with a future restart expected
	// When enabled, the operator will gracefully shut down the AIS cluster and scale cluster size to 0
	// No data or configuration will be deleted
	// +optional
	ShutdownCluster *bool `json:"shutdownCluster,omitempty"`

	// CleanupMetadata determines whether to clean up cluster and bucket metadata when the cluster is decommissioned.
	// When enabled, the cluster will fully decommission, removing metadata and optionally deleting user data.
	// When disabled, the operator will call the AIS shutdown API to preserve metadata before deleting other k8s resources.
	// The metadata stored in the state PVCs will be preserved to be usable in a future AIS deployment.
	// +optional
	CleanupMetadata *bool `json:"cleanupMetadata,omitempty"`

	// CleanupData determines whether to clean up PVCs and user data (including buckets and objects) when the cluster is decommissioned.
	// The reclamation of PVs linked to the PVCs depends on the PV reclaim policy or the default policy of the associated StorageClass.
	// This field is relevant only if you are deleting the CR (leading to decommissioning of the cluster).
	// +optional
	CleanupData *bool `json:"cleanupData,omitempty"`

	// Defines if AIS daemons should expose prometheus metrics
	// +optional
	EnablePromExporter *bool `json:"enablePromExporter,omitempty"`

	// Defines the cluster domain name for DNS. Default: cluster.local.
	// +optional
	ClusterDomain *string `json:"clusterDomain,omitempty"`

	// Secret name containing GCP credentials
	// +optional
	GCPSecretName *string `json:"gcpSecretName,omitempty"`

	// Secret name containing AWS credentials
	// +optional
	AWSSecretName *string `json:"awsSecretName,omitempty"`

	// Logs directory on host to store AIS logs
	// +optional
	LogsDirectory string `json:"logsDir,omitempty"`

	// Secret name containing TLS cert/key
	// +optional
	TLSSecretName *string `json:"tlsSecretName,omitempty"`

	// Name of Cert Manager CSI Issuer used for getting the cert/key
	// +optional
	TLSCertManagerIssuerName *string `json:"tlsCertManagerIssuerName,omitempty"`

	// Secret name containing AuthN's JWT signing key
	// +optional
	AuthNSecretName *string `json:"authNSecretName,omitempty"`

	// ImagePullScerets is an optional list of references to secrets in the same namespace to pull container images of AIS Daemons
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Deprecated: use TargetSpec.DisablePodAntiAffinity
	// DisablePodAntiAffinity, if set allows more than one target/proxy daemon pods to be scheduled on same K8s node.
	// +optional
	DisablePodAntiAffinity *bool `json:"disablePodAntiAffinity,omitempty"`

	// EnableExternalLB, if set, enables external access to AIS cluster using LoadBalancer service
	EnableExternalLB bool `json:"enableExternalLB"`
}

// AIStoreStatus defines the observed state of AIStore
type AIStoreStatus struct {
	// The state of a AIStore is a simple, high-level summary of where the cluster is in its lifecycle.
	// The conditions array field contain more detail about the cluster's status.
	// +optional
	State ClusterState `json:"state"`
	// Represents the observations of a AIStores's current state.
	// Known condition types are: "Initialized", "Created", and "Ready".
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`
	// Deprecated: this field is no longer used.
	// +optional
	ConsecutiveErrorCount int `json:"consecutive_error_count,omitempty"`
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

	// Size holds number of AIS Daemon (proxy/target) replicas.
	// Overrides value present in `AIStore` spec.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Size *int32 `json:"size,omitempty"`

	// ContainerSecurity holds the secrity context for AIS Daemon containers.
	// +optional
	ContainerSecurity *corev1.SecurityContext `json:"capabilities,omitempty"`

	// List of additional environment variables to set in the AIS Daemon container.
	// Overrides any default envs set by operator.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Affinity  - AIS Daemon pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// NodeSelector -  which must match a node's labels for the AIS Daemon pod to be scheduled on that node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations - list of tolerations for AIS Daemon pod
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// HostPort - host port to use for hostnetworking
	// +optional
	HostPort *int32 `json:"hostPort,omitempty"`
}

type TargetSpec struct {
	DaemonSpec `json:",inline"`
	Mounts     []Mount `json:"mounts"`
	// Deprecated: AllowSharedOrNoDisks - disables FsID and mountpath disks validation on target nodes
	// NOT recommended for production deployments
	// Use Mount.Label instead
	// +optional
	AllowSharedOrNoDisks *bool `json:"allowSharedNoDisks,omitempty"`

	// DisablePodAntiAffinity allows more than one target pod to be scheduled on same K8s node.
	// +optional
	DisablePodAntiAffinity *bool `json:"disablePodAntiAffinity,omitempty"`

	// hostNetwork - if set to true, the AIS Daemon pods for target are created in the host's network namespace (used for multihoming)
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`
}

type Mount struct {
	Path         string                `json:"path"`
	Size         resource.Quantity     `json:"size"`
	StorageClass *string               `json:"storageClass,omitempty"` // storage class for volume resource
	Selector     *metav1.LabelSelector `json:"selector,omitempty"`     // selector for choosing PVs
	// Mountpath labels can be used for mapping mountpaths to disks, enabling disk sharing,
	// defining storage classes for bucket-specific storage, and allowing user-defined mountpath
	// grouping for capacity and storage class differentiation
	Label *string `json:"label,omitempty"`
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

// AddOrUpdateCondition is used to add a new/update an existing condition type.
func (ais *AIStore) AddOrUpdateCondition(c *metav1.Condition) {
	c.ObservedGeneration = ais.GetGeneration()
	meta.SetStatusCondition(&ais.Status.Conditions, *c)
}

func (ais *AIStore) IsConditionTrue(conditionType ClusterConditionType) bool {
	return meta.IsStatusConditionTrue(ais.Status.Conditions, string(conditionType))
}

// SetCondition add a new condition and sets it to `True`.
func (ais *AIStore) SetCondition(conditionType ClusterConditionType) {
	var msg string
	switch conditionType {
	case ConditionInitialized:
		msg = "Success initializing cluster"
	case ConditionCreated:
		msg = "Success creating AIS cluster"
	case ConditionReady:
		msg = "Cluster is ready"
	case ConditionReadyRebalance:
		msg = "Cluster is ready to rebalance"
	}
	ais.AddOrUpdateCondition(&metav1.Condition{
		Type:    string(conditionType),
		Status:  metav1.ConditionTrue,
		Reason:  string(conditionType),
		Message: msg,
	})
}

// SetConditionFalse updates the given condition's status to `False`
//   - `reason` - tag why the condition is being set to `False`.
//   - `msg` - a human-readable message indicating details about state change.
func (ais *AIStore) SetConditionFalse(conditionType ClusterConditionType, reason ClusterConditionReason, msg string) {
	ais.AddOrUpdateCondition(&metav1.Condition{
		Type:    string(conditionType),
		Status:  metav1.ConditionFalse,
		Reason:  string(reason),
		Message: msg,
	})
}

func (ais *AIStore) SetState(state ClusterState) {
	ais.Status.State = state
}

func (ais *AIStore) HasState(state ClusterState) bool {
	return ais.Status.State == state
}

func (ais *AIStore) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.Name,
		Namespace: ais.Namespace,
	}
}

func (ais *AIStore) GetClusterDomain() string {
	if ais.Spec.ClusterDomain == nil {
		return defaultClusterDomain
	}
	return *ais.Spec.ClusterDomain
}

func (ais *AIStore) ProxyStatefulSetName() string {
	return ais.Name + "-" + aisapc.Proxy
}

func (ais *AIStore) DefaultPrimaryName() string {
	return ais.ProxyStatefulSetName() + "-0"
}

func (ais *AIStore) GetProxySize() int32 {
	if ais.Spec.ProxySpec.Size != nil {
		return *ais.Spec.ProxySpec.Size
	}
	return *ais.Spec.Size
}

func (ais *AIStore) GetTargetSize() int32 {
	if ais.Spec.TargetSpec.Size != nil {
		return *ais.Spec.TargetSpec.Size
	}
	return *ais.Spec.Size
}

func (ais *AIStore) GetDefaultProxyURL() string {
	scheme := "http"
	if ais.UseHTTPS() {
		scheme = "https"
	}
	primaryProxy := ais.DefaultPrimaryName()
	domain := ais.GetClusterDomain()
	svcName := ais.ProxyStatefulSetName()
	intraCtrlPort := ais.Spec.ProxySpec.IntraControlPort.String()
	// Example: http://ais-proxy-0.ais-proxy.ais.svc.cluster.local:51080
	return fmt.Sprintf("%s://%s.%s.%s.svc.%s:%s", scheme, primaryProxy, svcName, ais.Namespace, domain, intraCtrlPort)
}

func (ais *AIStore) ShouldStartShutdown() bool {
	return ais.Spec.ShutdownCluster != nil && *ais.Spec.ShutdownCluster && ais.HasState(ClusterReady)
}

func (ais *AIStore) ShouldBeShutdown() bool {
	return ais.Spec.ShutdownCluster != nil && *ais.Spec.ShutdownCluster
}

// ShouldDecommission Determines if we should begin decommissioning the cluster
func (ais *AIStore) ShouldDecommission() bool {
	// We should only begin decommissioning if
	// 1. CR is marked for deletion
	// 2. We aren't already in the decommission or final cleanup stages
	return !ais.IsDecommissioningOrCleaning() && ais.IsMarkedForDeletion()
}

func (ais *AIStore) IsDecommissioningOrCleaning() bool {
	return ais.HasState(ClusterDecommissioning) ||
		ais.HasState(ClusterCleanup) ||
		ais.HasState(HostCleanup) ||
		ais.HasState(ClusterFinalized)
}

func (ais *AIStore) IsMarkedForDeletion() bool {
	return !ais.GetDeletionTimestamp().IsZero()
}

// ShouldCleanupMetadata Determines if we are doing a full decommission -- unrecoverable, including metadata
func (ais *AIStore) ShouldCleanupMetadata() bool {
	return ais.Spec.CleanupMetadata != nil && *ais.Spec.CleanupMetadata
}

func (ais *AIStore) AllowTargetSharedNodes() bool {
	allowSharedNodes := ais.Spec.TargetSpec.DisablePodAntiAffinity != nil && *ais.Spec.TargetSpec.DisablePodAntiAffinity
	//nolint:all
	deprecatedAllow := ais.Spec.DisablePodAntiAffinity != nil && *ais.Spec.DisablePodAntiAffinity
	// Backwards compatible check -- allow if either is true
	return allowSharedNodes || deprecatedAllow
}

// CompareVersion returns true if the spec `aisnode` version is the same or newer than the one provided
func (ais *AIStore) CompareVersion(version string) (bool, error) {
	img := ais.Spec.NodeImage
	parts := strings.Split(img, ":")
	if len(parts) != 2 {
		return false, fmt.Errorf("image does not have a proper tag: %q", img)
	}
	// Allow for hyphen-separated tags, e.g. aisnode:v3.24-rc3
	tag := strings.Split(parts[1], "-")[0]
	if !semver.IsValid(tag) {
		return false, fmt.Errorf("image tag does not use semantic versioning, image: %q", img)
	}
	// Check version is at least the provided version
	return semver.Compare(tag, version) >= 0, nil
}

func (ais *AIStore) UseHTTPS() bool {
	return ais.UseHTTPSSecret() || ais.UseHTTPSCertManager()
}

func (ais *AIStore) UseHTTPSSecret() bool {
	return ais.Spec.TLSSecretName != nil
}

func (ais *AIStore) UseHTTPSCertManager() bool {
	return ais.Spec.TLSCertManagerIssuerName != nil
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
