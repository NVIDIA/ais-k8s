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

func NewProxyCM(ais *aisv1.AIStore, toUpdate *aiscmn.ConfigToUpdate) (*corev1.ConfigMap, error) {
	conf, err := jsoniter.MarshalToString(proxyConf(ais, toUpdate))
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			"ais.json":                         conf,
			"set_initial_primary_proxy_env.sh": initProxySh,
		},
	}, nil
}

// TODO: Have default values and update based on proxy config CRD
func proxyConf(ais *aisv1.AIStore, toUpdate *aiscmn.ConfigToUpdate) aiscmn.Config {
	conf := cmn.DefaultAISConf(ais)
	// Network hostnames are substituted in InitContainer.
	conf.Net.L4.PortStr = ais.Spec.ProxySpec.PublicPort.String()
	conf.Net.L4.PortIntraControlStr = ais.Spec.ProxySpec.IntraControlPort.String()
	conf.Net.L4.PortIntraDataStr = ais.Spec.ProxySpec.IntraDataPort.String()
	if toUpdate != nil {
		_ = conf.Apply(*toUpdate)
	}

	// TODO: Remove after `aisnode` image with fspath proxy validation fix is pushed
	conf.FSpaths.Paths = make(aiscmn.StringSet, len(ais.Spec.TargetSpec.Mounts))
	for _, res := range ais.Spec.TargetSpec.Mounts {
		conf.FSpaths.Paths.Add(res.Path)
	}
	return conf
}
