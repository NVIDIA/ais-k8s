// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      cmn.AISConfigMapName(ais, aisapc.Target),
		Namespace: ais.Namespace,
	}
}

func NewTargetCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConfStr, err := buildLocalConf(ais)
	if err != nil {
		return nil, err
	}
	pvcList, err := buildDataPVCList(ais)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmn.AISConfigMapName(ais, aisapc.Target),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			cmn.AISLocalConfigName: localConfStr,
			cmn.AISDataPVC:         pvcList,
		},
	}, nil
}

func buildDataPVCList(ais *aisv1.AIStore) (string, error) {
	dataVCs := defineDataPVCs(ais)
	vcNames := make([]string, 0, len(dataVCs))
	for i := range dataVCs {
		// Match the formatting of a volume claim template, once the pod name is inserted
		vcNames = append(vcNames, fmt.Sprintf("%s-${%s}", dataVCs[i].Name, cmn.EnvPodName))
	}
	return jsoniter.MarshalToString(vcNames)
}

func buildLocalConf(ais *aisv1.AIStore) (string, error) {
	serviceSpec := ais.Spec.TargetSpec.ServiceSpec
	netConfig := aiscmn.LocalNetConfig{
		Hostname:             "${AIS_PUBLIC_HOSTNAME}",
		HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
		HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
		Port:                 serviceSpec.PublicPort.IntValue(),
		PortIntraControl:     serviceSpec.IntraControlPort.IntValue(),
		PortIntraData:        serviceSpec.IntraDataPort.IntValue(),
	}
	return jsoniter.MarshalToString(templateLocalConf(&ais.Spec, &netConfig))
}

func templateLocalConf(spec *aisv1.AIStoreSpec, netConfig *aiscmn.LocalNetConfig) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: cmn.StateDir,
		LogDir:    cmn.LogsDir,
		HostNet:   *netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsWithLabels(&spec.TargetSpec, &localConf)
	}
	return localConf
}

func definePathsWithLabels(spec *aisv1.TargetSpec, conf *aiscmn.LocalConfig) {
	mounts := spec.Mounts
	if len(mounts) == 0 {
		return
	}
	conf.FSP.Paths = aiscos.NewStrKVs(len(mounts))
	for _, m := range mounts {
		if m.Label != nil {
			conf.FSP.Paths[m.Path] = *m.Label
		} else {
			conf.FSP.Paths[m.Path] = ""
		}
	}
}
