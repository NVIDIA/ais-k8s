/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
)

func TestConfigureCAVerification_SpecFalseVerifies(t *testing.T) {
	tlsConf := &tls.Config{}
	disableVerify := false
	ais := &aisv1.AIStore{
		Spec: aisv1.AIStoreSpec{
			OperatorSkipVerifyCrt: &disableVerify,
		},
	}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err != nil {
		t.Fatalf("configureCAVerification returned error: %v", err)
	}
	if tlsConf.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=false when spec explicitly sets false")
	}
	if tlsConf.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated when spec explicitly sets false")
	}
}

func TestConfigureCAVerification_SpecTrueSkipsVerification(t *testing.T) {
	tlsConf := &tls.Config{}
	enableSkip := true
	ais := &aisv1.AIStore{
		Spec: aisv1.AIStoreSpec{
			OperatorSkipVerifyCrt: &enableSkip,
		},
	}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err != nil {
		t.Fatalf("configureCAVerification returned error: %v", err)
	}
	if !tlsConf.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=true when spec explicitly sets true")
	}
}

func TestConfigureCAVerification_DefaultsToVerifyWhenUnset(t *testing.T) {
	tlsConf := &tls.Config{}
	ais := &aisv1.AIStore{}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err != nil {
		t.Fatalf("configureCAVerification returned error: %v", err)
	}
	if tlsConf.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=false when spec is unset")
	}
	if tlsConf.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated when spec is unset")
	}
}

func TestTLSSettings_PlaintextClusterHasNone(t *testing.T) {
	if got := tlsSettings(&aisv1.AIStore{}); got != "" {
		t.Errorf("TLS settings = %q, want empty for a cluster that does not use HTTPS", got)
	}
}

func TestTLSSettings_ChangesWithVerification(t *testing.T) {
	ais := httpsCluster()

	verifying := tlsSettings(ais)
	ais.Spec.OperatorSkipVerifyCrt = aisapc.Ptr(true)
	skipping := tlsSettings(ais)

	if verifying == "" {
		t.Fatal("expected TLS settings for a cluster using HTTPS")
	}
	if skipping == verifying {
		t.Error("expected new TLS settings after certificate verification was disabled")
	}
	ais.Spec.OperatorSkipVerifyCrt = aisapc.Ptr(false)
	if tightened := tlsSettings(ais); tightened != verifying {
		t.Errorf("TLS settings = %q, want %q when verification is explicitly required", tightened, verifying)
	}
}

func TestTLSSettings_ChangesWithClientCert(t *testing.T) {
	ais := httpsCluster()

	withoutCert := tlsSettings(ais)
	ais.Spec.ConfigToUpdate.Net.HTTP.ClientAuthTLS = aisapc.Ptr(int(tls.RequireAndVerifyClientCert))
	withCert := tlsSettings(ais)

	if withCert == withoutCert {
		t.Error("expected new TLS settings after the client certificate was requested")
	}
}

func httpsCluster() *aisv1.AIStore {
	return &aisv1.AIStore{
		Spec: aisv1.AIStoreSpec{
			ConfigToUpdate: &aisv1.ConfigToUpdate{
				Net: &aisv1.NetConfToUpdate{
					HTTP: &aisv1.HTTPConfToUpdate{UseHTTPS: aisapc.Ptr(true)},
				},
			},
		},
	}
}
