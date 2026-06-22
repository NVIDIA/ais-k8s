/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package config renders AuthN server configuration from AIStoreAuth resources.
package config

import (
	"strconv"

	aisauthn "github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn/cos"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
)

// Constants for operator-owned default values.
const (
	defaultDBFilePath  = "/etc/ais/authn/authn.db"
	defaultTLSCertPath = "/var/certs/tls.crt"
	defaultTLSKeyPath  = "/var/certs/tls.key"
)

// GenerateConfig maps AIStoreAuth spec.config and spec.tls into the full AuthN runtime config.
func GenerateConfig(authn *authv1alpha1.AIStoreAuth) (*aisauthn.Config, error) {
	conf := &aisauthn.Config{
		Log:     renderLogConfig(authn),
		Net:     renderNetConfig(authn),
		Server:  renderServerConfig(authn),
		Timeout: renderTimeoutConfig(authn),
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

func renderLogConfig(authn *authv1alpha1.AIStoreAuth) aisauthn.LogConf {
	var logCfg aisauthn.LogConf
	if authn.Spec.Config == nil || authn.Spec.Config.Log == nil {
		return logCfg
	}
	if authn.Spec.Config.Log.Level != nil {
		logCfg.Level = strconv.FormatInt(int64(*authn.Spec.Config.Log.Level), 10)
	}
	if authn.Spec.Config.Log.FlushInterval != nil {
		logCfg.FlushInterval = cos.Duration(authn.Spec.Config.Log.FlushInterval.Duration)
	}
	return logCfg
}

func renderNetConfig(authn *authv1alpha1.AIStoreAuth) aisauthn.NetConf {
	netCfg := aisauthn.NetConf{}
	netCfg.HTTP.Port = int(authn.ListenPort())
	if authn.Spec.Config != nil && authn.Spec.Config.Net != nil {
		if authn.Spec.Config.Net.ExternalURL != nil {
			netCfg.ExternalURL = *authn.Spec.Config.Net.ExternalURL
		}
	}

	if authn.HasTLSEnabled() {
		netCfg.HTTP.UseHTTPS = true
		netCfg.HTTP.Certificate = defaultTLSCertPath
		netCfg.HTTP.Key = defaultTLSKeyPath
	}
	return netCfg
}

func renderServerConfig(authn *authv1alpha1.AIStoreAuth) aisauthn.ServerConf {
	serverCfg := aisauthn.ServerConf{
		DBConf: aisauthn.DatabaseConf{Filepath: defaultDBFilePath},
	}
	if authn.Spec.Config != nil && authn.Spec.Config.Auth != nil {
		authSpec := authn.Spec.Config.Auth
		if authSpec.ExpirationTime != nil {
			serverCfg.Expire = cos.Duration(authSpec.ExpirationTime.Duration)
		}
		if authSpec.MaxTokenAge != nil {
			serverCfg.MaxTokenAge = cos.Duration(authSpec.MaxTokenAge.Duration)
		}
		if authSpec.DB != nil && authSpec.DB.Type != nil {
			serverCfg.DBConf.DBType = *authSpec.DB.Type
		}
	}

	if authn.Spec.Config != nil && authn.Spec.Config.Auth != nil &&
		authn.Spec.Config.Auth.SigningKey != nil {
		signingKey := aisauthn.SigningKeyConf{}
		signingKeySpec := authn.Spec.Config.Auth.SigningKey
		if signingKeySpec.Bits != nil {
			signingKey.Bits = int(*signingKeySpec.Bits)
		}
		if signingKeySpec.Mode != nil {
			signingKey.Mode = *signingKeySpec.Mode
		}
		if signingKey.Bits != 0 || signingKey.Mode != "" {
			serverCfg.SigningKey = signingKey
		}
	}
	return serverCfg
}

func renderTimeoutConfig(authn *authv1alpha1.AIStoreAuth) aisauthn.TimeoutConf {
	if authn.Spec.Config != nil && authn.Spec.Config.Timeout != nil &&
		authn.Spec.Config.Timeout.DefaultTimeout != nil {
		return aisauthn.TimeoutConf{
			Default: cos.Duration(authn.Spec.Config.Timeout.DefaultTimeout.Duration),
		}
	}
	return aisauthn.TimeoutConf{}
}
