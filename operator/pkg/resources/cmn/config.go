// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"context"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn/configs"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const currentAISVersion = "v3.24"

type ClusterConfigInterface interface {
	SetProxy(proxyURL string)
	GetRebalanceEnabled() bool
	SetRebalanceEnabled(enabled bool)
	SetAuthEnabled(enabled bool)
	GetBackend() *aiscmn.BackendConf
	Apply(newConf *aiscmn.ConfigToSet, cluster string) error
}

func DefaultAISConf(ctx context.Context, ais *aisv1.AIStore) ClusterConfigInterface {
	var conf ClusterConfigInterface
	use324, err := ais.CompareVersion(currentAISVersion)
	if err != nil || use324 {
		if err != nil {
			logf.FromContext(ctx).Error(err, "Error parsing aisnode image. Using latest default config.")
		}
		conf = &configs.V324AISConf
	} else {
		conf = &configs.V323AISConf
	}
	proxyURL := ais.GetDefaultProxyURL()
	conf.SetProxy(proxyURL)
	return conf
}
