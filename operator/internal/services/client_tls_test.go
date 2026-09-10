/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package services

import (
	"context"
	"crypto/tls"
	"testing"

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
