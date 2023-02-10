// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func globalConfigMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-global-cm"
}

func GlobalConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      globalConfigMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewGlobalCM(ais *aisv1.AIStore, toUpdate *aiscmn.ConfigToUpdate) (*corev1.ConfigMap, error) {
	globalConf := DefaultAISConf(ais)
	if toUpdate != nil {
		if err := globalConf.Apply(toUpdate, aisapc.Cluster); err != nil {
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
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      globalConfigMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			"ais.json":         conf,
			"ais_liveness.sh":  livenessSh,
			"ais_readiness.sh": readinessSh,
		},
	}, nil
}
