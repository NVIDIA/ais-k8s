// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"log"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
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
	labels := PodLabels(ais)
	return &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName(ais),
			Namespace: ais.Namespace,
			Labels:    labels,
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName:          headlessSVCName(ais),
			PodManagementPolicy:  apiv1.ParallelPodManagement,
			Replicas:             aisapc.Ptr(ais.GetTargetSize()),
			VolumeClaimTemplates: targetVC(ais),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: cmn.PrepareAnnotations(ais.Spec.TargetSpec.Annotations, ais.Spec.NetAttachment),
				},
				Spec: *targetPodSpec(ais, labels),
			},
		},
	}
}

func targetPodSpec(ais *aisv1.AIStore, labels map[string]string) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            "populate-env",
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env:             NewInitContainerEnv(ais),
				Args:            cmn.NewInitContainerArgs(aisapc.Target, ais.Spec.HostnameMap),
				VolumeMounts:    cmn.NewInitVolumeMounts(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "ais-node",
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Command:         []string{"aisnode"},
				Args:            cmn.NewAISContainerArgs(ais.GetTargetSize(), aisapc.Target),
				Env:             NewAISContainerEnv(ais),
				Ports:           cmn.NewDaemonPorts(&ais.Spec.TargetSpec.DaemonSpec),
				Resources:       ais.Spec.TargetSpec.Resources,
				SecurityContext: ais.Spec.TargetSpec.ContainerSecurity,
				VolumeMounts:    volumeMounts(ais),
				StartupProbe:    cmn.NewStartupProbe(ais, aisapc.Target),
				LivenessProbe:   cmn.NewLivenessProbe(ais, aisapc.Target),
				ReadinessProbe:  cmn.NewReadinessProbe(ais, aisapc.Target),
			},
		},
		HostNetwork:        ais.UseHostNetwork(),
		DNSPolicy:          ais.GetTargetDNSPolicy(),
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    ais.Spec.TargetSpec.SecurityContext,
		Affinity:           createTargetAffinity(ais, labels),
		NodeSelector:       ais.Spec.TargetSpec.NodeSelector,
		Volumes:            cmn.NewAISVolumes(ais, aisapc.Target),
		Tolerations:        ais.Spec.TargetSpec.Tolerations,
		ImagePullSecrets:   ais.Spec.ImagePullSecrets,
	}
	if ais.Spec.LogSidecarImage != nil {
		spec.Containers = append(spec.Containers, cmn.NewLogSidecar(*ais.Spec.LogSidecarImage, aisapc.Target))
	}
	return spec
}

func NewInitContainerEnv(ais *aisv1.AIStore) (initEnv []corev1.EnvVar) {
	initEnv = cmn.CommonInitEnv(ais)
	initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvServiceName, headlessSVCName(ais)))
	if ais.Spec.TargetSpec.HostPort != nil {
		initEnv = append(initEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
	}
	if ais.UseHostNetwork() {
		initEnv = append(initEnv, cmn.EnvFromValue(cmn.EnvHostNetwork, "true"))
	}
	return
}

func NewAISContainerEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	baseEnv := cmn.CommonEnv()
	if ais.Spec.TargetSpec.HostPort != nil {
		baseEnv = append(baseEnv, cmn.EnvFromFieldPath(cmn.EnvPublicHostname, "status.hostIP"))
	}
	return cmn.MergeEnvVars(baseEnv, ais.Spec.TargetSpec.Env)
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
