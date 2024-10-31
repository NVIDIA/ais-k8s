// Package services contains services for the operator to use when reconciling AIS
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

//go:generate mockgen -source $GOFILE -destination mocks/client_manager.go . AISClientManagerInterface

type (
	AISClientManagerInterface interface {
		GetClient(ctx context.Context, ais *aisv1.AIStore) (AIStoreClientInterface, error)
	}

	AISClientManager struct {
		mu        sync.RWMutex
		k8sClient *aisclient.K8sClient
		authN     AuthNClientInterface
		clientMap map[string]AIStoreClientInterface
	}
)

func NewAISClientManager(k8sClient *aisclient.K8sClient) *AISClientManager {
	return &AISClientManager{
		k8sClient: k8sClient,
		authN:     NewAuthNClient(),
		clientMap: make(map[string]AIStoreClientInterface, 16),
	}
}

// GetClient gets an AIStoreClientInterface for making request to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
func (m *AISClientManager) GetClient(ctx context.Context,
	ais *aisv1.AIStore,
) (client AIStoreClientInterface, err error) {
	// First get the client for the given cluster and return it if its params are still valid
	m.mu.RLock()
	client, exists := m.clientMap[ais.NamespacedName().String()]
	if exists && client.HasValidBaseParams(ais) {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()
	// If the client does not exist or is no longer valid, create and cache a new client with the new params
	url, err := m.getAISAPIEndpoint(ctx, ais)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to get AIS API parameters")
		return
	}

	var adminToken string
	if ais.Spec.AuthNSecretName != nil {
		adminToken, err = m.authN.getAdminToken(ctx)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get admin token for AuthN")
			return nil, err
		}
	}

	var pool *x509.CertPool
	if ais.GetHTTPSClientCA() != "" {
		cert, err := os.ReadFile(ais.GetHTTPSClientCA())
		if err != nil {
			return nil, err
		}
		pool = x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(cert); !ok {
			return nil, fmt.Errorf("operator tls: failed to append CA certs from PEM: %q", ais.GetHTTPSClientCA())
		}
	}
	tlsConf := &tls.Config{RootCAs: pool, InsecureSkipVerify: ais.GetHTTPSSkipVerifyCrt()}
	if ais.GetHTTPSCertificate() != "" && ais.GetHTTPSCertKey() != "" {
		tlsConf.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(ais.GetHTTPSCertificate(), ais.GetHTTPSCertKey())
			if err != nil {
				return nil, err
			}
			return &cert, nil
		}
	}

	client = NewAIStoreClient(ctx, url, adminToken, tlsConf)
	m.mu.Lock()
	m.clientMap[ais.NamespacedName().String()] = client
	m.mu.Unlock()
	return
}

func (m *AISClientManager) getAISAPIEndpoint(ctx context.Context,
	ais *aisv1.AIStore,
) (url string, err error) {
	var serviceHostname string

	// If LoadBalancer is configured use the LB service to contact the API.
	if ais.Spec.EnableExternalLB {
		var proxyLBSVC *corev1.Service
		proxyLBSVC, err = m.k8sClient.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
		if err != nil {
			return "", err
		}

		for _, ing := range proxyLBSVC.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				serviceHostname = ing.IP
				goto createParams
			}
		}
		err = fmt.Errorf("failed to fetch LoadBalancer service %q, err: %v", proxy.LoadBalancerSVCNSName(ais), err)
		return
	}

	// When operator is deployed within K8s cluster with no external LoadBalancer,
	// use the proxy headless service to request the API.
	serviceHostname = proxy.HeadlessSVCNSName(ais).Name + "." + ais.Namespace
createParams:
	var scheme string
	scheme = "http"
	if ais.UseHTTPS() {
		scheme = "https"
	}
	url = fmt.Sprintf("%s://%s:%s", scheme, serviceHostname, ais.Spec.ProxySpec.ServicePort.String())

	return
}
