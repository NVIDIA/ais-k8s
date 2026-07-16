/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"reflect"
	"testing"

	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func TestToApplyConfigurationPreservesKubernetesFields(t *testing.T) {
	value := corev1.LocalObjectReference{Name: "registry-creds"}
	configuration, err := toApplyConfiguration[*corev1ac.LocalObjectReferenceApplyConfiguration](value)
	if err != nil {
		t.Fatalf("convert to apply configuration: %v", err)
	}
	assertSameJSON(t, value, configuration)
}

func TestToApplyConfigurationReturnsMarshalError(t *testing.T) {
	if _, err := toApplyConfiguration[any](func() {}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestToApplyConfigurationReturnsConversionError(t *testing.T) {
	if _, err := toApplyConfiguration[int]("not-an-integer"); err == nil {
		t.Fatal("expected conversion error")
	}
}

func assertSameJSON(t *testing.T, expected, actual any) {
	t.Helper()
	var expectedJSON, actualJSON any

	data, err := jsoniter.Marshal(expected)
	if err != nil {
		t.Fatalf("marshal expected value: %v", err)
	}
	if err := jsoniter.Unmarshal(data, &expectedJSON); err != nil {
		t.Fatalf("decode expected value: %v", err)
	}
	data, err = jsoniter.Marshal(actual)
	if err != nil {
		t.Fatalf("marshal apply configuration: %v", err)
	}
	if err := jsoniter.Unmarshal(data, &actualJSON); err != nil {
		t.Fatalf("decode apply configuration: %v", err)
	}
	if !reflect.DeepEqual(expectedJSON, actualJSON) {
		t.Fatalf("apply configuration differs from source\nexpected: %#v\nactual:   %#v", expectedJSON, actualJSON)
	}
}
