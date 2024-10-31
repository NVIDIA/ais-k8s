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
	"path/filepath"
	"sync"

	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	APIModePublic  = "public"
	EnvSkipVerify  = "OPERATOR_SKIP_VERIFY_CRT"
	ClientCertFile = "tls.crt"
	ClientKeyFile  = "tls.key"
	ClientCAFile   = "ca.crt"
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

	tlsConf, err := m.getTLSConfig(ctx, ais)
	if err != nil {
		return nil, err
	}

	if tlsConf == nil {
		logger.Info("Creating AIS API client", "url", url, "authN", adminToken != "")
	} else {
		logger.Info("Creating HTTPS AIS API client", "url", url, "authN", adminToken != "", "tlsPath", m.getTLSPath(ais), "skipVerify", tlsConf.InsecureSkipVerify)
	}
	client = NewAIStoreClient(ctx, url, adminToken, ais.GetAPIMode(), tlsConf)
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

func (m *AISClientManager) getTLSConfig(ctx context.Context, ais *aisv1.AIStore) (*tls.Config, error) {
	if !ais.UseHTTPS() {
		return nil, nil
	}
	tlsDir := m.getTLSPath(ais)
	tlsConf := &tls.Config{}
	err := configureCAVerification(ctx, tlsConf, tlsDir)
	if err != nil {
		return nil, err
	}
	addClientCertIfRequested(ais, tlsConf, tlsDir)
	return tlsConf, err
}

func configureCAVerification(ctx context.Context, tlsConf *tls.Config, tlsDir string) error {
	skipVerify, err := aiscos.ParseBool(os.Getenv(EnvSkipVerify))
	if err != nil {
		return err
	}
	if skipVerify {
		tlsConf.InsecureSkipVerify = true
		return nil
	}
	// Add CA from our specified TLS config dir to the system trusted CA pool
	providedCA := filepath.Join(tlsDir, ClientCAFile)
	caPool, err := loadOptionalProvidedCA(providedCA)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to load AIS CA", "location", providedCA)
		return err
	}
	tlsConf.RootCAs = caPool
	return nil
}

func addClientCertIfRequested(ais *aisv1.AIStore, tlsConf *tls.Config, tlsDir string) {
	if tls.ClientAuthType(*ais.Spec.ConfigToUpdate.Net.HTTP.ClientAuthTLS) < tls.RequestClientCert {
		return
	}
	certPath := filepath.Join(tlsDir, ClientCertFile)
	keyPath := filepath.Join(tlsDir, ClientKeyFile)
	tlsConf.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		var cert tls.Certificate
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		return &cert, nil
	}
}

func (m *AISClientManager) getTLSPath(ais *aisv1.AIStore) string {
	if m.tlsOpts.CertPerCluster {
		return filepath.Join(m.tlsOpts.CertPath, ais.Namespace, ais.Name)
	}
	return m.tlsOpts.CertPath
}

// TODO: Support additional CA trust from configMap
func loadOptionalProvidedCA(caPath string) (*x509.CertPool, error) {
	cert, err := os.ReadFile(caPath)
	if err != nil {
		// If the CA does not exist, return nil which should default to the system pool
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if ok := pool.AppendCertsFromPEM(cert); !ok {
		return nil, fmt.Errorf("operator tls: failed to append CA certs from PEM: %q", caPath)
	}
	return pool, nil
}

func createAPIURL(https bool, hostname, port string) string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, hostname, port)
}
