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

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/aistore/proxy"
	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	APIModePublic  = "public"
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
		mu         sync.RWMutex
		k8sClient  *aisclient.K8sClient
		tlsOpts    AISClientTLSOpts
		authClient *AuthClient
		clientMap  map[string]*AIStoreClient
	}
)

func NewAISClientManager(k8sClient *aisclient.K8sClient, tlsOpts AISClientTLSOpts) *AISClientManager {
	return &AISClientManager{
		k8sClient:  k8sClient,
		tlsOpts:    tlsOpts,
		authClient: NewAuthClient(k8sClient),
		clientMap:  make(map[string]*AIStoreClient, 16),
	}
}

// GetClient gets an AIStoreClientInterface for making requests to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
// If the token is expired, refreshes it in-place.
func (m *AISClientManager) GetClient(ctx context.Context,
	ais *aisv1.AIStore,
) (AIStoreClientInterface, error) {
	logger := logf.FromContext(ctx).WithValues("cluster", ais.NamespacedName().String())
	m.mu.RLock()
	client, exists := m.clientMap[ais.NamespacedName().String()]
	m.mu.RUnlock()

	if exists {
		if err := m.ensureValidToken(ctx, ais, client); err != nil {
			return nil, err
		}
	}

	url, err := m.getAISAPIEndpoint(ctx, ais)
	if err != nil {
		logger.Error(err, "Failed to get AIS API parameters")
		return nil, err
	}

	// Check if the client params are valid
	if exists && client.HasValidBaseParams(ctx, ais, url) {
		return client, nil
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
	client = NewAIStoreClient(ctx, url, tokenInfo, ais.GetAPIMode(), tlsConf)
	m.mu.Lock()
	m.clientMap[ais.NamespacedName().String()] = client
	m.mu.Unlock()
	return client, nil
}

func (m *AISClientManager) ensureValidToken(ctx context.Context, ais *aisv1.AIStore, client *AIStoreClient) error {
	if !client.isTokenExpired() {
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
	logger.Info("Refreshing expired token", "tokenExpires", hasExpiration)
	client.refreshToken(tokenInfo)
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

func (m *AISClientManager) getTLSConfig(ctx context.Context, ais *aisv1.AIStore) (*tls.Config, error) {
	if !ais.UseHTTPS() {
		return nil, nil
	}
	tlsDir := m.getTLSPath(ais)
	tlsConf := &tls.Config{}
	err := configureCAVerification(ctx, ais, tlsConf, tlsDir)
	if err != nil {
		return nil, err
	}
	addClientCertIfRequested(ais, tlsConf, tlsDir)
	return tlsConf, err
}

func configureCAVerification(ctx context.Context, ais *aisv1.AIStore, tlsConf *tls.Config, tlsDir string) error {
	logger := logf.FromContext(ctx)
	if ais.Spec.OperatorSkipVerifyCrt != nil && *ais.Spec.OperatorSkipVerifyCrt {
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
