// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

func NewServiceLabels(aisName, component string) map[string]string {
	return map[string]string{
		LabelApp:               aisName,
		LabelAppPrefixed:       aisName,
		LabelComponentPrefixed: component,
	}
}
