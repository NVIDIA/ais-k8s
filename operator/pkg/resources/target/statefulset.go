// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"log"
	"strconv"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/proxy"
	"gopkg.in/inf.v0"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func PodLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		"app":       ais.Name,
		"component": aisapc.Target,
		"function":  "storage",
	}
}

func NewTargetSS(ais *aisv1.AIStore) *apiv1.StatefulSet {
	ls := PodLabels(ais)
	var (
		optionals   []corev1.EnvVar
		targetSize                   = ais.GetTargetSize()
		hostNetwork                  = false
		dnsPolicy   corev1.DNSPolicy = corev1.DNSClusterFirst // default value for DNSPolicy
	)
	if ais.Spec.TargetSpec.HostPort != nil {
		optionals = []corev1.EnvVar{
			cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"),
		}
	}
	if ais.Spec.TLSSecretName != nil {
		optionals = append(optionals, cmn.EnvFromValue(cmn.EnvUseHTTPS, "true"))
	}

	if ais.Spec.GCPSecretName != nil {
		// TODO -- FIXME: Remove hardcoding for path
		optionals = append(optionals, cmn.EnvFromValue(cmn.EnvGCPCredsPath, "/var/gcp/gcp.json"))
	}

	if ais.Spec.TargetSpec.HostNetwork != nil && *ais.Spec.TargetSpec.HostNetwork {
		hostNetwork = true
		dnsPolicy = corev1.DNSClusterFirstWithHostNet
		optionals = append(optionals, cmn.EnvFromValue(cmn.EnvHostNetwork, "true"))
	}

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
			Replicas:             &targetSize,
			VolumeClaimTemplates: targetVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ls,
					Annotations: cmn.ParseAnnotations(ais),
				},
				Spec: corev1.PodSpec{
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
								cmn.EnvFromValue(
									cmn.EnvEnableExternalAccess,
									strconv.FormatBool(ais.Spec.EnableExternalLB),
								),
								cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvDaemonRole, aisapc.Target),
								cmn.EnvFromValue(cmn.EnvProxyServiceName, proxy.HeadlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
							}, optionals...),
							Args: []string{
								"-c",
								"/bin/bash /var/ais_config_template/set_initial_target_env.sh",
							},
							Command:      []string{"/bin/bash"},
							VolumeMounts: cmn.NewInitVolumeMounts(ais, aisapc.Target),
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "ais-node",
							Image:           ais.Spec.NodeImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: append([]corev1.EnvVar{
								cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
								cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
								cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
								cmn.EnvFromValue(cmn.EnvCIDR, ""), // TODO: add
								cmn.EnvFromValue(cmn.EnvConfigFilePath, "/var/ais_config/ais.json"),
								cmn.EnvFromValue(cmn.EnvShutdownMarkerPath, "/var/ais_config"),
								cmn.EnvFromValue(cmn.EnvLocalConfigFilePath, "/var/ais_config/ais_local.json"),
								cmn.EnvFromValue(cmn.EnvStatsDConfig, "/var/statsd_config/statsd.json"),
								cmn.EnvFromValue(cmn.EnvDaemonRole, aisapc.Target),
								cmn.EnvFromValue(cmn.EnvEnablePrometheus,
									strconv.FormatBool(ais.Spec.EnablePromExporter != nil && *ais.Spec.EnablePromExporter)),
								cmn.EnvFromValue(cmn.EnvProxyServiceName, proxy.HeadlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvProxyServicePort, ais.Spec.ProxySpec.ServicePort.String()),
								cmn.EnvFromValue(cmn.EnvNodeServicePort, ais.Spec.TargetSpec.PublicPort.String()),
							}, optionals...),
							Ports:           cmn.NewDaemonPorts(ais.Spec.TargetSpec.DaemonSpec),
							SecurityContext: ais.Spec.TargetSpec.ContainerSecurity,
							VolumeMounts:    volumeMounts(ais),
							Lifecycle:       cmn.NewAISNodeLifecycle(),
							LivenessProbe:   cmn.NewAISLivenessProbe(),
							ReadinessProbe:  readinessProbe(ais.Spec.TargetSpec.ServicePort, ais.Spec.TLSSecretName != nil),
						},
					},
					HostNetwork:        hostNetwork,
					DNSPolicy:          dnsPolicy,
					ServiceAccountName: cmn.ServiceAccountName(ais),
					SecurityContext:    ais.Spec.TargetSpec.SecurityContext,
					Affinity:           createTargetAffinity(ais, ls),
					NodeSelector:       ais.Spec.TargetSpec.NodeSelector,
					Volumes:            cmn.NewAISVolumes(ais, aisapc.Target),
					Tolerations:        ais.Spec.TargetSpec.Tolerations,
					ImagePullSecrets:   ais.Spec.ImagePullSecrets,
				},
			},
		},
	}
}

func volumeMounts(ais *aisv1.AIStore) []corev1.VolumeMount {
	vols := cmn.NewAISVolumeMounts(ais, aisapc.Target)
	for _, res := range ais.Spec.TargetSpec.Mounts {
		vols = append(vols, corev1.VolumeMount{
			Name:      ais.Name + strings.ReplaceAll(res.Path, "/", "-"),
			MountPath: res.Path,
		})
	}
	return vols
}

func readinessProbe(port intstr.IntOrString, useHTTPS bool) *corev1.Probe {
	scheme := corev1.URISchemeHTTP
	if useHTTPS {
		scheme = corev1.URISchemeHTTPS
	}

	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/v1/health?readiness=true",
				Port:   port,
				Scheme: scheme,
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
	pvcs := make([]corev1.PersistentVolumeClaim, 0, int(ais.GetTargetSize()))
	for _, res := range ais.Spec.TargetSpec.Mounts {
		decSize := res.Size.AsDec()
		// Round down and get the unscaled int size
		roundedBytes, ok := decSize.Round(decSize, 0, inf.RoundDown).Unscaled()
		var size resource.Quantity
		if ok {
			size = *resource.NewQuantity(roundedBytes, res.Size.Format)
		} else {
			log.Printf("Could not convert %s to a whole byte number. Creating PVC without size spec\n", res.Size.String())
			size = resource.Quantity{}
		}
		pvcs = append(pvcs, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: ais.Name + strings.ReplaceAll(res.Path, "/", "-"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: size},
				},
				StorageClassName: res.StorageClass,
				Selector:         res.Selector,
			},
		})
	}
	if ais.Spec.StateStorageClass != nil {
		if statePVC := cmn.DefineStatePVC(ais, ais.Spec.StateStorageClass); statePVC != nil {
			pvcs = append(pvcs, *statePVC)
		}
	}
	return pvcs
}

func createTargetAffinity(ais *aisv1.AIStore, podLabels map[string]string) *corev1.Affinity {
	// Don't add additional rules to the affinity set in the target spec (can also be nil)
	if ais.AllowTargetSharedNodes() {
		return ais.Spec.TargetSpec.Affinity
	}
	return cmn.CreateAISAffinity(ais.Spec.TargetSpec.Affinity, podLabels)
}
