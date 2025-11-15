package truststore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// TestNewCertPool tests creating certificate pools with various configurations
func TestNewCertPool(t *testing.T) {
	tests := []struct {
		name        string
		setupCerts  func(t *testing.T) []string // Returns paths to created cert files
		config      Config
		expectError bool
		validate    func(t *testing.T, pool *x509.CertPool)
	}{
		{
			name: "load single CA certificate",
			setupCerts: func(t *testing.T) []string {
				certPath := createTestCACertFile(t, "test-ca-1")
				return []string{certPath}
			},
			config:      Config{},
			expectError: false,
			validate: func(t *testing.T, pool *x509.CertPool) {
				if pool == nil {
					t.Error("expected pool to be non-nil")
				}
			},
		},
		{
			name: "load multiple CA certificates",
			setupCerts: func(t *testing.T) []string {
				certPath1 := createTestCACertFile(t, "test-ca-1")
				certPath2 := createTestCACertFile(t, "test-ca-2")
				return []string{certPath1, certPath2}
			},
			config:      Config{},
			expectError: false,
			validate: func(t *testing.T, pool *x509.CertPool) {
				if pool == nil {
					t.Error("expected pool to be non-nil")
				}
			},
		},
		{
			name:       "no CA certificates configured",
			setupCerts: func(_ *testing.T) []string { return nil },
			config: Config{
				CACertPaths: []string{},
			},
			expectError: false,
			validate: func(t *testing.T, pool *x509.CertPool) {
				// Should still have system CAs
				if pool == nil {
					t.Error("expected pool to be non-nil")
				}
			},
		},
		{
			name:       "non-existent certificate file - logs warning but continues",
			setupCerts: func(_ *testing.T) []string { return []string{"/nonexistent/ca.crt"} },
			config: Config{
				CACertPaths: []string{"/nonexistent/ca.crt"},
			},
			expectError: false,
			validate: func(t *testing.T, pool *x509.CertPool) {
				if pool == nil {
					t.Error("expected pool to be non-nil")
				}
			},
		},
		{
			name: "invalid PEM format - logs warning but continues",
			setupCerts: func(t *testing.T) []string {
				tmpDir := t.TempDir()
				invalidCertPath := filepath.Join(tmpDir, "invalid.crt")
				err := os.WriteFile(invalidCertPath, []byte("not a valid PEM file"), 0o600)
				if err != nil {
					t.Fatalf("failed to write invalid cert file: %v", err)
				}
				return []string{invalidCertPath}
			},
			config:      Config{},
			expectError: false,
			validate: func(t *testing.T, pool *x509.CertPool) {
				if pool == nil {
					t.Error("expected pool to be non-nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPaths := tt.setupCerts(t)
			if certPaths != nil {
				tt.config.CACertPaths = certPaths
			}

			pool, err := NewCertPool(logr.Discard(), tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, pool)
				}
			}
		})
	}
}

// TestNewTLSConfig tests creating TLS configurations
func TestNewTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		setupCerts  func(t *testing.T) []string
		config      Config
		expectError bool
		validate    func(t *testing.T, tlsConfig *tls.Config)
	}{
		{
			name: "default TLS config with custom CA",
			setupCerts: func(t *testing.T) []string {
				certPath := createTestCACertFile(t, "test-ca")
				return []string{certPath}
			},
			config:      Config{},
			expectError: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				if tlsConfig == nil {
					t.Error("expected tlsConfig to be non-nil")
					return // fail before npe
				}
				if tlsConfig.MinVersion != tls.VersionTLS12 {
					t.Errorf("expected MinVersion to be TLS 1.2 (%d), got %d", tls.VersionTLS12, tlsConfig.MinVersion)
				}
				if tlsConfig.InsecureSkipVerify {
					t.Error("expected InsecureSkipVerify to be false")
				}
				if tlsConfig.RootCAs == nil {
					t.Error("expected RootCAs to be non-nil")
				}
			},
		},
		{
			name:       "no custom CAs",
			setupCerts: func(_ *testing.T) []string { return nil },
			config: Config{
				CACertPaths: []string{},
			},
			expectError: false,
			validate: func(t *testing.T, tlsConfig *tls.Config) {
				if tlsConfig == nil {
					t.Error("expected tlsConfig to be non-nil")
					return // no npe
				}
				if tlsConfig.RootCAs == nil {
					t.Error("expected RootCAs to be non-nil") // Should still have system CAs
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPaths := tt.setupCerts(t)
			if certPaths != nil {
				tt.config.CACertPaths = certPaths
			}

			tlsConfig, err := NewTLSConfig(logr.Discard(), tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, tlsConfig)
				}
			}
		})
	}
}

// Helper function to create a test CA certificate file
func createTestCACertFile(t *testing.T, commonName string) string {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, commonName+".crt")

	certPEM := createTestCACertPEM(t, commonName)
	err := os.WriteFile(certPath, certPEM, 0o600)
	if err != nil {
		t.Fatalf("failed to write test CA cert file: %v", err)
	}

	t.Logf("Created test CA cert: %s", certPath)
	return certPath
}

// Helper function to create a test CA certificate in PEM format
func createTestCACertPEM(t *testing.T, commonName string) []byte {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM
}
