// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"time"
)

const ClusterCreateInterval = time.Second

func GetClusterCreateTimeout() time.Duration {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 4 * time.Minute
	}
	return time.Minute
}

func GetClusterCreateLongTimeout() time.Duration {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 6 * time.Minute
	}
	return 2 * time.Minute
}

func GetLBExistenceTimeout() (timeout, interval time.Duration) {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 4 * time.Minute, 5 * time.Second
	}
	return 10 * time.Second, 200 * time.Millisecond
}
