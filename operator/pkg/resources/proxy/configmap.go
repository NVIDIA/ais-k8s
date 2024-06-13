// Package proxy contains k8s resources required by AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func configMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Proxy
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      configMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewProxyCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConf := localConfTemplate(ais.Spec.ProxySpec.ServiceSpec)
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
			"ais_local.json": confLocal,
		},
	}, nil
}

func localConfTemplate(spec aisv1.ServiceSpec) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    "/var/log/ais",
		HostNet: aiscmn.LocalNetConfig{
			Hostname:             "${AIS_PUBLIC_HOSTNAME}",
			HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
			HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
			Port:                 spec.PublicPort.IntValue(),
			PortIntraControl:     spec.IntraControlPort.IntValue(),
			PortIntraData:        spec.IntraDataPort.IntValue(),
		},
	}
	return localConf
}
