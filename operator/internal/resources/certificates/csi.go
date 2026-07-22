/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package certificates

import (
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	csiapisv1 "github.com/cert-manager/csi-driver/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CSIConfig contains the inputs used to request a certificate from the cert-manager CSI driver.
type CSIConfig struct {
	IssuerName  string
	IssuerKind  string
	CommonName  string
	DNSNames    []string
	Duration    *metav1.Duration
	RenewBefore *metav1.Duration
	Usages      []certmanagerv1.KeyUsage
}

// ToVolumeAttributes builds the volume attributes consumed by the cert-manager CSI driver.
func (config *CSIConfig) ToVolumeAttributes() map[string]string {
	attributes := map[string]string{
		csiapisv1.IssuerNameKey: config.IssuerName,
		csiapisv1.CommonNameKey: config.CommonName,
		csiapisv1.DNSNamesKey:   strings.Join(config.DNSNames, ","),
	}
	if config.IssuerKind != "" {
		attributes[csiapisv1.IssuerKindKey] = config.IssuerKind
	}
	if config.Duration != nil {
		attributes[csiapisv1.DurationKey] = config.Duration.Duration.String()
	}
	if config.RenewBefore != nil {
		attributes[csiapisv1.RenewBeforeKey] = config.RenewBefore.Duration.String()
	}
	if len(config.Usages) > 0 {
		usages := make([]string, len(config.Usages))
		for i := range config.Usages {
			usages[i] = string(config.Usages[i])
		}
		attributes[csiapisv1.KeyUsagesKey] = strings.Join(usages, ",")
	}
	return attributes
}
