/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/truststore"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Token exchange defaults
const (
	DefaultTokenPath             = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec // This is a file path, not a credential
	DefaultTokenExchangeEndpoint = "/token"
)

// RFC 8693 OAuth 2.0 Token Exchange constants
const (
	RFC8693GrantType           = "urn:ietf:params:oauth:grant-type:token-exchange"
	RFC8693SubjectTokenTypeJWT = "urn:ietf:params:oauth:token-type:jwt" //nolint:gosec // This is a URN identifier, not a credential
)

// TLS config cache defaults
const (
	// AuthTLSCacheTTLEnv defines the Environment variable to configure cache TTL (e.g., "1h", "30m", "6h")
	AuthTLSCacheTTLEnv       = "OPERATOR_AUTH_TLS_CACHE_TTL"
	defaultTLSConfigCacheTTL = 6 * time.Hour // Default: refresh every 6 hours to pick up certificate rotations
)

// Global TLS config cache TTL configuration
var (
	tlsConfigCacheTTL time.Duration // Configured TTL for cache entries
	tlsCacheTTLOnce   sync.Once     // Initialize TTL once
)

// TokenInfo contains token and optional expiration information
type TokenInfo struct {
	Token     string
	ExpiresAt time.Time
}

type (
	AuthNClientInterface interface {
		getAdminToken(ctx context.Context, ais *aisv1.AIStore) (*TokenInfo, error)
	}

	AuthNClient struct {
		k8sClient *aisclient.K8sClient
	}

	// AuthConfig interface for getting AuthN configuration
	AuthConfig interface {
		GetServiceURL() string
		IsTokenExchange() bool
		GetTokenPath() string
		GetTokenExchangeEndpoint() string
		GetOAuthLoginConf() *OAuthLoginConf
		GetSecretName() string
		GetSecretNamespace() string
		GetUserKey() string
		GetPassKey() string
		GetTLSConfig(ctx context.Context) (*tls.Config, error)
	}

	// OAuthLoginConf holds the parameters for an OAuth 2.0 password grant
	OAuthLoginConf struct {
		ClientID string
		Scope    *string
	}

	// credentials holds the username and password read from the configured login Secret
	credentials struct {
		user string
		pass string
	}

	// tlsCache holds a TLS config and rebuilds it after a TTL so CA/cert rotations take effect
	tlsCache struct {
		mu      sync.RWMutex
		config  *tls.Config
		created time.Time
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

func NewAuthNClient(k8sClient *aisclient.K8sClient) *AuthNClient {
	return &AuthNClient{
		k8sClient: k8sClient,
	}
}

// caCertPaths returns the configured CA path, falling back to the operator's default mount
// point when it exists (populated from an optional ConfigMap).
func caCertPaths(configured string) []string {
	if configured != "" {
		return []string{configured}
	}
	if _, err := os.Stat(DefaultAuthCACertPath); err == nil {
		return []string{DefaultAuthCACertPath}
	}
	return nil
}

// get returns the cached TLS config, rebuilding it from source once the cache TTL elapses
func (c *tlsCache) get(
	ctx context.Context,
	source func(context.Context) (truststore.Config, error),
	insecureSkipVerify bool,
) (*tls.Config, error) {
	logger := logf.FromContext(ctx)
	cacheTTL := getTLSConfigCacheTTL(ctx)

	c.mu.RLock()
	if c.config != nil && time.Since(c.created) < cacheTTL {
		tlsConfig := c.config
		created := c.created
		c.mu.RUnlock()
		logger.V(2).Info("Using cached TLS config", "age", time.Since(created), "ttl", cacheTTL)
		return tlsConfig, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Another goroutine may have refreshed the config while we waited for the write lock
	if c.config != nil && time.Since(c.created) < cacheTTL {
		logger.V(2).Info("Using cached TLS config (after lock)", "age", time.Since(c.created), "ttl", cacheTTL)
		return c.config, nil
	}

	trustConf, err := source(ctx)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := truststore.NewTLSConfig(logger.WithName("truststore"), trustConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}
	if insecureSkipVerify {
		logger.Info("WARNING: TLS certificate verification disabled (insecureSkipVerify=true)")
		tlsConfig.InsecureSkipVerify = true
	}

	c.config = tlsConfig
	c.created = time.Now()
	return tlsConfig, nil
}

// getAdminToken Gets an admin token for the given cluster using token exchange or configured credentials secret
func (c *AuthNClient) getAdminToken(ctx context.Context, ais *aisv1.AIStore) (*TokenInfo, error) {
	if ais.Spec.Auth == nil {
		return nil, nil
	}
	authConf, err := c.ResolveAuthConfig(ctx, ais)
	if err != nil || authConf == nil {
		return nil, err
	}
	logger := logf.FromContext(ctx)
	logger.Info("Using auth service configuration",
		"profileRef", ais.Spec.Auth.ProfileRef,
		"serviceURL", authConf.GetServiceURL(),
		"tokenExchange", authConf.IsTokenExchange())

	baseParams, err := newAuthBaseParams(ctx, authConf)
	if err != nil {
		logger.Error(err, "Failed to create auth service base params")
		return nil, fmt.Errorf("failed to create auth service base params: %w", err)
	}

	// Token exchange mode
	if authConf.IsTokenExchange() {
		return c.getTokenViaExchange(ctx, baseParams, ais, authConf)
	}
	// Username/password mode
	return c.getTokenViaPassword(ctx, baseParams, authConf)
}

// ResolveAuthConfig resolves the auth provider for the cluster, preferring the referenced
// AIStoreAuthProfile over the inline spec.auth fields
func (c *AuthNClient) ResolveAuthConfig(ctx context.Context, ais *aisv1.AIStore) (AuthConfig, error) {
	spec := ais.Spec.Auth

	var config AuthConfig
	if spec.ProfileRef != nil {
		profile, err := c.k8sClient.GetAuthProfile(ctx, spec.ProfileRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get AIStoreAuthProfile %q: %w", spec.ProfileRef.Name, err)
		}
		config = &AuthProfileConfig{profile: profile, k8sClient: c.k8sClient}
	} else {
		// Validate that exactly one auth method is configured
		if spec.TokenExchange == nil && spec.UsernamePassword == nil { //nolint:staticcheck // deprecated inline auth fields
			return nil, fmt.Errorf("invalid auth service configuration: exactly one of usernamePassword or tokenExchange must be specified")
		}
		config = &AuthSpecConfig{spec: spec, namespace: ais.Namespace}
	}
	return config, nil
}

// getSecretData Get the secret data from the specified secret name and namespace
func (c *AuthNClient) getSecretData(ctx context.Context, namespace, secretName string) (map[string][]byte, error) {
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

func (c *AuthNClient) getTokenViaPassword(ctx context.Context, bp *api.BaseParams, authConf AuthConfig) (*TokenInfo, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, params.URL, strings.NewReader(form.Encode()))
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
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &TokenInfo{
		Token:     tokenResp.AccessToken,
		ExpiresAt: expiresAt,
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
		Token:     tokenMsg.Token,
		ExpiresAt: time.Time{}, // Zero value = no expiration
	}, nil
}

func newAuthBaseParams(ctx context.Context, conf AuthConfig) (*api.BaseParams, error) {
	logger := logf.FromContext(ctx)

	transportArgs := cmn.TransportArgs{
		ClientTimeout:   10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)

	serviceURL := conf.GetServiceURL()

	// Only use TLS for HTTPS URLs
	if strings.HasPrefix(serviceURL, "https://") {
		tlsConfig, err := conf.GetTLSConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS config for auth service: %w", err)
		}

		transport.TLSClientConfig = tlsConfig
	} else {
		// HTTP connection - no TLS config needed
		logger.V(1).Info("Using HTTP (non-TLS) connection to auth service", "url", serviceURL)
	}

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.ClientTimeout,
		},
		URL: serviceURL,
		UA:  userAgent,
	}, nil
}

// getTLSConfigCacheTTL returns the configured cache TTL, reading from environment if set
// The TTL is initialized once and cached for the lifetime of the process
func getTLSConfigCacheTTL(ctx context.Context) time.Duration {
	tlsCacheTTLOnce.Do(func() {
		logger := logf.FromContext(ctx)
		ttlStr := os.Getenv(AuthTLSCacheTTLEnv)
		if ttlStr == "" {
			tlsConfigCacheTTL = defaultTLSConfigCacheTTL
			logger.Info("Using default TLS cache TTL", "ttl", tlsConfigCacheTTL)
			return
		}

		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			logger.Error(err, "Invalid OPERATOR_AUTH_TLS_CACHE_TTL, using default",
				"value", ttlStr, "default", defaultTLSConfigCacheTTL)
			tlsConfigCacheTTL = defaultTLSConfigCacheTTL
			return
		}

		if ttl < time.Minute {
			logger.Info("OPERATOR_AUTH_TLS_CACHE_TTL too short, using minimum 1 minute",
				"requested", ttl, "using", time.Minute)
			tlsConfigCacheTTL = time.Minute
			return
		}

		tlsConfigCacheTTL = ttl
		logger.Info("Using configured TLS cache TTL", "ttl", tlsConfigCacheTTL)
	})
	return tlsConfigCacheTTL
}

// getTokenViaExchange reads a token from filesystem and exchanges it with the configured auth service for an AIS token
func (*AuthNClient) getTokenViaExchange(ctx context.Context, bp *api.BaseParams, ais *aisv1.AIStore, conf AuthConfig) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	tokenPath := conf.GetTokenPath()
	endpoint := conf.GetTokenExchangeEndpoint()

	sourceToken, err := readTokenFromFile(tokenPath)
	if err != nil {
		logger.Error(err, "Failed to read source token", "path", tokenPath)
		return nil, fmt.Errorf("failed to read token from %s: %w", tokenPath, err)
	}

	// Get all audiences from the AIStore cluster's required claims configuration
	// If not configured, we pass an empty slice (don't request audiences if cluster doesn't require them)
	audiences := ais.GetRequiredAudiences()

	tokenInfo, err := exchangeTokenWithAuthSvc(ctx, bp, sourceToken, endpoint, audiences)
	if err != nil {
		logger.Error(err, "Failed to exchange token with auth service", "audiences", audiences)
		return nil, err
	}

	logger.Info("Successfully exchanged token with auth service", "tokenPath", tokenPath, "audiences", audiences)
	return tokenInfo, nil
}

// readTokenFromFile reads and returns a token from the specified file path
func readTokenFromFile(path string) (string, error) {
	tokenBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", fmt.Errorf("token file is empty: %s", path)
	}
	return token, nil
}

// exchangeTokenWithAuthSvc exchanges a source token (e.g., K8s SA token) for an AIS JWT token
// Implements RFC 8693 OAuth 2.0 Token Exchange specification
// See: https://datatracker.ietf.org/doc/html/rfc8693
func exchangeTokenWithAuthSvc(ctx context.Context, params *api.BaseParams, sourceToken, endpoint string, audiences []string) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	// RFC 8693 Section 2.1 - Request format (form-encoded)
	formData := url.Values{}
	formData.Set("grant_type", RFC8693GrantType)                   // REQUIRED
	formData.Set("subject_token", sourceToken)                     // REQUIRED
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
	var expiresAt time.Time
	if result.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
		logger.Info("Token exchange successful", "expires_in", result.ExpiresIn)
	} else {
		logger.Info("Token exchange successful", "no_expiration", true)
	}

	return &TokenInfo{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func doAuthSvcRequest(req *http.Request, params *api.BaseParams) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", params.UA)
	// #nosec G704 -- URL comes from trusted operator config, see newAuthBaseParams
	return params.Client.Do(req)
}
