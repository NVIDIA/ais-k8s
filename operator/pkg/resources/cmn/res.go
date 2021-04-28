// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	aisv1 "github.com/ais-operator/api/v1beta1"
)

func NewAISVolumes(ais *aisv1.AIStore, daeType string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "config-mount",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "config-template-mount",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ais.Name + "-" + daeType,
					},
				},
			},
		},
		{
			Name: "config-global",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: globalConfigMapName(ais),
					},
				},
			},
		},
		{
			Name: "env-mount",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path.Join(ais.Spec.HostpathPrefix, ais.Namespace, daeType+"_env"),
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "state-mount",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path.Join(ais.Spec.HostpathPrefix, ais.Namespace, daeType),
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "statsd-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ais.Name + "-statsd",
					},
				},
			},
		},
	}
}

func NewAISLivenessProbe(port intstr.IntOrString) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", "/var/ais_config/ais_liveness.sh"},
			},
		},
		InitialDelaySeconds: 90,
		PeriodSeconds:       5,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
	}
}

func NewAISNodeLifecycle() *corev1.Lifecycle {
	return &corev1.Lifecycle{
		PreStop: &corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", "-c", "/usr/bin/pkill -SIGINT aisnode"},
			},
		},
	}
}

func NewAISVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config-mount",
			MountPath: "/var/ais_config",
		},
		{
			Name:      "config-global",
			MountPath: "/var/ais_config/ais.json",
			SubPath:   "ais.json",
		},
		{
			Name:      "config-global",
			MountPath: "/var/ais_config/ais_liveness.sh",
			SubPath:   "ais_liveness.sh",
		},
		{
			Name:      "config-global",
			MountPath: "/var/ais_config/ais_readiness.sh",
			SubPath:   "ais_readiness.sh",
		},
		{
			Name:        "env-mount",
			MountPath:   "/var/ais_env",
			SubPathExpr: "$(MY_POD)",
		},
		{
			Name:        "state-mount",
			MountPath:   "/etc/ais",
			SubPathExpr: "$(MY_POD)",
		},
		{
			Name:      "statsd-config",
			MountPath: "/var/statsd_config",
		},
	}
}

func NewInitVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config-mount",
			MountPath: "/var/ais_config",
		},
		{
			Name:      "config-template-mount",
			MountPath: "/var/ais_config_template",
		},
		{
			Name:        "env-mount",
			MountPath:   "/var/ais_env",
			SubPathExpr: "$(MY_POD)",
		},
	}
}

func NewDaemonPorts(spec aisv1.DaemonSpec) []corev1.ContainerPort {
	var hostPort int32
	if spec.HostPort != nil {
		hostPort = *spec.HostPort
	}
	return []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: int32(spec.ServicePort.IntValue()),
			Protocol:      corev1.ProtocolTCP,
			HostPort:      hostPort,
		},
	}
}

func NewAISPodAffinity(ais *aisv1.AIStore, affinity *corev1.Affinity, podLabels map[string]string) *corev1.Affinity {
	var (
		antiAffinityDisabled = IsBoolSet(ais.Spec.DisablePodAntiAffinity)
		antiAffinity         *corev1.PodAntiAffinity
	)
	if affinity == nil && antiAffinityDisabled {
		return nil
	}

	if !antiAffinityDisabled {
		antiAffinity = &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: podLabels,
					},
					TopologyKey: corev1.LabelHostname,
				},
			},
		}
	}

	if affinity == nil {
		return &corev1.Affinity{
			PodAntiAffinity: antiAffinity,
		}
	}

	if affinity.PodAntiAffinity == nil {
		affinity.PodAntiAffinity = antiAffinity
	}
	return affinity
}

func hostPathTypePtr(v corev1.HostPathType) *corev1.HostPathType {
	return &v
}
