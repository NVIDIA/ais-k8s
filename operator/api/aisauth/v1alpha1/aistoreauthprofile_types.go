/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	DefaultAuthProfileUserKey = "SU-NAME"
	DefaultAuthProfilePassKey = "SU-PASS"
)

// AIStoreAuthProfile defines trusted authentication provider endpoint configuration.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=aisauthprofile
type AIStoreAuthProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AIStoreAuthProfileSpec `json:"spec,omitempty"`
}

// AIStoreAuthProfileList is a list of AIStoreAuthProfile resources.
// +kubebuilder:object:root=true
type AIStoreAuthProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIStoreAuthProfile `json:"items"`
}

// AIStoreAuthProfileSpec defines trusted authentication provider endpoint configuration.
// Exactly one of UsernamePassword or TokenExchange must be specified.
// +kubebuilder:validation:XValidation:rule="has(self.usernamePassword) != has(self.tokenExchange)",message="exactly one of usernamePassword or tokenExchange must be specified"
type AIStoreAuthProfileSpec struct {
	// ServiceURL is the trusted authentication provider base URL (scheme + host + optional port, no path).
	// +kubebuilder:validation:MinLength=1
	ServiceURL string `json:"serviceURL"`

	// TLS configuration for connections to the authentication provider.
	// +optional
	TLS *AuthProfileTLSConfig `json:"tls,omitempty"`

	// UsernamePassword configures static credential login against the authentication provider.
	// +optional
	UsernamePassword *AuthProfileUsernamePassword `json:"usernamePassword,omitempty"`

	// TokenExchange configures RFC 8693 token exchange.
	// +optional
	TokenExchange *AuthProfileTokenExchange `json:"tokenExchange,omitempty"`
}

// AuthProfileUsernamePassword defines static username/password login against the authentication provider.
type AuthProfileUsernamePassword struct {
	// Secret references the K8s Secret containing auth provider admin credentials.
	// +kubebuilder:validation:Required
	Secret AuthProfileSecret `json:"secret"`

	// LoginConf contains OAuth 2.0 password-grant login details for the authentication provider.
	// +optional
	LoginConf *AuthProfileLoginConf `json:"loginConf,omitempty"`
}

// AuthProfileSecret configures the Secret containing auth provider admin credentials.
// Used for username-password login.
type AuthProfileSecret struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Secret.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// UserKey defines the Secret key for the username the client will use for login.
	// Defaults to SU-NAME to match AuthN default admin secret format.
	// +optional
	UserKey string `json:"userKey,omitempty"`

	// PassKey defines the Secret key for the password the client will use for login.
	// Defaults to SU-PASS to match AuthN default admin secret format.
	// +optional
	PassKey string `json:"passKey,omitempty"`
}

// AuthProfileLoginConf defines fields used for getting a token from an OAuth 2.0 service via password login.
// These fields are coupled to the identity of the secret used for static password-based client auth.
type AuthProfileLoginConf struct {
	// ClientID serves as a public identifier for the client to pass to the auth provider
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientID"`

	// Scope passed to the auth provider contains a space-delimited string listing permissions requested
	// +optional
	Scope *string `json:"scope,omitempty"`
}

// AuthProfileTLSConfig holds TLS settings for authentication provider connections.
type AuthProfileTLSConfig struct {
	// CAConfigMapRef references a ConfigMap containing a PEM CA certificate.
	// +optional
	CAConfigMapRef *AuthProfileCAConfigMapRef `json:"caConfigMapRef,omitempty"`

	// InsecureSkipVerify disables TLS certificate verification (not recommended for production).
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// AuthProfileCAConfigMapRef references a CA certificate in a ConfigMap.
type AuthProfileCAConfigMapRef struct {
	// Namespace of the ConfigMap.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Name of the ConfigMap.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within the ConfigMap data containing the PEM certificate.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// AuthProfileTokenExchange holds RFC 8693 token exchange settings.
type AuthProfileTokenExchange struct {
	// Endpoint is the authentication provider token exchange path.
	// If not specified, defaults to "/token".
	// +kubebuilder:default:=/token
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// TokenExchangeEndpoint returns the endpoint of the auth provider used for token exchange
func (p *AIStoreAuthProfile) TokenExchangeEndpoint() string {
	if p.Spec.TokenExchange != nil {
		return p.Spec.TokenExchange.Endpoint
	}
	return ""
}

// UserKeyOrDefault returns the configured username Secret key, or the default if unset.
func (s *AuthProfileSecret) UserKeyOrDefault() string {
	if s.UserKey != "" {
		return s.UserKey
	}
	return DefaultAuthProfileUserKey
}

// PassKeyOrDefault returns the configured password Secret key, or the default if unset.
func (s *AuthProfileSecret) PassKeyOrDefault() string {
	if s.PassKey != "" {
		return s.PassKey
	}
	return DefaultAuthProfilePassKey
}
