// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"iter"

	"github.com/NVIDIA/aistore/cmn/cos"
	corev1 "k8s.io/api/core/v1"
)

func EnvFromFieldPath(envName, path string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  path,
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

// MergeEnvVars merges defaults and overrides envvars.
// For duplicate `Name` entries, overrides will take precedence.
func MergeEnvVars(defaults, overrides []corev1.EnvVar) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	exists := cos.StrSet{}
	for _, v := range overrides {
		exists.Add(v.Name)
		envVars = append(envVars, v)
	}

	for _, v := range defaults {
		if exists.Contains(v.Name) {
			continue
		}
		envVars = append(envVars, v)
	}
	return envVars
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

func IterPtr[Slice ~[]E, E any](s Slice) iter.Seq[*E] {
	return func(yield func(*E) bool) {
		for _, v := range s {
			if !yield(&v) {
				return
			}
		}
	}
}

func ValueOrDefault[T any](value, defaultValue *T) *T {
	if value == nil {
		return defaultValue
	}
	return value
}
