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
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
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
	}

	// AuthSpecConfig wraps the CRD AuthSpec configuration
	AuthSpecConfig struct {
		spec      *aisv1.AuthSpec
		namespace string // cluster namespace for default secret lookup
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
	return getTokenFromAuthN(ctx, authNBaseParams(authnConf), secretData)
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

func authNBaseParams(conf AuthConfig) *api.BaseParams {
	transportArgs := cmn.TransportArgs{
		Timeout:         10 * time.Second,
		UseHTTPProxyEnv: true,
	}
	transport := cmn.NewTransport(transportArgs)

	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &api.BaseParams{
		Client: &http.Client{
			Transport: transport,
			Timeout:   transportArgs.Timeout,
		},
		URL: conf.GetServiceURL(),
		UA:  userAgent,
	}
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

	tokenInfo, err := exchangeTokenWithAuthN(ctx, authNBaseParams(conf), sourceToken, endpoint)
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
