// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	"fmt"
	"path"
	"strconv"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/api/env"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/statsd"
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
					Annotations: cmn.PrepareAnnotations(ais.Spec.ProxySpec.Annotations, ais.Spec.NetAttachment),
				},
				Spec: proxyPodSpec(ais),
			},
		},
	}
}

/////////////////
//   helpers  //
////////////////

func proxyPodSpec(ais *aisv1.AIStore) corev1.PodSpec {
	var optionals []corev1.EnvVar
	if ais.Spec.ProxySpec.HostPort != nil {
		optionals = []corev1.EnvVar{
			cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"),
		}
	}
	if ais.Spec.GCPSecretName != nil {
		// TODO -- FIXME: Remove hardcoding for path
		optionals = append(optionals, cmn.EnvFromValue(cmn.EnvGCPCredsPath, "/var/gcp/gcp.json"))
	}
	if ais.UseHTTPS() {
		optionals = append(optionals, cmn.EnvFromValue(cmn.EnvUseHTTPS, "true"))
	}

	if ais.Spec.AuthNSecretName != nil {
		optionals = append(optionals, cmn.EnvFromSecret(env.AuthN.SecretKey, *ais.Spec.AuthNSecretName, cmn.EnvAuthNSecretKey))
	}

	return corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            "populate-env",
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env: append([]corev1.EnvVar{
					cmn.EnvFromFieldPath(cmn.EnvNodeName, "spec.nodeName"),
					cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
					cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
					cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)),
					cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
					cmn.EnvFromValue(cmn.EnvDefaultPrimaryPod, ais.DefaultPrimaryName()),
				}, optionals...),
				Args:         cmn.NewInitContainerArgs(aisapc.Proxy, ais.Spec.HostnameMap),
				VolumeMounts: cmn.NewInitVolumeMounts(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "ais-node",
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Command:         []string{"aisnode"},
				Args:            cmn.NewAISContainerArgs(ais, aisapc.Proxy),
				Env: cmn.MergeEnvVars(append([]corev1.EnvVar{
					cmn.EnvFromFieldPath(cmn.EnvNodeName, "spec.nodeName"),
					cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
					cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
					cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
					cmn.EnvFromValue(cmn.EnvShutdownMarkerPath, cmn.AisConfigDir),
					cmn.EnvFromValue(cmn.EnvCIDR, ""), // TODO: Should take from specs
					cmn.EnvFromValue(cmn.EnvConfigFilePath, path.Join(cmn.AisConfigDir, cmn.AISGlobalConfigName)),
					cmn.EnvFromValue(cmn.EnvLocalConfigFilePath, path.Join(cmn.AisConfigDir, cmn.AISLocalConfigName)),
					cmn.EnvFromValue(cmn.EnvStatsDConfig, path.Join(cmn.StatsDDir, statsd.ConfigFile)),
					cmn.EnvFromValue(cmn.EnvEnablePrometheus,
						strconv.FormatBool(ais.Spec.EnablePromExporter != nil && *ais.Spec.EnablePromExporter)),
					cmn.EnvFromValue(cmn.EnvNumTargets, strconv.Itoa(int(ais.GetTargetSize()))),
				}, optionals...), ais.Spec.ProxySpec.Env),
				Ports:           cmn.NewDaemonPorts(&ais.Spec.ProxySpec),
				Resources:       ais.Spec.ProxySpec.Resources,
				SecurityContext: ais.Spec.ProxySpec.ContainerSecurity,
				VolumeMounts:    cmn.NewAISVolumeMounts(ais, aisapc.Proxy),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Proxy),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Proxy),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Proxy),
			},
			cmn.NewLogSidecar(aisapc.Proxy),
		},
		Affinity:           cmn.CreateAISAffinity(ais.Spec.ProxySpec.Affinity, PodLabels(ais)),
		NodeSelector:       ais.Spec.ProxySpec.NodeSelector,
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    ais.Spec.ProxySpec.SecurityContext,
		Volumes:            cmn.NewAISVolumes(ais, aisapc.Proxy),
		Tolerations:        ais.Spec.ProxySpec.Tolerations,
		ImagePullSecrets:   ais.Spec.ImagePullSecrets,
	}
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
