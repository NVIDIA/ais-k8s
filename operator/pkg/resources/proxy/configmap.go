// Package proxy contains k8s resources required by AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/ownerref"
	jsoniter "github.com/json-iterator/go"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      cmn.AISConfigMapName(ais, aisapc.Proxy),
		Namespace: ais.Namespace,
	}
}

func NewProxyCM(ais *aisv1.AIStore) (*corev1ac.ConfigMapApplyConfiguration, error) {
	localConf := localConfTemplate(&ais.Spec.ProxySpec.ServiceSpec)
	confLocal, err := jsoniter.MarshalToString(localConf)
	if err != nil {
		return nil, err
	}
	return corev1ac.ConfigMap(cmn.AISConfigMapName(ais, aisapc.Proxy), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithData(map[string]string{
			cmn.AISLocalConfigName: confLocal,
		}), nil
}

func localConfTemplate(spec *aisv1.ServiceSpec) aiscmn.LocalConfig {
	return aiscmn.LocalConfig{
		ConfigDir: cmn.StateDir,
		LogDir:    cmn.LogsDir,
		HostNet: aiscmn.LocalNetConfig{
			Hostname:             "${AIS_PUBLIC_HOSTNAME}",
			HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
			HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
			Port:                 spec.PublicPort.IntValue(),
			PortIntraControl:     spec.IntraControlPort.IntValue(),
			PortIntraData:        spec.IntraDataPort.IntValue(),
		},
	}
}
