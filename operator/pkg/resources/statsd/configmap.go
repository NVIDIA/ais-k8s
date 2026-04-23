// Package statsd contains k8s resources required for statsd
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package statsd

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/ownerref"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
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

func NewStatsDCM(ais *aisv1.AIStore) *corev1ac.ConfigMapApplyConfiguration {
	return corev1ac.ConfigMap(ConfigMapName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithData(map[string]string{
			ConfigFile: `{
				"graphiteHost": "",
				"graphitePort": 2003
			}`,
		})
}
