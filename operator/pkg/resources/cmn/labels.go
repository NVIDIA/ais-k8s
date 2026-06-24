/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import "maps"

// LegacyLabels returns standard AIS daemon labels including unprefixed app and component
// for backward compatibility with external selectors.
func LegacyLabels(name, component string) map[string]string {
	return map[string]string{
		LabelApp:               name,
		LabelAppPrefixed:       name,
		LabelComponent:         component,
		LabelComponentPrefixed: component,
	}
}

// SelectorLabels returns labels used for resource selectors (StatefulSet, Service, PDB, list/delete).
func SelectorLabels(name, component string) map[string]string {
	return map[string]string{
		LabelAppPrefixed:       name,
		LabelComponentPrefixed: component,
	}
}

// MergePodLabels applies userLabels first, then specLabels so reserved keys cannot be overridden.
func MergePodLabels(userLabels, specLabels map[string]string) map[string]string {
	podLabels := map[string]string{}
	maps.Copy(podLabels, userLabels)
	maps.Copy(podLabels, specLabels)
	return podLabels
}
