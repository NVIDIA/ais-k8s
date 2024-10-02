// Package controllers contains k8s controller logic for AIS cluster
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"github.com/NVIDIA/aistore/api/env"
	"github.com/NVIDIA/aistore/cmn/cos"
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

func newAuthNConfig() authNConfig {
	protocol := "http"
	if useHTTPS, err := cos.IsParseEnvBoolOrDefault(env.AuthN.UseHTTPS, false); err == nil && useHTTPS {
		protocol = "https"
	}

	return authNConfig{
		adminUser: cos.GetEnvOrDefault(env.AuthN.AdminUsername, AuthNAdminUser),
		adminPass: cos.GetEnvOrDefault(env.AuthN.AdminPassword, AuthNAdminPass),
		host:      cos.GetEnvOrDefault(AuthNServiceHostVar, AuthNServiceHostName),
		port:      cos.GetEnvOrDefault(AuthNServicePortVar, AuthNServicePort),
		protocol:  protocol,
	}
}
