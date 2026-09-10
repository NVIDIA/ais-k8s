/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync/atomic"
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
		ctx    context.Context
		params *api.BaseParams
		mode   string
		// tokenInfo describes the token currently in params.
		tokenInfo TokenInfo
		// tokenRejected records that AIS answered an API call with 401 or 403.
		tokenRejected atomic.Bool
	}

	// authStatusTracker observes the auth status of the AIS API responses to a single client.
	authStatusTracker struct {
		base   http.RoundTripper
		client *AIStoreClient
	}
)

func (t *authStatusTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if resp == nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return resp, err
	}
	// Log the first rejection only, so a cluster rejecting every call does not flood the log
	if !t.client.tokenRejected.Swap(true) {
		logf.FromContext(t.client.ctx).Info("AIS API rejected the token, it will be refreshed on the next reconcile",
			"status", resp.StatusCode)
	}
	return resp, err
}

// HasValidBaseParams checks if the client can still reach the given AIS cluster.
func (c *AIStoreClient) HasValidBaseParams(_ context.Context, ais *aisv1.AIStore, expectedURL string) bool {
	if c.params == nil {
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
	return cos.IsHTTPS(c.params.URL) == ais.UseHTTPS()
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
	if c.tokenInfo.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(c.refreshMargin()).After(c.tokenInfo.ExpiresAt)
}

// refreshMargin returns how long before expiration a token is treated as expired: half the validity
// the token had when obtained, capped at TokenExpiryBuffer, or the full cap when that is unknown.
func (c *AIStoreClient) refreshMargin() time.Duration {
	if c.tokenInfo.ObtainedAt.IsZero() {
		return TokenExpiryBuffer
	}
	return min(c.tokenInfo.ExpiresAt.Sub(c.tokenInfo.ObtainedAt)/2, TokenExpiryBuffer)
}

// tokenRefreshReason reports why the client needs another token, or an empty string while the
// current one remains usable. profileGen is the generation-stamped identity of the cluster's auth profile.
func (c *AIStoreClient) tokenRefreshReason(profileGen string) string {
	if c.tokenInfo.ProfileGen != profileGen {
		switch {
		case c.tokenInfo.ProfileGen == "":
			return "noTokenForProfile"
		case profileGen == "":
			return "authProfileRemoved"
		default:
			return "authProfileChanged"
		}
	}
	switch {
	case c.tokenRejected.Load():
		return "rejectedByAIS"
	case c.isTokenExpired():
		return "expired"
	}
	return ""
}

// setToken makes tokenInfo the client's token. A nil tokenInfo leaves the client with no token.
func (c *AIStoreClient) setToken(tokenInfo *TokenInfo) {
	if tokenInfo == nil {
		c.tokenInfo = TokenInfo{}
	} else {
		c.tokenInfo = *tokenInfo
	}
	c.params.Token = c.tokenInfo.Token
	c.tokenRejected.Store(false)
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

	return reqParams.DoRequest()
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

func NewAIStoreClient(ctx context.Context, url string, tokenInfo *TokenInfo, mode string, tlsCfg *tls.Config) *AIStoreClient {
	client := &AIStoreClient{
		ctx:    ctx,
		params: buildBaseParams(url, "", tlsCfg),
		mode:   mode,
	}
	client.params.Client.Transport = &authStatusTracker{base: client.params.Client.Transport, client: client}
	client.setToken(tokenInfo)
	return client
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
