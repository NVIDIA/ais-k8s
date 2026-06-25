/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"os"
	"testing"

	aisv1 "github.com/ais-operator/api/v1beta1"
)

func TestConfigureCAVerification_UsesSpecValueWhenSet(t *testing.T) {
	t.Setenv(EnvSkipVerify, "true")
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
}

func TestConfigureCAVerification_FallsBackToEnvWhenSpecUnset(t *testing.T) {
	t.Setenv(EnvSkipVerify, "true")
	tlsConf := &tls.Config{}
	ais := &aisv1.AIStore{}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err != nil {
		t.Fatalf("configureCAVerification returned error: %v", err)
	}
	if !tlsConf.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=true from %s fallback", EnvSkipVerify)
	}
}

func TestConfigureCAVerification_InvalidEnvErrorsWhenSpecUnset(t *testing.T) {
	t.Setenv(EnvSkipVerify, "invalid-bool")
	tlsConf := &tls.Config{}
	ais := &aisv1.AIStore{}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid OPERATOR_SKIP_VERIFY_CRT value")
	}
}

func TestConfigureCAVerification_SpecTrueSkipsVerification(t *testing.T) {
	t.Setenv(EnvSkipVerify, "false")
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
	oldVal, hadVal := os.LookupEnv(EnvSkipVerify)
	if err := os.Unsetenv(EnvSkipVerify); err != nil {
		t.Fatalf("failed to unset %s: %v", EnvSkipVerify, err)
	}
	t.Cleanup(func() {
		if hadVal {
			_ = os.Setenv(EnvSkipVerify, oldVal)
		} else {
			_ = os.Unsetenv(EnvSkipVerify)
		}
	})

	tlsConf := &tls.Config{}
	ais := &aisv1.AIStore{}

	err := configureCAVerification(context.Background(), ais, tlsConf, t.TempDir())
	if err != nil {
		t.Fatalf("configureCAVerification returned error: %v", err)
	}
	if tlsConf.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=false when spec and env are unset")
	}
}
