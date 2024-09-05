// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"context"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	jsoniter "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type (
	// Override local config struct from aiscmn if needed, to support old FSP type
	v322LocalConfig struct {
		ConfigDir string                `json:"confdir"`
		LogDir    string                `json:"log_dir"`
		HostNet   aiscmn.LocalNetConfig `json:"host_net"`
		FSP       aiscmn.FSPConfV322    `json:"fspaths"`
		TestFSP   aiscmn.TestFSPConf    `json:"test_fspaths"`
	}
)

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      cmn.AISConfigMapName(ais, aisapc.Target),
		Namespace: ais.Namespace,
	}
}

func NewTargetCM(ctx context.Context, ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConfStr, err := buildLocalConf(ctx, ais)
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
		},
	}, nil
}

func buildLocalConf(ctx context.Context, ais *aisv1.AIStore) (string, error) {
	serviceSpec := ais.Spec.TargetSpec.ServiceSpec
	netConfig := aiscmn.LocalNetConfig{
		Hostname:             "${AIS_PUBLIC_HOSTNAME}",
		HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
		HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
		Port:                 serviceSpec.PublicPort.IntValue(),
		PortIntraControl:     serviceSpec.IntraControlPort.IntValue(),
		PortIntraData:        serviceSpec.IntraDataPort.IntValue(),
	}
	// Check if we support the new format for mpath labels to determine which conf version to use
	if checkLabelSupport(ctx, ais) {
		return jsoniter.MarshalToString(templateLocalConf(ctx, &ais.Spec, &netConfig))
	}
	return jsoniter.MarshalToString(templateOldLocalConf(&ais.Spec, &netConfig))
}

func templateLocalConf(ctx context.Context, spec *aisv1.AIStoreSpec, netConfig *aiscmn.LocalNetConfig) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: cmn.StateDir,
		LogDir:    cmn.LogsDir,
		HostNet:   *netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsWithLabels(ctx, &spec.TargetSpec, &localConf)
	}
	return localConf
}

func templateOldLocalConf(spec *aisv1.AIStoreSpec, netConfig *aiscmn.LocalNetConfig) v322LocalConfig {
	localConf := v322LocalConfig{
		ConfigDir: cmn.StateDir,
		LogDir:    cmn.LogsDir,
		HostNet:   *netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsNoLabels(&spec.TargetSpec, &localConf)
	}
	return localConf
}

func definePathsWithLabels(ctx context.Context, spec *aisv1.TargetSpec, conf *aiscmn.LocalConfig) {
	logger := logf.FromContext(ctx)

	mounts := spec.Mounts
	if len(mounts) == 0 {
		return
	}
	conf.FSP.Paths = cos.NewStrKVs(len(mounts))
	for _, m := range mounts {
		//nolint:all // Backwards compatible with old CR option
		if m.Label != nil {
			conf.FSP.Paths[m.Path] = *m.Label
		} else if spec.AllowSharedOrNoDisks != nil && *spec.AllowSharedOrNoDisks {
			// Support allowSharedNoDisks until removed from CR
			logger.Info("WARNING: Converting deprecated allowSharedNoDisks to mpath label")
			conf.FSP.Paths[m.Path] = "diskless"
		} else {
			conf.FSP.Paths[m.Path] = ""
		}
	}
}

func definePathsNoLabels(spec *aisv1.TargetSpec, conf *v322LocalConfig) {
	mounts := spec.Mounts
	conf.FSP.Paths = make(cos.StrSet, len(mounts))
	for _, m := range mounts {
		conf.FSP.Paths.Add(m.Path)
	}
}

func checkLabelSupport(ctx context.Context, ais *aisv1.AIStore) bool {
	logger := logf.FromContext(ctx)
	useLabels, err := ais.CompareVersion("v3.23")
	if err != nil {
		logger.Error(err, "Error parsing aisnode image. Assuming it supports mount-path labels!")
		return true
	}
	return useLabels
}
