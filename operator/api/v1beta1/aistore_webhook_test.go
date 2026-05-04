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

func TestValidateProxyUpdateTolerations(t *testing.T) {
	RegisterTestingT(t)

	toleration := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	t.Run("adding toleration to proxy spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		ais := &AIStore{}
		ais.Spec.ProxySpec.Tolerations = []corev1.Toleration{toleration}
		Expect(validateProxyUpdate(prev, ais)).To(Succeed())
	})

	t.Run("removing toleration from proxy spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		prev.Spec.ProxySpec.Tolerations = []corev1.Toleration{toleration}
		ais := &AIStore{}
		Expect(validateProxyUpdate(prev, ais)).To(Succeed())
	})

	t.Run("modifying toleration in proxy spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		prev.Spec.ProxySpec.Tolerations = []corev1.Toleration{toleration}
		ais := &AIStore{}
		modified := toleration
		modified.Effect = corev1.TaintEffectNoExecute
		ais.Spec.ProxySpec.Tolerations = []corev1.Toleration{modified}
		Expect(validateProxyUpdate(prev, ais)).To(Succeed())
	})
}

func TestValidateTargetUpdateTolerations(t *testing.T) {
	RegisterTestingT(t)

	toleration := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	t.Run("adding toleration to target spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		ais := &AIStore{}
		ais.Spec.TargetSpec.Tolerations = []corev1.Toleration{toleration}
		Expect(validateTargetUpdate(prev, ais)).To(Succeed())
	})

	t.Run("removing toleration from target spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		prev.Spec.TargetSpec.Tolerations = []corev1.Toleration{toleration}
		ais := &AIStore{}
		Expect(validateTargetUpdate(prev, ais)).To(Succeed())
	})

	t.Run("modifying toleration in target spec is allowed", func(_ *testing.T) {
		prev := &AIStore{}
		prev.Spec.TargetSpec.Tolerations = []corev1.Toleration{toleration}
		ais := &AIStore{}
		modified := toleration
		modified.Effect = corev1.TaintEffectNoExecute
		ais.Spec.TargetSpec.Tolerations = []corev1.Toleration{modified}
		Expect(validateTargetUpdate(prev, ais)).To(Succeed())
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
