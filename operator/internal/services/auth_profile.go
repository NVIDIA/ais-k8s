/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/NVIDIA/aistore/api"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/truststore"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// defaultAuthCADir is the directory scanned for any statically mounted custom Auth CA trust.
// Matches the directory mounted to the operator deployment by helm value "authCAConfigmapName"
const defaultAuthCADir = "/etc/ssl/certs/auth-ca"

// authProfileConfig wraps an AIStoreAuthProfile, the administrator-approved auth provider
type authProfileConfig struct {
	profile   *authv1alpha1.AIStoreAuthProfile
	k8sClient *aisclient.K8sClient
	tls       tlsCache
}

func (c *authProfileConfig) GetServiceURL() string { return c.profile.Spec.ServiceURL }

// Client returns API params for reaching the profile's auth service.
func (c *authProfileConfig) Client(ctx context.Context) (*api.BaseParams, error) {
	serviceURL := c.GetServiceURL()
	var tlsConf *tls.Config
	if strings.HasPrefix(serviceURL, "https://") {
		var err error
		tlsConf, err = c.tlsConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS config for auth service: %w", err)
		}
	}
	c.logNewClient(ctx, tlsConf)
	return newAuthBaseParams(serviceURL, tlsConf), nil
}

func (c *authProfileConfig) IsTokenExchange() bool { return c.profile.Spec.TokenExchange != nil }

func (c *authProfileConfig) GetSubjectTokenAudience() string {
	if c.profile.Spec.TokenExchange == nil {
		return ""
	}
	return c.profile.Spec.TokenExchange.SubjectTokenAudience
}

func (c *authProfileConfig) GetTokenExchangeEndpoint() string {
	if endpoint := c.profile.TokenExchangeEndpoint(); endpoint != "" {
		return endpoint
	}
	return DefaultTokenExchangeEndpoint
}

func (c *authProfileConfig) GetOAuthLoginConf() *OAuthLoginConf {
	if c.profile.Spec.UsernamePassword == nil || c.profile.Spec.UsernamePassword.LoginConf == nil {
		return nil
	}
	conf := c.profile.Spec.UsernamePassword.LoginConf
	return &OAuthLoginConf{ClientID: conf.ClientID, Endpoint: conf.Endpoint, Scope: conf.Scope}
}

func (c *authProfileConfig) GetSecretName() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.Name
	}
	return ""
}

func (c *authProfileConfig) GetSecretNamespace() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.Namespace
	}
	return ""
}

func (c *authProfileConfig) GetUserKey() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.UserKeyOrDefault()
	}
	return ""
}

func (c *authProfileConfig) GetPassKey() string {
	if secret := c.loginSecret(); secret != nil {
		return secret.PassKeyOrDefault()
	}
	return ""
}

func (c *authProfileConfig) tlsConfig(ctx context.Context) (*tls.Config, error) {
	insecureSkipVerify := c.profile.Spec.TLS != nil && c.profile.Spec.TLS.InsecureSkipVerify
	return c.tls.get(ctx, c.trustStoreConfig, insecureSkipVerify)
}

func (c *authProfileConfig) logNewClient(ctx context.Context, tlsConf *tls.Config) {
	logger := logf.FromContext(ctx).WithValues(
		"profileRef", c.profile.GetName(),
		"serviceURL", c.GetServiceURL(),
		"tokenExchange", c.IsTokenExchange(),
	)
	msg := "Creating authentication service client"
	if tlsConf == nil {
		msg += ". Warning: TLS not enabled"
	} else if tlsConf.InsecureSkipVerify {
		msg += ". Warning: TLS certificate verification disabled"
	}
	logger.Info(msg)
}

func (c *authProfileConfig) loginSecret() *authv1alpha1.AuthProfileSecret {
	if c.profile.Spec.UsernamePassword == nil {
		return nil
	}
	return &c.profile.Spec.UsernamePassword.Secret
}

// trustStoreConfig resolves the profile's CA certificate. It prefers the ConfigMap referenced
// by the profile; otherwise it falls back to any CA files mounted under defaultAuthCADir.
func (c *authProfileConfig) trustStoreConfig(ctx context.Context) (truststore.Config, error) {
	if c.profile.Spec.TLS == nil || c.profile.Spec.TLS.CAConfigMapRef == nil {
		return authCATrustStoreConfig(ctx, defaultAuthCADir)
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

// authCATrustStoreConfig scans caDir for .crt/.pem CA files.
func authCATrustStoreConfig(ctx context.Context, caDir string) (truststore.Config, error) {
	logger := logf.FromContext(ctx)
	if _, err := os.Stat(caDir); os.IsNotExist(err) {
		logger.Info("No path found with additional Auth CA certs", "path", caDir)
		return truststore.Config{}, nil
	} else if err != nil {
		return truststore.Config{}, fmt.Errorf("failed to stat Auth CA dir %s: %w", caDir, err)
	}
	certPaths, err := findCerts(caDir, []string{".crt", ".pem"})
	if err != nil {
		return truststore.Config{}, fmt.Errorf("failed to search Auth CA dir %s: %w", caDir, err)
	}
	return truststore.Config{CACertPaths: certPaths}, nil
}
