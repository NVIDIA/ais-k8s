// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"
	"path"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	corev1 "k8s.io/api/core/v1"
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
						SecretName:  *secret.namePtr,
						DefaultMode: &cmn.SecretDefaultMode,
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

func newTargetPVCs(ais *aisv1.AIStore) []corev1.PersistentVolumeClaim {
	// Add PVCs for AIS data storage
	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(ais.Spec.TargetSpec.Mounts))
	for _, mnt := range ais.Spec.TargetSpec.Mounts {
		if mnt.IsHostPath() {
			continue
		}
		pvcs = append(pvcs, *mnt.BuildPVC(ais.Name))
	}
	// If using a storage class for state storage, add PVCs for state
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
