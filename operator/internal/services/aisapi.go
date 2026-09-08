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
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	jsoniter "github.com/json-iterator/go"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

//go:generate mockgen -source $GOFILE -destination mocks/client.go . AIStoreClientInterface

const (
	userAgent = "ais-operator"
	// TokenExpiryBuffer is used to calculate the safe window to trigger refresh before token expiration
	TokenExpiryBuffer = 5 * time.Minute
)

type (
	AIStoreClientInterface interface {
		DecommissionCluster(rmUserData bool) error
		DecommissionNode(actValue *apc.ActValRmNode) (xid string, err error)
		GetClusterMap() (smap *meta.Smap, err error)
		Health(readyToRebalance bool) error
		SetClusterConfigUsingMsg(config jsoniter.RawMessage) error
		SetPrimaryProxy(newPrimaryID, newPrimaryURL string, force bool) error
		ShutdownCluster() error
		StartMaintenance(actValue *apc.ActValRmNode) (string, error)
		HasValidBaseParams(context context.Context, ais *aisv1.AIStore, expectedURL string) bool
	}

	AIStoreClient struct {
		ctx             context.Context
		params          *api.BaseParams
		mode            string
		tlsCfg          *tls.Config
		tokenObtainedAt time.Time
		tokenExpireAt   time.Time
		authFailed      bool
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

	// Check if token is expired
	if c.isTokenExpired() {
		logf.FromContext(ctx).Info("Token expired or expiring soon", "expiresAt", c.tokenExpireAt)
		return false
	}

	// Determine whether HTTPS should be used based on the presence of a TLS secret / TLS issuer and
	// verify if the URL's protocol matches the expected protocol (HTTPS or HTTP)
	if cos.IsHTTPS(c.params.URL) != ais.UseHTTPS() {
		return false
	}

	// Check if the client parameters are aligned with the requested auth
	if ais.GetAuthProfileRef() != nil {
		return c.params.Token != ""
	}
	return c.params.Token == ""
}

// syncPublicURL adopts discoveredURL as the client's endpoint in public API mode, where the endpoint
// follows whichever proxy pod is ready.
func (c *AIStoreClient) syncPublicURL(ctx context.Context, discoveredURL string) {
	if c.params == nil || c.mode != APIModePublic || c.params.URL == discoveredURL {
		return
	}
	// A scheme change needs a new transport, so leave the URL to be rejected later
	if cos.IsHTTPS(c.params.URL) != cos.IsHTTPS(discoveredURL) {
		return
	}
	logf.FromContext(ctx).Info("Updating public AIS API endpoint", "previous", c.params.URL, "current", discoveredURL)
	c.params.URL = discoveredURL
}

// isTokenExpired checks if the token is expired or expiring soon (within the refresh margin)
func (c *AIStoreClient) isTokenExpired() bool {
	// Zero time means no expiration tracking
	if c.tokenExpireAt.IsZero() {
		return false
	}
	return time.Now().Add(c.refreshMargin()).After(c.tokenExpireAt)
}

// refreshMargin returns how long before expiration a token is treated as expired: half the validity
// the token had when obtained, capped at TokenExpiryBuffer, or the full cap when that is unknown.
func (c *AIStoreClient) refreshMargin() time.Duration {
	if c.tokenObtainedAt.IsZero() {
		return TokenExpiryBuffer
	}
	return min(c.tokenExpireAt.Sub(c.tokenObtainedAt)/2, TokenExpiryBuffer)
}

// refreshToken updates the token and expiration time in-place
func (c *AIStoreClient) refreshToken(tokenInfo *TokenInfo) {
	if tokenInfo == nil {
		c.params.Token = ""
		c.tokenObtainedAt = time.Time{}
		c.tokenExpireAt = time.Time{}
		return
	}
	c.params.Token = tokenInfo.Token
	c.tokenObtainedAt = tokenInfo.ObtainedAt
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

// SetClusterConfigUsingMsg applies config serialized as JSON to the cluster.
// It sends pre-rendered JSON rather than cmn.ConfigToSet so that option names deprecated
// in AIS v5.0.0 survive the round trip to remain parseable by older AIS versions.
func (c *AIStoreClient) SetClusterConfigUsingMsg(config jsoniter.RawMessage) error {
	body, err := jsoniter.Marshal(apc.ActMsg{Action: apc.ActSetConfig, Value: config})
	if err != nil {
		return err
	}
	baseParams := *c.params
	baseParams.Method = http.MethodPut

	reqParams := api.AllocRp()
	defer api.FreeRp(reqParams)
	reqParams.BaseParams = baseParams
	reqParams.Path = apc.URLPathClu.S
	reqParams.Body = body
	reqParams.Header = http.Header{cos.HdrContentType: []string{cos.ContentJSON}}

	err = reqParams.DoRequest()
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
	var tokenObtainedAt, tokenExpireAt time.Time
	if tokenInfo != nil {
		token = tokenInfo.Token
		tokenObtainedAt = tokenInfo.ObtainedAt
		tokenExpireAt = tokenInfo.ExpiresAt
	}

	return &AIStoreClient{
		ctx:             ctx,
		params:          buildBaseParams(url, token, tlsCfg),
		mode:            mode,
		tlsCfg:          tlsCfg,
		tokenObtainedAt: tokenObtainedAt,
		tokenExpireAt:   tokenExpireAt,
	}
}

func buildBaseParams(url, token string, tlsCfg *tls.Config) *api.BaseParams {
	transportArgs := cmn.TransportArgs{
		ClientTimeout:   10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)
	transport.TLSClientConfig = tlsCfg

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.ClientTimeout,
		},
		URL:   url,
		Token: token,
		UA:    userAgent,
	}
}
