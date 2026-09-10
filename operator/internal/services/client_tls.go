/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ClientCertFile = "tls.crt"
	ClientKeyFile  = "tls.key"
	ClientCAFile   = "ca.crt"
	CAMountPath    = "/etc/ais/ca"
)

// AISClientTLSOpts locates the certificates the operator presents and trusts when calling AIS.
type AISClientTLSOpts struct {
	CertPath       string
	CertPerCluster bool
}

func (m *AISClientManager) getTLSConfig(ctx context.Context, ais *aisv1.AIStore) (*tls.Config, error) {
	if !ais.UseHTTPS() {
		return nil, nil
	}
	tlsDir := m.getTLSPath(ais)
	tlsConf := &tls.Config{}
	err := configureCAVerification(ctx, ais, tlsConf, tlsDir)
	if err != nil {
		return nil, err
	}
	addClientCertIfRequested(ais, tlsConf, tlsDir)
	return tlsConf, nil
}

func configureCAVerification(ctx context.Context, ais *aisv1.AIStore, tlsConf *tls.Config, tlsDir string) error {
	logger := logf.FromContext(ctx)
	if ais.Spec.OperatorSkipVerifyCrt != nil && *ais.Spec.OperatorSkipVerifyCrt {
		tlsConf.InsecureSkipVerify = true
		return nil
	}

	// Add CA from our specified TLS config dir to the system trusted CA pool
	providedCA := filepath.Join(tlsDir, ClientCAFile)
	caPool, err := loadOptionalProvidedCA(logger, providedCA)
	if err != nil {
		logger.Error(err, "Failed to load AIS CA", "location", providedCA)
		return err
	}
	tlsConf.RootCAs = caPool
	return nil
}

func addClientCertIfRequested(ais *aisv1.AIStore, tlsConf *tls.Config, tlsDir string) {
	if !ais.ShouldIncludeClientCert() {
		return
	}
	certPath := filepath.Join(tlsDir, ClientCertFile)
	keyPath := filepath.Join(tlsDir, ClientKeyFile)
	tlsConf.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		var cert tls.Certificate
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		return &cert, nil
	}
}

func (m *AISClientManager) getTLSPath(ais *aisv1.AIStore) string {
	if m.tlsOpts.CertPerCluster {
		return filepath.Join(m.tlsOpts.CertPath, ais.Namespace, ais.Name)
	}
	return m.tlsOpts.CertPath
}

// loadOptionalProvidedCA returns the system trust pool extended with the CA at caPath and any
// certificates mounted at CAMountPath. A missing CA at caPath is not an error.
func loadOptionalProvidedCA(logger logr.Logger, caPath string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		logger.Error(err, "Failed to load system cert pool")
		return nil, err
	}
	err = appendCertIfExists(logger, pool, caPath)
	if err != nil {
		logger.Error(err, "Failed to append CA cert to pool", "path", caPath)
		return nil, err
	}
	mountedPaths, err := mountedCAPaths(logger)
	if err != nil {
		return nil, err
	}
	for _, path := range mountedPaths {
		if appendErr := appendCertIfExists(logger, pool, path); appendErr != nil {
			logger.Error(appendErr, "Failed to add new trusted CA", "path", path)
		}
	}
	return pool, nil
}

// mountedCAPaths returns the paths of the additional CA certificates provided at CAMountPath.
func mountedCAPaths(logger logr.Logger) ([]string, error) {
	_, err := os.Stat(CAMountPath)
	if os.IsNotExist(err) {
		logger.Info("No path found with additional CA certs", "path", CAMountPath)
		return nil, nil
	} else if err != nil {
		logger.Error(err, "Failed to stat CA mount path", "path", CAMountPath)
		return nil, err
	}
	certPaths, err := findCerts(CAMountPath, []string{".crt", ".pem"})
	if err != nil {
		// Non-fatal error if we cannot load additional trust, log and continue
		logger.Error(err, "Failed to search cert paths", "caRoot", CAMountPath)
		return nil, nil
	}
	return certPaths, nil
}

func appendCertIfExists(logger logr.Logger, pool *x509.CertPool, path string) error {
	cert, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	logger.Info("Adding trusted CA certificate", "path", path)
	if ok := pool.AppendCertsFromPEM(cert); !ok {
		return fmt.Errorf("failed to append existing CA certs from PEM: %q", path)
	}
	return nil
}

func findCerts(root string, exts []string) ([]string, error) {
	var certPaths []string
	err := filepath.WalkDir(root, func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		// Filter out entries starting with two dots -- K8s-managed hidden directories
		if strings.HasPrefix(d.Name(), "..") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		for _, ext := range exts {
			if filepath.Ext(d.Name()) == ext {
				certPaths = append(certPaths, s)
				break
			}
		}
		return nil
	})
	return certPaths, err
}
