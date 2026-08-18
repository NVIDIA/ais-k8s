/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/truststore"
	"k8s.io/apimachinery/pkg/types"
)

// AuthProfileConfig wraps an AIStoreAuthProfile, the administrator-approved auth provider
type AuthProfileConfig struct {
	profile   *authv1alpha1.AIStoreAuthProfile
	k8sClient *aisclient.K8sClient
	tls       tlsCache
}

func (c *AuthProfileConfig) GetServiceURL() string { return c.profile.Spec.ServiceURL }

func (c *AuthProfileConfig) IsTokenExchange() bool { return c.profile.Spec.TokenExchange != nil }

// GetTokenPath returns an empty path because profiles do not define projected token location for clients.
// Clients, including the operator, must configure their own token location.
func (*AuthProfileConfig) GetTokenPath() string { return "" }

func (c *AuthProfileConfig) GetSubjectTokenAudience() string {
	if c.profile.Spec.TokenExchange == nil {
		return ""
	}
	return c.profile.Spec.TokenExchange.SubjectTokenAudience
}

func (c *AuthProfileConfig) GetTokenExchangeEndpoint() string {
	if endpoint := c.profile.TokenExchangeEndpoint(); endpoint != "" {
		return endpoint
	}
	return DefaultTokenExchangeEndpoint
}

func (c *AuthProfileConfig) GetOAuthLoginConf() *OAuthLoginConf {
	if c.profile.Spec.UsernamePassword == nil || c.profile.Spec.UsernamePassword.LoginConf == nil {
		return nil
	}
	conf := c.profile.Spec.UsernamePassword.LoginConf
	return &OAuthLoginConf{ClientID: conf.ClientID, Endpoint: conf.Endpoint, Scope: conf.Scope}
}

func (c *AuthProfileConfig) GetSecretName() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.Name
	}
	return ""
}

func (c *AuthProfileConfig) GetSecretNamespace() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.Namespace
	}
	return ""
}

func (c *AuthProfileConfig) GetUserKey() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.UserKeyOrDefault()
	}
	return ""
}

func (c *AuthProfileConfig) GetPassKey() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.PassKeyOrDefault()
	}
	return ""
}

func (c *AuthProfileConfig) GetTLSConfig(ctx context.Context) (*tls.Config, error) {
	insecureSkipVerify := c.profile.Spec.TLS != nil && c.profile.Spec.TLS.InsecureSkipVerify
	return c.tls.get(ctx, c.trustStoreConfig, insecureSkipVerify)
}

func (c *AuthProfileConfig) loginSecret() *authv1alpha1.AuthProfileSecret {
	if c.profile.Spec.UsernamePassword == nil {
		return nil
	}
	return &c.profile.Spec.UsernamePassword.Secret
}

// trustStoreConfig resolves the profile's CA certificate, which lives in a ConfigMap rather
// than on the operator's filesystem.
func (c *AuthProfileConfig) trustStoreConfig(ctx context.Context) (truststore.Config, error) {
	if c.profile.Spec.TLS == nil || c.profile.Spec.TLS.CAConfigMapRef == nil {
		return truststore.Config{CACertPaths: caCertPaths("")}, nil
	}
	ref := c.profile.Spec.TLS.CAConfigMapRef
	name := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	configMap, err := c.k8sClient.GetConfigMap(ctx, name)
	if err != nil {
		return truststore.Config{}, fmt.Errorf("failed to get CA ConfigMap %s: %w", name, err)
	}
	caPEM, ok := configMap.Data[ref.Key]
	if !ok {
		return truststore.Config{}, fmt.Errorf("CA ConfigMap %s has no key %q", name, ref.Key)
	}
	return truststore.Config{CAPEMs: [][]byte{[]byte(caPEM)}}, nil
}
