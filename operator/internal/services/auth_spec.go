/*
 * Copyright (c) 2024-2026, NVIDIA CORPORATION. All rights reserved.
 */

//nolint:staticcheck // SA1019: AuthSpecConfig intentionally reads deprecated inline auth fields
package services

import (
	"context"
	"crypto/tls"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/truststore"
)

// Auth service API defaults
const (
	DefaultAuthNServiceURL = "https://ais-authn.ais:52001"
	DefaultAuthCACertPath  = "/etc/ssl/certs/auth-ca/ca.crt"

	AuthNSecretRefName = "SU-NAME"
	AuthNSecretRefPass = "SU-PASS"
)

// AuthSpecConfig wraps the CRD AuthSpec configuration
type AuthSpecConfig struct {
	spec      *aisv1.AuthSpec
	namespace string // cluster namespace for default secret lookup
	tls       tlsCache
}

func (c *AuthSpecConfig) GetServiceURL() string {
	serviceURL := DefaultAuthNServiceURL
	if c.spec.ServiceURL != nil {
		serviceURL = *c.spec.ServiceURL
	}
	return serviceURL
}

func (c *AuthSpecConfig) IsTokenExchange() bool {
	return c.spec.TokenExchange != nil
}

func (c *AuthSpecConfig) GetTokenPath() string {
	if c.spec.TokenExchange != nil && c.spec.TokenExchange.TokenPath != nil {
		return *c.spec.TokenExchange.TokenPath
	}
	return DefaultTokenPath
}

func (c *AuthSpecConfig) GetTokenExchangeEndpoint() string {
	if c.spec.TokenExchange != nil && c.spec.TokenExchange.TokenExchangeEndpoint != nil {
		return *c.spec.TokenExchange.TokenExchangeEndpoint
	}
	return DefaultTokenExchangeEndpoint
}

func (c *AuthSpecConfig) GetOAuthLoginConf() *OAuthLoginConf {
	if c.spec.UsernamePassword == nil || c.spec.UsernamePassword.LoginConf == nil {
		return nil
	}
	conf := c.spec.UsernamePassword.LoginConf
	return &OAuthLoginConf{ClientID: conf.ClientID, Scope: conf.Scope}
}

func (c *AuthSpecConfig) GetSecretName() string {
	if c.spec.UsernamePassword != nil {
		return c.spec.UsernamePassword.SecretName
	}
	return ""
}

func (c *AuthSpecConfig) GetSecretNamespace() string {
	if c.spec.UsernamePassword != nil && c.spec.UsernamePassword.SecretNamespace != nil {
		return *c.spec.UsernamePassword.SecretNamespace
	}
	return c.namespace
}

func (*AuthSpecConfig) GetUserKey() string { return AuthNSecretRefName }

func (*AuthSpecConfig) GetPassKey() string { return AuthNSecretRefPass }

func (c *AuthSpecConfig) GetCACertPath() string {
	if c.spec.TLS != nil {
		return c.spec.TLS.CACertPath
	}
	return ""
}

func (c *AuthSpecConfig) GetInsecureSkipVerify() bool {
	if c.spec.TLS != nil {
		return c.spec.TLS.InsecureSkipVerify
	}
	return false
}

func (c *AuthSpecConfig) GetTLSConfig(ctx context.Context) (*tls.Config, error) {
	return c.tls.get(ctx, func(context.Context) (truststore.Config, error) {
		return truststore.Config{CACertPaths: caCertPaths(c.GetCACertPath())}, nil
	}, c.GetInsecureSkipVerify())
}
