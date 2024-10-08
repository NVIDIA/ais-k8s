// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
	"path"
	"path/filepath"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
)

func PrepareAnnotations(annotations map[string]string, netAttachment *string) map[string]string {
	newAnnotations := map[string]string{}
	if netAttachment != nil {
		newAnnotations[nadv1.NetworkAttachmentAnnot] = *netAttachment
	}
	for k, v := range annotations {
		newAnnotations[k] = v
	}
	return newAnnotations
}

// NewLogSidecar Defines a container that mounts the location of logs and redirects output to the pod's stdout
func NewLogSidecar(daeType string) corev1.Container {
	logFile := filepath.Join(LogsDir, fmt.Sprintf("ais%s.INFO", daeType))
	return corev1.Container{
		Name:            "ais-logs",
		Image:           "docker.io/library/busybox:1.36.1",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", fmt.Sprintf("tail -n+1 -F %s", logFile)},
		VolumeMounts:    []corev1.VolumeMount{newLogsVolumeMount(daeType)},
		Env:             []corev1.EnvVar{EnvFromFieldPath(EnvPodName, "metadata.name")},
	}
}

func NewInitContainerArgs(daeType string, hostnameMap map[string]string) []string {
	args := []string{
		"-role=" + daeType,
		"-local_config_template=" + path.Join(InitConfTemplateDir, AISLocalConfigName),
		"-output_local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
	}
	if len(hostnameMap) != 0 {
		args = append(args, "-hostname_map_file="+path.Join(initGlobalConfDir, hostnameMapFileName))
	}
	return args
}

func NewAISContainerArgs(ais *aisv1.AIStore, daeType string) []string {
	args := []string{
		"-config=" + path.Join(AisConfigDir, AISGlobalConfigName),
		"-local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
		"-role=" + daeType,
	}
	if daeType == aisapc.Proxy {
		args = append(args, fmt.Sprintf("-ntargets=%d", ais.GetTargetSize()))
	}
	return args
}
