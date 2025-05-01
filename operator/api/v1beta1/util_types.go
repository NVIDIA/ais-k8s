// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"github.com/NVIDIA/aistore/cmn/cos"
)

// Empty type is needed because declaring `map[string]struct{}` or `map[string]interface{}`
// raises error "name requested for invalid type: struct{}/interface{}".
// For more information see:
//   - https://github.com/kubernetes-sigs/controller-tools/issues/636
//   - https://github.com/kubernetes-sigs/kubebuilder/issues/528
type Empty struct{}

// Duration is wrapper over `cos.Duration` that overrides type in generated manifests.
// +kubebuilder:validation:Type=string
type Duration cos.Duration

func (d Duration) MarshalJSON() ([]byte, error)        { return cos.Duration(d).MarshalJSON() }
func (d *Duration) UnmarshalJSON(b []byte) (err error) { return (*cos.Duration)(d).UnmarshalJSON(b) }

// SizeIEC is wrapper over `cos.SizeIEC` that overrides type in generated manifests.
// +kubebuilder:validation:Type=string
type SizeIEC cos.SizeIEC

func (s SizeIEC) MarshalJSON() ([]byte, error)        { return cos.SizeIEC(s).MarshalJSON() }
func (s *SizeIEC) UnmarshalJSON(b []byte) (err error) { return (*cos.SizeIEC)(s).UnmarshalJSON(b) }
