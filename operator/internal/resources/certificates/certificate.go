/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package certificates

import (
	"net"
	"slices"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmapiv1ac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/certmanager/v1"
	cmmetav1ac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/meta/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultDuration    = 8760 * time.Hour
	defaultRenewBefore = 720 * time.Hour
	defaultIssuerKind  = "ClusterIssuer"
	issuerGroup        = "cert-manager.io"
)

// SpecConfig contains the common inputs used to build a cert-manager Certificate spec.
type SpecConfig struct {
	SecretName  string
	IssuerName  string
	IssuerKind  string
	Duration    *metav1.Duration
	RenewBefore *metav1.Duration
	Usages      []certmanagerv1.KeyUsage
}

// NewSpec builds the shared portion of a cert-manager Certificate spec.
func NewSpec(config *SpecConfig, dnsNames, ipAddresses []string) *cmapiv1ac.CertificateSpecApplyConfiguration {
	issuerKind := config.IssuerKind
	if issuerKind == "" {
		issuerKind = defaultIssuerKind
	}
	duration := defaultDuration
	if config.Duration != nil {
		duration = config.Duration.Duration
	}
	renewBefore := defaultRenewBefore
	if config.RenewBefore != nil {
		renewBefore = config.RenewBefore.Duration
	}

	issuerRef := cmmetav1ac.IssuerReference().
		WithName(config.IssuerName).
		WithKind(issuerKind).
		WithGroup(issuerGroup)

	return cmapiv1ac.CertificateSpec().
		WithSecretName(config.SecretName).
		WithDuration(metav1.Duration{Duration: duration}).
		WithRenewBefore(metav1.Duration{Duration: renewBefore}).
		WithUsages(config.Usages...).
		WithDNSNames(dnsNames...).
		WithIPAddresses(ipAddresses...).
		WithIssuerRef(issuerRef)
}

// AppendHosts classifies hosts as DNS names or IP addresses and appends them to the corresponding SAN list.
func AppendHosts(dnsNames, ipAddresses []string, hosts ...string) ([]string, []string) {
	for _, host := range hosts {
		if host == "" {
			continue
		}
		if net.ParseIP(host) != nil {
			ipAddresses = append(ipAddresses, host)
		} else {
			dnsNames = append(dnsNames, host)
		}
	}
	return dnsNames, ipAddresses
}

// LoadBalancerEndpoints returns every external IP and hostname reported by the Services.
func LoadBalancerEndpoints(services ...corev1.Service) []string {
	var endpoints []string
	for i := range services {
		for _, ingress := range services[i].Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				endpoints = append(endpoints, ingress.IP)
			}
			if ingress.Hostname != "" {
				endpoints = append(endpoints, ingress.Hostname)
			}
		}
	}
	return endpoints
}

// NormalizeSANs removes empty and duplicate SANs and returns them in stable order.
func NormalizeSANs(dnsNames, ipAddresses []string) ([]string, []string) {
	return normalize(dnsNames), normalize(ipAddresses)
}

func normalize(values []string) []string {
	values = slices.DeleteFunc(values, func(value string) bool { return value == "" })
	slices.Sort(values)
	return slices.Compact(values)
}
