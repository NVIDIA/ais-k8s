/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"fmt"
	"strings"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	certres "github.com/ais-operator/internal/resources/certificates"
	"github.com/ais-operator/internal/resources/ownerref"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmapiv1ac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func NewCertificate(ais *aisv1.AIStore, publicHosts []string) *cmapiv1ac.CertificateApplyConfiguration {
	certConfig := ais.GetTLSCertificate()
	if certConfig == nil {
		return nil
	}

	dnsNames, ipAddresses := buildCertificateSANs(ais, publicHosts)
	spec := certres.NewSpec(&certres.SpecConfig{
		SecretName:  CertificateSecretName(ais),
		IssuerName:  certConfig.IssuerRef.Name,
		IssuerKind:  certConfig.IssuerRef.Kind,
		Duration:    certConfig.Duration,
		RenewBefore: certConfig.RenewBefore,
		Usages: []certmanagerv1.KeyUsage{
			certmanagerv1.UsageDigitalSignature,
			certmanagerv1.UsageKeyEncipherment,
			certmanagerv1.UsageServerAuth,
			certmanagerv1.UsageClientAuth,
		},
	}, dnsNames, ipAddresses)

	return cmapiv1ac.Certificate(certificateName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithSpec(spec)
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

func buildCertificateSANs(ais *aisv1.AIStore, publicHosts []string) (dnsNames, ipAddresses []string) {
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
				dnsNames, ipAddresses = certres.AppendHosts(dnsNames, ipAddresses, host)
			}
		}
	}

	dnsNames, ipAddresses = certres.AppendHosts(dnsNames, ipAddresses, publicHosts...)

	// Add user-specified additional DNS names
	if certConfig := ais.GetTLSCertificate(); certConfig != nil {
		dnsNames = append(dnsNames, certConfig.AdditionalDNSNames...)
	}

	return certres.NormalizeSANs(dnsNames, ipAddresses)
}

func TLSCertificate(ais *aisv1.AIStore) *certmanagerv1.Certificate {
	nn := CertificateNSName(ais)
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
	}
}
