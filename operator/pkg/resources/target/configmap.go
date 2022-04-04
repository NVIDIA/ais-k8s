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

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
)

func configMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Target
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      configMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewTargetCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConf := cmn.LocalConfTemplate(ais.Spec.TargetSpec.ServiceSpec, ais.Spec.TargetSpec.Mounts)
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
			"set_initial_target_env.sh": initTargetSh,
			"ais_local.json":            confLocal,
		},
	}, nil
}
