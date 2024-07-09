// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func globalConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-global-cm"
}

func NewGlobalCM(ais *aisv1.AIStore, toUpdate *aisv1.ConfigToUpdate) (*corev1.ConfigMap, error) {
	globalConf := DefaultAISConf(ais)
	if toUpdate != nil {
		toSet, err := convertConfig(toUpdate)
		if err != nil {
			return nil, err
		}
		if err := globalConf.Apply(toSet, aisapc.Cluster); err != nil {
			return nil, err
		}
	}
	if ais.Spec.AWSSecretName != nil || ais.Spec.GCPSecretName != nil {
		if globalConf.Backend.Conf == nil {
			globalConf.Backend.Conf = make(map[string]interface{}, 8)
		}
		if ais.Spec.AWSSecretName != nil {
			globalConf.Backend.Conf["aws"] = aisv1.Empty{}
		}
		if ais.Spec.GCPSecretName != nil {
			globalConf.Backend.Conf["gcp"] = aisv1.Empty{}
		}
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
			aisGlobalConfigFileName: conf,
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
