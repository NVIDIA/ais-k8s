/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/aistore/proxy"
	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const APIModePublic = "public"

//go:generate mockgen -source $GOFILE -destination mocks/client_manager.go . AISClientManagerInterface

type (
	AISClientManagerInterface interface {
		GetClient(ctx context.Context, ais *aisv1.AIStore) (AIStoreClientInterface, error)
	}

	AISClientManager struct {
		mu         sync.RWMutex
		k8sClient  *aisclient.K8sClient
		tlsOpts    AISClientTLSOpts
		authClient *AuthClient
		clientMap  map[string]*cachedClient
	}

	// cachedClient is a client for one AIS cluster, with the TLS settings it was built for.
	cachedClient struct {
		client      *AIStoreClient
		tlsSettings string
	}
)

func NewAISClientManager(k8sClient *aisclient.K8sClient, tlsOpts AISClientTLSOpts) *AISClientManager {
	return &AISClientManager{
		k8sClient:  k8sClient,
		tlsOpts:    tlsOpts,
		authClient: NewAuthClient(k8sClient),
		clientMap:  make(map[string]*cachedClient, 16),
	}
}

// GetClient gets an AIStoreClientInterface for making requests to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
// It replaces the token of a cached client when that token is no longer usable.
func (m *AISClientManager) GetClient(ctx context.Context,
	ais *aisv1.AIStore,
) (AIStoreClientInterface, error) {
	logger := logf.FromContext(ctx).WithValues("cluster", ais.NamespacedName().String())
	m.mu.RLock()
	cached, exists := m.clientMap[ais.NamespacedName().String()]
	m.mu.RUnlock()

	url, err := m.getAISAPIEndpoint(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get AIS API parameters")
		return nil, err
	}

	settings := tlsSettings(ais)

	// Check if the client params are valid
	if exists && cached.tlsSettings == settings {
		cached.client.syncPublicURL(ctx, url)
		if cached.client.HasValidBaseParams(ctx, ais, url) {
			if tokErr := m.ensureValidToken(ctx, ais, cached.client); tokErr != nil {
				return nil, tokErr
			}
			return cached.client, nil
		}
	}

	// Attempt to get an authN token using the spec.auth field
	tokenInfo, err := m.authClient.getAdminToken(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get admin token for AuthN")
		return nil, err
	}

	tlsConf, err := m.getTLSConfig(ctx, ais)
	if err != nil {
		return nil, err
	}

	logNewClient(logger, tokenInfo, tlsConf, url)
	client := NewAIStoreClient(ctx, url, tokenInfo, ais.GetAPIMode(), tlsConf)
	m.mu.Lock()
	m.clientMap[ais.NamespacedName().String()] = &cachedClient{client: client, tlsSettings: settings}
	m.mu.Unlock()
	return client, nil
}

// ensureValidToken replaces the token of a cached client when that token is no longer usable.
func (m *AISClientManager) ensureValidToken(ctx context.Context, ais *aisv1.AIStore, client *AIStoreClient) error {
	profileGen, err := m.authClient.profileGeneration(ctx, ais)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to resolve auth profile generation",
			"cluster", ais.NamespacedName().String())
		return err
	}
	reason := client.tokenRefreshReason(profileGen)
	if reason == "" {
		return nil
	}
	profile := "none"
	if profileRef := ais.GetAuthProfileRef(); profileRef != nil {
		profile = profileRef.Name
	}
	logger := logf.FromContext(ctx).WithValues("cluster", ais.NamespacedName().String(), "profile", profile)
	tokenInfo, err := m.authClient.getAdminToken(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get admin token for refresh")
		return err
	}

	hasExpiration := tokenInfo != nil && !tokenInfo.ExpiresAt.IsZero()
	logger.Info("Refreshing AIS API token", "reason", reason, "tokenExpires", hasExpiration)
	client.setToken(tokenInfo)
	return nil
}

func logNewClient(logger logr.Logger, tokenInfo *TokenInfo, tlsConf *tls.Config, url string) {
	msg := "Creating AIS API client"
	hasToken := tokenInfo != nil && tokenInfo.Token != ""
	clientLogger := logger.WithValues("url", url, "hasToken", hasToken)
	if hasToken {
		clientLogger = clientLogger.WithValues("tokenExpires", !tokenInfo.ExpiresAt.IsZero())
	}
	if tlsConf == nil {
		msg += ". Warning: TLS not enabled"
	} else if tlsConf.InsecureSkipVerify {
		msg += ". Warning: TLS certificate verification disabled"
	}
	clientLogger.Info(msg)
}

func (m *AISClientManager) getAISAPIEndpoint(ctx context.Context,
	ais *aisv1.AIStore,
) (string, error) {
	switch {
	case ais.GetAPIMode() == APIModePublic:
		hostname, err := m.getPublicAISHostname(ctx, ais)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get public AIS API parameters")
			return "", err
		}
		proxyPublicPort := ais.ProxyPublicPort()
		return cmn.AISHostURL(ais, hostname, proxyPublicPort.String()), nil
	// If LoadBalancer is configured use the LB service to contact the API.
	case ais.ProxyExternalAccessEnabled():
		proxyLBSVC, svcErr := m.k8sClient.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
		if svcErr != nil {
			return "", svcErr
		}
		var hostname string
		for _, ing := range proxyLBSVC.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				hostname = ing.IP
				break
			}
			if ing.Hostname != "" {
				hostname = ing.Hostname
				break
			}
		}
		if hostname == "" {
			return "", fmt.Errorf("proxy load balancer svc %q has no ingress IP or hostname", proxy.LoadBalancerSVCNSName(ais))
		}
		externalPort := ais.ProxyExternalPort()
		return cmn.AISHostURL(ais, hostname, externalPort.String()), nil
	// When operator is deployed within K8s cluster with no external LoadBalancer,
	// use the proxy headless service to request the API.
	default:
		return cmn.IntraClusterURL(ais), nil
	}
}

func (m *AISClientManager) getPublicAISHostname(ctx context.Context, ais *aisv1.AIStore) (hostname string, err error) {
	// Find ANY ready proxy pod and return the public endpoint
	selector := proxy.SelectorLabels(ais)
	pods, err := m.k8sClient.ListReadyPods(ctx, ais, selector)
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no ready pods found matching selector %v", selector)
	}
	return pods.Items[0].Status.HostIP, nil
}
