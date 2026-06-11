// Package v1alpha1 contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerConfSpec configures token issuance, signing, and user storage.
type ServerConfSpec struct {
	// ExpirationTime is the default lifetime for issued JWTs.
	// +optional
	ExpirationTime *metav1.Duration `json:"expirationTime,omitempty"`

	// MaxTokenAge caps how long any issued JWT may remain valid.
	// +optional
	MaxTokenAge *metav1.Duration `json:"maxTokenAge,omitempty"`

	// SigningKey configures JWT signing parameters in AuthN config. Ignored when hmacSecret is set.
	// +optional
	SigningKey *SigningKeySpec `json:"signingKey,omitempty"`

	// DB configures persistent storage for users and roles.
	// +optional
	DB *DBSpec `json:"db,omitempty"`
}

// SigningKeySpec configures JWT signing key parameters in AuthN config.
type SigningKeySpec struct {
	// Bits is the RSA key size when using RSA signing.
	// +kubebuilder:validation:Minimum=2048
	// +optional
	Bits *int32 `json:"bits,omitempty"`

	// Mode set to "external" when the signing key is managed outside the server (no auto-generation or API rotation).
	// +kubebuilder:validation:Enum=external
	// +optional
	Mode *string `json:"mode,omitempty"`
}

// ConfigSpec holds non-secret authN application config
type ConfigSpec struct {
	// Auth configures token issuance, signing, and user storage.
	// +optional
	Auth *ServerConfSpec `json:"auth,omitempty"`

	// Log configures process logging.
	// +optional
	Log *LogSpec `json:"log,omitempty"`

	// Net configuration (external URL for OIDC discovery, etc.).
	// +optional
	Net *NetSpec `json:"net,omitempty"`

	// Timeout configures HTTP handler timeouts.
	// +optional
	Timeout *TimeoutSpec `json:"timeout,omitempty"`
}

// LogSpec configures AuthN process logging.
type LogSpec struct {
	// Level is the log verbosity (0-5, higher is more verbose).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +optional
	Level *int32 `json:"level,omitempty"`

	// FlushInterval controls how often log buffers are flushed to disk.
	// +optional
	FlushInterval *metav1.Duration `json:"flushInterval,omitempty"`
}

type NetSpec struct {
	// ExternalURL is advertised for OIDC discovery when using RSA signing.
	// Example: https://ais-authn.ais.svc.cluster.local:52001
	// +optional
	ExternalURL *string `json:"externalURL,omitempty"`

	// HTTP configures the AuthN listen port and TLS settings.
	// +optional
	HTTP *HTTPConfSpec `json:"http,omitempty"`
}

// HTTPConfSpec configures the AuthN HTTP(S) listener.
type HTTPConfSpec struct {
	// Port is the HTTP(S) listen port.
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// DBSpec configures persistent AuthN user storage.
type DBSpec struct {
	// Type selects the user database backend.
	// +kubebuilder:validation:Enum=BuntDB
	// +optional
	Type *string `json:"type,omitempty"`
}

type TimeoutSpec struct {
	// Default timeout for AuthN HTTP handlers.
	// +optional
	DefaultTimeout *metav1.Duration `json:"defaultTimeout,omitempty"`
}

// CertIssuerRef references a cert-manager Issuer or ClusterIssuer.
type CertIssuerRef struct {
	// Name of the issuer.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Kind is Issuer or ClusterIssuer (default: ClusterIssuer).
	// +kubebuilder:validation:Enum=Issuer;ClusterIssuer
	// +kubebuilder:default:=ClusterIssuer
	// +optional
	Kind string `json:"kind,omitempty"`
}

// TLSSpec configures TLS for the AuthN HTTP server.
// TLS is active when secretName or certificate is set.
// +kubebuilder:validation:XValidation:rule="[has(self.secretName), has(self.certificate)].filter(x, x).size() <= 1",message="specify only one of secretName or certificate"
type TLSSpec struct {
	// SecretName references an existing kubernetes.io/tls Secret (tls.crt / tls.key).
	// +optional
	SecretName *string `json:"secretName,omitempty"`

	// Certificate configures cert-manager certificate provisioning for AuthN TLS.
	// +optional
	Certificate *TLSCertificateConfig `json:"certificate,omitempty"`
}

// TLSCertificateConfig configures cert-manager certificate provisioning for AuthN.
type TLSCertificateConfig struct {
	// IssuerRef references a cert-manager Issuer or ClusterIssuer.
	// +kubebuilder:validation:Required
	IssuerRef CertIssuerRef `json:"issuerRef"`

	// AdditionalDNSNames are extra DNS Subject Alternative Names to include
	// beyond the AuthN service DNS names the operator derives automatically.
	// +optional
	AdditionalDNSNames []string `json:"additionalDNSNames,omitempty"`

	// AdditionalIPAddresses are extra IP Subject Alternative Names to include.
	// +optional
	AdditionalIPAddresses []string `json:"additionalIPAddresses,omitempty"`

	// Duration is the certificate validity period.
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`

	// RenewBefore triggers renewal this long before expiry.
	// +optional
	RenewBefore *metav1.Duration `json:"renewBefore,omitempty"`
}

// PersistenceSpec configures the AuthN data volume (users, RSA keys).
// When neither hostPath nor storageClass is set, the operator provisions node-local (host-path) storage.
// +kubebuilder:validation:XValidation:rule="[has(self.hostPath), has(self.storageClass)].filter(x, x).size() <= 1",message="specify at most one of hostPath or storageClass"
type PersistenceSpec struct {
	// Size of the requested PVC when using storageClass.
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`

	// HostPath creates a node-local PV backed by a directory on the node.
	// +optional
	HostPath *string `json:"hostPath,omitempty"`

	// StorageClass provisions a dynamic PVC.
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`
}

// ExternalAccessSpec configures optional external Services (NodePort, LoadBalancer).
type ExternalAccessSpec struct {
	// NodePort exposes external access via a fixed node port. Setting this block enables it.
	// +optional
	NodePort *NodePortSpec `json:"nodePort,omitempty"`

	// LoadBalancer exposes external access via a cloud LB. Setting this block enables it.
	// +optional
	LoadBalancer *LoadBalancerSpec `json:"loadBalancer,omitempty"`
}

// NodePortSpec exposes AuthN via a NodePort Service.
type NodePortSpec struct {
	// Port on each node (30000-32767).
	// +kubebuilder:validation:Minimum=30000
	// +kubebuilder:validation:Maximum=32767
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// LoadBalancerSpec exposes AuthN via a cloud LoadBalancer Service.
type LoadBalancerSpec struct {
	// Port is the LoadBalancer Service port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default:=52001
	// +optional
	Port int32 `json:"port,omitempty"`

	// ClusterIP pins a static cluster IP for the LoadBalancer Service.
	// When empty, Kubernetes assigns one from the service CIDR range.
	// +optional
	ClusterIP *string `json:"clusterIP,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DeploymentSpec configures the AuthN Deployment.
type DeploymentSpec struct {
	// Image is the AuthN container image
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ImagePullPolicy for the AuthN container.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default:=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// +optional
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// LivenessProbe overrides the container liveness probe.
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// ReadinessProbe overrides the container readiness probe.
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
}

// AIStoreAuthSpec defines the desired AuthN server deployment.
type AIStoreAuthSpec struct {
	// Admin credentials Secret reference.
	// +optional
	AdminSecret *corev1.LocalObjectReference `json:"adminSecret,omitempty"`

	// HMACSecret names a Secret in the CR namespace holding the HMAC signing key.
	// When set, selects HMAC signing. Omit for RSA (keys on the persistence volume).
	// +optional
	HMACSecret *corev1.LocalObjectReference `json:"hmacSecret,omitempty"`

	// RSAPassphraseSecret names a Secret in the CR namespace holding the RSA key passphrase.
	// +optional
	RSAPassphraseSecret *corev1.LocalObjectReference `json:"rsaPassphraseSecret,omitempty"`

	// Config holds non-secret AuthN runtime settings.
	// +optional
	Config *ConfigSpec `json:"config,omitempty"`

	// TLS configures TLS for the AuthN HTTP server.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`

	// Persistence configures durable storage for AuthN state (users, RSA keys).
	// When omitted, defaults to a node-local (host-path) storage.
	// +kubebuilder:default:={}
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`

	// ExternalAccess configures optional external Services (NodePort, LoadBalancer).
	// +optional
	ExternalAccess *ExternalAccessSpec `json:"externalAccess,omitempty"`

	// Deployment configures the AuthN Deployment (image, pod policy).
	// +kubebuilder:validation:Required
	Deployment DeploymentSpec `json:"deployment"`
}

// AIStoreAuthStatus defines the observed state of the AuthN server.
type AIStoreAuthStatus struct {
	// Conditions describe the current state of the AuthN deployment.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ServiceURL is the in-cluster base URL operators and clients should use.
	// Example: https://ais-authn.ais.svc.cluster.local:52001
	// +optional
	ServiceURL string `json:"serviceURL,omitempty"`

	// ReadyReplicas reflects the number of ready pods in the managed Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aisauth
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.serviceURL"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AIStoreAuth is the Schema for the AuthN authentication server API.
// Deploy with the AIS operator; one AIStoreAuth instance is typically shared per namespace or environment.
type AIStoreAuth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIStoreAuthSpec   `json:"spec,omitempty"`
	Status AIStoreAuthStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIStoreAuthList contains a list of AIStoreAuth.
type AIStoreAuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIStoreAuth `json:"items"`
}
