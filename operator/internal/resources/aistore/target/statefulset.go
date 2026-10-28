/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */

package target

import (
	"path/filepath"
	"strconv"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/ownerref"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.TargetStatefulSetName(),
		Namespace: ais.Namespace,
	}
}

// BasicLabels defines labels for target pods and statefulset
// Includes legacy labels for compatibility with older StatefulSets that may still select on
// non-prefixed 'app' and 'component' labels
func BasicLabels(ais *aisv1.AIStore) map[string]string {
	return cmn.LegacyLabels(ais.Name, aisapc.Target)
}

func SelectorLabels(ais *aisv1.AIStore) map[string]string {
	return cmn.SelectorLabels(ais.Name, aisapc.Target)
}

func NewTargetSS(ais *aisv1.AIStore, expectedSize int32) *apiv1.StatefulSet {
	basicLabels := BasicLabels(ais)
	basicLabels[ownerref.ManagedByLabel] = ownerref.ManagedByValue
	podLabels := cmn.MergePodLabels(ais.Spec.TargetSpec.Labels, basicLabels)

	ss := &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ais.TargetStatefulSetName(),
			Namespace:   ais.Namespace,
			Labels:      basicLabels,
			Annotations: map[string]string{cmn.RestartConfigHashAnnotation: ais.Annotations[cmn.RestartConfigHashAnnotation]},
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: SelectorLabels(ais),
			},
			ServiceName:         ais.TargetHeadlessSVCNSName().Name,
			PodManagementPolicy: apiv1.ParallelPodManagement,
			Replicas:            &expectedSize,
			UpdateStrategy: apiv1.StatefulSetUpdateStrategy{
				Type: apiv1.OnDeleteStatefulSetStrategyType,
			},
			VolumeClaimTemplates: newTargetPVCs(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: cmn.PrepareAnnotations(ais.Spec.TargetSpec.Annotations, ais.Spec.NetAttachment, aisapc.Ptr(ais.Annotations[cmn.RestartConfigHashAnnotation])),
				},
				Spec: *targetPodSpec(ais),
			},
		},
	}
	if ais.Spec.TargetSpec.PVCRetentionPolicy != nil {
		ss.Spec.PersistentVolumeClaimRetentionPolicy = ais.Spec.TargetSpec.PVCRetentionPolicy
	}
	return ss
}

func targetPodSpec(ais *aisv1.AIStore) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            cmn.InitContainerName,
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullAlways,
				Env:             NewInitContainerEnv(ais),
				Resources:       *cmn.NewInitResourceReq(),
				Args:            cmn.NewInitContainerArgs(aisapc.Target, ais.Spec.HostnameMap),
				VolumeMounts:    cmn.NewInitVolumeMounts(),
				SecurityContext: cmn.RestrictedSecurityContext(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            cmn.AISContainerName,
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Command:         []string{"aisnode"},
				Args:            cmn.NewAISContainerArgs(ais.GetTargetSize(), aisapc.Target),
				Env:             NewAISContainerEnv(ais),
				Ports:           cmn.NewDaemonPorts(ais.Spec.TargetSpec.HostPort, ais.TargetPublicPort()),
				Resources:       *cmn.NewResourceReq(ais, &ais.Spec.TargetSpec.Resources),
				SecurityContext: cmn.GetAISSecurityContext(&ais.Spec.TargetSpec.DaemonSpec),
				VolumeMounts:    newVolumeMounts(ais),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Target),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Target),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Target),
			},
		},
		HostNetwork:        ais.UseHostNetwork(),
		DNSPolicy:          ais.GetTargetDNSPolicy(),
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    cmn.GetPodSecurityContext(&ais.Spec.TargetSpec.DaemonSpec),
		Affinity:           createTargetAffinity(ais, SelectorLabels(ais)),
		NodeSelector:       ais.Spec.TargetSpec.NodeSelector,
		Volumes:            newVolumes(ais),
		Tolerations:        ais.Spec.TargetSpec.Tolerations,
	}
	// Apply priority class if specified to prevent eviction during node pressure
	if ais.Spec.PriorityClassName != nil {
		spec.PriorityClassName = *ais.Spec.PriorityClassName
	}
	if ais.GetLogSidecarImage() != "" {
		spec.Containers = append(spec.Containers, cmn.NewLogSidecar(ais, aisapc.Target))
	}
	return spec
}

func NewInitContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	initEnv := append(cmn.CommonInitEnv(ais), cmn.EnvFromValue(cmn.EnvServiceName, ais.TargetHeadlessSVCNSName().Name))
	if domain := ais.TargetPublicDomain(); domain != "" {
		return append(initEnv, publicDomainEnv(domain)...)
	}
	return append(initEnv, publicNetEnv(ais)...)
}

// publicDomainEnv makes aisinit advertise <pod>.<domain> as the target's public hostname.
func publicDomainEnv(domain string) []corev1.EnvVar {
	hostname := "$(" + cmn.EnvPodName + ")." + domain
	return []corev1.EnvVar{
		// If the public domain option is set, this tells AIS to use public domain DNS for target redirects.
		// AIS versions before 5.2 ignore AIS_PUBLIC_HOSTNAME_OVERRIDE. For them, aisinit replaces the public
		// hostname with the LoadBalancer IP if `ENABLE_EXTERNAL_ACCESS` is true, so force the pod hostname
		// through AIS_PUBLIC_HOSTNAME and skip that replacement.
		cmn.EnvFromValue(cmn.EnvEnableExternalAccess, "false"),
		cmn.EnvFromValue(cmn.EnvPublicDNSMode, string(aisv1.PubNetDNSModeIP)),
		cmn.EnvFromValue(cmn.EnvPublicHostname, hostname),
		cmn.EnvFromValue(cmn.EnvPublicHostnameOverride, hostname),
	}
}

// publicNetEnv makes aisinit advertise the public hostname that the spec networking selects.
func publicNetEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	externalAccess := ais.TargetExternalAccessEnabled()
	env := append([]corev1.EnvVar{cmn.EnvFromValue(cmn.EnvEnableExternalAccess, strconv.FormatBool(externalAccess))},
		cmn.PublicDNSModeEnv(ais)...)
	// With external access, aisinit advertises the LoadBalancer IP instead.
	if ais.Spec.TargetSpec.HostPort != nil && !externalAccess {
		env = append(env, cmn.HostPortHostnameEnv(ais))
	}
	if ais.UseHostNetwork() {
		env = append(env, cmn.EnvFromValue(cmn.EnvHostNetwork, "true"))
	}
	return env
}

func NewAISContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	baseEnv := cmn.CommonEnv(ais)
	if ais.Spec.HasGCPBackend() {
		baseEnv = append(baseEnv, cmn.EnvFromValue(cmn.EnvGoogleCreds, filepath.Join(DefaultGCPDir, DefaultGCPConfig)))
	}
	if ais.Spec.HasOCIBackend() {
		baseEnv = append(baseEnv,
			cmn.EnvFromValue(cmn.EnvOCIConfig, filepath.Join(DefaultOCIDir, DefaultOCIConfig)),
		)
	}
	return cmn.MergeEnvVars(baseEnv, ais.Spec.TargetSpec.Env)
}

func createTargetAffinity(ais *aisv1.AIStore, labels map[string]string) *corev1.Affinity {
	// Don't add additional rules to the affinity set in the target spec (can also be nil)
	if ais.AllowTargetSharedNodes() {
		return ais.Spec.TargetSpec.Affinity
	}
	return cmn.CreateAISAffinity(ais.Spec.TargetSpec.Affinity, labels)
}
