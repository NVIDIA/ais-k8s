// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"log"
	"path"
	"strconv"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/statsd"
	"gopkg.in/inf.v0"
	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
								cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
								cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)),
								cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
								cmn.EnvFromValue(
									cmn.EnvEnableExternalAccess,
									strconv.FormatBool(ais.Spec.EnableExternalLB),
								),
							}, optionals...),
							Args:         cmn.NewInitContainerArgs(aisapc.Target, ais.Spec.HostnameMap),
							VolumeMounts: cmn.NewInitVolumeMounts(),
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "ais-node",
							Image:           ais.Spec.NodeImage,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"aisnode"},
							Args:            cmn.NewAISContainerArgs(ais, aisapc.Target),
							Env: append([]corev1.EnvVar{
								cmn.EnvFromFieldPath(cmn.EnvPodName, "metadata.name"),
								cmn.EnvFromValue(cmn.EnvNS, ais.Namespace),
								cmn.EnvFromValue(cmn.EnvClusterDomain, ais.GetClusterDomain()),
								cmn.EnvFromValue(cmn.EnvCIDR, ""), // TODO: add
								cmn.EnvFromValue(cmn.EnvConfigFilePath, path.Join(cmn.AisConfigDir, cmn.AISGlobalConfigName)),
								cmn.EnvFromValue(cmn.EnvShutdownMarkerPath, cmn.AisConfigDir),
								cmn.EnvFromValue(cmn.EnvLocalConfigFilePath, path.Join(cmn.AisConfigDir, cmn.AISLocalConfigName)),
								cmn.EnvFromValue(cmn.EnvStatsDConfig, path.Join(cmn.StatsDDir, statsd.ConfigFile)),
								cmn.EnvFromValue(
									cmn.EnvEnablePrometheus,
									strconv.FormatBool(ais.Spec.EnablePromExporter != nil && *ais.Spec.EnablePromExporter),
								),
							}, optionals...),
							Ports:           cmn.NewDaemonPorts(&ais.Spec.TargetSpec.DaemonSpec),
							SecurityContext: ais.Spec.TargetSpec.ContainerSecurity,
							VolumeMounts:    volumeMounts(ais),
							StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Target),
							LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Target),
							ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Target),
						},
						cmn.NewLogSidecar(aisapc.Target),
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
