// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024-2025, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
	"path"
	"path/filepath"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	LabelApp                = "app"
	LabelComponent          = "component"
	LabelPrefix             = "app.kubernetes.io/"
	LabelAppPrefixed        = LabelPrefix + "name"
	LabelComponentPrefixed  = LabelPrefix + "component"
	DefaultConfigStorageReq = int64(16 * aiscos.MiB)
	DefaultLogsStorageReq   = int64(512 * aiscos.MiB)
	DefaultMiscStorageReq   = int64(128 * aiscos.MiB)
)

func PrepareAnnotations(annotations map[string]string, netAttachment, restartHash *string) map[string]string {
	newAnnotations := map[string]string{}
	if netAttachment != nil {
		newAnnotations[nadv1.NetworkAttachmentAnnot] = *netAttachment
	}
	if restartHash != nil {
		newAnnotations[RestartConfigHashAnnotation] = *restartHash
	}
	if len(annotations) == 0 {
		return newAnnotations
	}
	for k, v := range annotations {
		newAnnotations[k] = v
	}
	return newAnnotations
}

// NewLogSidecar Defines a container that mounts the location of AIS info logs
func NewLogSidecar(image, daeType string) corev1.Container {
	logFile := filepath.Join(LogsDir, fmt.Sprintf("ais%s.INFO", daeType))
	return corev1.Container{
		Name:            "ais-logs",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{logFile},
		VolumeMounts:    []corev1.VolumeMount{newLogsVolumeMount(daeType)},
		Env:             []corev1.EnvVar{EnvFromFieldPath(EnvPodName, "metadata.name")},
	}
}

func NewInitContainerArgs(daeType string, hostnameMap map[string]string) []string {
	args := []string{
		"-role=" + daeType,
		"-local_config_template=" + path.Join(InitConfTemplateDir, AISLocalConfigName),
		"-output_local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
		"-cluster_config_override=" + path.Join(InitGlobalConfDir, AISGlobalConfigName),
		"-output_cluster_config=" + path.Join(AisConfigDir, AISGlobalConfigName),
	}
	if len(hostnameMap) != 0 {
		args = append(args, "-hostname_map_file="+path.Join(InitGlobalConfDir, hostnameMapFileName))
	}
	return args
}

func NewAISContainerArgs(targetSize int32, daeType string) []string {
	args := []string{
		"-config=" + path.Join(AisConfigDir, AISGlobalConfigName),
		"-local_config=" + path.Join(AisConfigDir, AISLocalConfigName),
		"-role=" + daeType,
	}
	if daeType == aisapc.Proxy {
		args = append(args, fmt.Sprintf("-ntargets=%d", targetSize))
	}
	return args
}

func NewInitResourceReq() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// Init uses 3 mounts for templates and the final output -- to be safe, request space for all 3
			corev1.ResourceEphemeralStorage: *resource.NewQuantity(DefaultConfigStorageReq*3, resource.BinarySI),
		},
	}
}

func NewResourceReq(ais *aisv1.AIStore, reqs *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	if reqs.Requests.StorageEphemeral() != nil && !reqs.Requests.StorageEphemeral().IsZero() {
		return reqs
	}
	if reqs.Requests == nil {
		reqs.Requests = corev1.ResourceList{}
	}
	// Reserve at least enough for max total logs + generated config from init + container images etc.
	storageBytes := DefaultLogsStorageReq
	if ais.MaxLogTotal() != nil {
		storageBytes = int64(*ais.MaxLogTotal())
	}
	storageBytes = storageBytes + DefaultConfigStorageReq + DefaultMiscStorageReq
	reqs.Requests[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(storageBytes, resource.BinarySI)
	return reqs
}

func IsStatefulSetReady(ss *appsv1.StatefulSet, desired int32) bool {
	if *ss.Spec.Replicas != desired {
		return false
	}
	if ss.Status.UpdateRevision != "" && ss.Status.CurrentRevision != ss.Status.UpdateRevision {
		return false
	}
	if desired != ss.Status.Replicas {
		return false
	}
	if desired != ss.Status.CurrentReplicas {
		return false
	}
	return desired == ss.Status.ReadyReplicas
}
