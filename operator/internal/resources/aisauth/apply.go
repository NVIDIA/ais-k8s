/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// toApplyConfiguration converts between matching Kubernetes API and generated apply-configuration
// types.
func toApplyConfiguration[T any](value any) (T, error) {
	var config T
	data, err := jsoniter.Marshal(value)
	if err != nil {
		return config, fmt.Errorf("marshal %T as apply configuration: %w", value, err)
	}
	if err := jsoniter.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("convert %T to apply configuration: %w", value, err)
	}
	return config, nil
}
