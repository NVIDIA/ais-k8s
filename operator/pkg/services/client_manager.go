package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const userAgent = "ais-operator"

type (
	AISClientManagerInterface interface {
		GetClient(ctx context.Context, ais *aisv1.AIStore) (AIStoreClientInterface, error)
		GetPrimaryClient(ctx context.Context, ais *aisv1.AIStore) (client AIStoreClientInterface, isPrimary bool, err error)
	}

	AISClientManager struct {
		mu         sync.RWMutex
		isExternal bool
		k8sClient  *aisclient.K8sClient
		authN      authNConfig
		clientMap  map[string]AIStoreClientInterface
	}
)

func NewAISClientManager(k8sClient *aisclient.K8sClient, external bool) *AISClientManager {
	return &AISClientManager{
		isExternal: external,
		k8sClient:  k8sClient,
		authN:      newAuthNConfig(),
		clientMap:  make(map[string]AIStoreClientInterface, 16),
	}
}

func (m *AISClientManager) GetClient(ctx context.Context, ais *aisv1.AIStore) (AIStoreClientInterface, error) {
	return m.getClientForCluster(ctx, ais)
}

// GetPrimaryClient gets a client for the primary proxy if we are running as an internal client, otherwise it will back and return a client to ANY proxy
func (m *AISClientManager) GetPrimaryClient(ctx context.Context, ais *aisv1.AIStore) (client AIStoreClientInterface, isPrimary bool, err error) {
	// Get a general client, return it if we're external
	client, err = m.getClientForCluster(ctx, ais)
	if err != nil || m.isExternal {
		return
	}
	// If we are running as an internal client, return a client for the primary proxy
	smap, err := client.GetClusterMap()
	if err != nil || smap == nil {
		logf.FromContext(ctx).Error(err, "Failed to get cluster map")
		return
	}
	isPrimary = true
	client = NewAIStoreClient(buildBaseParams(smap.Primary.URL(cmn.NetPublic), client.GetAuthToken()))
	return
}

// getClientForCluster gets an AIStoreClientInterface for making request to the given AIS cluster.
// Gets a cached object if exists, else creates a new one.
func (m *AISClientManager) getClientForCluster(ctx context.Context,
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
	baseParams, err := m.newAISBaseParams(ctx, ais)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to get AIS API parameters")
		return
	}
	client = NewAIStoreClient(baseParams)
	m.mu.Lock()
	m.clientMap[ais.NamespacedName().String()] = client
	m.mu.Unlock()
	return
}

func (m *AISClientManager) newAISBaseParams(ctx context.Context,
	ais *aisv1.AIStore,
) (params *api.BaseParams, err error) {
	var (
		serviceHostname string
		token           string
	)
	// If LoadBalancer is configured and `isExternal` flag is set use the LB service to contact the API.
	if m.isExternal && ais.Spec.EnableExternalLB {
		var proxyLBSVC *corev1.Service
		proxyLBSVC, err = m.k8sClient.GetService(ctx, proxy.LoadBalancerSVCNSName(ais))
		if err != nil {
			return nil, err
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
	url := fmt.Sprintf("%s://%s:%s", scheme, serviceHostname, ais.Spec.ProxySpec.ServicePort.String())

	// Get admin token if AuthN is enabled
	token, err = m.getAdminToken(ctx, ais)
	if err != nil {
		return nil, err
	}

	return buildBaseParams(url, token), nil
}

// getAdminToken retrieves an admin token from AuthN service for the given AIS cluster.
func (m *AISClientManager) getAdminToken(ctx context.Context, ais *aisv1.AIStore) (string, error) {
	if ais.Spec.AuthNSecretName == nil {
		return "", nil
	}

	authNURL := fmt.Sprintf("%s://%s:%s", m.authN.protocol, m.authN.host, m.authN.port)
	authNBP := buildBaseParams(authNURL, "")
	zeroDuration := time.Duration(0)

	tokenMsg, err := authn.LoginUser(*authNBP, m.authN.adminUser, m.authN.adminPass, &zeroDuration)
	if err != nil {
		return "", fmt.Errorf("failed to login admin user to AuthN: %w", err)
	}

	logf.FromContext(ctx).Info("Successfully logged in as Admin to AuthN")
	return tokenMsg.Token, nil
}

func buildBaseParams(url, token string) *api.BaseParams {
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
		URL:   url,
		Token: token,
		UA:    userAgent,
	}
}
