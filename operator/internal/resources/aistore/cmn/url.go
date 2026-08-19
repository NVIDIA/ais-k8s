/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"fmt"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/opinfo"
)

// ClusterDomain returns the DNS domain the cluster's services are addressed under.
func ClusterDomain(ais *aisv1.AIStore) string {
	if ais.Spec.ClusterDomain == nil {
		return opinfo.ClusterDomain()
	}
	return *ais.Spec.ClusterDomain
}

// DefaultProxyURL returns the URL of the proxy that starts out as primary.
func DefaultProxyURL(ais *aisv1.AIStore) string {
	// Example: https://ais-proxy-0.ais-proxy.ais.svc.cluster.local:51082
	return fmt.Sprintf("%s://%s.%s.%s.%s", urlScheme(ais), ais.DefaultPrimaryName(),
		ais.ProxyStatefulSetName(), ais.Namespace, controlSvcSuffix(ais))
}

// IntraClusterURL returns the URL of the cluster-internal proxy service on the public network.
func IntraClusterURL(ais *aisv1.AIStore) string {
	// Example: https://ais-proxy.ais.svc.cluster.local:51080
	return fmt.Sprintf("%s://%s.%s.%s", urlScheme(ais),
		ais.ProxyStatefulSetName(), ais.Namespace, publicSvcSuffix(ais))
}

// DiscoveryProxyURL returns the URL of the proxy service on the intra-control network.
func DiscoveryProxyURL(ais *aisv1.AIStore) string {
	// Example: https://ais-proxy.ais.svc.cluster.local:51082
	return fmt.Sprintf("%s://%s.%s.%s", urlScheme(ais),
		ais.ProxyStatefulSetName(), ais.Namespace, controlSvcSuffix(ais))
}

func urlScheme(ais *aisv1.AIStore) string {
	if ais.UseHTTPS() {
		return "https"
	}
	return "http"
}

func controlSvcSuffix(ais *aisv1.AIStore) string {
	return fmt.Sprintf("svc.%s:%s", ClusterDomain(ais), ais.Spec.ProxySpec.IntraControlPort.String())
}

func publicSvcSuffix(ais *aisv1.AIStore) string {
	return fmt.Sprintf("svc.%s:%s", ClusterDomain(ais), ais.Spec.ProxySpec.PublicPort.String())
}
