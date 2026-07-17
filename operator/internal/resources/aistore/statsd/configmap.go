/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package statsd is deprecated. StatsD support was deprecated in AIStore
// in v3.28 and dropped in v4.0. Resource-name helpers are maintained here
// for cleanup of past deployments.
package statsd

import (
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

func ConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-statsd"
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ConfigMapName(ais),
		Namespace: ais.Namespace,
	}
}
