// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
	"path"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	configVolume         = "config-mount"
	configGlobalVolume   = "config-global"
	configTemplateVolume = "config-template"
	envVolume            = "env-mount"
	stateVolume          = "state-mount"
	awsSecretVolume      = "aws-creds"
	gcpSecretVolume      = "gcp-creds"
	tlsSecretVolume      = "tls-certs"
	logsVolume           = "logs-dir"

	configTemplateDir = "/var/ais_config_template"
	globalConfigDir   = "/var/global_config"
	aisConfigDir      = "/var/ais_config"

	aisGlobalConfigFileName = "ais.json"
	aisLocalConfigName      = "ais_local.json"
	hostnameMapFileName     = "hostname_map.json"
)

func NewAISVolumes(ais *aisv1.AIStore, daeType string) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: configVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: configTemplateVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ais.Name + "-" + daeType,
					},
				},
			},
		},
		{
			Name: configGlobalVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: globalConfigMapName(ais),
					},
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

	// Only create hostpath volumes if no storage class is provided for state
	if ais.Spec.StateStorageClass == nil {
		hostpathVolumes := []corev1.Volume{
			{
				Name: envVolume,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						//nolint:all
						Path: path.Join(*ais.Spec.HostpathPrefix, ais.Namespace, ais.Name, daeType+"_env"),
						Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
					},
				},
			},
			{
				Name: stateVolume,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						//nolint:all
						Path: path.Join(*ais.Spec.HostpathPrefix, ais.Namespace, ais.Name, daeType),
						Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
					},
				},
			},
		}
		volumes = append(volumes, hostpathVolumes...)
	}

	if ais.Spec.AWSSecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: awsSecretVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *ais.Spec.AWSSecretName,
				},
			},
		})
	}
	if ais.Spec.GCPSecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: gcpSecretVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *ais.Spec.GCPSecretName,
				},
			},
		})
	}

	if ais.Spec.TLSSecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: tlsSecretVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *ais.Spec.TLSSecretName,
				},
			},
		})
	}

	if ais.Spec.LogsDirectory != "" {
		volumes = append(volumes, corev1.Volume{
			Name: logsVolume,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path.Join(ais.Spec.LogsDirectory, ais.Namespace, ais.Name, daeType),
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		})
	}

	return volumes
}

func NewAISLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", path.Join(aisConfigDir, "ais_liveness.sh")},
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
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", "-c", "/usr/bin/pkill -SIGINT aisnode"},
			},
		},
	}
}

func NewAISVolumeMounts(ais *aisv1.AIStore, daeType string) []corev1.VolumeMount {
	spec := ais.Spec
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: aisConfigDir,
		},
		{
			Name:      configGlobalVolume,
			MountPath: path.Join(aisConfigDir, aisGlobalConfigFileName),
			SubPath:   aisGlobalConfigFileName,
		},
		{
			Name:      configGlobalVolume,
			MountPath: path.Join(aisConfigDir, "ais_liveness.sh"),
			SubPath:   "ais_liveness.sh",
		},
		{
			Name:      configGlobalVolume,
			MountPath: path.Join(aisConfigDir, "ais_readiness.sh"),
			SubPath:   "ais_readiness.sh",
		},
		{
			Name:      "statsd-config",
			MountPath: "/var/statsd_config",
		},
	}

	hostMountSubPath := getHostMountSubPath(daeType)
	if spec.StateStorageClass != nil {
		volumeName := getPVCPrefix(ais) + "state"
		dynamicMounts := []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: "/var/ais_env",
				SubPath:   "env/",
			},
			{
				Name:      volumeName,
				MountPath: "/etc/ais",
				SubPath:   "state/",
			},
		}
		volumeMounts = append(volumeMounts, dynamicMounts...)
	} else {
		hostMounts := []corev1.VolumeMount{
			{
				Name:        envVolume,
				MountPath:   "/var/ais_env",
				SubPathExpr: hostMountSubPath,
			},
			{
				Name:        stateVolume,
				MountPath:   "/etc/ais",
				SubPathExpr: hostMountSubPath,
			},
		}
		volumeMounts = append(volumeMounts, hostMounts...)
	}

	if spec.AWSSecretName != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      awsSecretVolume,
			ReadOnly:  true,
			MountPath: "/root/.aws",
		})
	}
	if spec.GCPSecretName != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      gcpSecretVolume,
			ReadOnly:  true,
			MountPath: "/var/gcp",
		})
	}
	if spec.TLSSecretName != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      tlsSecretVolume,
			ReadOnly:  true,
			MountPath: "/var/certs",
		})
	}
	if spec.LogsDirectory != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:        logsVolume,
			MountPath:   "/var/log/ais",
			SubPathExpr: hostMountSubPath,
		})
	}

	return volumeMounts
}

func NewInitContainerArgs(daeType string, hostnameMap map[string]string) []string {
	args := []string{
		"-role=" + daeType,
		"-local_config_template=" + path.Join(configTemplateDir, aisLocalConfigName),
		"-output_local_config=" + path.Join(aisConfigDir, aisLocalConfigName),
	}
	if len(hostnameMap) != 0 {
		args = append(args, "-hostname_map_file="+path.Join(globalConfigDir, hostnameMapFileName))
	}
	return args
}

func NewInitVolumeMounts(ais *aisv1.AIStore, daeType string) []corev1.VolumeMount {
	hostMountSubPath := getHostMountSubPath(daeType)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: aisConfigDir,
		},
		{
			Name:      configTemplateVolume,
			MountPath: configTemplateDir,
		},
		{
			Name:      configGlobalVolume,
			MountPath: globalConfigDir,
		},
	}

	if ais.Spec.StateStorageClass != nil {
		dynamicMounts := []corev1.VolumeMount{
			{
				Name:      getPVCPrefix(ais) + "state",
				MountPath: "/var/ais_env",
				SubPath:   "env/",
			},
		}
		volumeMounts = append(volumeMounts, dynamicMounts...)
	} else {
		hostMounts := []corev1.VolumeMount{
			{
				Name:        envVolume,
				MountPath:   "/var/ais_env",
				SubPathExpr: hostMountSubPath,
			},
		}
		volumeMounts = append(volumeMounts, hostMounts...)
	}

	return volumeMounts
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

func CreateAISAffinity(affinity *corev1.Affinity, podLabels map[string]string) *corev1.Affinity {
	// If we have no affinity defined in spec, define an empty one
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}

	// If we have an affinity but no specific PodAntiAffinity, set it
	if affinity.PodAntiAffinity == nil {
		affinity.PodAntiAffinity = createPodAntiAffinity(podLabels)
	}

	return affinity
}

func createPodAntiAffinity(podLabels map[string]string) *corev1.PodAntiAffinity {
	// Pods matching podLabels may not be scheduled on the same hostname
	labelAffinity := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: podLabels,
		},
		TopologyKey: corev1.LabelHostname,
	}

	return &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			labelAffinity,
		},
	}
}

func getHostMountSubPath(daeType string) string {
	// Always use the pod name as sub path for targets, since target pods are bound to specific nodes
	if daeType == aisapc.Target {
		return "$(MY_POD)"
	}
	return ""
}

func hostPathTypePtr(v corev1.HostPathType) *corev1.HostPathType {
	return &v
}

// Generate PVC claim ref for a specific namespace and cluster
func getPVCPrefix(ais *aisv1.AIStore) string {
	return fmt.Sprintf("%s-%s-", ais.Namespace, ais.Name)
}

// DefineStatePVC Define a PVC to use for pod state using dynamically configured volumes
func DefineStatePVC(ais *aisv1.AIStore, storageClass *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: getPVCPrefix(ais) + "state",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
			StorageClassName: storageClass,
		},
	}
}
