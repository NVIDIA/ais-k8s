// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/api/env"
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

func DefaultPrimaryNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      ais.DefaultPrimaryName(),
		Namespace: ais.Namespace,
	}
}

func NewProxyStatefulSet(ais *aisv1.AIStore, size int32) *apiv1.StatefulSet {
	ls := PodLabels(ais)
	return &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ais.ProxyStatefulSetName(),
			Namespace: ais.Namespace,
			Labels:    ls,
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			ServiceName:          headlessSVCName(ais),
			PodManagementPolicy:  apiv1.ParallelPodManagement,
			Replicas:             &size,
			VolumeClaimTemplates: proxyVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ls,
					Annotations: cmn.PrepareAnnotations(ais.Spec.ProxySpec.Annotations, ais.Spec.NetAttachment, aisapc.Ptr(ais.Annotations[cmn.RestartConfigHashAnnotation])),
				},
				Spec: *proxyPodSpec(ais),
			},
		},
	}
}

/////////////////
//   helpers  //
////////////////

func proxyPodSpec(ais *aisv1.AIStore) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            "populate-env",
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env:             NewInitContainerEnv(ais),
				Args:            cmn.NewInitContainerArgs(aisapc.Proxy, ais.Spec.HostnameMap),
				VolumeMounts:    cmn.NewInitVolumeMounts(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "ais-node",
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Command:         []string{"aisnode"},
				Args:            cmn.NewAISContainerArgs(ais.GetTargetSize(), aisapc.Proxy),
				Env:             NewAISContainerEnv(ais),
				Ports:           cmn.NewDaemonPorts(&ais.Spec.ProxySpec),
				Resources:       ais.Spec.ProxySpec.Resources,
				SecurityContext: ais.Spec.ProxySpec.ContainerSecurity,
				VolumeMounts:    cmn.NewAISVolumeMounts(ais, aisapc.Proxy),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Proxy),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Proxy),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Proxy),
			},
		},
		Affinity:           cmn.CreateAISAffinity(ais.Spec.ProxySpec.Affinity, PodLabels(ais)),
		NodeSelector:       ais.Spec.ProxySpec.NodeSelector,
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    ais.Spec.ProxySpec.SecurityContext,
		Volumes:            cmn.NewAISVolumes(ais, aisapc.Proxy),
		Tolerations:        ais.Spec.ProxySpec.Tolerations,
		ImagePullSecrets:   ais.Spec.ImagePullSecrets,
	}
	if ais.Spec.LogSidecarImage != nil {
		spec.Containers = append(spec.Containers, cmn.NewLogSidecar(*ais.Spec.LogSidecarImage, aisapc.Proxy))
	}
	return spec
}

func NewInitContainerEnv(ais *aisv1.AIStore) (initEnv []corev1.EnvVar) {
	initEnv = cmn.CommonInitEnv(ais)
	initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)))
	if ais.Spec.ProxySpec.HostPort != nil {
		initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
	}
	return
}

func NewAISContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	baseEnv := cmn.CommonEnv()
	if ais.Spec.ProxySpec.HostPort != nil {
		baseEnv = append(baseEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
	}
	if ais.Spec.AuthNSecretName != nil {
		baseEnv = append(baseEnv, cmn.EnvFromSecret(env.AisAuthSecretKey, *ais.Spec.AuthNSecretName, cmn.EnvAuthNSecretKey))
	}
	return cmn.MergeEnvVars(baseEnv, ais.Spec.ProxySpec.Env)
}

func PodLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		"app":       ais.Name,
		"component": aisapc.Proxy,
		"function":  "gateway",
	}
}

func proxyVC(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	if ais.Spec.StateStorageClass != nil {
		if statePVC := cmn.DefineStatePVC(ais, ais.Spec.StateStorageClass); statePVC != nil {
			return []corev1.PersistentVolumeClaim{*statePVC}
		}
	}
	return nil
}
