// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/ownerref"
	jsoniter "github.com/json-iterator/go"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func globalConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-global-cm"
}

// NewGlobalCM creates the apply config for the global configmap mounted by AIS pods.
func NewGlobalCM(ais *aisv1.AIStore) (*corev1ac.ConfigMapApplyConfiguration, error) {
	globalConf, err := GenerateGlobalConfig(ais)
	if err != nil {
		return nil, err
	}
	conf, err := jsoniter.MarshalToString(globalConf)
	if err != nil {
		return nil, err
	}
	data := map[string]string{
		AISGlobalConfigName: conf,
	}
	if ais.Spec.HostnameMap != nil {
		hostnameMap, err := jsoniter.MarshalToString(ais.Spec.HostnameMap)
		if err != nil {
			return nil, err
		}
		data[hostnameMapFileName] = hostnameMap
	}
	return corev1ac.ConfigMap(globalConfigMapName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithData(data), nil
}

func AISConfigMapName(ais *aisv1.AIStore, daeType string) string {
	return ais.Name + "-" + daeType
}
