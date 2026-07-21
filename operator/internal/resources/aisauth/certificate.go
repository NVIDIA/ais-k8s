/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"
	"net/url"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	certres "github.com/ais-operator/internal/resources/certificates"
	"github.com/ais-operator/internal/resources/ownerref"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmapiv1ac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const clusterDomain = "cluster.local"

// CertificateName returns the name of the cert-manager Certificate managed for AuthN.
func CertificateName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name + "-authn-tls-cert"
}

// CertificateNSName returns the namespaced name of the managed Certificate.
func CertificateNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: CertificateName(authn), Namespace: authn.Namespace}
}

// TLSCertificate returns the typed Certificate used for lookups and deletion.
func TLSCertificate(authn *authv1alpha1.AIStoreAuth) *certmanagerv1.Certificate {
	name := CertificateNSName(authn)
	return &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace}}
}

// NewCertificate builds the cert-manager Certificate used by the AuthN server.
func NewCertificate(ctx context.Context, authn *authv1alpha1.AIStoreAuth, externalEndpoints []string) *cmapiv1ac.CertificateApplyConfiguration {
	config := authn.GetTLSCertificate()
	if config == nil {
		return nil
	}

	dnsNames, ipAddresses := certificateSANs(ctx, authn, config, externalEndpoints)
	spec := certres.NewSpec(&certres.SpecConfig{
		SecretName:  authn.GetTLSSecretName(),
		IssuerName:  config.IssuerRef.Name,
		IssuerKind:  config.IssuerRef.Kind,
		Duration:    config.Duration,
		RenewBefore: config.RenewBefore,
		Usages: []certmanagerv1.KeyUsage{
			certmanagerv1.UsageDigitalSignature,
			certmanagerv1.UsageKeyEncipherment,
			certmanagerv1.UsageServerAuth,
		},
	}, dnsNames, ipAddresses)

	return cmapiv1ac.Certificate(CertificateName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithSpec(spec)
}

// certificateSANs derives the addresses clients use to reach AuthN.
func certificateSANs(
	ctx context.Context,
	authn *authv1alpha1.AIStoreAuth,
	config *authv1alpha1.TLSCertificateConfig,
	externalEndpoints []string,
) (dnsNames, ipAddresses []string) {
	// Reserve localhost, four Service DNS names, and one possible external URL hostname.
	dnsNames = make([]string, 0, 6+len(externalEndpoints)+len(config.AdditionalDNSNames))
	// Reserve the loopback address and one possible external URL IP address.
	ipAddresses = make([]string, 0, 2+len(externalEndpoints)+len(config.AdditionalIPAddresses))
	dnsNames = append(dnsNames, "localhost")
	ipAddresses = append(ipAddresses, "127.0.0.1")
	dnsNames = appendServiceDNSNames(dnsNames, ServiceName(authn), authn.Namespace)
	dnsNames, ipAddresses = certres.AppendHosts(dnsNames, ipAddresses, externalEndpoints...)
	if externalURLHost := configuredExternalURLHost(ctx, authn); externalURLHost != "" {
		dnsNames, ipAddresses = certres.AppendHosts(dnsNames, ipAddresses, externalURLHost)
	}

	dnsNames = append(dnsNames, config.AdditionalDNSNames...)
	ipAddresses = append(ipAddresses, config.AdditionalIPAddresses...)
	return certres.NormalizeSANs(dnsNames, ipAddresses)
}

func configuredExternalURLHost(ctx context.Context, authn *authv1alpha1.AIStoreAuth) string {
	if authn.Spec.Config == nil || authn.Spec.Config.Net == nil || authn.Spec.Config.Net.ExternalURL == nil {
		return ""
	}
	externalURL, err := url.Parse(*authn.Spec.Config.Net.ExternalURL)
	if err != nil {
		logf.FromContext(ctx).V(1).Info("Failed to parse external URL, excluding it from certificate SANs", "error", err)
		return ""
	}
	return externalURL.Hostname()
}

func appendServiceDNSNames(names []string, serviceName, namespace string) []string {
	return append(names,
		serviceName,
		fmt.Sprintf("%s.%s", serviceName, namespace),
		fmt.Sprintf("%s.%s.svc", serviceName, namespace),
		fmt.Sprintf("%s.%s.svc.%s", serviceName, namespace, clusterDomain),
	)
}
