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
	conf, err := jsoniter.MarshalToString(targetConf(ais, customConfig))
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
		},
	}, nil
}

// TODO: Have default values and update target conf from config
func targetConf(ais *aisv1.AIStore, customConfig *aiscmn.ConfigToUpdate) aiscmn.Config {
	conf := cmn.DefaultAISConf(ais)

	conf.Net.L4.PortStr = ais.Spec.TargetSpec.PublicPort.String()
	conf.Net.L4.PortIntraControlStr = ais.Spec.TargetSpec.IntraControlPort.String()
	conf.Net.L4.PortIntraDataStr = ais.Spec.TargetSpec.IntraDataPort.String()
	conf.FSpaths.Paths = make(aiscmn.StringSet, len(ais.Spec.TargetSpec.Mounts))
	if customConfig != nil {
		_ = conf.Apply(*customConfig)
	}

	for _, res := range ais.Spec.TargetSpec.Mounts {
		conf.FSpaths.Paths.Add(res.Path)
	}
	return conf
}
