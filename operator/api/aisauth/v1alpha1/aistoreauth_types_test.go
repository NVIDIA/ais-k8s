/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAIStoreAuthTLSHelpers(t *testing.T) {
	tests := []struct {
		name              string
		authn             *AIStoreAuth
		hasTLSEnabled     bool
		useTLSSecret      bool
		useTLSCertificate bool
		tlsSecretName     string
	}{
		{
			name: "no tls",
			authn: &AIStoreAuth{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-authn"},
			},
		},
		{
			name: "existing secret",
			authn: &AIStoreAuth{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-authn"},
				Spec: AIStoreAuthSpec{
					TLS: &TLSSpec{
						SecretName: ptr("custom-tls"),
					},
				},
			},
			hasTLSEnabled: true,
			useTLSSecret:  true,
			tlsSecretName: "custom-tls",
		},
		{
			name: "empty secret name",
			authn: &AIStoreAuth{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-authn"},
				Spec: AIStoreAuthSpec{
					TLS: &TLSSpec{
						SecretName: ptr(""),
					},
				},
			},
		},
		{
			name: "cert-manager certificate",
			authn: &AIStoreAuth{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-authn"},
				Spec: AIStoreAuthSpec{
					TLS: &TLSSpec{
						Certificate: &TLSCertificateConfig{
							IssuerRef: CertIssuerRef{Name: testIssuerName()},
						},
					},
				},
			},
			hasTLSEnabled:     true,
			useTLSCertificate: true,
			tlsSecretName:     "ais-authn" + "-authn-tls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.authn.HasTLSEnabled()).To(Equal(tt.hasTLSEnabled))
			g.Expect(tt.authn.UseTLSSecret()).To(Equal(tt.useTLSSecret))
			g.Expect(tt.authn.UseTLSCertificate()).To(Equal(tt.useTLSCertificate))
			g.Expect(tt.authn.GetTLSSecretName()).To(Equal(tt.tlsSecretName))
		})
	}
}

func TestAIStoreAuthListenPort(t *testing.T) {
	tests := []struct {
		name string
		spec AIStoreAuthSpec
		port int32
	}{
		{
			name: "default",
			port: 52001,
		},
		{
			name: "no http config",
			spec: AIStoreAuthSpec{
				Config: &ConfigSpec{Net: &NetSpec{}},
			},
			port: 52001,
		},
		{
			name: "configured",
			spec: AIStoreAuthSpec{
				Config: &ConfigSpec{
					Net: &NetSpec{
						HTTP: &HTTPConfSpec{Port: ptr(int32(53001))},
					},
				},
			},
			port: 53001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			authn := &AIStoreAuth{Spec: tt.spec}
			g.Expect(authn.ListenPort()).To(Equal(tt.port))
		})
	}
}

func TestPersistenceSpecMode(t *testing.T) {
	tests := []struct {
		name              string
		persistence       PersistenceSpec
		usesStorageClass  bool
		usesExistingVol   bool
		storageSizeString string
	}{
		{
			name:              "storage class",
			persistence:       PersistenceSpec{StorageClass: ptr("openebs-hostpath")},
			usesStorageClass:  true,
			storageSizeString: "256Mi",
		},
		{
			name:              "existing volume",
			persistence:       PersistenceSpec{VolumeName: ptr("existing-pv")},
			usesExistingVol:   true,
			storageSizeString: "256Mi",
		},
		{
			name:              "explicit size",
			persistence:       PersistenceSpec{StorageClass: ptr("fast"), Size: quantityPtr("1Gi")},
			usesStorageClass:  true,
			storageSizeString: "1Gi",
		},
		{
			name:              "empty storage class is not dynamic mode",
			persistence:       PersistenceSpec{StorageClass: ptr("")},
			storageSizeString: "256Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := tt.persistence
			g.Expect(p.UsesStorageClass()).To(Equal(tt.usesStorageClass))
			g.Expect(p.UsesExistingVolume()).To(Equal(tt.usesExistingVol))
			size := p.StorageSize()
			g.Expect(size.String()).To(Equal(tt.storageSizeString))
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}

func quantityPtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

// testIssuerName avoids gosec G101 false positives by avoiding hardcoding a
// test issuer name directly inside the TLS certificate test case.
func testIssuerName() string {
	return "test-issuer"
}
