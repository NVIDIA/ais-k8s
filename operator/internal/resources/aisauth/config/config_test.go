/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package config_test

import (
	"testing"
	"time"

	aisauthn "github.com/NVIDIA/aistore/api/authn"
	"github.com/NVIDIA/aistore/cmn/cos"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnconfig "github.com/ais-operator/internal/resources/aisauth/config"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateConfig(t *testing.T) {
	g := NewWithT(t)

	authn := newTestAIStoreAuth()
	cfg, err := generateConfig(authn)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.Net.HTTP.Port).To(Equal(52001))
	g.Expect(cfg.Server.DBConf.Filepath).To(Equal("/etc/ais/authn/authn.db"))
}

func TestGenerateConfigUsesProvidedPaths(t *testing.T) {
	g := NewWithT(t)
	authn := newTestAIStoreAuth()
	authn.Spec.TLS = &authv1alpha1.TLSSpec{
		Certificate: &authv1alpha1.TLSCertificateConfig{
			IssuerRef: authv1alpha1.CertIssuerRef{Name: testIssuerName()},
		},
	}
	paths := authnconfig.Paths{
		Database:       "/custom/state/authn.db",
		TLSCertificate: "/custom/tls/server.crt",
		TLSKey:         "/custom/tls/server.key",
	}

	cfg, err := authnconfig.GenerateConfig(authn, paths)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.Server.DBConf.Filepath).To(Equal(paths.Database))
	g.Expect(cfg.Net.HTTP.Certificate).To(Equal(paths.TLSCertificate))
	g.Expect(cfg.Net.HTTP.Key).To(Equal(paths.TLSKey))
}

func TestGenerateConfigSigningKey(t *testing.T) {
	g := NewWithT(t)
	bits := int32(4096)
	mode := aisauthn.SigningKeyModeExternal
	authn := newTestAIStoreAuth()
	authn.Spec.Config = &authv1alpha1.ConfigSpec{
		Auth: &authv1alpha1.ServerConfSpec{
			SigningKey: &authv1alpha1.SigningKeySpec{
				Bits: &bits,
				Mode: &mode,
			},
		},
	}

	cfg, err := generateConfig(authn)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.Server.SigningKey.Bits).To(Equal(4096))
	g.Expect(cfg.Server.SigningKey.Mode).To(Equal(aisauthn.SigningKeyModeExternal))
}

func TestGenerateConfigIndividualAuthFields(t *testing.T) {
	t.Run("renders log config", func(t *testing.T) {
		g := NewWithT(t)
		level := int32(5)
		flushInterval := metav1.Duration{Duration: 11 * time.Second}
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Log: &authv1alpha1.LogSpec{
				Level:         &level,
				FlushInterval: &flushInterval,
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Log.Level).To(Equal("5"))
		g.Expect(cfg.Log.FlushInterval).To(Equal(cos.Duration(11 * time.Second)))
	})

	t.Run("renders net config", func(t *testing.T) {
		g := NewWithT(t)
		port := int32(53001)
		externalURL := "https://authn.example.test:53001"
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{
				ExternalURL: &externalURL,
				HTTP:        &authv1alpha1.HTTPConfSpec{Port: &port},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Net.ExternalURL).To(Equal(externalURL))
		g.Expect(cfg.Net.HTTP.Port).To(Equal(53001))
	})

	t.Run("renders tls paths", func(t *testing.T) {
		g := NewWithT(t)
		authn := newTestAIStoreAuth()
		authn.Spec.TLS = &authv1alpha1.TLSSpec{
			Certificate: &authv1alpha1.TLSCertificateConfig{
				IssuerRef: authv1alpha1.CertIssuerRef{Name: testIssuerName()},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Net.HTTP.UseHTTPS).To(BeTrue())
		g.Expect(cfg.Net.HTTP.Certificate).To(Equal("/var/certs/tls.crt"))
		g.Expect(cfg.Net.HTTP.Key).To(Equal("/var/certs/tls.key"))
	})

	t.Run("renders expiration time", func(t *testing.T) {
		g := NewWithT(t)
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Auth: &authv1alpha1.ServerConfSpec{
				ExpirationTime: &metav1.Duration{Duration: 12 * time.Hour},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Server.Expire).To(Equal(cos.Duration(12 * time.Hour)))
	})

	t.Run("renders max token age", func(t *testing.T) {
		g := NewWithT(t)
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Auth: &authv1alpha1.ServerConfSpec{
				MaxTokenAge: &metav1.Duration{Duration: 72 * time.Hour},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Server.MaxTokenAge).To(Equal(cos.Duration(72 * time.Hour)))
	})

	t.Run("renders timeout", func(t *testing.T) {
		g := NewWithT(t)
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Timeout: &authv1alpha1.TimeoutSpec{
				DefaultTimeout: &metav1.Duration{Duration: 45 * time.Second},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Timeout.Default).To(Equal(cos.Duration(45 * time.Second)))
	})

	t.Run("includes db type", func(t *testing.T) {
		g := NewWithT(t)
		dbType := "BuntDB"
		authn := newTestAIStoreAuth()
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Auth: &authv1alpha1.ServerConfSpec{
				DB: &authv1alpha1.DBSpec{Type: &dbType},
			},
		}

		cfg, err := generateConfig(authn)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.Server.DBConf.DBType).To(Equal("BuntDB"))
		g.Expect(cfg.Server.DBConf.Filepath).To(Equal("/etc/ais/authn/authn.db"))
	})
}

func generateConfig(authn *authv1alpha1.AIStoreAuth) (*aisauthn.Config, error) {
	return authnconfig.GenerateConfig(authn, authnconfig.Paths{
		Database:       "/etc/ais/authn/authn.db",
		TLSCertificate: "/var/certs/tls.crt",
		TLSKey:         "/var/certs/tls.key",
	})
}

func newTestAIStoreAuth() *authv1alpha1.AIStoreAuth {
	return &authv1alpha1.AIStoreAuth{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ais-authn",
			Namespace: "ais",
		},
		Spec: authv1alpha1.AIStoreAuthSpec{
			Deployment: authv1alpha1.DeploymentSpec{
				Container: authv1alpha1.ContainerSpec{
					Image: "docker.io/aistorage/authn:v4.5",
				},
			},
		},
	}
}

func testIssuerName() string {
	return "test-issuer"
}
