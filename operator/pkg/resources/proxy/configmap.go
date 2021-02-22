// Package proxy contains k8s resources required by AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

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
	return ais.Name + "-" + aiscmn.Proxy
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      configMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewProxyCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConf := cmn.LocalConfTemplate(ais.Spec.ProxySpec.ServiceSpec, nil)
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
			"ais_local.json":                   confLocal,
			"set_initial_primary_proxy_env.sh": initProxySh,
		},
	}, nil
}
