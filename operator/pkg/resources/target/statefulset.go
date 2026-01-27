// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"maps"
	"path/filepath"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func statefulSetName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Target
}

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      statefulSetName(ais),
		Namespace: ais.Namespace,
	}
}

func BasicLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		cmn.LabelApp:               ais.Name,
		cmn.LabelAppPrefixed:       ais.Name,
		cmn.LabelComponent:         aisapc.Target,
		cmn.LabelComponentPrefixed: aisapc.Target,
	}
}

// RequiredPodLabels contains backwards compatible pod labels for selecting pods on older clusters
// TODO: Remove in release 3.0
func RequiredPodLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		cmn.LabelApp:       ais.Name,
		cmn.LabelComponent: aisapc.Target,
	}
}

func NewTargetSS(ais *aisv1.AIStore, expectedSize int32) *apiv1.StatefulSet {
	basicLabels := BasicLabels(ais)
	podLabels := map[string]string{}
	maps.Copy(podLabels, BasicLabels(ais))
	maps.Copy(podLabels, ais.Spec.TargetSpec.Labels)

	return &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        statefulSetName(ais),
			Namespace:   ais.Namespace,
			Labels:      basicLabels,
			Annotations: map[string]string{cmn.RestartConfigHashAnnotation: ais.Annotations[cmn.RestartConfigHashAnnotation]},
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: basicLabels,
			},
			ServiceName:         headlessSVCName(ais.Name),
			PodManagementPolicy: apiv1.ParallelPodManagement,
			Replicas:            &expectedSize,
			UpdateStrategy: apiv1.StatefulSetUpdateStrategy{
				Type: apiv1.OnDeleteStatefulSetStrategyType,
			},
			VolumeClaimTemplates: targetPVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: cmn.PrepareAnnotations(ais.Spec.TargetSpec.Annotations, ais.Spec.NetAttachment, aisapc.Ptr(ais.Annotations[cmn.RestartConfigHashAnnotation])),
				},
				Spec: *targetPodSpec(ais),
			},
		},
	}
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
				Ports:           cmn.NewDaemonPorts(&ais.Spec.TargetSpec.DaemonSpec),
				Resources:       *cmn.NewResourceReq(ais, &ais.Spec.TargetSpec.Resources),
				SecurityContext: ais.Spec.TargetSpec.ContainerSecurity,
				VolumeMounts:    newVolumeMounts(ais),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Target),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Target),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Target),
			},
		},
		HostNetwork:        ais.UseHostNetwork(),
		DNSPolicy:          ais.GetTargetDNSPolicy(),
		ServiceAccountName: cmn.ServiceAccountName(ais),
		// By default, Kubernetes sets non-nil `SecurityContext`. So we have do that too,
		// otherwise during comparison we will always fail (nil vs non-nil).
		//
		// See: https://github.com/kubernetes/kubernetes/blob/fa03b93d25a5a22d4f91e4c44f66fc69a6f69a35/pkg/apis/core/v1/defaults.go#L215-L236
		SecurityContext: cmn.ValueOrDefault(ais.Spec.TargetSpec.SecurityContext, &corev1.PodSecurityContext{}),
		Affinity:        createTargetAffinity(ais, BasicLabels(ais)),
		NodeSelector:    ais.Spec.TargetSpec.NodeSelector,
		Volumes:         newVolumes(ais),
		Tolerations:     ais.Spec.TargetSpec.Tolerations,
	}
	// Apply priority class if specified to prevent eviction during node pressure
	if ais.Spec.PriorityClassName != nil {
		spec.PriorityClassName = *ais.Spec.PriorityClassName
	}
	if ais.Spec.LogSidecarImage != nil {
		spec.Containers = append(spec.Containers, cmn.NewLogSidecar(*ais.Spec.LogSidecarImage, aisapc.Target, ais.Spec.LogSidecarResources))
	}
	return spec
}

func NewInitContainerEnv(ais *aisv1.AIStore) (initEnv []corev1.EnvVar) {
	initEnv = cmn.CommonInitEnv(ais)
	initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais.Name)))
	if ais.Spec.TargetSpec.HostPort != nil {
		if ais.UseNodeNameForPublicNet() {
			initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "spec.nodeName"))
		} else {
			initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
		}
	}
	if ais.UseHostNetwork() {
		initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvHostNetwork, "true"))
	}
	return
}

func NewAISContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	baseEnv := cmn.CommonEnv()
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

func createTargetAffinity(ais *aisv1.AIStore, basicLabels map[string]string) *corev1.Affinity {
	// Don't add additional rules to the affinity set in the target spec (can also be nil)
	if ais.AllowTargetSharedNodes() {
		return ais.Spec.TargetSpec.Affinity
	}
	return cmn.CreateAISAffinity(ais.Spec.TargetSpec.Affinity, basicLabels)
}
