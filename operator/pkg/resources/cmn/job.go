// Package cmn provides utilities for common AIS cluster resources
// Creates a cleanup job for a specific node
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"
	"strings"

	aisv1 "github.com/ais-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCleanupJob creates a cleanup job for a specific node
func NewCleanupJob(ais *aisv1.AIStore, nodeName string) *batchv1.Job {
	ttl := int32(0) // delete the pod as soon as it is completed
	jobName := fmt.Sprintf("cleanup-%s-", strings.ReplaceAll(nodeName, ".", "-"))
	//nolint:all
	hostpathPrefix := *ais.Spec.HostpathPrefix
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: jobName,
			Namespace:    ais.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Affinity:      createNodeAffinitySpec(nodeName),
					Containers:    createContainerSpec(hostpathPrefix),
					Volumes:       createVolumeSpec(hostpathPrefix),
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

// createNodeAffinitySpec constructs the node affinity for the job
func createNodeAffinitySpec(nodeName string) *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{nodeName},
							},
						},
					},
				},
			},
		},
	}
}

// createContainerSpec constructs the container spec for the job
func createContainerSpec(hostpathPrefix string) []corev1.Container {
	return []corev1.Container{
		{
			Name:    "cleanup",
			Image:   "aistorage/ais-operator-helper:latest",
			Command: []string{"/cleanup-helper", "-dir=" + hostpathPrefix},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "hostpath",
					MountPath: hostpathPrefix,
				},
			},
		},
	}
}

// createVolumeSpec constructs the volume spec for the job
func createVolumeSpec(hostpathPrefix string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "hostpath",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostpathPrefix,
				},
			},
		},
	}
}
