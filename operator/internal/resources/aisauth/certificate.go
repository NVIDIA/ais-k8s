/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"
	"net/url"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/opinfo"
	certres "github.com/ais-operator/internal/resources/certificates"
	"github.com/ais-operator/internal/resources/ownerref"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmapiv1ac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

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

	dnsNames := certificateDNSNames(ctx, authn, config, externalEndpoints)
	spec := certres.NewSpec(&certres.SpecConfig{
		SecretName:  authn.GetTLSSecretName(),
		IssuerName:  config.IssuerRef.Name,
		IssuerKind:  config.IssuerRef.Kind,
		Duration:    config.Duration,
		RenewBefore: config.RenewBefore,
		Usages:      serverCertUsages(),
	}, dnsNames, nil)

	return cmapiv1ac.Certificate(CertificateName(authn), authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn)).
		WithSpec(spec)
}

func tlsCSIVolumeAttributes(ctx context.Context, authn *authv1alpha1.AIStoreAuth) map[string]string {
	if !authn.UseTLSCSI() {
		return nil
	}
	config := authn.GetTLSCertificate()
	// LoadBalancer addresses are omitted because changing Service status in the
	// pod template would roll the Deployment.
	dnsNames := certificateDNSNames(ctx, authn, config, nil)
	return (&certres.CSIConfig{
		IssuerName:  config.IssuerRef.Name,
		IssuerKind:  config.IssuerRef.Kind,
		CommonName:  fmt.Sprintf("%s.%s", ServiceName(authn), authn.Namespace),
		DNSNames:    dnsNames,
		Duration:    config.Duration,
		RenewBefore: config.RenewBefore,
		Usages:      serverCertUsages(),
	}).ToVolumeAttributes()
}

func serverCertUsages() []certmanagerv1.KeyUsage {
	return []certmanagerv1.KeyUsage{
		certmanagerv1.UsageDigitalSignature,
		certmanagerv1.UsageKeyEncipherment,
		certmanagerv1.UsageServerAuth,
	}
}

// certificateDNSNames derives the DNS names clients use to reach AuthN.
func certificateDNSNames(
	ctx context.Context,
	authn *authv1alpha1.AIStoreAuth,
	config *authv1alpha1.TLSCertificateConfig,
	externalEndpoints []string,
) (dnsNames []string) {
	// Reserve localhost, four Service DNS names, and one possible external URL hostname.
	dnsNames = make([]string, 0, 6+len(externalEndpoints)+len(config.AdditionalDNSNames))
	dnsNames = append(dnsNames, "localhost")
	dnsNames = appendServiceDNSNames(dnsNames, ServiceName(authn), authn.Namespace)
	dnsNames, _ = certres.AppendHosts(dnsNames, nil, externalEndpoints...)
	if externalURLHost := configuredExternalURLHost(ctx, authn); externalURLHost != "" {
		dnsNames, _ = certres.AppendHosts(dnsNames, nil, externalURLHost)
	}

	dnsNames = append(dnsNames, config.AdditionalDNSNames...)
	dnsNames, _ = certres.NormalizeSANs(dnsNames, nil)
	return dnsNames
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
		fmt.Sprintf("%s.%s.svc.%s", serviceName, namespace, opinfo.ClusterDomain()),
	)
}
