/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
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
			tlsSecretName:     "ais-authn" + "-tls",
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

func ptr[T any](v T) *T {
	return &v
}

// testIssuerName avoids gosec G101 false positives by avoiding hardcoding a
// test issuer name directly inside the TLS certificate test case.
func testIssuerName() string {
	return "test-issuer"
}
