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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	APIModePublic  = "public"
	EnvSkipVerify  = "OPERATOR_SKIP_VERIFY_CRT"
	ClientCertFile = "tls.crt"
	ClientKeyFile  = "tls.key"
	ClientCAFile   = "ca.crt"
	CAMountPath    = "/etc/ais/ca"
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
		authN:     NewAuthNClient(k8sClient),
		clientMap: make(map[string]AIStoreClientInterface, 16),
	}
}

// GetClient gets an AIStoreClientInterface for making request to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
// If the token is expired, refreshes it in-place.
func (m *AISClientManager) GetClient(ctx context.Context,
	ais *aisv1.AIStore,
) (client AIStoreClientInterface, err error) {
	logger := logf.FromContext(ctx)
	m.mu.RLock()
	client, exists := m.clientMap[ais.NamespacedName().String()]
	m.mu.RUnlock()

	// If client exists and token is expired, refresh it
	if exists {
		if concreteClient, ok := client.(*AIStoreClient); ok && concreteClient.isTokenExpired() {
			tokenInfo, err := m.authN.getAdminToken(ctx, ais)
			if err != nil {
				logger.Error(err, "Failed to get admin token for refresh")
				return nil, err
			}

			hasExpiration := tokenInfo != nil && !tokenInfo.ExpiresAt.IsZero()
			logger.Info("Refreshing expired token", "tokenExpires", hasExpiration)
			concreteClient.refreshToken(tokenInfo)
		}
	}

	// Check if the client params are valid
	if exists && client.HasValidBaseParams(ctx, ais) {
		return
	}

	url, err := m.getAISAPIEndpoint(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get AIS API parameters")
		return
	}

	// Attempt to get an authN token from the secret mapped for this cluster in the configmap
	tokenInfo, err := m.authN.getAdminToken(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get admin token for AuthN")
		return nil, err
	}

	tlsConf, err := m.getTLSConfig(ctx, ais)
	if err != nil {
		return nil, err
	}

	hasToken := tokenInfo != nil && tokenInfo.Token != ""
	hasExpiration := tokenInfo != nil && !tokenInfo.ExpiresAt.IsZero()
	if tlsConf == nil {
		logger.Info("Creating AIS API client", "url", url, "authN", hasToken, "tokenExpires", hasExpiration)
	} else {
		logger.Info("Creating HTTPS AIS API client", "url", url, "authN", hasToken, "tokenExpires", hasExpiration, "tlsPath", m.getTLSPath(ais), "skipVerify", tlsConf.InsecureSkipVerify)
	}
	client = NewAIStoreClient(ctx, url, tokenInfo, ais.GetAPIMode(), tlsConf)
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
	pods, err := m.k8sClient.ListReadyPods(ctx, ais, proxy.BasicLabels(ais))
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no ready pods found matching selector %q", proxy.BasicLabels(ais))
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
	logger := logf.FromContext(ctx)
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
	caPool, err := loadOptionalProvidedCA(logger, providedCA)
	if err != nil {
		logger.Error(err, "Failed to load AIS CA", "location", providedCA)
		return err
	}
	tlsConf.RootCAs = caPool
	return nil
}

func addClientCertIfRequested(ais *aisv1.AIStore, tlsConf *tls.Config, tlsDir string) {
	if !ais.ShouldIncludeClientCert() {
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

func loadOptionalProvidedCA(logger logr.Logger, caPath string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		logger.Error(err, "Failed to load system cert pool")
		return nil, err
	}
	err = appendCertIfExists(logger, pool, caPath)
	if err != nil {
		logger.Error(err, "Failed to append CA cert to pool", "path", caPath)
		return nil, err
	}
	// Load any additional certs provided from configMap
	_, err = os.Stat(CAMountPath)
	if os.IsNotExist(err) {
		logger.Info("No path found with additional CA certs", "path", CAMountPath)
		return pool, nil
	} else if err != nil {
		logger.Error(err, "Failed to stat CA mount path", "path", CAMountPath)
		return nil, err
	}
	certPaths, err := findCerts(CAMountPath, []string{".crt", ".pem"})
	if err != nil {
		// Non-fatal error if we cannot load additional trust, log and continue
		logger.Error(err, "Failed to search cert paths", "caRoot", CAMountPath)
		return pool, nil
	}
	for _, path := range certPaths {
		if appendErr := appendCertIfExists(logger, pool, path); appendErr != nil {
			logger.Error(err, "Failed to add new trusted CA", "path", path)
		}
	}
	return pool, err
}

func appendCertIfExists(logger logr.Logger, pool *x509.CertPool, path string) error {
	cert, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	logger.Info("Adding trusted CA certificate", "path", path)
	if ok := pool.AppendCertsFromPEM(cert); !ok {
		return fmt.Errorf("failed to append existing CA certs from PEM: %q", path)
	}
	return nil
}

func findCerts(root string, exts []string) ([]string, error) {
	var certPaths []string
	err := filepath.WalkDir(root, func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		// Filter out entries starting with two dots -- K8s-managed hidden directories
		if strings.HasPrefix(d.Name(), "..") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		for _, ext := range exts {
			if filepath.Ext(d.Name()) == ext {
				certPaths = append(certPaths, s)
				break
			}
		}
		return nil
	})
	return certPaths, err
}

func createAPIURL(https bool, hostname, port string) string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, hostname, port)
}
