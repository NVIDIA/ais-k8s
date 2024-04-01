// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"
	"os"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func NewTargetCM(ais *aisv1.AIStore) (*corev1.ConfigMap, error) {
	localConfStr, err := buildLocalConf(ais.Spec)
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
			"ais_local.json":            localConfStr,
		},
	}, nil
}

func buildLocalConf(spec aisv1.AIStoreSpec) (string, error) {
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
	if checkLabelSupport(spec) {
		return jsoniter.MarshalToString(templateLocalConf(spec, netConfig))
	}
	return jsoniter.MarshalToString(templateOldLocalConf(spec, netConfig))
}

func templateLocalConf(spec aisv1.AIStoreSpec, netConfig aiscmn.LocalNetConfig) aiscmn.LocalConfig {
	localConf := aiscmn.LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    "/var/log/ais",
		HostNet:   netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsWithLabels(spec.TargetSpec, &localConf)
	}
	return localConf
}

func templateOldLocalConf(spec aisv1.AIStoreSpec, netConfig aiscmn.LocalNetConfig) v322LocalConfig {
	localConf := v322LocalConfig{
		ConfigDir: "/etc/ais",
		LogDir:    "/var/log/ais",
		HostNet:   netConfig,
	}
	if len(spec.TargetSpec.Mounts) > 0 {
		definePathsNoLabels(spec.TargetSpec, &localConf)
	}
	return localConf
}

func definePathsWithLabels(spec aisv1.TargetSpec, conf *aiscmn.LocalConfig) {
	mounts := spec.Mounts
	if len(mounts) == 0 {
		return
	}
	conf.FSP.Paths = cos.NewStrKVs(len(mounts))
	for _, m := range mounts {
		//nolint:all // Backwards compatible with old CR option
		if m.Label != nil {
			fmt.Printf("Using provided mountpath labels for aisnode image > 3.22\n")
			conf.FSP.Paths[m.Path] = *m.Label
		} else if spec.AllowSharedOrNoDisks != nil && *spec.AllowSharedOrNoDisks {
			// Support allowSharedNoDisks until removed from CR
			fmt.Fprintf(os.Stderr, "Converting deprecated allowSharedNoDisks to mpath label!\n")
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

func checkLabelSupport(spec aisv1.AIStoreSpec) bool {
	parts := strings.Split(spec.NodeImage, ":")
	if len(parts) != 2 {
		fmt.Printf("Image '%s' does not have a proper tag.\n", spec.NodeImage)
		return true
	}
	tag := parts[1]
	if !semver.IsValid(tag) {
		fmt.Printf("Image '%s' does not use semantic versioning, assuming it supports labels.\n", spec.NodeImage)
		return true
	}
	// Check version is at least 3.23
	if semver.Compare(spec.NodeImage, "v3.23") >= 0 {
		return true
	}
	return false
}
