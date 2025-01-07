// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/api/env"
	"github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type authNConfig struct {
	adminUser string
	adminPass string
	port      string
	host      string
	protocol  string
}

// AuthN constants
const (
	AuthNServiceHostName = "ais-authn.ais"
	AuthNServicePort     = "52001"
	AuthNAdminUser       = "admin"
	AuthNAdminPass       = "admin"

	AuthNServiceHostVar = "AIS_AUTHN_SERVICE_HOST"
	AuthNServicePortVar = "AIS_AUTHN_SERVICE_PORT"
)

type (
	AuthNClientInterface interface {
		getAdminToken(ctx context.Context) (string, error)
	}

	AuthNClient struct {
		config *authNConfig
	}
)

func NewAuthNClient() *AuthNClient {
	return &AuthNClient{
		config: newAuthNConfig(),
	}
}

// getAdminToken retrieves an admin token from AuthN service for the given AIS cluster.
func (c AuthNClient) getAdminToken(ctx context.Context) (string, error) {
	authNURL := fmt.Sprintf("%s://%s:%s", c.config.protocol, c.config.host, c.config.port)
	authNBP := authNBaseParams(authNURL, "")
	zeroDuration := time.Duration(0)

	tokenMsg, err := authn.LoginUser(*authNBP, c.config.adminUser, c.config.adminPass, &zeroDuration)
	if err != nil {
		return "", fmt.Errorf("failed to login admin user to AuthN: %w", err)
	}

	logf.FromContext(ctx).Info("Successfully logged in as Admin to AuthN")
	return tokenMsg.Token, nil
}

func newAuthNConfig() *authNConfig {
	protocol := "http"
	if useHTTPS, err := cos.IsParseEnvBoolOrDefault(env.AisAuthUseHTTPS, false); err == nil && useHTTPS {
		protocol = "https"
	}

	return &authNConfig{
		adminUser: cos.GetEnvOrDefault(env.AisAuthAdminUsername, AuthNAdminUser),
		adminPass: cos.GetEnvOrDefault(env.AisAuthAdminPassword, AuthNAdminPass),
		host:      cos.GetEnvOrDefault(AuthNServiceHostVar, AuthNServiceHostName),
		port:      cos.GetEnvOrDefault(AuthNServicePortVar, AuthNServicePort),
		protocol:  protocol,
	}
}

func authNBaseParams(url, token string) *api.BaseParams {
	transportArgs := cmn.TransportArgs{
		Timeout:         10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.Timeout,
		},
		URL:   url,
		Token: token,
		UA:    userAgent,
	}
}
