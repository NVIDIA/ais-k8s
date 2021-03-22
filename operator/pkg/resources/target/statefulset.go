// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"strconv"
	"strings"

	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
)

func statefulSetName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aiscmn.Target
}

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      statefulSetName(ais),
		Namespace: ais.Namespace,
	}
}

func PodLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		"app":       ais.Name,
		"component": aiscmn.Target,
		"function":  "storage",
	}
}

func NewTargetSS(ais *aisv1.AIStore) *apiv1.StatefulSet {
	ls := PodLabels(ais)
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
			ServiceName:          headlessSVCName(ais),
			PodManagementPolicy:  apiv1.ParallelPodManagement,
			Replicas:             &ais.Spec.Size,
			VolumeClaimTemplates: targetVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:            "populate-env",
							Image:           ais.Spec.InitImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env: []corev1.EnvVar{
								cmn.EnvFromFieldPath(cmn.EnvNodeName, "spec.nodeName"),
								cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
								cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
								cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
								cmn.EnvFromValue(
									cmn.EnvEnableExternalAccess,
									strconv.FormatBool(ais.Spec.EnableExternalLB),
								),
								cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvDaemonRole, aiscmn.Target),
								cmn.EnvFromValue(cmn.EnvProxyServiceName, proxy.HeadlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
							},
							Args: []string{
								"-c",
								"/bin/bash /var/ais_config_template/set_initial_target_env.sh",
							},
							Command:      []string{"/bin/bash"},
							VolumeMounts: cmn.NewInitVolumeMounts(),
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "ais-node",
							Image:           ais.Spec.NodeImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
								cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
								cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
								cmn.EnvFromValue(cmn.EnvCIDR, ""), // TODO: add
								cmn.EnvFromValue(cmn.ENVConfigFilePath, "/var/ais_config/ais.json"),
								cmn.EnvFromValue(cmn.ENVLocalConfigFilePath, "/var/ais_config/ais_local.json"),
								cmn.EnvFromValue(cmn.EnvStatsDConfig, "/var/statsd_config/statsd.json"),
								cmn.EnvFromValue(cmn.EnvDaemonRole, aiscmn.Target),
								cmn.EnvFromValue(
									cmn.EnvNoDiskIO,
									strconv.FormatBool(ais.Spec.TargetSpec.NoDiskIO.Enabled),
								),
								cmn.EnvFromValue(cmn.EnvDryObjSize, ais.Spec.TargetSpec.NoDiskIO.DryObjSize.String()),
								cmn.EnvFromValue(cmn.EnvProxyServiceName, proxy.HeadlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(ais.Spec.TargetSpec.ServicePort.IntValue()),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: ais.Spec.TargetSpec.ContainerSecurity,
							VolumeMounts:    volumeMounts(ais),
							Lifecycle:       cmn.NewAISNodeLifecycle(),
							LivenessProbe:   cmn.NewAISLivenessProbe(ais.Spec.TargetSpec.ServicePort),
							ReadinessProbe:  readinessProbe(ais.Spec.TargetSpec.ServicePort),
						},
					},
					ServiceAccountName: cmn.ServiceAccountName(ais),
					SecurityContext:    ais.Spec.TargetSpec.SecurityContext,
					Affinity:           cmn.NewAISPodAffinity(ais, ais.Spec.TargetSpec.Affinity, ls),
					Volumes:            cmn.NewAISVolumes(ais, aiscmn.Target),
					Tolerations:        ais.Spec.TargetSpec.Tolerations,
				},
			},
		},
	}
}

func volumeMounts(ais *aisv1.AIStore) []corev1.VolumeMount {
	vols := cmn.NewAISVolumeMounts()
	for _, res := range ais.Spec.TargetSpec.Mounts {
		vols = append(vols, corev1.VolumeMount{
			Name:      ais.Name + strings.ReplaceAll(res.Path, "/", "-"),
			MountPath: res.Path,
		})
	}
	return vols
}

func readinessProbe(port intstr.IntOrString) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health?readiness=true",
				Port: port,
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       5,
		FailureThreshold:    8,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
	}
}

func targetVC(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	pvcs := make([]corev1.PersistentVolumeClaim, 0, int(ais.Spec.Size))
	for _, res := range ais.Spec.TargetSpec.Mounts {
		pvcs = append(pvcs, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: ais.Name + strings.ReplaceAll(res.Path, "/", "-"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: res.Size},
				},
				StorageClassName: res.StorageClass,
			},
		})
	}
	return pvcs
}
