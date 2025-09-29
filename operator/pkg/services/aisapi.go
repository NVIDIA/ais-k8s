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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
		StartMaintenance(actValue *apc.ActValRmNode) (string, error)
		HasValidBaseParams(context context.Context, ais *aisv1.AIStore) bool
	}

	AIStoreClient struct {
		ctx    context.Context
		params *api.BaseParams
		mode   string
		tlsCfg *tls.Config
	}
)

// HasValidBaseParams checks if the client has valid params for the given AIS cluster configuration
func (c *AIStoreClient) HasValidBaseParams(ctx context.Context, ais *aisv1.AIStore) bool {
	if c.params == nil {
		return false
	}
	// Check for an apiMode change in spec
	if c.mode != ais.GetAPIMode() {
		return false
	}
	// If using public API, no k8s service to automate changing endpoints, verify params still valid
	if c.mode == APIModePublic {
		err := c.Health(false)
		if err != nil {
			logf.FromContext(ctx).Info("AIS API health check failed", "url", c.params.URL, "err", err.Error())
			return false
		}
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
	return api.DecommissionCluster(*c.params, rmUserData)
}

func (c *AIStoreClient) DecommissionNode(actValue *apc.ActValRmNode) (string, error) {
	return api.DecommissionNode(*c.params, actValue)
}

func (c *AIStoreClient) GetClusterMap() (smap *meta.Smap, err error) {
	return api.GetClusterMap(*c.params)
}

func (c *AIStoreClient) Health(readyToRebalance bool) error {
	return api.Health(*c.params, readyToRebalance)
}

func (c *AIStoreClient) SetClusterConfigUsingMsg(config *cmn.ConfigToSet, transient bool) error {
	return api.SetClusterConfigUsingMsg(*c.params, config, transient)
}

func (c *AIStoreClient) SetPrimaryProxy(newPrimaryID, newPrimaryURL string, force bool) error {
	return api.SetPrimary(*c.params, newPrimaryID, newPrimaryURL, force)
}

func (c *AIStoreClient) ShutdownCluster() error {
	return api.ShutdownCluster(*c.params)
}

func (c *AIStoreClient) StartMaintenance(actValue *apc.ActValRmNode) (string, error) {
	return api.StartMaintenance(*c.params, actValue)
}

func NewAIStoreClient(ctx context.Context, url, token, mode string, tlsCfg *tls.Config) *AIStoreClient {
	return &AIStoreClient{
		ctx:    ctx,
		params: buildBaseParams(url, token, tlsCfg),
		mode:   mode,
		tlsCfg: tlsCfg,
	}
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
