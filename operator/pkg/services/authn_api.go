// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	OperatorNamespace    = "OPERATOR_NAMESPACE"
	AuthNServiceHostName = "ais-authn.ais"
	AuthNServicePort     = "52001"

	AuthNConfigMapVar  = "AIS_AUTHN_CM"
	AuthNSecretRefName = "SU-NAME"
	AuthNSecretRefPass = "SU-PASS"

	// Token exchange defaults
	DefaultTokenPath             = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec // This is a file path, not a credential
	DefaultTokenExchangeEndpoint = "/token"                                              //nolint:gosec // This is a URL path, not a credential
)

type (
	AuthNClientInterface interface {
		getAdminToken(ctx context.Context, ais *aisv1.AIStore) (string, error)
	}

	AuthNClient struct {
		k8sClient *aisclient.K8sClient
	}

	AuthNClusterConfig struct {
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
)

func NewAuthNClient(k8sClient *aisclient.K8sClient) *AuthNClient {
	return &AuthNClient{
		k8sClient: k8sClient,
	}
}

// getAdminToken Gets an admin token for the given cluster using the credentials secret referenced by the operator's authN configmap
func (c *AuthNClient) getAdminToken(ctx context.Context, ais *aisv1.AIStore) (string, error) {
	authnConf, err := c.getAuthConfig(ctx, ais)
	if err != nil || authnConf == nil {
		return "", err
	}

	// Token exchange mode
	if authnConf.UseTokenExchange {
		return c.getTokenViaExchange(ctx, authnConf)
	}

	// Username/password mode (existing)
	if authnConf.SecretName == "" {
		return "", nil
	}
	secretData, err := c.getSecretData(ctx, authnConf.SecretNamespace, authnConf.SecretName)
	if err != nil || secretData == nil {
		return "", err
	}
	return getTokenFromAuthN(ctx, authNBaseParams(authnConf), secretData)
}

// getAuthConfig Gets the data from the configmap defined by `AIS_AUTHN_CM`
func (c *AuthNClient) getAuthConfig(ctx context.Context, ais *aisv1.AIStore) (*AuthNClusterConfig, error) {
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
	var conf *AuthNClusterConfig
	if err = json.Unmarshal([]byte(confJSON), &conf); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to unmarshal entry for cluster %s in AuthN ConfigMap %s in namespace %s", key, cmName, cmNs))
		return nil, err
	}
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
func getTokenFromAuthN(ctx context.Context, params *api.BaseParams, secretData map[string][]byte) (string, error) {
	logger := logf.FromContext(ctx)
	zeroDuration := time.Duration(0)
	user := string(secretData[AuthNSecretRefName])
	pass := string(secretData[AuthNSecretRefPass])
	tokenMsg, err := authn.LoginUser(*params, user, pass, &zeroDuration)
	if err != nil {
		return "", fmt.Errorf("failed to login %q user to AuthN: %w", user, err)
	}

	logger.Info(fmt.Sprintf("Successfully fetched token for user %q from AuthN", user))
	return tokenMsg.Token, nil
}

func authNBaseParams(conf *AuthNClusterConfig) *api.BaseParams {
	host := conf.Host
	port := conf.Port
	if host == "" {
		host = AuthNServiceHostName
	}
	if port == "" {
		port = AuthNServicePort
	}

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
		URL: createAPIURL(conf.TLS, host, port),
		UA:  userAgent,
	}
}

// getTokenViaExchange reads a token from filesystem and exchanges it with AuthN for an AIS token
func (*AuthNClient) getTokenViaExchange(ctx context.Context, conf *AuthNClusterConfig) (string, error) {
	logger := logf.FromContext(ctx)

	tokenPath := conf.TokenPath
	if tokenPath == "" {
		tokenPath = DefaultTokenPath
	}

	endpoint := conf.TokenExchangeEndpoint
	if endpoint == "" {
		endpoint = DefaultTokenExchangeEndpoint
	}

	sourceToken, err := readTokenFromFile(tokenPath)
	if err != nil {
		logger.Error(err, "Failed to read source token", "path", tokenPath)
		return "", fmt.Errorf("failed to read token from %s: %w", tokenPath, err)
	}

	aisToken, err := exchangeTokenWithAuthN(ctx, authNBaseParams(conf), sourceToken, endpoint)
	if err != nil {
		logger.Error(err, "Failed to exchange token with AuthN")
		return "", err
	}

	logger.Info("Successfully exchanged token with AuthN", "tokenPath", tokenPath)
	return aisToken, nil
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
func exchangeTokenWithAuthN(ctx context.Context, params *api.BaseParams, sourceToken, endpoint string) (string, error) {
	logger := logf.FromContext(ctx)

	reqBody := map[string]interface{}{
		"token": sourceToken,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal exchange request: %w", err)
	}

	url := params.URL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", params.UA)

	resp, err := params.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode exchange response: %w", err)
	}

	if result.Token == "" {
		return "", fmt.Errorf("exchange response contained empty token")
	}

	logger.Info("Token exchange successful")
	return result.Token, nil
}
