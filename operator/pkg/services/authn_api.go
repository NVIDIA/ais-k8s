// Package services contains services for the operator to use when reconciling AIS
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
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/truststore"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// AuthN constants
const (
	OperatorNamespace      = "OPERATOR_NAMESPACE"
	DefaultAuthNServiceURL = "http://ais-authn.ais:52001"

	AuthNConfigMapVar  = "AIS_AUTHN_CM"
	AuthNSecretRefName = "SU-NAME"
	AuthNSecretRefPass = "SU-PASS"

	// Token exchange defaults
	DefaultTokenPath             = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec // This is a file path, not a credential
	DefaultTokenExchangeEndpoint = "/token"

	// RFC 8693 OAuth 2.0 Token Exchange constants
	RFC8693GrantType           = "urn:ietf:params:oauth:grant-type:token-exchange"
	RFC8693SubjectTokenTypeJWT = "urn:ietf:params:oauth:token-type:jwt" //nolint:gosec // This is a URN identifier, not a credential

	// TLS config cache defaults
	// Environment variable to configure cache TTL: OPERATOR_AUTH_TLS_CACHE_TTL (e.g., "1h", "30m", "6h")
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
		GetSecretName() string
		GetSecretNamespace() string
		GetCACertPath() string
		GetInsecureSkipVerify() bool
		GetTLSConfig(ctx context.Context) (*tls.Config, error)
	}

	// AuthNConfigMapConfig is the legacy ConfigMap-based configuration
	AuthNConfigMapConfig struct {
		TLS             bool   `json:"tls"`
		Host            string `json:"host"`
		Port            string `json:"port"`
		SecretNamespace string `json:"secretNamespace"`
		SecretName      string `json:"secretName"`
		// Token exchange fields
		UseTokenExchange      bool   `json:"useTokenExchange"`
		TokenPath             string `json:"tokenPath"`
		TokenExchangeEndpoint string `json:"tokenExchangeEndpoint"`
		// TLS configuration fields
		CACertPath         string `json:"caCertPath,omitempty"`
		InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
		// TLS config caching
		tlsConfig  *tls.Config
		tlsCreated time.Time
		tlsMu      sync.RWMutex
	}

	// AuthSpecConfig wraps the CRD AuthSpec configuration
	AuthSpecConfig struct {
		spec      *aisv1.AuthSpec
		namespace string // cluster namespace for default secret lookup
		// TLS config caching
		tlsConfig  *tls.Config
		tlsCreated time.Time
		tlsMu      sync.RWMutex
	}
)

func NewAuthNClient(k8sClient *aisclient.K8sClient) *AuthNClient {
	return &AuthNClient{
		k8sClient: k8sClient,
	}
}

// AuthNConfigMapConfig implements AuthConfig interface
func (c *AuthNConfigMapConfig) GetServiceURL() string {
	return createAPIURL(c.TLS, c.Host, c.Port)
}

func (c *AuthNConfigMapConfig) IsTokenExchange() bool {
	return c.UseTokenExchange
}

func (c *AuthNConfigMapConfig) GetTokenPath() string {
	return c.TokenPath
}

func (c *AuthNConfigMapConfig) GetTokenExchangeEndpoint() string {
	return c.TokenExchangeEndpoint
}

func (c *AuthNConfigMapConfig) GetSecretName() string {
	return c.SecretName
}

func (c *AuthNConfigMapConfig) GetSecretNamespace() string {
	return c.SecretNamespace
}

func (c *AuthNConfigMapConfig) GetCACertPath() string {
	return c.CACertPath
}

func (c *AuthNConfigMapConfig) GetInsecureSkipVerify() bool {
	return c.InsecureSkipVerify
}

func (c *AuthNConfigMapConfig) GetTLSConfig(ctx context.Context) (*tls.Config, error) {
	return getTLSConfigWithCache(ctx, &c.tlsMu, &c.tlsConfig, &c.tlsCreated, c.GetCACertPath(), c.GetInsecureSkipVerify())
}

// AuthSpecConfig implements AuthConfig interface
func (c *AuthSpecConfig) GetServiceURL() string {
	serviceURL := DefaultAuthNServiceURL
	if c.spec.ServiceURL != nil {
		serviceURL = *c.spec.ServiceURL
	}
	return serviceURL
}

func (c *AuthSpecConfig) IsTokenExchange() bool {
	return c.spec.TokenExchange != nil
}

func (c *AuthSpecConfig) GetTokenPath() string {
	if c.spec.TokenExchange != nil && c.spec.TokenExchange.TokenPath != nil {
		return *c.spec.TokenExchange.TokenPath
	}
	return DefaultTokenPath
}

func (c *AuthSpecConfig) GetTokenExchangeEndpoint() string {
	if c.spec.TokenExchange != nil && c.spec.TokenExchange.TokenExchangeEndpoint != nil {
		return *c.spec.TokenExchange.TokenExchangeEndpoint
	}
	return DefaultTokenExchangeEndpoint
}

func (c *AuthSpecConfig) GetSecretName() string {
	if c.spec.UsernamePassword != nil {
		return c.spec.UsernamePassword.SecretName
	}
	return ""
}

func (c *AuthSpecConfig) GetSecretNamespace() string {
	if c.spec.UsernamePassword != nil && c.spec.UsernamePassword.SecretNamespace != nil {
		return *c.spec.UsernamePassword.SecretNamespace
	}
	return c.namespace
}

func (c *AuthSpecConfig) GetCACertPath() string {
	if c.spec.TLS != nil {
		return c.spec.TLS.CACertPath
	}
	return ""
}

func (c *AuthSpecConfig) GetInsecureSkipVerify() bool {
	if c.spec.TLS != nil {
		return c.spec.TLS.InsecureSkipVerify
	}
	return false
}

func (c *AuthSpecConfig) GetTLSConfig(ctx context.Context) (*tls.Config, error) {
	return getTLSConfigWithCache(ctx, &c.tlsMu, &c.tlsConfig, &c.tlsCreated, c.GetCACertPath(), c.GetInsecureSkipVerify())
}

// getTLSConfigWithCache is a helper function that implements the TLS config caching logic
// shared by both AuthNConfigMapConfig and AuthSpecConfig
func getTLSConfigWithCache(ctx context.Context, mu *sync.RWMutex, cachedConfig **tls.Config, cachedTime *time.Time, caCertPath string, insecureSkipVerify bool) (*tls.Config, error) {
	logger := logf.FromContext(ctx)
	cacheTTL := getTLSConfigCacheTTL(ctx)

	mu.RLock()
	// Check if we have a valid cached config
	if *cachedConfig != nil && time.Since(*cachedTime) < cacheTTL {
		tlsConfig := *cachedConfig
		mu.RUnlock()
		logger.V(2).Info("Using cached TLS config", "age", time.Since(*cachedTime), "ttl", cacheTTL)
		return tlsConfig, nil
	}
	mu.RUnlock()

	// Need to create/refresh TLS config
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if *cachedConfig != nil && time.Since(*cachedTime) < cacheTTL {
		logger.V(2).Info("Using cached TLS config (after lock)", "age", time.Since(*cachedTime), "ttl", cacheTTL)
		return *cachedConfig, nil
	}

	// Create new TLS config
	var caCertPaths []string
	if caCertPath != "" {
		caCertPaths = []string{caCertPath}
	}
	logger.V(1).Info("Creating new TLS config", "caCertPath", caCertPath)
	tlsConfig, err := truststore.NewTLSConfig(logger.WithName("truststore"), truststore.Config{
		CACertPaths: caCertPaths,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	// Apply insecureSkipVerify if configured
	if insecureSkipVerify {
		logger.Info("WARNING: TLS certificate verification disabled (insecureSkipVerify=true)")
		tlsConfig.InsecureSkipVerify = true
	}

	// Cache the new config
	*cachedConfig = tlsConfig
	*cachedTime = time.Now()

	return tlsConfig, nil
}

// getAdminToken Gets an admin token for the given cluster using the credentials secret referenced by the operator's authN configmap
func (c *AuthNClient) getAdminToken(ctx context.Context, ais *aisv1.AIStore) (*TokenInfo, error) {
	authnConf, err := c.getAuthConfig(ctx, ais)
	if err != nil || authnConf == nil {
		return nil, err
	}

	// Token exchange mode
	if authnConf.IsTokenExchange() {
		return c.getTokenViaExchange(ctx, authnConf)
	}

	// Username/password mode (existing)
	if authnConf.GetSecretName() == "" {
		return nil, nil
	}
	secretData, err := c.getSecretData(ctx, authnConf.GetSecretNamespace(), authnConf.GetSecretName())
	if err != nil || secretData == nil {
		return nil, err
	}
	baseParams, err := newAuthNBaseParams(ctx, authnConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create AuthN base params: %w", err)
	}
	return getTokenFromAuthN(ctx, baseParams, secretData)
}

// getAuthConfig Gets the AuthN configuration from the CRD first, falls back to ConfigMap
func (c *AuthNClient) getAuthConfig(ctx context.Context, ais *aisv1.AIStore) (AuthConfig, error) {
	// First, check if AuthN configuration is in the CRD spec
	if ais.Spec.Auth != nil {
		return c.getAuthConfigFromCRD(ctx, ais)
	}

	// Fall back to ConfigMap for backward compatibility
	return c.getAuthConfigFromConfigMap(ctx, ais)
}

// getAuthConfigFromCRD extracts AuthN configuration from the AIStore CRD spec
func (*AuthNClient) getAuthConfigFromCRD(ctx context.Context, ais *aisv1.AIStore) (AuthConfig, error) {
	logger := logf.FromContext(ctx)
	spec := ais.Spec.Auth

	// Validate that exactly one auth method is configured
	if spec.TokenExchange == nil && spec.UsernamePassword == nil {
		return nil, fmt.Errorf("invalid AuthN configuration: exactly one of usernamePassword or tokenExchange must be specified")
	}

	config := &AuthSpecConfig{
		spec:      spec,
		namespace: ais.Namespace,
	}

	logger.Info("Using AuthN configuration from CRD",
		"serviceURL", config.GetServiceURL(),
		"tokenExchange", config.IsTokenExchange())

	return config, nil
}

// getAuthConfigFromConfigMap Gets the data from the configmap defined by `AIS_AUTHN_CM` (legacy)
func (c *AuthNClient) getAuthConfigFromConfigMap(ctx context.Context, ais *aisv1.AIStore) (AuthConfig, error) {
	logger := logf.FromContext(ctx)
	// Get the authN credentials secret name for this cluster, if it exists
	cmName, found := os.LookupEnv(AuthNConfigMapVar)
	if !found {
		return nil, nil
	}
	cmNs, found := os.LookupEnv(OperatorNamespace)
	if !found {
		logger.Info("OPERATOR_NAMESPACE environment variable not set, failed to find a ConfigMap for AuthN")
		return nil, nil
	}
	cm, err := c.k8sClient.GetConfigMap(ctx, types.NamespacedName{Name: cmName, Namespace: cmNs})
	// If the config map doesn't exist we haven't configured the operator to use authN at all
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get AuthN ConfigMap %s in namespace %s", cmName, cmNs))
		return nil, err
	}
	if cm == nil || cm.Data == nil {
		return nil, fmt.Errorf("AuthN ConfigMap %s in namespace %s has no data", cmName, cmNs)
	}
	key := ais.Namespace + "-" + ais.Name
	confJSON, ok := cm.Data[key]
	if !ok {
		return nil, nil
	}
	var conf *AuthNConfigMapConfig
	if err = json.Unmarshal([]byte(confJSON), &conf); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to unmarshal entry for cluster %s in AuthN ConfigMap %s in namespace %s", key, cmName, cmNs))
		return nil, err
	}
	logger.Info("Using AuthN configuration from ConfigMap (DEPRECATED - consider migrating to CRD spec.auth)",
		"configMap", cmName,
		"cluster", key)

	return conf, nil
}

// getSecretData Get the secret data from the specified secret name and namespace
func (c *AuthNClient) getSecretData(ctx context.Context, namespace, secretName string) (map[string][]byte, error) {
	logger := logf.FromContext(ctx)
	// Look up the secret credentials and use them to obtain a token
	secret, err := c.k8sClient.GetSecret(ctx, types.NamespacedName{Name: secretName, Namespace: namespace})
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get AuthN credentials secret %s in namespace %s", secretName, namespace))
		return nil, err
	}
	if secret == nil || len(secret.Data) == 0 {
		return nil, fmt.Errorf("AuthN Secret %s in namespace %s has no data", secretName, namespace)
	}
	return secret.Data, nil
}

// getTokenFromAuthN retrieves an admin token from AuthN using the username and password from the provided secret data
func getTokenFromAuthN(ctx context.Context, params *api.BaseParams, secretData map[string][]byte) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)
	zeroDuration := time.Duration(0)
	user := string(secretData[AuthNSecretRefName])
	pass := string(secretData[AuthNSecretRefPass])
	tokenMsg, err := authn.LoginUser(*params, user, pass, &zeroDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to login %q user to AuthN: %w", user, err)
	}

	logger.Info(fmt.Sprintf("Successfully fetched token for user %q from AuthN", user))
	// Username/password mode doesn't provide expiration info
	return &TokenInfo{
		Token:     tokenMsg.Token,
		ExpiresAt: time.Time{}, // Zero value = no expiration
	}, nil
}

func newAuthNBaseParams(ctx context.Context, conf AuthConfig) (*api.BaseParams, error) {
	logger := logf.FromContext(ctx)

	transportArgs := cmn.TransportArgs{
		Timeout:         10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)

	serviceURL := conf.GetServiceURL()

	// Only use TLS for HTTPS URLs
	if strings.HasPrefix(serviceURL, "https://") {
		tlsConfig, err := conf.GetTLSConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS config for Auth service: %w", err)
		}

		transport.TLSClientConfig = tlsConfig
	} else {
		// HTTP connection - no TLS config needed
		logger.V(1).Info("Using HTTP (non-TLS) connection to Auth service", "url", serviceURL)
	}

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.Timeout,
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

// getTokenViaExchange reads a token from filesystem and exchanges it with AuthN for an AIS token
func (*AuthNClient) getTokenViaExchange(ctx context.Context, conf AuthConfig) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	tokenPath := conf.GetTokenPath()
	endpoint := conf.GetTokenExchangeEndpoint()

	sourceToken, err := readTokenFromFile(tokenPath)
	if err != nil {
		logger.Error(err, "Failed to read source token", "path", tokenPath)
		return nil, fmt.Errorf("failed to read token from %s: %w", tokenPath, err)
	}

	baseParams, err := newAuthNBaseParams(ctx, conf)
	if err != nil {
		logger.Error(err, "Failed to create AuthN base params")
		return nil, fmt.Errorf("failed to create AuthN base params: %w", err)
	}

	tokenInfo, err := exchangeTokenWithAuthN(ctx, baseParams, sourceToken, endpoint)
	if err != nil {
		logger.Error(err, "Failed to exchange token with AuthN")
		return nil, err
	}

	logger.Info("Successfully exchanged token with AuthN", "tokenPath", tokenPath)
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

// exchangeTokenWithAuthN exchanges a source token (e.g., K8s SA token) for an AIS JWT token
// Implements RFC 8693 OAuth 2.0 Token Exchange specification
// See: https://datatracker.ietf.org/doc/html/rfc8693
func exchangeTokenWithAuthN(ctx context.Context, params *api.BaseParams, sourceToken, endpoint string) (*TokenInfo, error) {
	logger := logf.FromContext(ctx)

	// RFC 8693 Section 2.1 - Request format (form-encoded)
	formData := url.Values{}
	formData.Set("grant_type", RFC8693GrantType)                   // REQUIRED
	formData.Set("subject_token", sourceToken)                     // REQUIRED
	formData.Set("subject_token_type", RFC8693SubjectTokenTypeJWT) // REQUIRED

	requestURL := params.URL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", params.UA)

	resp, err := params.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	// RFC 8693 Section 2.2 - Response format (REQUIRED fields only)
	var result struct {
		AccessToken     string `json:"access_token"`         // REQUIRED
		IssuedTokenType string `json:"issued_token_type"`    // REQUIRED
		TokenType       string `json:"token_type"`           // REQUIRED
		ExpiresIn       int    `json:"expires_in,omitempty"` // Not required by RFC but needed for token expiration
		Token           string `json:"token"`                // Legacy: backward compatibility
	}

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
