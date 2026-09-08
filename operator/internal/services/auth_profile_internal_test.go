/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAuthCATrustStoreConfig_MissingDirYieldsEmptyConfig(t *testing.T) {
	conf, err := authCATrustStoreConfig(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("authCATrustStoreConfig returned error: %v", err)
	}
	if len(conf.CACertPaths) != 0 || len(conf.CAPEMs) != 0 {
		t.Fatalf("expected empty config for missing dir, got %+v", conf)
	}
}

func TestAuthCATrustStoreConfig_LoadsCrtAndPemFiles(t *testing.T) {
	dir := t.TempDir()
	crtPath := filepath.Join(dir, "ca.crt")
	pemPath := filepath.Join(dir, "ca.pem")
	txtPath := filepath.Join(dir, "readme.txt")
	for _, path := range []string{crtPath, pemPath, txtPath} {
		if err := os.WriteFile(path, []byte("cert-data"), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	conf, err := authCATrustStoreConfig(context.Background(), dir)
	if err != nil {
		t.Fatalf("authCATrustStoreConfig returned error: %v", err)
	}
	if len(conf.CACertPaths) != 2 {
		t.Fatalf("expected 2 CA cert paths (.crt and .pem only), got %v", conf.CACertPaths)
	}
	for _, want := range []string{crtPath, pemPath} {
		found := slices.Contains(conf.CACertPaths, want)
		if !found {
			t.Fatalf("expected %s in CACertPaths, got %v", want, conf.CACertPaths)
		}
	}
}
