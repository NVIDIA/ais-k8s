// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"
	"log"
	"path"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"gopkg.in/inf.v0"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	awsSecretVolume = "aws-creds"
	gcpSecretVolume = "gcp-creds" //nolint:gosec // False positive
	ociSecretVolume = "oci-creds"
)

// Container mount locations for cloud provider configs
const (
	DefaultGCPDir    = "/var/gcp"
	DefaultGCPConfig = "gcp.json"
	DefaultAWSDir    = "/root/.aws"
	DefaultOCIDir    = "/root/.oci"
	DefaultOCIConfig = "config"
)

const hostMountPrefix = "host-data-mount"

func newVolumes(ais *aisv1.AIStore) []corev1.Volume {
	volumes := cmn.NewAISVolumes(ais, aisapc.Target)
	volumes = appendCloudVolumes(ais, volumes)
	volumes = appendHostPathDataVolumes(ais, volumes)
	return volumes
}

func appendCloudVolumes(ais *aisv1.AIStore, volumes []corev1.Volume) []corev1.Volume {
	type cloudSecret struct {
		namePtr    *string
		volumeName string
	}

	secrets := []cloudSecret{
		{ais.Spec.AWSSecretName, awsSecretVolume},
		{ais.Spec.GCPSecretName, gcpSecretVolume},
		{ais.Spec.OCISecretName, ociSecretVolume},
	}

	for _, secret := range secrets {
		if secret.namePtr != nil {
			volumes = append(volumes, corev1.Volume{
				Name: secret.volumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: *secret.namePtr,
					},
				},
			})
		}
	}

	return volumes
}

func appendHostPathDataVolumes(ais *aisv1.AIStore, volumes []corev1.Volume) []corev1.Volume {
	mounts := ais.Spec.TargetSpec.Mounts
	for i, mnt := range mounts {
		// Only creating new volumes for HostPath mounts
		if !mnt.IsHostPath() {
			continue
		}
		volumes = append(volumes, corev1.Volume{
			Name: getHostPathVolumeName(i),
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path.Join(mnt.Path, ais.Namespace, ais.Name, aisapc.Target),
					Type: aisapc.Ptr(corev1.HostPathDirectoryOrCreate),
				},
			},
		})
	}
	return volumes
}

func defineDataPVCs(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(ais.Spec.TargetSpec.Mounts))
	for _, mnt := range ais.Spec.TargetSpec.Mounts {
		if mnt.IsHostPath() {
			continue
		}
		decSize := mnt.Size.AsDec()
		// Round down and get the unscaled int size
		roundedBytes, ok := decSize.Round(decSize, 0, inf.RoundDown).Unscaled()
		var size resource.Quantity
		if ok {
			size = *resource.NewQuantity(roundedBytes, mnt.Size.Format)
		} else {
			log.Printf("Could not convert %s to a whole byte number. Creating PVC without size spec\n", mnt.Size.String())
			size = resource.Quantity{}
		}
		// For backwards compatibility, this PVC naming convention must stay the same
		// When used in a Statefulset volume claim template, the actual PVC created will be:
		//   <pvcTemplateName>-<statefulsetName>-<ordinal>
		// The path /ais/nvme0n1 with a cluster named `ais` will produce a PVC for pod ais-target-0:
		//   ais-ais-nvme0n1-ais-target-0
		pvcs = append(pvcs, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: mnt.GetPVCName(ais.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: size},
				},
				StorageClassName: mnt.StorageClass,
				Selector:         mnt.Selector,
			},
		})
	}
	return pvcs
}

func targetPVC(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	pvcs := defineDataPVCs(ais)
	if ais.Spec.StateStorageClass != nil {
		if statePVC := cmn.DefineStatePVC(ais, ais.Spec.StateStorageClass); statePVC != nil {
			pvcs = append(pvcs, *statePVC)
		}
	}
	return pvcs
}

func newVolumeMounts(ais *aisv1.AIStore) []corev1.VolumeMount {
	vm := cmn.NewAISVolumeMounts(ais, aisapc.Target)
	vm = appendCloudVolumeMounts(&ais.Spec, vm)
	vm = appendDataVolumeMounts(ais, vm)
	return vm
}

func appendCloudVolumeMounts(spec *aisv1.AIStoreSpec, mounts []corev1.VolumeMount) []corev1.VolumeMount {
	type cloudConfig struct {
		secretName *string
		defaultDir string
		volumeName string
	}

	configs := []cloudConfig{
		{spec.AWSSecretName, DefaultAWSDir, awsSecretVolume},
		{spec.GCPSecretName, DefaultGCPDir, gcpSecretVolume},
		{spec.OCISecretName, DefaultOCIDir, ociSecretVolume},
	}

	for _, cfg := range configs {
		if cfg.secretName != nil {
			mounts = cmn.AppendSimpleReadOnlyMount(mounts, cfg.volumeName, cfg.defaultDir)
		}
	}
	return mounts
}

func appendDataVolumeMounts(ais *aisv1.AIStore, vm []corev1.VolumeMount) []corev1.VolumeMount {
	for i, mnt := range ais.Spec.TargetSpec.Mounts {
		var name string
		if mnt.IsHostPath() {
			name = getHostPathVolumeName(i)
		} else {
			name = mnt.GetPVCName(ais.Name)
		}
		vm = append(vm, corev1.VolumeMount{
			Name:      name,
			MountPath: mnt.Path,
		})
	}
	return vm
}

// getHostPathVolumeName returns a consistent Volume name identifier for HostPath mounts
// This avoids any limitations on the total length of the Volume name
// Since HostPath Volumes are defined local to the pod, there is no constraint of cross-pod or cross-cluster uniqueness
func getHostPathVolumeName(index int) string {
	return fmt.Sprintf("%s-%d", hostMountPrefix, index)
}
