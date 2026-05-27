// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisenv "github.com/NVIDIA/aistore/api/env"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.ProxyStatefulSetName(),
		Namespace: ais.Namespace,
	}
}

func PodName(ais *aisv1.AIStore, idx int32) string {
	return fmt.Sprintf("%s-%d", ais.ProxyStatefulSetName(), idx)
}

// BasicLabels defines labels for proxy pods and statefulset
// Includes legacy labels for compatibility with older StatefulSets that may still select on
// non-prefixed 'app' and 'component' labels
func BasicLabels(ais *aisv1.AIStore) map[string]string {
	return cmn.LegacyLabels(ais.Name, aisapc.Proxy)
}

func SelectorLabels(ais *aisv1.AIStore) map[string]string {
	return cmn.SelectorLabels(ais.Name, aisapc.Proxy)
}

func DefaultPrimaryNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.DefaultPrimaryName(),
		Namespace: ais.Namespace,
	}
}

func NewProxyStatefulSet(ais *aisv1.AIStore, size int32) *apiv1.StatefulSet {
	basicLabels := BasicLabels(ais)
	podLabels := cmn.MergePodLabels(ais.Spec.ProxySpec.Labels, basicLabels)

	ss := &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ais.ProxyStatefulSetName(),
			Namespace: ais.Namespace,
			Labels:    basicLabels,
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: SelectorLabels(ais),
			},
			ServiceName:          headlessSVCName(ais.Name),
			PodManagementPolicy:  apiv1.ParallelPodManagement,
			Replicas:             &size,
			VolumeClaimTemplates: proxyVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: cmn.PrepareAnnotations(ais.Spec.ProxySpec.Annotations, ais.Spec.NetAttachment, aisapc.Ptr(ais.Annotations[cmn.RestartConfigHashAnnotation])),
				},
				Spec: *proxyPodSpec(ais),
			},
		},
	}
	if ais.Spec.ProxySpec.PVCRetentionPolicy != nil {
		ss.Spec.PersistentVolumeClaimRetentionPolicy = ais.Spec.ProxySpec.PVCRetentionPolicy
	}
	return ss
}

/////////////////
//   helpers  //
////////////////

func proxyPodSpec(ais *aisv1.AIStore) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            cmn.InitContainerName,
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullAlways,
				Env:             NewInitContainerEnv(ais),
				Resources:       *cmn.NewInitResourceReq(),
				Args:            cmn.NewInitContainerArgs(aisapc.Proxy, ais.Spec.HostnameMap),
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
				Args:            cmn.NewAISContainerArgs(ais.GetTargetSize(), aisapc.Proxy),
				Env:             NewAISContainerEnv(ais),
				Ports:           cmn.NewDaemonPorts(&ais.Spec.ProxySpec),
				Resources:       *cmn.NewResourceReq(ais, &ais.Spec.ProxySpec.Resources),
				SecurityContext: cmn.GetAISSecurityContext(&ais.Spec.ProxySpec),
				VolumeMounts:    newVolumeMounts(ais),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Proxy),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Proxy),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Proxy),
			},
		},
		Affinity:           cmn.CreateAISAffinity(ais.Spec.ProxySpec.Affinity, SelectorLabels(ais)),
		NodeSelector:       ais.Spec.ProxySpec.NodeSelector,
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    cmn.GetPodSecurityContext(&ais.Spec.ProxySpec),
		Volumes:            newVolumes(ais),
		Tolerations:        ais.Spec.ProxySpec.Tolerations,
	}
	// Apply priority class if specified to prevent eviction during node pressure
	if ais.Spec.PriorityClassName != nil {
		spec.PriorityClassName = *ais.Spec.PriorityClassName
	}
	if ais.GetLogSidecarImage() != "" {
		spec.Containers = append(spec.Containers, cmn.NewLogSidecar(ais, aisapc.Proxy))
	}
	return spec
}

func NewInitContainerEnv(ais *aisv1.AIStore) (initEnv []corev1.EnvVar) {
	initEnv = cmn.CommonInitEnv(ais)
	initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais.Name)))
	if ais.Spec.ProxySpec.HostPort != nil {
		if ais.UseNodeNameForPublicNet() {
			initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "spec.nodeName"))
		} else {
			initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
		}
	}
	return
}

func NewAISContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	baseEnv := cmn.CommonEnv()
	if ais.Spec.AuthNSecretName != nil {
		baseEnv = append(baseEnv, cmn.EnvFromSecret(aisenv.AisAuthSecretKey, *ais.Spec.AuthNSecretName, cmn.EnvAuthNSecretKey))
	}
	return cmn.MergeEnvVars(baseEnv, ais.Spec.ProxySpec.Env)
}

func proxyVC(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	if ais.Spec.StateStorageClass != nil {
		if statePVC := cmn.DefineStatePVC(ais, ais.Spec.StateStorageClass); statePVC != nil {
			return []corev1.PersistentVolumeClaim{*statePVC}
		}
	}
	return nil
}
