// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
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
	TLSCertFileName = "tls.crt"
	TLSKeyFileName  = "tls.key"
	TLSCAFileName   = "ca.crt"
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

	if tlsVol := getTLSVolume(ais, daeType); tlsVol != nil {
		volumes = append(volumes, *tlsVol)
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

// getTLSVolume returns the appropriate TLS volume based on cluster configuration.
func getTLSVolume(ais *v1beta1.AIStore, daeType string) *corev1.Volume {
	var source corev1.VolumeSource

	switch {
	case ais.UseTLSCSI():
		source = getTLSCSIVolumeSource(ais, daeType)
	case ais.UseTLSCertificate(), ais.UseTLSSecret():
		source = getTLSSecretVolumeSource(ais)
	default:
		return nil
	}

	return &corev1.Volume{
		Name:         tlsSecretVolume,
		VolumeSource: source,
	}
}

func getTLSCSIVolumeSource(ais *v1beta1.AIStore, daeType string) corev1.VolumeSource {
	certConfig := ais.GetTLSCertificate()
	if certConfig == nil {
		return corev1.VolumeSource{}
	}
	name := ais.Name + "-" + daeType
	issuerRef := certConfig.IssuerRef
	dnsNames, _ := buildCertificateSANs(ais)

	attrs := map[string]string{
		csiapisv1.IssuerNameKey: issuerRef.Name,
		csiapisv1.CommonNameKey: fmt.Sprintf("%s.%s", name, ais.Namespace),
		csiapisv1.DNSNamesKey:   strings.Join(dnsNames, ","),
	}

	if issuerRef.Kind != "" {
		attrs[csiapisv1.IssuerKindKey] = issuerRef.Kind
	}

	return corev1.VolumeSource{
		CSI: &corev1.CSIVolumeSource{
			Driver:           csiapis.GroupName,
			VolumeAttributes: attrs,
			ReadOnly:         aisapc.Ptr(true),
		},
	}
}

func getTLSSecretVolumeSource(ais *v1beta1.AIStore) corev1.VolumeSource {
	return corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: ais.GetTLSSecretName(),
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

	if ais.HasTLSEnabled() {
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
