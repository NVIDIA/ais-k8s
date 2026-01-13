// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"path"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/statsd"
	csiapis "github.com/cert-manager/csi-driver/pkg/apis"
	csiapisv1 "github.com/cert-manager/csi-driver/pkg/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// StateDir Container-internal location of configs and current state of the aisnode
	StateDir = "/etc/ais"
	// InitConfTemplateDir Container-internal location of config template, mounted from the config map
	InitConfTemplateDir = "/var/ais_config_template"
	// AisConfigDir Container-internal location of initial config, written by init container and used at aisnode start
	AisConfigDir      = "/var/ais_config"
	LogsDir           = "/var/log/ais"
	StatsDDir         = "/var/statsd_config"
	InitGlobalConfDir = "/var/global_config"

	// Other container mount locations
	certsDir        = "/var/certs"
	tracesDir       = "/var/traces"
	OIDCCAFileName  = "ca.crt"
	OIDCCAMountPath = "/etc/ais/oidc-ca"

	hostnameMapFileName = "hostname_map.json"
	AISGlobalConfigName = "ais.json"
	AISLocalConfigName  = "ais_local.json"

	StatsDVolume         = "statsd-config"
	configTemplateVolume = "config-template"
	configVolume         = "config-mount"
	configGlobalVolume   = "config-global"
	stateVolume          = "state-mount"
	tlsSecretVolume      = "tls-certs"
	tracingSecretVolume  = "tracing-token"
	logsVolume           = "logs-dir"
)

func NewAISVolumes(ais *v1beta1.AIStore, daeType string) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: configTemplateVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: AISConfigMapName(ais, daeType),
					},
				},
			},
		},
		{
			Name: configVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
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
			Name: StatsDVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: statsd.ConfigMapName(ais),
					},
				},
			},
		},
		newLogsVolume(ais, daeType),
	}

	// Only create hostpath volumes if no storage class is provided for state
	if ais.Spec.StateStorageClass == nil {
		hostpathVolumes := []corev1.Volume{
			{
				Name: stateVolume,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						//nolint:all
						Path: path.Join(*ais.Spec.HostpathPrefix, ais.Namespace, ais.Name, daeType),
						Type: aisapc.Ptr(corev1.HostPathDirectoryOrCreate),
					},
				},
			},
		}
		volumes = append(volumes, hostpathVolumes...)
	}

	if ais.Spec.TLSCertManagerIssuerName != nil {
		name := ais.Name + "-" + daeType
		volumes = append(volumes, corev1.Volume{
			Name: tlsSecretVolume,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver: csiapis.GroupName,
					VolumeAttributes: map[string]string{
						csiapisv1.IssuerNameKey: *ais.Spec.TLSCertManagerIssuerName,
						csiapisv1.CommonNameKey: name + ".${POD_NAMESPACE}",
						csiapisv1.DNSNamesKey: strings.Join(
							[]string{
								"${POD_NAME}.${POD_NAMESPACE}.svc." + ais.GetClusterDomain(),
								name + ".${POD_NAMESPACE}.svc." + ais.GetClusterDomain(),
								name + ".${POD_NAMESPACE}.svc",
								name,
							},
							","),
					},
					ReadOnly: aisapc.Ptr(true),
				},
			},
		})
	} else if ais.Spec.TLSSecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: tlsSecretVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *ais.Spec.TLSSecretName,
				},
			},
		})
	}

	if ais.Spec.TracingTokenSecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: tracingSecretVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *ais.Spec.TracingTokenSecretName,
				},
			},
		})
	}
	return volumes
}

func newLogsVolume(ais *v1beta1.AIStore, daeType string) corev1.Volume {
	if ais.Spec.LogsDirectory != "" {
		return corev1.Volume{
			Name: logsVolume,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path.Join(ais.Spec.LogsDirectory, ais.Namespace, ais.Name, daeType),
					Type: aisapc.Ptr(corev1.HostPathDirectoryOrCreate),
				},
			},
		}
	}
	return corev1.Volume{
		Name: logsVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func NewAISVolumeMounts(ais *v1beta1.AIStore, daeType string) []corev1.VolumeMount {
	spec := &ais.Spec
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: AisConfigDir,
		},
		{
			Name:      StatsDVolume,
			MountPath: StatsDDir,
		},
		newLogsVolumeMount(daeType),
	}

	if spec.StateStorageClass != nil {
		volumeName := getStatePVCName(ais)
		dynamicMounts := []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: StateDir,
			},
		}
		volumeMounts = append(volumeMounts, dynamicMounts...)
	} else {
		hostMountSubPath := getHostMountSubPath(daeType)
		hostMounts := []corev1.VolumeMount{
			{
				Name:        stateVolume,
				MountPath:   StateDir,
				SubPathExpr: hostMountSubPath,
			},
		}
		volumeMounts = append(volumeMounts, hostMounts...)
	}

	if spec.TLSCertManagerIssuerName != nil || spec.TLSSecretName != nil {
		volumeMounts = AppendSimpleReadOnlyMount(volumeMounts, tlsSecretVolume, certsDir)
	}
	if spec.TracingTokenSecretName != nil {
		volumeMounts = AppendSimpleReadOnlyMount(volumeMounts, tracingSecretVolume, tracesDir)
	}
	return volumeMounts
}

func AppendSimpleReadOnlyMount(mounts []corev1.VolumeMount, name, mountPath string) []corev1.VolumeMount {
	return append(mounts, corev1.VolumeMount{
		Name:      name,
		ReadOnly:  true,
		MountPath: mountPath,
	})
}

func newLogsVolumeMount(daeType string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:        logsVolume,
		MountPath:   LogsDir,
		SubPathExpr: getHostMountSubPath(daeType),
	}
}

func NewInitVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configTemplateVolume,
			MountPath: InitConfTemplateDir,
		},
		{
			Name:      configVolume,
			MountPath: AisConfigDir,
		},
		{
			Name:      configGlobalVolume,
			MountPath: InitGlobalConfDir,
		},
	}
	return volumeMounts
}

func getHostMountSubPath(daeType string) string {
	// Always use the pod name as sub path for targets, since target pods are bound to specific nodes
	if daeType == aisapc.Target {
		return "$(MY_POD)"
	}
	return ""
}
