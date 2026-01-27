// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	aisv1 "github.com/ais-operator/api/v1beta1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultCertDuration    = 8760 * time.Hour // 1 year
	defaultCertRenewBefore = 720 * time.Hour  // 30 days
)

func certificateName(ais *aisv1.AIStore) string {
	return fmt.Sprintf("%s-tls-cert", ais.Name)
}

// CertificateSecretName returns the name of the TLS secret created by the Certificate
func CertificateSecretName(ais *aisv1.AIStore) string {
	return fmt.Sprintf("%s-tls", ais.Name)
}

// CertificateNSName returns the namespaced name of the Certificate resource
func CertificateNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      certificateName(ais),
		Namespace: ais.Namespace,
	}
}

func NewCertificate(ais *aisv1.AIStore) *certmanagerv1.Certificate {
	certConfig := ais.GetTLSCertificate()
	if certConfig == nil {
		return nil
	}

	issuerKind := certConfig.IssuerRef.Kind
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	duration := defaultCertDuration
	if certConfig.Duration != nil {
		duration = certConfig.Duration.Duration
	}
	renewBefore := defaultCertRenewBefore
	if certConfig.RenewBefore != nil {
		renewBefore = certConfig.RenewBefore.Duration
	}

	// Build DNS names and IP addresses
	dnsNames, ipAddresses := buildCertificateSANs(ais)

	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName(ais),
			Namespace: ais.Namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			SecretName:  CertificateSecretName(ais),
			Duration:    &metav1.Duration{Duration: duration},
			RenewBefore: &metav1.Duration{Duration: renewBefore},
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageServerAuth,
				certmanagerv1.UsageClientAuth,
			},
			DNSNames:    dnsNames,
			IPAddresses: ipAddresses,
			IssuerRef: cmmeta.ObjectReference{
				Name:  certConfig.IssuerRef.Name,
				Kind:  issuerKind,
				Group: "cert-manager.io",
			},
		},
	}
}

func addServiceDNSNames(names []string, svcName, namespace, clusterDomain string) []string {
	return append(names,
		// Used for registration of targets/proxies
		svcName,
		// Used for operator communication
		fmt.Sprintf("%s.%s", svcName, namespace),
		// Consistent URL for client pods
		fmt.Sprintf("%s.%s.svc.%s", svcName, namespace, clusterDomain),
	)
}

func buildCertificateSANs(ais *aisv1.AIStore) (dnsNames, ipAddresses []string) {
	clusterDomain := ais.GetClusterDomain()

	// Add DNS names for proxy service
	dnsNames = addServiceDNSNames(dnsNames, fmt.Sprintf("%s-proxy", ais.Name), ais.Namespace, clusterDomain)

	// Add DNS names for target service
	dnsNames = addServiceDNSNames(dnsNames, fmt.Sprintf("%s-target", ais.Name), ais.Namespace, clusterDomain)

	// Add wildcard DNS names (for intra-cluster communication)
	dnsNames = append(dnsNames,
		fmt.Sprintf("*.%s-proxy.%s.svc.%s", ais.Name, ais.Namespace, clusterDomain),
		fmt.Sprintf("*.%s-target.%s.svc.%s", ais.Name, ais.Namespace, clusterDomain),
	)

	// Add node names/IPs for direct communication (multi-homing support)
	if ais.Spec.HostnameMap != nil {
		for _, allHosts := range ais.Spec.HostnameMap {
			for _, host := range strings.Split(allHosts, ",") {
				host = strings.TrimSpace(host)
				if host != "" {
					addHostOrIP(host, &dnsNames, &ipAddresses)
				}
			}
		}
	}

	// Add auto-discovered node names
	for _, nodeName := range ais.Status.AutoScaleStatus.ExpectedTargetNodes {
		addHostOrIP(nodeName, &dnsNames, &ipAddresses)
	}
	for _, nodeName := range ais.Status.AutoScaleStatus.ExpectedProxyNodes {
		addHostOrIP(nodeName, &dnsNames, &ipAddresses)
	}

	// Add user-specified additional DNS names
	if certConfig := ais.GetTLSCertificate(); certConfig != nil {
		dnsNames = append(dnsNames, certConfig.AdditionalDNSNames...)
	}

	return dnsNames, ipAddresses
}

func addHostOrIP(host string, dnsNames, ipAddresses *[]string) {
	if net.ParseIP(host) != nil {
		*ipAddresses = append(*ipAddresses, host)
	} else {
		*dnsNames = append(*dnsNames, host)
	}
}

func DeleteCertificateIfExists(ctx context.Context, k8sClient interface {
	DeleteResourceIfExists(context.Context, client.Object) (bool, error)
}, ais *aisv1.AIStore) (bool, error) {
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName(ais),
			Namespace: ais.Namespace,
		},
	}

	return k8sClient.DeleteResourceIfExists(ctx, cert)
}
