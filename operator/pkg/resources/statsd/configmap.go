// Package statsd contains k8s resources required for statsd
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package statsd

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const ConfigFile = "statsd.json"

func ConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-statsd"
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ConfigMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewStatsDCM(ais *aisv1.AIStore) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			ConfigFile: `{
				"graphiteHost": "",
				"graphitePort": 2003
			}`,
		},
	}
}
