// Package truststore provides certificate trust store management for secure TLS connections
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package truststore

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/go-logr/logr"
)

// Config configures certificate trust store loading for secure connections
type Config struct {
	// CACertPaths is a list of filesystem paths to CA certificate files (PEM format)
	// These certificates will be added to the trust store for verifying connections
	// Example: ["/etc/ssl/certs/custom-ca.crt", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"]
	CACertPaths []string
}

// NewCertPool creates a new x509.CertPool with system CAs and custom CA certificates
// It loads certificates in the following order:
// 1. System CA certificates (as the base)
// 2. Custom CA certificates from configured paths
//
// Missing or invalid certificate files are logged as warnings but don't cause errors.
// This allows for graceful degradation and optional certificates.
func NewCertPool(logger logr.Logger, config Config) (*x509.CertPool, error) {
	// Load system CA certificates as the base
	var certPool *x509.CertPool
	systemCAs, err := x509.SystemCertPool()
	if err != nil {
		logger.V(1).Info("Unable to load system CA certificates, using empty pool", "error", err)
		certPool = x509.NewCertPool()
	} else {
		certPool = systemCAs
	}

	// Load CA certificates from configured paths
	for _, certPath := range config.CACertPaths {
		caCert, err := os.ReadFile(certPath)
		if err != nil {
			// Log warning but don't fail for optional certificates
			logger.V(1).Info("Unable to load CA certificate (file may not exist)", "path", certPath, "error", err)
			continue
		}

		if !certPool.AppendCertsFromPEM(caCert) {
			logger.Info("Warning: Failed to parse CA certificate, skipping", "path", certPath)
			continue
		}

		logger.Info("Loaded CA certificate", "path", certPath)
	}

	return certPool, nil
}

// NewTLSConfig creates a new tls.Config with certificates loaded from the trust store
// The returned tls.Config will have MinVersion set to TLS 1.2 and RootCAs populated
// from the trust store. Consumers should further configure the tls.Config as needed
// (e.g., setting InsecureSkipVerify, ServerName, etc.)
func NewTLSConfig(logger logr.Logger, config Config) (*tls.Config, error) {
	// Load CA certificate pool from trust store
	certPool, err := NewCertPool(logger, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate pool: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}

	return tlsConfig, nil
}
