// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func globalConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-global-cm"
}

// NewGlobalCM creates the content for the configmap mounted by AIS pods based on provided spec and cluster state.
func NewGlobalCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	globalConf, err := GenerateGlobalConfig(ais)
	if err != nil {
		return nil, err
	}
	conf, err := jsoniter.MarshalToString(globalConf)
	if err != nil {
		return nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      globalConfigMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			AISGlobalConfigName: conf,
		},
	}
	if ais.Spec.HostnameMap != nil {
		hostnameMap, err := jsoniter.MarshalToString(ais.Spec.HostnameMap)
		if err != nil {
			return nil, err
		}
		cm.Data[hostnameMapFileName] = hostnameMap
	}
	return cm, nil
}

func AISConfigMapName(ais *aisv1.AIStore, daeType string) string {
	return ais.Name + "-" + daeType
}
