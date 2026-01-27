// Package v1beta1 contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"crypto/tls"
	"fmt"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
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
	azureStorageAccount  = "AZURE_STORAGE_ACCOUNT"
	azureStorageKey      = "AZURE_STORAGE_KEY"
	aisAzureURL          = "AIS_AZURE_URL"
)

// PubNetDNSMode defines allowed values for publicNetDNSMode spec option
type PubNetDNSMode string

const (
	PubNetDNSModeIP   PubNetDNSMode = "IP"
	PubNetDNSModeNode PubNetDNSMode = "Node"
	PubNetDNSModePod  PubNetDNSMode = "Pod"
)

// NOTE: json tags are required. Any new fields you add must have json tags for the fields to be serialized.
// IMPORTANT: Run "make" to regenerate code after modifying this file

// AuthSpec defines the configuration for accessing AuthN service
// Exactly one of UsernamePassword or TokenExchange must be specified
// +kubebuilder:validation:XValidation:rule="(has(self.usernamePassword) && !has(self.tokenExchange)) || (!has(self.usernamePassword) && has(self.tokenExchange))",message="exactly one of usernamePassword or tokenExchange must be specified"
type AuthSpec struct {
	// ServiceURL is the base URL of the AuthN service (scheme + host + optional port, no path)
	// Supports formats: "http://hostname[:port]" or "https://hostname[:port]"
	// TLS is determined from the URL scheme (https = TLS enabled)
	// Port defaults to 80 for http and 443 for https if not specified
	// If not specified, defaults to "http://ais-authn.ais:52001"
	// +optional
	ServiceURL *string `json:"serviceURL,omitempty"`

	// UsernamePassword authentication configuration using static credentials
	// +optional
	UsernamePassword *UsernamePasswordAuth `json:"usernamePassword,omitempty"`

	// TokenExchange authentication configuration using RFC 8693 OAuth 2.0 Token Exchange
	// This is the preferred method for workload identity and eliminates the need for static credentials
	// +optional
	TokenExchange *TokenExchangeAuth `json:"tokenExchange,omitempty"`

	// TLS configuration for secure connections with Auth service
	// +optional
	TLS *AuthTLSConfig `json:"tls,omitempty"`
}

// AuthTLSConfig defines TLS configuration for Auth connections
type AuthTLSConfig struct {
	// CACertPath is a filesystem path to a CA certificate file (PEM format)
	// This certificate will be added to the trust store for verifying Auth service certificates
	// Example: "/etc/ssl/certs/custom-ca.crt"
	// +optional
	CACertPath string `json:"caCertPath,omitempty"`

	// InsecureSkipVerify disables TLS certificate verification (not recommended for production)
	// If true, the operator will accept any certificate presented by the Auth service
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// UsernamePasswordAuth defines authentication using static username/password credentials
type UsernamePasswordAuth struct {
	// SecretName is the name of the secret containing auth service admin credentials (SU-NAME and SU-PASS)
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`

	// SecretNamespace is the namespace of the secret containing auth service admin credentials
	// If not specified, defaults to the AIStore cluster namespace
	// +optional
	SecretNamespace *string `json:"secretNamespace,omitempty"`

	// LoginConf contains details for OAuth 2.0 compliant password-based login
	// If not set, the operator will log in using the native AIStore AuthN service API
	// +optional
	LoginConf *AuthServerLoginConf `json:"loginConf,omitempty"`
}

// AuthServerLoginConf defines fields used for getting a token from any OAuth 2.0 service
type AuthServerLoginConf struct {
	// Client ID for the auth service, used when fetching a token
	ClientID string `json:"clientID"`
	// Scope to pass when fetching a token from the configured auth service
	// +optional
	Scope *string `json:"scope,omitempty"`
}

// TokenExchangeAuth defines authentication using RFC 8693 OAuth 2.0 Token Exchange
type TokenExchangeAuth struct {
	// TokenPath is the path to the service account token file
	// If not specified, defaults to "/var/run/secrets/kubernetes.io/serviceaccount/token"
	// +optional
	TokenPath *string `json:"tokenPath,omitempty"`

	// TokenExchangeEndpoint is the AuthN endpoint for token exchange
	// If not specified, defaults to "/token"
	// +optional
	TokenExchangeEndpoint *string `json:"tokenExchangeEndpoint,omitempty"`
}

// AdminClientSpec defines the optional admin client
type AdminClientSpec struct {
	// Enabled controls whether the admin client deployment is created.
	// When AdminClient is specified without this field, it defaults to true.
	// Set to false to disable the admin client while preserving other configuration.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Image is the container image for the admin client
	// Defaults to "aistorage/ais-util:latest"
	// +optional
	Image *string `json:"image,omitempty"`

	// ImagePullPolicy for the admin client container
	// +optional
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Resources for the admin client container
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Annotations to apply to the admin client
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to apply to the admin client in addition to the default labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// NodeSelector for admin client scheduling
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity scheduling rules for the admin client
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations for the admin client
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Env allows specifying additional environment variables for the admin client container
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// CAConfigMap specifies the ConfigMap containing the CA bundle for TLS trust
	// When specified, the ConfigMap is mounted and AIS_CLIENT_CA is set
	// +optional
	CAConfigMap *CAConfigMapRef `json:"caConfigMap,omitempty"`
}

// CAConfigMapRef references a ConfigMap containing a CA certificate bundle
type CAConfigMapRef struct {
	// Name of the ConfigMap containing the CA bundle
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within the ConfigMap containing the CA certificate bundle in PEM format
	// Defaults to "trust-bundle.pem"
	// +optional
	Key *string `json:"key,omitempty"`
}

// TLSCertificateMode defines the certificate delivery mechanism
type TLSCertificateMode string

const (
	// TLSCertificateModeSecret uses a shared Secret mounted to all pods
	TLSCertificateModeSecret TLSCertificateMode = "secret"
	// TLSCertificateModeCSI uses cert-manager CSI driver for per-pod certificates
	TLSCertificateModeCSI TLSCertificateMode = "csi"
)

// TLSSpec configures TLS certificate provisioning
// +kubebuilder:validation:XValidation:rule="[has(self.secretName), has(self.certificate)].filter(x, x).size() <= 1",message="specify only one: secretName or certificate"
type TLSSpec struct {
	// SecretName references an existing TLS secret
	// +optional
	SecretName *string `json:"secretName,omitempty"`

	// Certificate configures cert-manager certificate generation
	// +optional
	Certificate *TLSCertificateConfig `json:"certificate,omitempty"`
}

// TLSCertificateConfig configures cert-manager certificate generation
type TLSCertificateConfig struct {
	// IssuerRef references a cert-manager issuer
	IssuerRef CertIssuerRef `json:"issuerRef"`

	// AdditionalDNSNames are extra DNS names to include in the certificate
	// +optional
	AdditionalDNSNames []string `json:"additionalDNSNames,omitempty"`

	// Duration is the lifetime of the certificate (default: 8760h = 1 year)
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`

	// RenewBefore is when to start renewing (default: 720h = 30 days before expiry)
	// +optional
	RenewBefore *metav1.Duration `json:"renewBefore,omitempty"`

	// Mode specifies how certificates are delivered: "secret" (default) or "csi"
	// +kubebuilder:validation:Enum=secret;csi
	// +kubebuilder:default:=secret
	// +optional
	Mode TLSCertificateMode `json:"mode,omitempty"`
}

// CertIssuerRef references a cert-manager Issuer or ClusterIssuer
type CertIssuerRef struct {
	// Name of the issuer
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Kind is Issuer or ClusterIssuer (default: ClusterIssuer)
	// +kubebuilder:validation:Enum=Issuer;ClusterIssuer
	// +kubebuilder:default:=ClusterIssuer
	// +optional
	Kind string `json:"kind,omitempty"`
}

// AIStoreSpec defines the desired state of AIStore
// +kubebuilder:validation:XValidation:rule="(has(self.targetSpec.size) && has(self.proxySpec.size)) || has(self.size)",message="Invalid cluster size, it is either not specified or value is not valid"
// +kubebuilder:validation:XValidation:rule="[has(self.tls), has(self.tlsCertificate), has(self.tlsCertManagerIssuerName), has(self.tlsSecretName)].filter(x, x).size() <= 1",message="specify only one TLS option: tls, tlsCertificate, tlsCertManagerIssuerName, or tlsSecretName"
type AIStoreSpec struct {
	// Size of the cluster i.e. number of proxies and number of targets.
	// This can be changed by specifying size in either `proxySpec` or `targetSpec`.
	// +kubebuilder:validation:Minimum=-1
	// +optional
	Size *int32 `json:"size,omitempty"`
	// Container image used for `aisnode` container.
	// +kubebuilder:validation:MinLength=1
	NodeImage string `json:"nodeImage"`
	// Container image used for `ais-init` container.
	// +kubebuilder:validation:MinLength=1
	InitImage string `json:"initImage"`
	// Container image used for `ais-logs` container.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Optional
	LogSidecarImage *string `json:"logSidecarImage"`
	// StateStorageClass recommended if possible
	// See docs/state_storage.md
	// Path on host used for state
	// +optional
	HostpathPrefix *string `json:"hostpathPrefix,omitempty"`
	// Used for creating dynamic volumes for storing state
	// +optional
	StateStorageClass *string         `json:"stateStorageClass,omitempty"`
	ConfigToUpdate    *ConfigToUpdate `json:"configToUpdate,omitempty"`

	// IssuerCAConfigMap is the name of a ConfigMap containing the CA certificate bundle
	// for verifying OIDC issuer certificates. When set, the ConfigMap will be mounted
	// to proxy pods at /etc/ais/oidc-ca and spec.configToUpdate.auth.oidc.issuer_ca_bundle
	// will be automatically configured to reference it.
	// The ConfigMap must contain a key named "ca.crt" with the CA bundle in PEM format.
	// +kubebuilder:validation:MinLength=1
	// +optional
	IssuerCAConfigMap *string `json:"issuerCAConfigMap,omitempty"`

	// Map of primary host to comma-separated string of all hosts for multi-home
	// +optional
	HostnameMap map[string]string `json:"hostnameMap,omitempty"`
	// Which mode to use when communicating with the deployed AIS cluster's API
	// Defaults to use internal headless proxy service if not provided
	// +optional
	APIMode *string `json:"apiMode,omitempty"`
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

	// Defines the cluster domain name for DNS. Default: cluster.local.
	// +optional
	ClusterDomain *string `json:"clusterDomain,omitempty"`

	// Secret name containing GCP config and credentials
	// +optional
	GCPSecretName *string `json:"gcpSecretName,omitempty"`

	// Secret name containing AWS config and credentials
	// +optional
	AWSSecretName *string `json:"awsSecretName,omitempty"`

	// Secret name containing OCI config and credentials
	// +optional
	OCISecretName *string `json:"ociSecretName,omitempty"`

	// Logs directory on host to store AIS logs
	// +optional
	LogsDirectory string `json:"logsDir,omitempty"`

	// TLS configures TLS certificate provisioning
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`

	// Deprecated: Use spec.tls.certificate instead
	// +optional
	TLSCertificate *TLSCertificateConfig `json:"tlsCertificate,omitempty"`

	// Deprecated: Use spec.tls.secretName instead
	// +optional
	TLSSecretName *string `json:"tlsSecretName,omitempty"`

	// Secret name containing OTEL trace-exporter token.
	TracingTokenSecretName *string `json:"tracingTokenSecretName,omitempty"`

	// Deprecated: Use spec.tls.certificate with mode: csi instead
	// +optional
	TLSCertManagerIssuerName *string `json:"tlsCertManagerIssuerName,omitempty"`

	// Secret name containing AuthN's JWT signing key
	// +optional
	AuthNSecretName *string `json:"authNSecretName,omitempty"`

	// Auth specifies the Auth service configuration for admin authentication
	// If not specified, the operator will look for configuration in the legacy ConfigMap
	// +optional
	Auth *AuthSpec `json:"auth,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to pull container images of AIS Daemons
	// Applied to the service account so all pods inherit authentication automatically
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// EnableExternalLB, if set, enables external access to AIS cluster using LoadBalancer service
	EnableExternalLB bool `json:"enableExternalLB"`

	// PublicNetDNSMode Defines the public network DNS name to use with hostPort.
	// Defaults to 'IP' to use the host IP.
	// Other options include:
	//  'Node' to use the K8s node DNS name
	//  'Pod' to use the pod DNS name (resolves to host IP when used with host networking)
	// +kubebuilder:validation:Enum=IP;Node;Pod
	// +kubebuilder:default:=IP
	// +optional
	PublicNetDNSMode *PubNetDNSMode `json:"publicNetDNSMode,omitempty"`

	// AdminClient specifies the optional admin client deployment
	// The deployment is automatically configured to connect to this AIS cluster
	// +optional
	AdminClient *AdminClientSpec `json:"adminClient,omitempty"`

	// PriorityClassName specifies the priority class name for AIS daemon pods (proxy and target).
	// Setting a high priority class prevents pods from being evicted during node pressure events.
	// See: https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`

	// LogSidecarResources specifies resource requirements for the ais-logs sidecar container.
	// Setting requests equal to limits gives the pod Guaranteed QoS, protecting it from eviction.
	// If not specified, the sidecar runs with no resource constraints (BestEffort for that container).
	// +optional
	LogSidecarResources *corev1.ResourceRequirements `json:"logSidecarResources,omitempty"`
}

// AIStoreStatus defines the observed state of AIStore
type AIStoreStatus struct {
	// The state of a AIStore is a simple, high-level summary of where the cluster is in its lifecycle.
	// The conditions array field contain more detail about the cluster's status.
	// +optional
	State ClusterState `json:"state"`

	// AutoScaleStatus is used to track what nodes the controller
	// has discovered for the cluster
	// this is only used for clusters that are set to auto-scale
	// +optional
	AutoScaleStatus AutoScaleStatus `json:"autoscaleStatus"`

	// IntraClusterURL is the in cluster url for the AIS cluster
	// +optional
	IntraClusterURL string `json:"intraClusterURL"`
	// ClusterID is a unique identifier for the cluster.
	// +optional
	ClusterID string `json:"clusterID,omitempty"`
	// Represents the observations of a AIStores's current state.
	// Known condition types are: "Initialized", "Created", and "Ready".
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`
}

type AutoScaleStatus struct {
	// ProxyNodes is a list of nodes that have matched the node selector
	// this is only used for auto-scaling clusters
	// +optional
	ExpectedProxyNodes []string `json:"expectedProxyNodes"`

	// TargetNodes is a list of nodes that have matched the node selector
	// this is only used for auto-scaling clusters
	// +optional
	ExpectedTargetNodes []string `json:"expectedTargetNodes"`
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
	// +kubebuilder:validation:Minimum=-1
	// +optional
	Size *int32 `json:"size,omitempty"`

	// AutoScaleConf contains additional configuration for auto-scaling (size == -1)
	// +optional
	AutoScaleConf *AutoScaleConf `json:"autoScale,omitempty"`

	// Annotations holds additional pod annotations for AIStore daemon pods.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels holds additional pod labels for AIStore daemon pods.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Compute Resources required by AIStore daemon pods.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ContainerSecurity holds the security context for AIS Daemon containers.
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
	// HostPort - Port to bind directly to a specific port on the host
	// +optional
	HostPort *int32 `json:"hostPort,omitempty"`
}

type TargetSpec struct {
	DaemonSpec `json:",inline"`
	Mounts     []Mount `json:"mounts"`
	// DisablePodAntiAffinity allows more than one target pod to be scheduled on same K8s node.
	// +optional
	DisablePodAntiAffinity *bool `json:"disablePodAntiAffinity,omitempty"`

	// hostNetwork - if set to true, the AIS Daemon pods for target are created in the host's network namespace (used for multihoming)
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`

	// PodDisruptionBudget specifies the PDB configuration for target pods.
	// When enabled, a PodDisruptionBudget is created to protect target pods from voluntary evictions
	// during node drain or cluster scale-down operations by cloud providers.
	// +optional
	PodDisruptionBudget *PDBSpec `json:"pdb,omitempty"`
}

// PDBSpec defines the PodDisruptionBudget configuration for target pods
type PDBSpec struct {
	// Enabled controls whether a PodDisruptionBudget is created for target pods.
	// When true, the operator will create and manage a PDB for the target StatefulSet.
	// Defaults to false if not specified.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MaxUnavailable specifies the maximum number of target pods that can be unavailable
	// during voluntary disruptions. It can be represented as an absolute number (e.g. 1)
	// or a percentage (e.g. "10%"). Setting to 0 prevents any voluntary evictions.
	// Defaults to 0 if not specified.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

type Mount struct {
	Path string            `json:"path"`
	Size resource.Quantity `json:"size"`
	// +optional
	StorageClass *string `json:"storageClass,omitempty"` // storage class for volume resource
	// +optional
	UseHostPath *bool                 `json:"useHostPath,omitempty"` // skip PVs and mount directly on the host
	Selector    *metav1.LabelSelector `json:"selector,omitempty"`    // selector for choosing PVs
	// Mountpath labels can be used for mapping mountpaths to disks, enabling disk sharing,
	// defining storage classes for bucket-specific storage, and allowing user-defined mountpath
	// grouping for capacity and storage class differentiation
	Label *string `json:"label,omitempty"`
}

type AutoScaleConf struct {
	// Maximum size of a given node type when auto-scaling is enabled
	// +kubebuilder:validation:Minimum=1
	// +optional
	SizeLimit *int32 `json:"sizeLimit,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AIStore is the Schema for the aistores API.
//
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="The current state of the resource"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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
	if ais.IsProxyAutoScaling() {
		proxyNodes := int32(len(ais.Status.AutoScaleStatus.ExpectedProxyNodes))
		autoScaleConf := ais.Spec.ProxySpec.AutoScaleConf
		if autoScaleConf != nil && autoScaleConf.SizeLimit != nil {
			return min(proxyNodes, *autoScaleConf.SizeLimit)
		}
		return proxyNodes
	}
	if ais.Spec.ProxySpec.Size != nil {
		return *ais.Spec.ProxySpec.Size
	}
	return *ais.Spec.Size
}

func (ais *AIStore) GetTargetSize() int32 {
	if ais.IsTargetAutoScaling() {
		targetNodes := int32(len(ais.Status.AutoScaleStatus.ExpectedTargetNodes))
		autoScaleConf := ais.Spec.TargetSpec.AutoScaleConf
		if autoScaleConf != nil && autoScaleConf.SizeLimit != nil {
			return min(targetNodes, *autoScaleConf.SizeLimit)
		}
		return targetNodes
	}
	if ais.Spec.TargetSpec.Size != nil {
		return *ais.Spec.TargetSpec.Size
	}
	return *ais.Spec.Size
}

func (ais *AIStore) GetDefaultProxyURL() string {
	primaryProxy := ais.DefaultPrimaryName()
	svcName := ais.ProxyStatefulSetName()
	svcSuffix := ais.getControlSvcSuffix()
	// Example: http://ais-proxy-0.ais-proxy.ais.svc.cluster.local:51080
	return fmt.Sprintf("%s://%s.%s.%s.%s", ais.getScheme(), primaryProxy, svcName, ais.Namespace, svcSuffix)
}

func (ais *AIStore) GetIntraClusterURL() string {
	svcName := ais.ProxyStatefulSetName()
	svcSuffix := ais.getPublicSvcSuffix()
	// Example: http://ais-proxy.ais.svc.cluster.local:51080
	return fmt.Sprintf("%s://%s.%s.%s", ais.getScheme(), svcName, ais.Namespace, svcSuffix)
}

func (ais *AIStore) GetDiscoveryProxyURL() string {
	svcName := ais.ProxyStatefulSetName()
	svcSuffix := ais.getControlSvcSuffix()
	// Example: http://ais-proxy.ais.svc.cluster.local:51080
	return fmt.Sprintf("%s://%s.%s.%s", ais.getScheme(), svcName, ais.Namespace, svcSuffix)
}

func (ais *AIStore) UseNodeNameForPublicNet() bool {
	if ais.Spec.PublicNetDNSMode != nil && *ais.Spec.PublicNetDNSMode == PubNetDNSModeNode {
		return true
	}
	return false
}

func (ais *AIStore) getScheme() string {
	if ais.UseHTTPS() {
		return "https"
	}
	return "http"
}

func (ais *AIStore) getControlSvcSuffix() string {
	intraCtrlPort := ais.Spec.ProxySpec.IntraControlPort.String()
	return fmt.Sprintf("svc.%s:%s", ais.GetClusterDomain(), intraCtrlPort)
}

func (ais *AIStore) getPublicSvcSuffix() string {
	pubPort := ais.Spec.ProxySpec.PublicPort.String()
	return fmt.Sprintf("svc.%s:%s", ais.GetClusterDomain(), pubPort)
}

func (ais *AIStore) ShouldStartShutdown() bool {
	return ais.ShouldBeShutdown() && ais.HasState(ClusterReady)
}

func (ais *AIStore) ShouldBeShutdown() bool {
	return ais.Spec.ShutdownCluster != nil && *ais.Spec.ShutdownCluster
}

func (ais *AIStore) UseHostNetwork() bool {
	return ais.Spec.TargetSpec.HostNetwork != nil && *ais.Spec.TargetSpec.HostNetwork
}

func (ais *AIStore) ShouldIncludeClientCert() bool {
	if ais.Spec.ConfigToUpdate == nil ||
		ais.Spec.ConfigToUpdate.Net == nil ||
		ais.Spec.ConfigToUpdate.Net.HTTP == nil ||
		ais.Spec.ConfigToUpdate.Net.HTTP.ClientAuthTLS == nil {
		return false
	}
	return tls.ClientAuthType(*ais.Spec.ConfigToUpdate.Net.HTTP.ClientAuthTLS) > tls.NoClientCert
}

func (ais *AIStore) IsFullyAutoScaling() bool {
	return ais.GetTargetSize() == -1 && ais.GetProxySize() == -1
}

func (ais *AIStore) IsTargetAutoScaling() bool {
	if ais.Spec.Size != nil {
		if *ais.Spec.Size == -1 {
			return true
		}
	}
	if ais.Spec.TargetSpec.Size != nil && *ais.Spec.TargetSpec.Size == -1 {
		return true
	}
	return false
}

func (ais *AIStore) IsProxyAutoScaling() bool {
	if ais.Spec.Size != nil {
		if *ais.Spec.Size == -1 {
			return true
		}
	}
	if ais.Spec.ProxySpec.Size != nil && *ais.Spec.ProxySpec.Size == -1 {
		return true
	}
	return false
}

func (m *Mount) IsHostPath() bool {
	return m.UseHostPath != nil && *m.UseHostPath
}

// GetPVCName returns the associated PVC name we expect to mount for a given MountPath
// This must follow the same convention as our existing automation for PVC creation
func (m *Mount) GetPVCName(aisName string) string {
	return aisName + strings.ReplaceAll(m.Path, "/", "-")
}

func (ais *AIStore) GetTargetDNSPolicy() corev1.DNSPolicy {
	if ais.UseHostNetwork() {
		return corev1.DNSClusterFirstWithHostNet
	}
	return corev1.DNSClusterFirst
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

// GetAllTolerations returns tolerations for all proxy and target pods
func (ais *AIStore) GetAllTolerations() []corev1.Toleration {
	return mergeTolerationsUnique(ais.Spec.ProxySpec.Tolerations, ais.Spec.TargetSpec.Tolerations)
}

func mergeTolerationsUnique(a, b []corev1.Toleration) []corev1.Toleration {
	out := make([]corev1.Toleration, 0, len(a)+len(b))
	out = append(out, a...)

	for i := range b {
		tb := &b[i]
		exists := false
		for j := range out {
			if out[j].MatchToleration(tb) {
				exists = true
				break
			}
		}
		if !exists {
			out = append(out, *tb)
		}
	}
	return out
}

func (ais *AIStore) AllowTargetSharedNodes() bool {
	return ais.Spec.TargetSpec.DisablePodAntiAffinity != nil && *ais.Spec.TargetSpec.DisablePodAntiAffinity
}

func (ais *AIStore) TargetPDBEnabled() bool {
	pdb := ais.Spec.TargetSpec.PodDisruptionBudget
	return pdb != nil && pdb.Enabled
}

func (ais *AIStore) GetTargetPDBMaxUnavailable() intstr.IntOrString {
	pdb := ais.Spec.TargetSpec.PodDisruptionBudget
	if pdb != nil && pdb.MaxUnavailable != nil {
		return *pdb.MaxUnavailable
	}
	return intstr.FromInt32(0)
}

// AdminClientEnabled returns true if the admin client should be deployed
func (ais *AIStore) AdminClientEnabled() bool {
	if ais.Spec.AdminClient == nil {
		return false
	}
	// Enabled defaults to true when AdminClient is specified
	return ais.Spec.AdminClient.Enabled == nil || *ais.Spec.AdminClient.Enabled
}

// AdminClientName returns the name for the admin client deployment
func (ais *AIStore) AdminClientName() string {
	return ais.Name + "-client"
}

func (s *AIStoreSpec) hasAWSBackend() bool {
	return s.AWSSecretName != nil || s.isProviderInConf(aisapc.AWS)
}

func (s *AIStoreSpec) HasGCPBackend() bool {
	return s.GCPSecretName != nil || s.isProviderInConf(aisapc.GCP)
}

func (s *AIStoreSpec) HasOCIBackend() bool {
	return s.OCISecretName != nil || s.isProviderInConf(aisapc.OCI)
}

func (s *AIStoreSpec) hasAzureBackend() bool {
	return s.HasAzureConfig() || s.isProviderInConf(aisapc.Azure)
}

func (s *AIStoreSpec) HasAzureConfig() bool {
	var azureEnvVars = []string{
		azureStorageAccount,
		azureStorageKey,
		aisAzureURL,
	}
	for _, env := range s.TargetSpec.Env {
		for _, key := range azureEnvVars {
			if env.Name == key {
				return true
			}
		}
	}
	return false
}

func (s *AIStoreSpec) isProviderInConf(provider string) bool {
	if backend := s.GetBackendConfig(); backend != nil {
		_, exists := backend[provider]
		return exists
	}
	return false
}

func (s *AIStoreSpec) GetBackendConfig() map[string]Empty {
	if s.ConfigToUpdate == nil || s.ConfigToUpdate.Backend == nil {
		return nil
	}
	return *s.ConfigToUpdate.Backend
}

func (s *AIStoreSpec) HasCloudBackend() bool {
	return s.hasAWSBackend() || s.HasGCPBackend() || s.HasOCIBackend() || s.hasAzureBackend()
}

func (ais *AIStore) UseHTTPS() bool {
	return ais.Spec.ConfigToUpdate != nil && ais.Spec.ConfigToUpdate.Net != nil && ais.Spec.ConfigToUpdate.Net.HTTP != nil && ais.Spec.ConfigToUpdate.Net.HTTP.UseHTTPS != nil && *ais.Spec.ConfigToUpdate.Net.HTTP.UseHTTPS
}

func (ais *AIStore) GetTLSSpec() *TLSSpec {
	// New field to take precedence
	if ais.Spec.TLS != nil {
		return ais.Spec.TLS
	}
	// Handle deprecated fields
	if ais.Spec.TLSSecretName != nil {
		return &TLSSpec{SecretName: ais.Spec.TLSSecretName}
	}
	if ais.Spec.TLSCertificate != nil {
		return &TLSSpec{Certificate: ais.Spec.TLSCertificate}
	}
	if ais.Spec.TLSCertManagerIssuerName != nil {
		return &TLSSpec{
			Certificate: &TLSCertificateConfig{
				IssuerRef: CertIssuerRef{Name: *ais.Spec.TLSCertManagerIssuerName},
				Mode:      TLSCertificateModeCSI,
			},
		}
	}
	return nil
}

// HasTLSEnabled returns true if any TLS configuration is specified
func (ais *AIStore) HasTLSEnabled() bool {
	return ais.GetTLSSpec() != nil
}

// GetTLSCertificate returns the TLS certificate config if present
func (ais *AIStore) GetTLSCertificate() *TLSCertificateConfig {
	tlsSpec := ais.GetTLSSpec()
	if tlsSpec != nil {
		return tlsSpec.Certificate
	}
	return nil
}

func (ais *AIStore) UseTLSSecret() bool {
	tlsSpec := ais.GetTLSSpec()
	return tlsSpec != nil && tlsSpec.SecretName != nil && *tlsSpec.SecretName != ""
}

func (ais *AIStore) UseTLSCertificate() bool {
	certConfig := ais.GetTLSCertificate()
	if certConfig == nil {
		return false
	}
	return certConfig.Mode != TLSCertificateModeCSI
}

func (ais *AIStore) UseTLSCSI() bool {
	certConfig := ais.GetTLSCertificate()
	if certConfig == nil {
		return false
	}
	return certConfig.Mode == TLSCertificateModeCSI
}

func (ais *AIStore) GetTLSSecretName() string {
	if ais.UseTLSSecret() {
		return *ais.GetTLSSpec().SecretName
	}
	if ais.UseTLSCertificate() {
		return fmt.Sprintf("%s-tls", ais.Name)
	}
	return ""
}

func (ais *AIStore) GetAPIMode() string {
	if ais.Spec.APIMode != nil {
		return *ais.Spec.APIMode
	}
	return ""
}

func (ais *AIStore) MaxLogTotal() *SizeIEC {
	if ais.Spec.ConfigToUpdate == nil || ais.Spec.ConfigToUpdate.Log == nil {
		return nil
	}
	return ais.Spec.ConfigToUpdate.Log.MaxTotal
}

// GetRequiredAudiences extracts all audiences from the AIStore cluster's required claims if set.
// Returns nil if not configured
func (ais *AIStore) GetRequiredAudiences() []string {
	if ais.Spec.ConfigToUpdate == nil ||
		ais.Spec.ConfigToUpdate.Auth == nil ||
		ais.Spec.ConfigToUpdate.Auth.RequiredClaims == nil ||
		ais.Spec.ConfigToUpdate.Auth.RequiredClaims.Aud == nil {
		return nil
	}
	return *ais.Spec.ConfigToUpdate.Auth.RequiredClaims.Aud
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
