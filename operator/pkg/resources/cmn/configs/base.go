// Package configs provides AIS cluster config types and defaults
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package configs

import aiscmn "github.com/NVIDIA/aistore/cmn"

type BaseClusterConfig struct {
	aiscmn.ClusterConfig
}

func (c *BaseClusterConfig) SetProxy(proxyURL string) {
	c.Proxy = aiscmn.ProxyConf{
		PrimaryURL:   proxyURL,
		OriginalURL:  proxyURL,
		DiscoveryURL: proxyURL,
	}
}

func (c *BaseClusterConfig) GetRebalanceEnabled() bool {
	return c.Rebalance.Enabled
}

func (c *BaseClusterConfig) SetRebalanceEnabled(enabled bool) {
	c.Rebalance.Enabled = enabled
}

func (c *BaseClusterConfig) SetAuthEnabled(enabled bool) {
	c.Auth.Enabled = enabled
}

func (c *BaseClusterConfig) GetBackend() *aiscmn.BackendConf {
	return &c.Backend
}

func (c *BaseClusterConfig) Apply(newConf *aiscmn.ConfigToSet, cluster string) error {
	return aiscmn.CopyProps(newConf, c, cluster)
}
