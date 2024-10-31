// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	"github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
)

//go:generate mockgen -source $GOFILE -destination mocks/client.go . AIStoreClientInterface

const userAgent = "ais-operator"

type (
	AIStoreClientInterface interface {
		DecommissionCluster(rmUserData bool) error
		DecommissionNode(actValue *apc.ActValRmNode) (xid string, err error)
		GetClusterMap() (smap *meta.Smap, err error)
		Health(readyToRebalance bool) error
		SetClusterConfigUsingMsg(configToUpdate *cmn.ConfigToSet, transient bool) error
		SetPrimaryProxy(newPrimaryID, newPrimaryURL string, force bool) error
		ShutdownCluster() error
		HasValidBaseParams(ais *aisv1.AIStore) bool
	}

	AIStoreClient struct {
		ctx    context.Context
		params *api.BaseParams
		tlsCfg *tls.Config
	}
)

// HasValidBaseParams checks if the client has valid params for the given AIS cluster configuration
func (c *AIStoreClient) HasValidBaseParams(ais *aisv1.AIStore) bool {
	if c.params == nil {
		return false
	}
	// Determine whether HTTPS should be used based on the presence of a TLS secret / TLS issuer and
	// verify if the URL's protocol matches the expected protocol (HTTPS or HTTP)
	httpsCheck := cos.IsHTTPS(c.params.URL) == ais.UseHTTPS()

	// Check if the token and AuthN secret are correctly aligned:
	// - Valid if both are either set or both are unset
	authNCheck := (c.params.Token == "" && ais.Spec.AuthNSecretName == nil) ||
		(c.params.Token != "" && ais.Spec.AuthNSecretName != nil)

	return httpsCheck && authNCheck
}

func (c *AIStoreClient) DecommissionCluster(rmUserData bool) error {
	params, err := c.getPrimaryParams()
	if err != nil {
		return err
	}
	return api.DecommissionCluster(*params, rmUserData)
}

func (c *AIStoreClient) DecommissionNode(actValue *apc.ActValRmNode) (string, error) {
	return api.DecommissionNode(*c.params, actValue)
}

func (c *AIStoreClient) GetClusterMap() (smap *meta.Smap, err error) {
	return api.GetClusterMap(*c.params)
}

func (c *AIStoreClient) Health(readyToRebalance bool) error {
	// TODO: Drop requirement for primary for AIS >= v3.25 (keep now for backwards compat)
	primaryParams, err := c.getPrimaryParams()
	if err != nil {
		return err
	}
	return api.Health(*primaryParams, readyToRebalance)
}

func (c *AIStoreClient) SetClusterConfigUsingMsg(config *cmn.ConfigToSet, transient bool) error {
	return api.SetClusterConfigUsingMsg(*c.params, config, transient)
}

func (c *AIStoreClient) SetPrimaryProxy(newPrimaryID, newPrimaryURL string, force bool) error {
	return api.SetPrimary(*c.params, newPrimaryID, newPrimaryURL, force)
}

func (c *AIStoreClient) ShutdownCluster() error {
	primaryParams, err := c.getPrimaryParams()
	if err != nil {
		return err
	}
	return api.ShutdownCluster(*primaryParams)
}

func (c *AIStoreClient) getPrimaryParams() (*api.BaseParams, error) {
	smap, err := c.GetClusterMap()
	if err != nil || smap == nil {
		return nil, err
	}
	return buildBaseParams(smap.Primary.URL(cmn.NetPublic), c.getAuthToken(), c.getTLSCfg()), nil
}

func NewAIStoreClient(ctx context.Context, url, token string, tlsCfg *tls.Config) *AIStoreClient {
	params := buildBaseParams(url, token, tlsCfg)
	return &AIStoreClient{
		ctx:    ctx,
		params: params,
		tlsCfg: tlsCfg,
	}
}

func (c *AIStoreClient) getAuthToken() string {
	if c.params == nil {
		return ""
	}
	return c.params.Token
}

func (c *AIStoreClient) getTLSCfg() *tls.Config {
	return c.tlsCfg
}

func buildBaseParams(url, token string, tlsCfg *tls.Config) *api.BaseParams {
	transportArgs := cmn.TransportArgs{
		Timeout:         10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)
	transport.TLSClientConfig = tlsCfg

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
