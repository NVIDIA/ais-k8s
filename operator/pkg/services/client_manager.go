// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"fmt"
	"sync"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	APIModePublic = "public"
)

//go:generate mockgen -source $GOFILE -destination mocks/client_manager.go . AISClientManagerInterface

type (
	AISClientTLSOpts struct {
		CertPath       string
		CertPerCluster bool
	}

	AISClientManagerInterface interface {
		GetClient(ctx context.Context, ais *aisv1.AIStore) (AIStoreClientInterface, error)
	}

	AISClientManager struct {
		mu        sync.RWMutex
		k8sClient *aisclient.K8sClient
		tlsOpts   AISClientTLSOpts
		authN     AuthNClientInterface
		clientMap map[string]AIStoreClientInterface
	}
)

func NewAISClientManager(k8sClient *aisclient.K8sClient, tlsOpts AISClientTLSOpts) *AISClientManager {
	return &AISClientManager{
		k8sClient: k8sClient,
		tlsOpts:   tlsOpts,
		authN:     NewAuthNClient(),
		clientMap: make(map[string]AIStoreClientInterface, 16),
	}
}

// GetClient gets an AIStoreClientInterface for making request to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
func (m *AISClientManager) GetClient(ctx context.Context,
	ais *aisv1.AIStore,
) (client AIStoreClientInterface, err error) {
	logger := logf.FromContext(ctx)
	// First get the client for the given cluster and return it if its params are still valid
	m.mu.RLock()
	client, exists := m.clientMap[ais.NamespacedName().String()]
	if exists && client.HasValidBaseParams(ctx, ais) {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()
	// If the client does not exist or is no longer valid, create and cache a new client with the new params
	url, err := m.getAISAPIEndpoint(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get AIS API parameters")
		return
	}

	var adminToken string
	if ais.Spec.AuthNSecretName != nil {
		adminToken, err = m.authN.getAdminToken(ctx)
		if err != nil {
			logger.Error(err, "Failed to get admin token for AuthN")
			return nil, err
		}
	}
	logger.Info("Creating AIS API client", "url", url, "authN", adminToken != "")
	client = NewAIStoreClient(ctx, url, adminToken, ais.GetAPIMode())
	m.mu.Lock()
	m.clientMap[ais.NamespacedName().String()] = client
	m.mu.Unlock()
	return
}

func (m *AISClientManager) getAISAPIEndpoint(ctx context.Context,
	ais *aisv1.AIStore,
) (string, error) {
	var (
		err      error
		hostname string
		port     string
	)

	switch {
	case ais.GetAPIMode() == APIModePublic:
		hostname, err = m.getPublicAISHostname(ctx, ais)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get public AIS API parameters")
			return "", err
		}
		port = ais.Spec.ProxySpec.PublicPort.String()
	// If LoadBalancer is configured use the LB service to contact the API.
	case ais.Spec.EnableExternalLB:
		proxyLBSVC, svcErr := m.k8sClient.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
		if svcErr != nil {
			return "", svcErr
		}

		for _, ing := range proxyLBSVC.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				hostname = ing.IP
				break
			}
		}
		if hostname == "" {
			return "", fmt.Errorf("proxy load balancer svc %q has no ingress IP", proxy.LoadBalancerSVCNSName(ais))
		}
		port = ais.Spec.ProxySpec.ServicePort.String()
	// When operator is deployed within K8s cluster with no external LoadBalancer,
	// use the proxy headless service to request the API.
	default:
		hostname = proxy.HeadlessSVCNSName(ais).Name + "." + ais.Namespace
		port = ais.Spec.ProxySpec.ServicePort.String()
	}
	return createAPIURL(ais.UseHTTPS(), hostname, port), nil
}

func (m *AISClientManager) getPublicAISHostname(ctx context.Context, ais *aisv1.AIStore) (hostname string, err error) {
	// Find ANY ready proxy pod and return the public endpoint
	pods, err := m.k8sClient.ListReadyPods(ctx, ais, proxy.PodLabels(ais))
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no ready pods found matching selector %q", proxy.PodLabels(ais))
	}
	return pods.Items[0].Status.HostIP, nil
}

func createAPIURL(https bool, hostname, port string) string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, hostname, port)
}
