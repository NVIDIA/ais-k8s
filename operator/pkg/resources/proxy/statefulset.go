// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package proxy

import (
	"fmt"
	"strconv"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func statefulSetName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Proxy
}

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      statefulSetName(ais),
		Namespace: ais.Namespace,
	}
}

func PodName(ais *aisv1.AIStore, idx int32) string {
	return fmt.Sprintf("%s-%d", statefulSetName(ais), idx)
}

// DefaultPrimaryName returns name of pod used as default Primary
func DefaultPrimaryName(ais *aisv1.AIStore) string {
	return statefulSetName(ais) + "-0"
}

func DefaultPrimaryNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      DefaultPrimaryName(ais),
		Namespace: ais.Namespace,
	}
}

func NewProxyStatefulSet(ais *aisv1.AIStore, size int32) *apiv1.StatefulSet {
	ls := PodLabels(ais)
	proxySpec := proxyPodSpec(ais)
	return &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName(ais),
			Namespace: ais.Namespace,
			Labels:    ls,
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			ServiceName:         HeadlessSVCName(ais),
			PodManagementPolicy: apiv1.ParallelPodManagement,
			Replicas:            &size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: proxySpec,
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

	return corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            "populate-env",
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env: append([]corev1.EnvVar{
					cmn.EnvFromFieldPath(cmn.EnvNodeName, "spec.nodeName"),
					cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
					cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
					cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
					cmn.EnvFromValue(cmn.EnvServiceName, HeadlessSVCName(ais)),
					cmn.EnvFromValue(cmn.EnvDaemonRole, aisapc.Proxy),
					cmn.EnvFromValue(cmn.EnvProxyServiceName, HeadlessSVCName(ais)),
					cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
					cmn.EnvFromValue(cmn.EnvDefaultPrimaryPod, DefaultPrimaryName(ais)),
				}, optionals...),
				Args:         []string{"-c", "/bin/bash /var/ais_config_template/set_initial_primary_proxy_env.sh"},
				Command:      []string{"/bin/bash"},
				VolumeMounts: cmn.NewInitVolumeMounts(ais.Spec.DisablePodAntiAffinity),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "ais-node",
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Env: append([]corev1.EnvVar{
					cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
					cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
					cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
					cmn.EnvFromValue(cmn.EnvShutdownMarkerPath, "/var/ais_config"),
					cmn.EnvFromValue(cmn.EnvCIDR, ""), // TODO: Should take from specs
					cmn.EnvFromValue(cmn.ENVConfigFilePath, "/var/ais_config/ais.json"),
					cmn.EnvFromValue(cmn.ENVLocalConfigFilePath, "/var/ais_config/ais_local.json"),
					cmn.EnvFromValue(cmn.EnvStatsDConfig, "/var/statsd_config/statsd.json"),
					cmn.EnvFromValue(cmn.EnvEnablePrometheus,
						strconv.FormatBool(ais.Spec.EnablePromExporter != nil && *ais.Spec.EnablePromExporter)),
					cmn.EnvFromValue(cmn.EnvDaemonRole, aisapc.Proxy),
					cmn.EnvFromValue(cmn.EnvNumTargets, strconv.Itoa(int(ais.Spec.Size))),
					cmn.EnvFromValue(cmn.EnvProxyServiceName, HeadlessSVCName(ais)),
					cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
					cmn.EnvFromValue(cmn.EnvNodeServicePort, ais.Spec.ProxySpec.PublicPort.String()),
				}, optionals...),
				Ports:           cmn.NewDaemonPorts(ais.Spec.ProxySpec),
				SecurityContext: ais.Spec.ProxySpec.ContainerSecurity,
				VolumeMounts:    cmn.NewAISVolumeMounts(ais),
				Lifecycle:       cmn.NewAISNodeLifecycle(),
				LivenessProbe:   cmn.NewAISLivenessProbe(),
				ReadinessProbe:  readinessProbe(),
			},
		},
		Affinity:           cmn.NewAISPodAffinity(ais, ais.Spec.ProxySpec.Affinity, PodLabels(ais)),
		NodeSelector:       ais.Spec.ProxySpec.NodeSelector,
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    ais.Spec.ProxySpec.SecurityContext,
		Volumes:            cmn.NewAISVolumes(ais, aisapc.Proxy),
		Tolerations:        ais.Spec.ProxySpec.Tolerations,
	}
}

func PodLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		"app":       ais.Name,
		"component": aisapc.Proxy,
		"function":  "gateway",
	}
}

func readinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", "/var/ais_config/ais_readiness.sh"},
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
	}
}
