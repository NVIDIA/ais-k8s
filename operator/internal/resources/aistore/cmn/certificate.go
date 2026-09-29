/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"fmt"
	"slices"
	"strings"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	certres "github.com/ais-operator/internal/resources/certificates"
	"github.com/ais-operator/internal/resources/ownerref"
	"github.com/ais-operator/internal/svcaddr"
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
		Usages:      certUsages(),
	}, dnsNames, ipAddresses)

	return cmapiv1ac.Certificate(certificateName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithSpec(spec)
}

func publicCertificateName(ais *aisv1.AIStore) string {
	return fmt.Sprintf("%s-tls-pub-cert", ais.Name)
}

// NewPublicCertificate builds the Certificate AIS serves on the public network.
func NewPublicCertificate(ais *aisv1.AIStore) *cmapiv1ac.CertificateApplyConfiguration {
	if !ais.UsePublicTLSCertificate() {
		return nil
	}
	certConfig := ais.GetPublicTLSCertificate()
	spec := certres.NewSpec(&certres.SpecConfig{
		SecretName:  ais.GetPublicTLSSecretName(),
		IssuerName:  certConfig.IssuerRef.Name,
		IssuerKind:  certConfig.IssuerRef.Kind,
		Duration:    certConfig.Duration,
		RenewBefore: certConfig.RenewBefore,
		Usages:      publicCertUsages(),
	}, buildPublicCertificateSANs(ais), nil)

	return cmapiv1ac.Certificate(publicCertificateName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithSpec(spec)
}

// buildPublicCertificateSANs returns the SANs of the public network certificate. The list holds
// only the names the spec gives.
func buildPublicCertificateSANs(ais *aisv1.AIStore) []string {
	certConfig := ais.GetPublicTLSCertificate()
	if certConfig == nil {
		return nil
	}
	dnsNames := append(slices.Clone(certConfig.AdditionalDNSNames), ais.GetPublicTLSServerName())
	dnsNames, _ = certres.NormalizeSANs(dnsNames, nil)
	return dnsNames
}

// certUsages returns the key usages AIS nodes need to both serve and dial TLS.
func certUsages() []certmanagerv1.KeyUsage {
	return []certmanagerv1.KeyUsage{
		certmanagerv1.UsageDigitalSignature,
		certmanagerv1.UsageKeyEncipherment,
		certmanagerv1.UsageServerAuth,
		certmanagerv1.UsageClientAuth,
	}
}

// publicCertUsages returns the key usages for a certificate AIS only serves.
func publicCertUsages() []certmanagerv1.KeyUsage {
	return []certmanagerv1.KeyUsage{
		certmanagerv1.UsageDigitalSignature,
		certmanagerv1.UsageKeyEncipherment,
		certmanagerv1.UsageServerAuth,
	}
}

func newCSIConfig(
	certConfig *aisv1.TLSCertificateConfig, commonName string, dnsNames []string, usages []certmanagerv1.KeyUsage,
) *certres.CSIConfig {
	return &certres.CSIConfig{
		IssuerName:  certConfig.IssuerRef.Name,
		IssuerKind:  certConfig.IssuerRef.Kind,
		CommonName:  commonName,
		DNSNames:    dnsNames,
		Duration:    certConfig.Duration,
		RenewBefore: certConfig.RenewBefore,
		Usages:      usages,
	}
}

func addServiceDNSNames(names []string, svc types.NamespacedName) []string {
	return append(names,
		// Used for registration of targets/proxies
		svc.Name,
		// Used for operator communication
		fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
		// Consistent URL for client pods
		svcaddr.ServiceFQDN(svc),
	)
}

func buildCertificateSANs(ais *aisv1.AIStore, publicHosts []string) (dnsNames, ipAddresses []string) {
	// Add DNS names for proxy service
	proxySvc := ais.ProxyHeadlessSVCNSName()
	dnsNames = addServiceDNSNames(dnsNames, proxySvc)

	// Add DNS names for target service
	targetSvc := ais.TargetHeadlessSVCNSName()
	dnsNames = addServiceDNSNames(dnsNames, targetSvc)

	// Add wildcard DNS names (for intra-cluster communication)
	dnsNames = append(dnsNames,
		svcaddr.WildcardServiceFQDN(proxySvc),
		svcaddr.WildcardServiceFQDN(targetSvc),
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

func PublicTLSCertificate(ais *aisv1.AIStore) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: publicCertificateName(ais), Namespace: ais.Namespace},
	}
}
