// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package target

import (
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/cmn"
)

func configMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aiscmn.Target
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      configMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewTargetCM(ais *aisv1.AIStore, customConfig *aiscmn.ConfigToUpdate) (*corev1.ConfigMap, error) {
	globalConf, localConf := targetConf(ais, customConfig)
	conf, err := jsoniter.MarshalToString(globalConf)
	if err != nil {
		return nil, err
	}
	confLocal, err := jsoniter.MarshalToString(localConf)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			"ais.json":                  conf,
			"set_initial_target_env.sh": initTargetSh,
			"ais_local.json":            confLocal,
		},
	}, nil
}

func targetConf(ais *aisv1.AIStore, toUpdate *aiscmn.ConfigToUpdate) (aiscmn.Config, aiscmn.LocalConfig) {
	conf := cmn.DefaultAISConf(ais)
	if toUpdate != nil {
		_ = conf.Apply(*toUpdate)
	}
	localConf := cmn.LocalConfTemplate(ais.Spec.TargetSpec.ServiceSpec, ais.Spec.TargetSpec.Mounts)
	return conf, localConf
}
