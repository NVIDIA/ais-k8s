// Package v1beta1 contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// runTolerationUpdateScenarios exercises add/remove/modify toleration paths for proxy or target updates.
func runTolerationUpdateScenarios(
	t *testing.T,
	component string,
	validate func(prev, ais *AIStore) error,
	setTolerations func(a *AIStore, tols []corev1.Toleration),
) {
	t.Helper()

	toleration := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	t.Run("adding toleration to "+component+" spec is allowed", func(subT *testing.T) {
		RegisterTestingT(subT)
		prev := &AIStore{}
		ais := &AIStore{}
		setTolerations(ais, []corev1.Toleration{toleration})
		Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("removing toleration from "+component+" spec is allowed", func(subT *testing.T) {
		RegisterTestingT(subT)
		prev := &AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &AIStore{}
		Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("modifying toleration in "+component+" spec is allowed", func(subT *testing.T) {
		RegisterTestingT(subT)
		prev := &AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &AIStore{}
		modified := toleration
		modified.Effect = corev1.TaintEffectNoExecute
		setTolerations(ais, []corev1.Toleration{modified})
		Expect(validate(prev, ais)).To(Succeed())
	})
}

func TestUsesEmptyDirForMetadata(t *testing.T) {
	tests := []struct {
		name     string
		ptr      *bool
		expected bool
	}{
		{"nil pointer returns false", nil, false},
		{"false pointer returns false", aisapc.Ptr(false), false},
		{"true pointer returns true", aisapc.Ptr(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.UseEmptyDirForMetadata = tt.ptr
			Expect(ais.UsesEmptyDirForMetadata()).To(Equal(tt.expected))
		})
	}
}

func TestValidateStateStorage(t *testing.T) {
	tests := []struct {
		name              string
		hostpathPrefix    *string
		stateStorageClass *string
		useEmptyDir       *bool
		wantErr           bool
		wantWarning       bool
	}{
		{
			name:        "only useEmptyDirForMetadata is valid",
			useEmptyDir: aisapc.Ptr(true),
		},
		{
			name:           "only hostpathPrefix is valid",
			hostpathPrefix: aisapc.Ptr("/mnt"),
		},
		{
			name:              "only stateStorageClass is valid",
			stateStorageClass: aisapc.Ptr("my-sc"),
		},
		{
			name:           "hostpathPrefix and stateStorageClass emits legacy warning",
			hostpathPrefix: aisapc.Ptr("/mnt"),
			stateStorageClass: aisapc.Ptr("my-sc"),
			wantWarning:    true,
		},
		{
			name:           "useEmptyDir and hostpathPrefix errors",
			useEmptyDir:    aisapc.Ptr(true),
			hostpathPrefix: aisapc.Ptr("/mnt"),
			wantErr:        true,
		},
		{
			name:              "useEmptyDir and stateStorageClass errors",
			useEmptyDir:       aisapc.Ptr(true),
			stateStorageClass: aisapc.Ptr("my-sc"),
			wantErr:           true,
		},
		{
			name:              "all three set errors",
			useEmptyDir:       aisapc.Ptr(true),
			hostpathPrefix:    aisapc.Ptr("/mnt"),
			stateStorageClass: aisapc.Ptr("my-sc"),
			wantErr:           true,
		},
		{
			name:    "none set errors",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.HostpathPrefix = tt.hostpathPrefix
			ais.Spec.StateStorageClass = tt.stateStorageClass
			ais.Spec.UseEmptyDirForMetadata = tt.useEmptyDir
			warns, err := ais.validateStateStorage()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			if tt.wantWarning {
				Expect(warns).ToNot(BeEmpty())
			} else {
				Expect(warns).To(BeEmpty())
			}
		})
	}
}

func TestValidateProxyUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Proxy, validateProxyUpdate, func(a *AIStore, tols []corev1.Toleration) {
		a.Spec.ProxySpec.Tolerations = tols
	})
}

func TestValidateTargetUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Target, validateTargetUpdate, func(a *AIStore, tols []corev1.Toleration) {
		a.Spec.TargetSpec.Tolerations = tols
	})
}

func TestAIStoreValidateSize(t *testing.T) {
	tests := []struct {
		name       string // description of this test case
		want       admission.Warnings
		wantErr    bool
		proxySize  *int32
		targetSize *int32
		size       *int32
	}{
		{
			"Proxy size is -1 thus proxy autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](-1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"target size is -1 thus target autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
			nil,
		},
		{
			" size is -1 thus autoscaling is true",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](-1),
		},
		{
			"autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
		},
		{
			"not autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"not autoscaling with just size",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](1),
		},
		{
			"invalid size",
			nil,
			true,
			nil,
			nil,
			aisapc.Ptr[int32](-2),
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-2),
			nil,
		},
		{
			"invalid proxy size",
			nil,
			true,
			aisapc.Ptr[int32](-2),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid proxy size;0",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid target size;0",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](0),
			nil,
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
		},
	}
	for _, tt := range tests {
		RegisterTestingT(t)
		t.Run(tt.name, func(t *testing.T) {
			var ais AIStore
			ais.Spec.ProxySpec.Size = tt.proxySize
			ais.Spec.TargetSpec.Size = tt.targetSize
			ais.Spec.Size = tt.size
			got, gotErr := ais.validateSize()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("validateSize() failed: %v for test %s", gotErr, tt.name)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("validateSize() succeeded unexpectedly")
			}
			Expect(got).To(Equal(tt.want))
		})
	}
}
