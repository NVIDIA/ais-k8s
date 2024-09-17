// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn/configs"
	jsoniter "github.com/json-iterator/go"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const currentAISVersion = "v3.24"

type ClusterConfigInterface interface {
	SetProxy(proxyURL string)
	GetBackend() *aiscmn.BackendConf
	IsRebalanceEnabled() *bool
	Apply(newConf *aiscmn.ConfigToSet, cluster string) error
}

func DefaultAISConf(ctx context.Context, ais *aisv1.AIStore) ClusterConfigInterface {
	// TODO: cache defaults unless version changes
	use324, err := ais.CompareVersion(currentAISVersion)
	if err != nil || use324 {
		if err != nil {
			logf.FromContext(ctx).Error(err, "Error parsing aisnode image. Using latest default config.")
		}
		return configs.NewV324ClusterConfig()
	}
	return configs.NewV323ClusterConfig()
}

// GenerateGlobalConfig creates a full global config file for AIS
//
//	This starts with default AIS config, applies any changes from the AIS spec,
//	and also applies any cluster or state specific changes.
//	Note that the result can be out of sync with the actual spec depending on cluster state
func GenerateGlobalConfig(ctx context.Context, ais *aisv1.AIStore) (ClusterConfigInterface, error) {
	// Start with default
	globalConf := DefaultAISConf(ctx, ais)

	// Apply conf changes based on other spec options
	proxyURL := ais.GetDefaultProxyURL()
	globalConf.SetProxy(proxyURL)
	if ais.Spec.AWSSecretName != nil || ais.Spec.GCPSecretName != nil {
		if globalConf.GetBackend().Conf == nil {
			globalConf.GetBackend().Conf = make(map[string]interface{}, 8)
		}
		if ais.Spec.AWSSecretName != nil {
			globalConf.GetBackend().Conf["aws"] = aisv1.Empty{}
		}
		if ais.Spec.GCPSecretName != nil {
			globalConf.GetBackend().Conf["gcp"] = aisv1.Empty{}
		}
	}

	// Apply changes from AIS spec.configToUpdate considering current state
	configToSet, err := GenerateConfigToSet(ctx, ais)
	if err != nil {
		return nil, err
	}
	err = globalConf.Apply(configToSet, aisapc.Cluster)
	if err != nil {
		return nil, err
	}
	return globalConf, nil
}

// GenerateConfigToSet determines the actual config we want to apply (on top of defaults), starting from the provided spec
func GenerateConfigToSet(ctx context.Context, ais *aisv1.AIStore) (*aiscmn.ConfigToSet, error) {
	specConfig := &aisv1.ConfigToUpdate{}
	if ais.Spec.ConfigToUpdate != nil {
		// Deep copy to avoid modifying the spec itself
		specConfig = ais.Spec.ConfigToUpdate.DeepCopy()
	}
	logger := logf.FromContext(ctx)
	// Override rebalance if the cluster is not ready for it (starting up, scaling, upgrading)
	if ais.IsConditionTrue(aisv1.ConditionReadyRebalance) {
		// If not provided, reset to default
		if !specConfig.IsRebalanceEnabledSet() {
			specConfig.UpdateRebalanceEnabled(DefaultAISConf(ctx, ais).IsRebalanceEnabled())
		}
	} else {
		logger.Info("Setting rebalance disabled in spec config because of condition")
		specConfig.UpdateRebalanceEnabled(aisapc.Ptr(false))
	}

	if ais.Spec.AuthNSecretName != nil {
		specConfig.EnableAuth()
	}
	return specConfig.Convert()
}

// HashConfigToSet generates a hash of the given config
func HashConfigToSet(c *aiscmn.ConfigToSet) (string, error) {
	data, err := jsoniter.Marshal(c)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
