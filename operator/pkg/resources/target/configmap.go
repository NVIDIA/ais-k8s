// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"context"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/mod/semver"
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

func configMapName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Target
}

func ConfigMapNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      configMapName(ais),
		Namespace: ais.Namespace,
	}
}

func NewTargetCM(ctx context.Context, ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConfStr, err := buildLocalConf(ctx, ais.Spec)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(ais),
			Namespace: ais.Namespace,
		},
		Data: map[string]string{
			"ais_local.json": localConfStr,
		},
	}, nil
}

func buildLocalConf(ctx context.Context, spec aisv1.AIStoreSpec) (string, error) {
	serviceSpec := spec.TargetSpec.ServiceSpec
	netConfig := aiscmn.LocalNetConfig{
		Hostname:             "${AIS_PUBLIC_HOSTNAME}",
		HostnameIntraControl: "${AIS_INTRA_HOSTNAME}",
		HostnameIntraData:    "${AIS_DATA_HOSTNAME}",
		Port:                 serviceSpec.PublicPort.IntValue(),
		PortIntraControl:     serviceSpec.IntraControlPort.IntValue(),
		PortIntraData:        serviceSpec.IntraDataPort.IntValue(),
	}
	// Check if we support the new format for mpath labels to determine which conf version to use
	if checkLabelSupport(ctx, spec) {
		return jsoniter.MarshalToString(templateLocalConf(ctx, spec, netConfig))
	}
	return jsoniter.MarshalToString(templateOldLocalConf(spec, netConfig))
}

func templateLocalConf(ctx context.Context, spec aisv1.AIStoreSpec, netConfig aiscmn.LocalNetConfig) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    cmn.LogsDir,
		HostNet:   netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsWithLabels(ctx, spec.TargetSpec, &localConf)
	}
	return localConf
}

func templateOldLocalConf(spec aisv1.AIStoreSpec, netConfig aiscmn.LocalNetConfig) v322LocalConfig {
	localConf := v322LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    cmn.LogsDir,
		HostNet:   netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsNoLabels(spec.TargetSpec, &localConf)
	}
	return localConf
}

func definePathsWithLabels(ctx context.Context, spec aisv1.TargetSpec, conf *aiscmn.LocalConfig) {
	logger := logf.FromContext(ctx)

	mounts := spec.Mounts
	if len(mounts) == 0 {
		return
	}
	conf.FSP.Paths = cos.NewStrKVs(len(mounts))
	for _, m := range mounts {
		//nolint:all // Backwards compatible with old CR option
		if m.Label != nil {
			logger.Info("Using provided mountpath labels for aisnode image > 3.22")
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

func definePathsNoLabels(spec aisv1.TargetSpec, conf *v322LocalConfig) {
	mounts := spec.Mounts
	conf.FSP.Paths = make(cos.StrSet, len(mounts))
	for _, m := range mounts {
		conf.FSP.Paths.Add(m.Path)
	}
}

func checkLabelSupport(ctx context.Context, spec aisv1.AIStoreSpec) bool {
	logger := logf.FromContext(ctx)

	parts := strings.Split(spec.NodeImage, ":")
	if len(parts) != 2 {
		logger.Info("Image does not have a proper tag", "node_image", spec.NodeImage)
		return true
	}
	tag := parts[1]
	if !semver.IsValid(tag) {
		logger.Info("Image does not use semantic versioning, assuming it supports labels", "node_image", spec.NodeImage)
		return true
	}
	// Check version is at least v3.23
	if semver.Compare(tag, "v3.23") >= 0 {
		logger.Info("Image supports labels", "node_image", spec.NodeImage)
		return true
	}
	logger.Info("Image tag < v3.23, hence proceeding without labels", "node_image", spec.NodeImage)
	return false
}
