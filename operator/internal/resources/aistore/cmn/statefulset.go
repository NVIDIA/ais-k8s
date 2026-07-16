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
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	InitContainerName       = "populate-env"
	AISContainerName        = "ais-node"
	LabelApp                = "app"
	LabelComponent          = "component"
	LabelPrefix             = "app.kubernetes.io/"
	LabelAppPrefixed        = LabelPrefix + "name"
	LabelComponentPrefixed  = LabelPrefix + "component"
	LabelManagedBy          = LabelPrefix + "managed-by"
	LabelManagedByValue     = "ais-operator"
	DefaultConfigStorageReq = int64(16 * aiscos.MiB)
	DefaultLogsStorageReq   = int64(512 * aiscos.MiB)
	DefaultMiscStorageReq   = int64(128 * aiscos.MiB)
)

// DefaultPodSecurityContext returns the default pod-level SecurityContext shared by all
// containers in the pod (init, aisnode, log sidecar).
func DefaultPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		// aisnode and log-sidecar images currently run as root
		RunAsNonRoot:   aisapc.Ptr(false),
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// DefaultAISContainerSecurityContext returns the default container-level SecurityContext for
// the primary aisnode container.
func DefaultAISContainerSecurityContext() *corev1.SecurityContext {
	// The main AIS container requires writing to the internal root filesystem as of v4.6
	// so ReadOnlyRootFilesystem must remain false
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: aisapc.Ptr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// RestrictedSecurityContext returns the stricter container-level default used by the init
// and log-sidecar containers.
func RestrictedSecurityContext() *corev1.SecurityContext {
	sc := DefaultAISContainerSecurityContext()
	sc.ReadOnlyRootFilesystem = aisapc.Ptr(true)
	return sc
}

// GetPodSecurityContext resolves the pod-level SecurityContext for a DaemonSpec, falling
// back to DefaultPodSecurityContext when unset.
func GetPodSecurityContext(s *aisv1.DaemonSpec) *corev1.PodSecurityContext {
	if s.SecurityContext != nil {
		return s.SecurityContext
	}
	return DefaultPodSecurityContext()
}

// GetAISSecurityContext resolves the primary AIS container SecurityContext for a DaemonSpec.
// Precedence: AISContainerSecurityContext > deprecated Capabilities > DefaultAISContainerSecurityContext.
func GetAISSecurityContext(s *aisv1.DaemonSpec) *corev1.SecurityContext {
	if s.AISContainerSecurityContext != nil {
		return s.AISContainerSecurityContext
	}
	if s.Capabilities != nil { //nolint:staticcheck // backwards compatibility for deprecated Capabilities field
		return s.Capabilities //nolint:staticcheck // deprecated Capabilities field
	}
	return DefaultAISContainerSecurityContext()
}

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
func NewLogSidecar(ais *aisv1.AIStore, daeType string) corev1.Container {
	logFile := filepath.Join(LogsDir, fmt.Sprintf("ais%s.INFO", daeType))
	container := corev1.Container{
		Name:            "ais-logs",
		Image:           ais.GetLogSidecarImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{logFile},
		VolumeMounts:    []corev1.VolumeMount{newLogsVolumeMount(daeType)},
		Env:             []corev1.EnvVar{EnvFromFieldPath(EnvPodName, "metadata.name")},
		SecurityContext: RestrictedSecurityContext(),
	}
	if resources := ais.GetLogSidecarResources(); resources != nil {
		container.Resources = *resources
	}
	return container
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

// NewInitResourceReq returns fixed resource requirements for the init container.
// CPU and memory requests/limits are set to equal values (1 CPU, 1Gi memory) to ensure
// Guaranteed QoS class when the main container also has matching requests and limits.
func NewInitResourceReq() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("1"),
			corev1.ResourceMemory:           resource.MustParse("1Gi"),
			corev1.ResourceEphemeralStorage: *resource.NewQuantity(DefaultConfigStorageReq*3, resource.BinarySI),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
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
