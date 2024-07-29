// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	corev1 "k8s.io/api/core/v1"
)

func EnvFromFieldPath(envName, path string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: path,
			},
		},
	}
}

func EnvFromValue(envName, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  envName,
		Value: value,
	}
}

func EnvFromSecret(envName, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret,
				},
				Key: key,
			},
		},
	}
}

// IsBoolSet checks if a boolean pointer is set to true.
func IsBoolSet(v *bool) bool {
	return v != nil && *v
}

func AnyFunc(funcs ...func() (bool, error)) (bool, error) {
	if len(funcs) == 0 {
		panic("at least one function expected")
	}

	atLeastOneTrue := false
	for _, f := range funcs {
		val, err := f()
		if err != nil {
			return false, err
		}
		atLeastOneTrue = atLeastOneTrue || val
	}
	return atLeastOneTrue, nil
}
