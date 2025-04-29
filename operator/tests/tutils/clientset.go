// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// NewClientset returns a kubernetes.Clientset created w/ the current
// in-cluster or KUBECONFIG environment.
func NewClientset() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating clientset: %w", err)
	}
	return cs, nil
}
