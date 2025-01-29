// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"crypto/sha256"
	"encoding/hex"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
)

const (
	defaultRebalanceState       = true
	ConfigHashAnnotation        = "config.aistore.nvidia.com/hash"
	RestartConfigHashAnnotation = "config.aistore.nvidia.com/restart-hash"
	RestartConfigHashInitial    = ".initial"
)

// GenerateGlobalConfig creates the initial config override to supply to an AIS daemon pod
//
//	This pulls configs from the AIS spec and includes cluster or state specific changes.
//	Note that the result can be out of sync with the actual spec depending on cluster state
func GenerateGlobalConfig(ais *aisv1.AIStore) (*aiscmn.ConfigToSet, error) {
	// Create initial configuration with changes that we do NOT want to update with spec, e.g. primary proxy
	conf := newInitialConfig(ais)
	// Apply changes from AIS spec considering current state
	configToSet, err := GenerateConfigToSet(ais)
	if err != nil {
		return nil, err
	}
	conf.Merge(configToSet)
	return conf, nil
}

func newInitialConfig(ais *aisv1.AIStore) *aiscmn.ConfigToSet {
	defaultURL := aisapc.Ptr(ais.GetDefaultProxyURL())
	discoveryURL := aisapc.Ptr(ais.GetDiscoveryProxyURL())
	conf := &aiscmn.ConfigToSet{
		Proxy: &aiscmn.ProxyConfToSet{
			PrimaryURL:   defaultURL,
			OriginalURL:  defaultURL,
			DiscoveryURL: discoveryURL,
		},
	}
	configureBackend(conf, &ais.Spec)
	return conf
}

func configureBackend(conf *aiscmn.ConfigToSet, spec *aisv1.AIStoreSpec) {
	if spec.AWSSecretName != nil || spec.GCPSecretName != nil {
		if conf.Backend == nil {
			conf.Backend = &aiscmn.BackendConf{}
		}
		if conf.Backend.Conf == nil {
			conf.Backend.Conf = make(map[string]interface{}, 8)
		}
		if spec.AWSSecretName != nil {
			conf.Backend.Conf["aws"] = aisv1.Empty{}
		}
		if spec.GCPSecretName != nil {
			conf.Backend.Conf["gcp"] = aisv1.Empty{}
		}
	}
}

// GenerateConfigToSet determines the actual config we want to apply based on config overrides provided in spec
func GenerateConfigToSet(ais *aisv1.AIStore) (*aiscmn.ConfigToSet, error) {
	specConfig := &aisv1.ConfigToUpdate{}
	if ais.Spec.ConfigToUpdate != nil {
		// Deep copy to avoid modifying the spec itself
		specConfig = ais.Spec.ConfigToUpdate.DeepCopy()
	}
	// Override rebalance if the cluster is not ready for it (starting up, scaling, upgrading)
	if ais.IsConditionTrue(aisv1.ConditionReadyRebalance) {
		// If not provided, reset to default
		if !specConfig.IsRebalanceEnabledSet() {
			specConfig.UpdateRebalanceEnabled(aisapc.Ptr(defaultRebalanceState))
		}
	} else {
		specConfig.UpdateRebalanceEnabled(aisapc.Ptr(false))
	}

	if ais.Spec.AuthNSecretName != nil {
		specConfig.EnableAuth()
	}
	return specConfig.Convert()
}

func HashGlobalConfig(c *aiscmn.ConfigToSet) (string, error) {
	data, err := jsoniter.Marshal(c)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Generates a hash of ONLY configs that should trigger cluster restart upon change
func HashRestartConfigs(c *aiscmn.ConfigToSet) (string, error) {
	checksum := sha256.Sum256([]byte{})
	if c.Net != nil && c.Net.HTTP != nil {
		confNetHTTP, err := jsoniter.Marshal(*c.Net.HTTP)
		if err != nil {
			return "", err
		}
		checksum = sha256.Sum256(confNetHTTP)
	}
	return hex.EncodeToString(checksum[:]), nil
}
