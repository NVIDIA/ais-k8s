/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/opinfo"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Token exchange defaults
const (
	DefaultTokenExchangeEndpoint = "/token"
	// DefaultSubjectTokenAudience is a stand-in for empty audiences to tell the API server not to fill in its own URL.
	// It must not match any value in the K8s cluster's --api-audiences, but can otherwise be any string.
	DefaultSubjectTokenAudience = "ais-auth-svc" //nolint:gosec // not a credential

	// subjectTokenExpiration is the lifetime requested for minted subject tokens.
	// The API server rejects anything shorter than 10 minutes.
	subjectTokenExpiration = 10 * time.Minute
)

// RFC 8693 OAuth 2.0 Token Exchange constants
const (
	RFC8693GrantType           = "urn:ietf:params:oauth:grant-type:token-exchange"
	RFC8693SubjectTokenTypeJWT = "urn:ietf:params:oauth:token-type:jwt" //nolint:gosec // This is a URN identifier, not a credential
)

// TokenInfo contains token and optional expiration information
type TokenInfo struct {
	Token string
	// ObtainedAt is when the operator got the token, not the token's own iat claim
	ObtainedAt time.Time
	ExpiresAt  time.Time
	// ProfileGen identifies the AIStoreAuthProfile object and generation that issued the token
	ProfileGen string
}

type (
	AuthClient struct {
		k8sClient *aisclient.K8sClient
	}

	// AuthConfig interface for getting the configuration for the authentication service
	AuthConfig interface {
		Client(ctx context.Context) (*api.BaseParams, error)
		GetServiceURL() string
		IsTokenExchange() bool
		GetSubjectTokenAudience() string
		GetTokenExchangeEndpoint() string
		GetOAuthLoginConf() *OAuthLoginConf
		GetSecretName() string
		GetSecretNamespace() string
		GetUserKey() string
		GetPassKey() string
		GetProfileGeneration() string
	}

	// OAuthLoginConf holds the parameters for an OAuth 2.0 password grant
	OAuthLoginConf struct {
		ClientID string
		Endpoint string
		Scope    *string
	}

	// credentials holds the username and password read from the configured login Secret
	credentials struct {
		user string
		pass string
	}

	// RFC 8693 Section 2.2 - Response format (REQUIRED fields only)
	oauthTokenResponse struct {
		// Required by RFC
		// #nosec G117 -- Not a secret
		AccessToken string `json:"access_token"`
		// Required by RFC
		TokenType string `json:"token_type"`
		// Not required by RFC but needed for token expiration
		ExpiresIn int `json:"expires_in,omitempty"`
	}

	tokenExchangeResponse struct {
		oauthTokenResponse
		IssuedTokenType string `json:"issued_token_type"` // REQUIRED
		Token           string `json:"token"`             // Legacy: backward compatibility
	}
)

func NewAuthClient(k8sClient *aisclient.K8sClient) *AuthClient {
	return &AuthClient{
		k8sClient: k8sClient,
	}
}

// getAdminToken Gets an admin token for the given cluster using token exchange or configured credentials secret
func (c *AuthClient) getAdminToken(ctx context.Context, ais *aisv1.AIStore) (*TokenInfo, error) {
	authConf, err := c.ResolveAuthConfig(ctx, ais)
	if err != nil || authConf == nil {
		return nil, err
	}
	baseParams, err := authConf.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service client: %w", err)
	}

	tokenInfo, err := c.fetchToken(ctx, baseParams, ais, authConf)
	if err != nil || tokenInfo == nil {
		return nil, err
	}
	tokenInfo.ProfileGen = authConf.GetProfileGeneration()
	return tokenInfo, nil
}

// fetchToken obtains a token using the login method the auth configuration specifies.
func (c *AuthClient) fetchToken(ctx context.Context, bp *api.BaseParams, ais *aisv1.AIStore,
	authConf AuthConfig,
) (*TokenInfo, error) {
	// Token exchange mode
	if authConf.IsTokenExchange() {
		return c.getTokenViaExchange(ctx, bp, ais, authConf)
	}
	// Username/password mode
	return c.getTokenViaPassword(ctx, bp, authConf)
}

// profileGeneration reads the generation-stamped identity of the AIStoreAuthProfile the cluster references.
func (c *AuthClient) profileGeneration(ctx context.Context, ais *aisv1.AIStore) (string, error) {
	profile, err := c.k8sClient.GetReferencedAuthProfile(ctx, ais)
	if err != nil {
		return "", err
	}
	return authProfileGeneration(profile), nil
}

// ResolveAuthConfig resolves the auth provider from the referenced AIStoreAuthProfile
func (c *AuthClient) ResolveAuthConfig(ctx context.Context, ais *aisv1.AIStore) (AuthConfig, error) {
	profile, err := c.k8sClient.GetReferencedAuthProfile(ctx, ais)
	if err != nil || profile == nil {
		return nil, err
	}
	return &authProfileConfig{profile: profile, k8sClient: c.k8sClient}, nil
}

// getSecretData Get the secret data from the specified secret name and namespace
func (c *AuthClient) getSecretData(ctx context.Context, namespace, secretName string) (map[string][]byte, error) {
	logger := logf.FromContext(ctx)
	// Look up the secret credentials and use them to obtain a token
	secret, err := c.k8sClient.GetSecret(ctx, types.NamespacedName{Name: secretName, Namespace: namespace})
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get auth credentials secret %s in namespace %s", secretName, namespace))
		return nil, err
	}
	if secret == nil || len(secret.Data) == 0 {
		return nil, fmt.Errorf("auth Secret %s in namespace %s has no data", secretName, namespace)
	}
	return secret.Data, nil
}

func (c *AuthClient) getTokenViaPassword(ctx context.Context, bp *api.BaseParams, authConf AuthConfig) (*TokenInfo, error) {
	if authConf.GetSecretName() == "" {
		return nil, nil
	}
	secretData, err := c.getSecretData(ctx, authConf.GetSecretNamespace(), authConf.GetSecretName())
	if err != nil || secretData == nil {
		return nil, err
	}
	userBytes, ok := secretData[authConf.GetUserKey()]
	if !ok || len(userBytes) == 0 {
		return nil, fmt.Errorf("auth Secret %s/%s missing key %q", authConf.GetSecretNamespace(), authConf.GetSecretName(), authConf.GetUserKey())
	}
	passBytes, ok := secretData[authConf.GetPassKey()]
	if !ok || len(passBytes) == 0 {
		return nil, fmt.Errorf("auth Secret %s/%s missing key %q", authConf.GetSecretNamespace(), authConf.GetSecretName(), authConf.GetPassKey())
	}
	creds := credentials{
		user: string(userBytes),
		pass: string(passBytes),
	}
	oauthConf := authConf.GetOAuthLoginConf()
	if oauthConf == nil {
		// Use AIS authN service if no OAuth configuration
		return getTokenFromAuthN(ctx, bp, creds)
	}
	return getTokenFromOAuth(ctx, bp, creds, oauthConf)
}

// getTokenFromOAuth retrieves an admin token from an OAuth standard issuer using the provided credentials
func getTokenFromOAuth(ctx context.Context, params *api.BaseParams, creds credentials, oauthConf *OAuthLoginConf) (*TokenInfo, error) {
	// Prepare form values; Scope is optional, omit if nil
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", oauthConf.ClientID)
	form.Set("username", creds.user)
	form.Set("password", creds.pass)
	if oauthConf.Scope != nil {
		form.Set("scope", *oauthConf.Scope)
	}

	requestURL := params.URL
	if oauthConf.Endpoint != "" {
		var err error
		requestURL, err = url.JoinPath(params.URL, oauthConf.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth login request URL: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth login request: %w", err)
	}
	resp, err := doAuthSvcRequest(req, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send OAuth login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OAuth login failed: %s", string(bodyBytes))
	}

	var tokenResp oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode OAuth token response: %w", err)
	}
	if !strings.EqualFold(tokenResp.TokenType, "bearer") {
		return nil, fmt.Errorf("unexpected token_type: %s", tokenResp.TokenType)
	}
	logf.FromContext(ctx).Info(fmt.Sprintf("Successfully fetched token for user %q from auth service", creds.user))
	// expires_in is optional per RFC 6749; leave the zero value so the token is not treated as expired
	obtainedAt := time.Now()
	var expiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		expiresAt = obtainedAt.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	return &TokenInfo{
		Token:      tokenResp.AccessToken,
		ObtainedAt: obtainedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

// getTokenFromAuthN retrieves an admin token from AuthN using the provided credentials
func getTokenFromAuthN(ctx context.Context, params *api.BaseParams, creds credentials) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)
	zeroDuration := time.Duration(0)
	tokenMsg, err := authn.LoginUser(*params, creds.user, creds.pass, &zeroDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to login %q user to AuthN: %w", creds.user, err)
	}

	logger.Info(fmt.Sprintf("Successfully fetched token for user %q from AuthN", creds.user))
	// Username/password mode doesn't provide expiration info
	return &TokenInfo{
		Token:      tokenMsg.Token,
		ObtainedAt: time.Now(),
		ExpiresAt:  time.Time{}, // Zero value = no expiration
	}, nil
}

// newAuthBaseParams builds the API params used for auth service requests. A nil tlsConf leaves the
// transport without TLS.
func newAuthBaseParams(serviceURL string, tlsConf *tls.Config) *api.BaseParams {
	transportArgs := cmn.TransportArgs{
		ClientTimeout:   10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)
	if tlsConf != nil {
		transport.TLSClientConfig = tlsConf
	}

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.ClientTimeout,
		},
		URL: serviceURL,
		UA:  userAgent,
	}
}

// getTokenViaExchange either loads a fixed token or mints a subject token based on the operator's identity.
// It then exchanges it with the configured auth service for an AIS token
func (c *AuthClient) getTokenViaExchange(ctx context.Context, bp *api.BaseParams, ais *aisv1.AIStore, conf AuthConfig) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	endpoint := conf.GetTokenExchangeEndpoint()

	aud := conf.GetSubjectTokenAudience()
	if aud == "" {
		logger.Info("WARNING: no subject token audience provided for exchange, using default audience", "audience", DefaultSubjectTokenAudience)
		// An empty audience is filled in by the K8s API server to allow requests to the API server itself.
		// This default is provided to ensure the token the operator mints can ONLY be used with an auth service
		// that does not validate audiences.
		//
		// If the auth service DOES validate audience, the required audience must be added to the AIStoreAuthProfile.
		aud = DefaultSubjectTokenAudience
	}
	subjectToken, err := c.mintSubjectToken(ctx, aud)
	if err != nil {
		return nil, err
	}

	// Get all audiences from the AIStore cluster's required claims configuration
	// If not configured, we pass an empty slice (don't request audiences if cluster doesn't require them)
	audiences := ais.RequiredAudiences()

	tokenInfo, err := exchangeTokenWithAuthSvc(ctx, bp, subjectToken, endpoint, audiences)
	if err != nil {
		logger.Error(err, "Failed to exchange token with auth service", "audiences", audiences)
		return nil, err
	}

	logger.Info("Successfully exchanged token with auth service", "audiences", audiences)
	return tokenInfo, nil
}

// mintSubjectToken mints a short-lived token for the operator's ServiceAccount bound to the configured audience.
func (c *AuthClient) mintSubjectToken(ctx context.Context, audience string) (string, error) {
	logger := logf.FromContext(ctx)
	if audience == "" {
		return "", errors.New("audience is required to mint subject token")
	}
	sa := opinfo.ServiceAccount()
	req, err := c.k8sClient.CreateServiceAccountToken(ctx, sa, audience, subjectTokenExpiration)
	if err != nil {
		logger.Error(err, "Failed to mint subject token", "serviceAccount", sa.String(), "audience", audience)
		return "", fmt.Errorf("failed to mint token for ServiceAccount %s with audience %q: %w", sa, audience, err)
	}
	logger.V(2).Info("Minted service account token", "serviceAccount", sa.String(), "audience", audience,
		"expiration", req.Status.ExpirationTimestamp)
	return req.Status.Token, nil
}

// exchangeTokenWithAuthSvc exchanges a subject token (e.g., K8s SA token) for an AIS JWT token
// Implements RFC 8693 OAuth 2.0 Token Exchange specification
// See: https://datatracker.ietf.org/doc/html/rfc8693
func exchangeTokenWithAuthSvc(ctx context.Context, params *api.BaseParams, subjectToken, endpoint string, audiences []string) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	// RFC 8693 Section 2.1 - Request format (form-encoded)
	formData := url.Values{}
	formData.Set("grant_type", RFC8693GrantType)                   // REQUIRED
	formData.Set("subject_token", subjectToken)                    // REQUIRED
	formData.Set("subject_token_type", RFC8693SubjectTokenTypeJWT) // REQUIRED
	// RFC 8693 Section 2.1 - audience parameter (OPTIONAL but recommended)
	// Specifies the target audience(s) for the issued token
	// Per RFC 8693, the audience parameter can appear multiple times
	for _, audience := range audiences {
		if audience != "" {
			formData.Add("audience", audience)
		}
	}

	requestURL, err := url.JoinPath(params.URL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange request: %w", err)
	}
	resp, err := doAuthSvcRequest(req, params)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result tokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode exchange response: %w", err)
	}

	// RFC 8693 Section 2.2.1 - access_token is REQUIRED
	token := result.AccessToken
	if token == "" {
		// Fall back to legacy "token" field for backward compatibility
		token = result.Token
	}

	if token == "" {
		return nil, fmt.Errorf("exchange response missing required 'access_token' field")
	}

	// RFC 8693 Section 2.2.1 - token_type is REQUIRED
	if result.TokenType == "" {
		logger.Info("Warning: token_type missing in response (RFC 8693 violation)")
	}

	// RFC 8693 Section 2.2.1 - issued_token_type is REQUIRED
	if result.IssuedTokenType == "" {
		logger.Info("Warning: issued_token_type missing in response (RFC 8693 violation)")
	}

	// Calculate expiration time if provided
	obtainedAt := time.Now()
	var expiresAt time.Time
	if result.ExpiresIn > 0 {
		expiresAt = obtainedAt.Add(time.Duration(result.ExpiresIn) * time.Second)
		logger.Info("Token exchange successful", "expires_in", result.ExpiresIn)
	} else {
		logger.Info("Token exchange successful", "no_expiration", true)
	}

	return &TokenInfo{
		Token:      token,
		ObtainedAt: obtainedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

func doAuthSvcRequest(req *http.Request, params *api.BaseParams) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", params.UA)
	// #nosec G704 -- URL comes from trusted operator config, see newAuthBaseParams
	return params.Client.Do(req)
}
