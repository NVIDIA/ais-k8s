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

const (
	userAgent = "ais-operator"
	// TokenExpiryBuffer is the safety margin before token expiration to trigger refresh
	// If a token expires in less than this duration, it will be considered invalid
	TokenExpiryBuffer = 5 * time.Minute
)

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
		HasValidBaseParams(context context.Context, ais *aisv1.AIStore, expectedURL string) bool
	}

	AIStoreClient struct {
		ctx           context.Context
		params        *api.BaseParams
		mode          string
		tlsCfg        *tls.Config
		tokenExpireAt time.Time
		authFailed    bool
	}
)

// IsAuthError returns true if the error is an HTTP 401 or 403 from the AIS API,
// indicating the token is invalid or has been revoked.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	herr, ok := err.(*cmn.ErrHTTP)
	if !ok {
		return false
	}
	return herr.Status == http.StatusUnauthorized || herr.Status == http.StatusForbidden
}

// checkAuthErr inspects an error from an AIS API call and marks the client's
// token as failed if the cluster responded with 401/403. This ensures the next
// call to GetClient will discard the cached client and fetch a fresh token.
func (c *AIStoreClient) checkAuthErr(err error) {
	if IsAuthError(err) {
		c.authFailed = true
		logf.FromContext(c.ctx).Info("AIS API returned auth error, token will be refreshed on next reconcile")
	}
}

// HasValidBaseParams checks if the client has valid params for the given AIS cluster configuration
func (c *AIStoreClient) HasValidBaseParams(ctx context.Context, ais *aisv1.AIStore, expectedURL string) bool {
	if c.params == nil {
		return false
	}

	// If a previous API call returned 401/403, force token refresh
	if c.authFailed {
		logf.FromContext(ctx).Info("Token previously rejected by AIS (401/403), recreating client")
		return false
	}

	// Check if the URL has changed
	if c.params.URL != expectedURL {
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

	// Check if token is expired
	if c.isTokenExpired() {
		logf.FromContext(ctx).Info("Token expired or expiring soon", "expiresAt", c.tokenExpireAt)
		return false
	}

	// Determine whether HTTPS should be used based on the presence of a TLS secret / TLS issuer and
	// verify if the URL's protocol matches the expected protocol (HTTPS or HTTP)
	httpsCheck := cos.IsHTTPS(c.params.URL) == ais.UseHTTPS()

	// Check if the token and AuthN configuration are correctly aligned:
	// - If Auth or AuthNSecretName is configured, token should be present
	// - If neither is configured, token should be empty
	hasAuthConfig := ais.Spec.Auth != nil || ais.Spec.AuthNSecretName != nil
	authNCheck := (c.params.Token == "" && !hasAuthConfig) ||
		(c.params.Token != "" && hasAuthConfig)

	return httpsCheck && authNCheck
}

// isTokenExpired checks if the token is expired or expiring soon (within TokenExpiryBuffer)
func (c *AIStoreClient) isTokenExpired() bool {
	// Zero time means no expiration tracking
	if c.tokenExpireAt.IsZero() {
		return false
	}
	// Token is considered expired if it expires within TokenExpiryBuffer (5 minutes)
	return time.Now().Add(TokenExpiryBuffer).After(c.tokenExpireAt)
}

// refreshToken updates the token and expiration time in-place
func (c *AIStoreClient) refreshToken(tokenInfo *TokenInfo) {
	if tokenInfo == nil {
		c.params.Token = ""
		c.tokenExpireAt = time.Time{}
		return
	}
	c.params.Token = tokenInfo.Token
	c.tokenExpireAt = tokenInfo.ExpiresAt
}

func (c *AIStoreClient) DecommissionCluster(rmUserData bool) error {
	err := api.DecommissionCluster(*c.params, rmUserData)
	c.checkAuthErr(err)
	return err
}

func (c *AIStoreClient) DecommissionNode(actValue *apc.ActValRmNode) (string, error) {
	xid, err := api.DecommissionNode(*c.params, actValue)
	c.checkAuthErr(err)
	return xid, err
}

func (c *AIStoreClient) GetClusterMap() (smap *meta.Smap, err error) {
	smap, err = api.GetClusterMap(*c.params)
	c.checkAuthErr(err)
	return
}

func (c *AIStoreClient) Health(readyToRebalance bool) error {
	err := api.Health(*c.params, readyToRebalance)
	c.checkAuthErr(err)
	return err
}

func (c *AIStoreClient) SetClusterConfigUsingMsg(config *cmn.ConfigToSet, transient bool) error {
	err := api.SetClusterConfigUsingMsg(*c.params, config, transient)
	c.checkAuthErr(err)
	return err
}

func (c *AIStoreClient) SetPrimaryProxy(newPrimaryID, newPrimaryURL string, force bool) error {
	err := api.SetPrimary(*c.params, newPrimaryID, newPrimaryURL, force)
	c.checkAuthErr(err)
	return err
}

func (c *AIStoreClient) ShutdownCluster() error {
	err := api.ShutdownCluster(*c.params)
	c.checkAuthErr(err)
	return err
}

func (c *AIStoreClient) StartMaintenance(actValue *apc.ActValRmNode) (string, error) {
	xid, err := api.StartMaintenance(*c.params, actValue)
	c.checkAuthErr(err)
	return xid, err
}

func NewAIStoreClient(ctx context.Context, url string, tokenInfo *TokenInfo, mode string, tlsCfg *tls.Config) *AIStoreClient {
	var token string
	var tokenExpireAt time.Time
	if tokenInfo != nil {
		token = tokenInfo.Token
		tokenExpireAt = tokenInfo.ExpiresAt
	}

	return &AIStoreClient{
		ctx:           ctx,
		params:        buildBaseParams(url, token, tlsCfg),
		mode:          mode,
		tlsCfg:        tlsCfg,
		tokenExpireAt: tokenExpireAt,
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
