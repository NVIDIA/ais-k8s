// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"fmt"
	"strings"

	aismeta "github.com/NVIDIA/aistore/core/meta"
	"github.com/ais-operator/pkg/resources/cmn"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func shouldUpdatePodTemplate(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if len(desired.Spec.Containers) != len(current.Spec.Containers) {
		return true, "updating desired containers"
	}

	for _, daemon := range []struct {
		desiredContainer *corev1.Container
		currentContainer *corev1.Container
	}{
		{&desired.Spec.InitContainers[0], &current.Spec.InitContainers[0]},
		{&desired.Spec.Containers[0], &current.Spec.Containers[0]},
	} {
		if daemon.desiredContainer.Image != daemon.currentContainer.Image {
			return true, fmt.Sprintf("updating image for %q container", daemon.desiredContainer.Name)
		}
		if shouldUpdateEnv(daemon.desiredContainer.Name, daemon.desiredContainer.Env, daemon.currentContainer.Env) {
			return true, fmt.Sprintf("updating env variables for %q container", daemon.desiredContainer.Name)
		}
		if shouldUpdateResources(&daemon.desiredContainer.Resources, &daemon.currentContainer.Resources) {
			return true, fmt.Sprintf("updating resource requests/limits for %q container", daemon.desiredContainer.Name)
		}
	}

	if shouldUpdateAnnotations(desired.Annotations, current.Annotations) {
		return true, "updating annotations"
	}

	if !equality.Semantic.DeepEqual(desired.Labels, current.Labels) {
		return true, "updating labels"
	}

	// Both `desired.Spec.SecurityContext` and `current.Spec.SecurityContext` are
	// expected to be non-nil here as `SecurityContext` should be set by default.
	if !equality.Semantic.DeepEqual(desired.Spec.SecurityContext, current.Spec.SecurityContext) {
		return true, "updating security context"
	}

	// We already know desired number of containers matches current here,
	// so if using sidecar, compare the images of the sidecar container.
	if len(desired.Spec.Containers) > 1 {
		if desired.Spec.Containers[1].Image != current.Spec.Containers[1].Image {
			return true, fmt.Sprintf("updating image for %q container", desired.Spec.Containers[1].Name)
		}
	}

	return false, ""
}

func shouldUpdateResources(desired, current *corev1.ResourceRequirements) bool {
	// TODO: Remove check in next major version (causes cluster restart)
	// If we already have ephemeral storage request, do a full comparison
	if current.Requests.StorageEphemeral() != nil && !current.Requests.StorageEphemeral().IsZero() {
		return !equality.Semantic.DeepEqual(desired, current)
	}
	// Do not sync if the only change is *adding* ephemeral storage request
	desFiltered := desired.DeepCopy()
	delete(desFiltered.Requests, corev1.ResourceEphemeralStorage)
	return !equality.Semantic.DeepEqual(desFiltered, current)
}

// Ignore removed "cmn.EnvPublicHostname" removed from AIS container in v2.9.1 to avoid rollout
// TODO: Remove in next major release
func shouldUpdateEnv(name string, desired, current []corev1.EnvVar) bool {
	// Only avoid the env var sync for AIS containers
	if name != cmn.AISContainerName {
		return !equality.Semantic.DeepEqual(desired, current)
	}
	// Deep copy a list of env vars but ignore cmn.EnvPublicHostname
	normalize := func(envs []corev1.EnvVar) []corev1.EnvVar {
		out := make([]corev1.EnvVar, 0, len(envs))
		for _, e := range envs {
			if e.Name != cmn.EnvPublicHostname {
				out = append(out, e)
			}
		}
		return out
	}
	return !equality.Semantic.DeepEqual(normalize(desired), normalize(current))
}

func shouldUpdateAnnotations(desired, current map[string]string) bool {
	if equality.Semantic.DeepDerivative(desired, current) {
		return false
	}
	restartHash, exists := desired[cmn.RestartConfigHashAnnotation]
	// At this point annotations are not equal -- If the restart hash does not exist trigger sync
	if !exists {
		return true
	}
	// If the hash is different and NOT initial, trigger sync
	nonInitial := !strings.HasSuffix(restartHash, cmn.RestartConfigHashInitial)
	if nonInitial && restartHash != current[cmn.RestartConfigHashAnnotation] {
		return true
	}
	// Compare the desired to current WITHOUT the restart hash and trigger if not equivalent
	desiredCopy := make(map[string]string)
	for k, v := range desired {
		desiredCopy[k] = v
	}
	delete(desiredCopy, cmn.RestartConfigHashAnnotation)
	return !equality.Semantic.DeepDerivative(desiredCopy, current)
}

func syncPodTemplate(desired, current *corev1.PodTemplateSpec) (updated bool) {
	for _, daemon := range []struct {
		desiredContainer *corev1.Container
		currentContainer *corev1.Container
	}{
		{&desired.Spec.InitContainers[0], &current.Spec.InitContainers[0]},
		{&desired.Spec.Containers[0], &current.Spec.Containers[0]},
	} {
		if equality.Semantic.DeepDerivative(*daemon.desiredContainer, *daemon.currentContainer) {
			continue
		}
		*daemon.currentContainer = *daemon.desiredContainer
		updated = true
	}

	if !equality.Semantic.DeepDerivative(desired.Annotations, current.Annotations) {
		current.Annotations = desired.Annotations
		updated = true
	}

	if !equality.Semantic.DeepEqual(desired.Labels, current.Labels) {
		current.Labels = desired.Labels
		updated = true
	}

	if !equality.Semantic.DeepEqual(desired.Spec.SecurityContext, current.Spec.SecurityContext) {
		current.Spec.SecurityContext = desired.Spec.SecurityContext
		updated = true
	}

	if syncSidecarContainer(desired, current) {
		updated = true
	}

	return
}

func findAISNodeByPodName(nodeMap aismeta.NodeMap, podName string) (*aismeta.Snode, error) {
	for _, node := range nodeMap {
		if strings.HasPrefix(node.ControlNet.Hostname, podName) {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no matching AIS node found for pod %q", podName)
}

func syncSidecarContainer(desired, current *corev1.PodTemplateSpec) (updated bool) {
	// We have no sidecar, and don't want one
	if len(desired.Spec.Containers) < 2 && len(current.Spec.Containers) < 2 {
		return false
	}
	// We want to remove the sidecar
	if len(desired.Spec.Containers) < 2 && len(current.Spec.Containers) > 1 {
		current.Spec.Containers = current.Spec.Containers[:1]
		return true
	}
	// Add a new sidecar
	if len(desired.Spec.Containers) > 1 && len(current.Spec.Containers) < 2 {
		current.Spec.Containers = append(current.Spec.Containers, desired.Spec.Containers[1])
		return true
	}
	// If sidecar is already updated, no change
	if equality.Semantic.DeepDerivative(desired.Spec.Containers[1], current.Spec.Containers[1]) {
		return false
	}
	current.Spec.Containers[1] = desired.Spec.Containers[1]
	return true
}

func (*AIStoreReconciler) isStatefulSetReady(desiredSize int32, ss *appsv1.StatefulSet) bool {
	specReplicas := *ss.Spec.Replicas

	// Must match size provided in AIS cluster spec
	if specReplicas != desiredSize {
		return false
	}

	// If update revision exists, all replicas must be updated
	if ss.Status.UpdateRevision != "" && specReplicas != ss.Status.UpdatedReplicas {
		return false
	}

	// Ensure there are no extra (terminating) pods still counted
	if ss.Status.Replicas != specReplicas {
		return false
	}

	// To be ready, spec must match status.ReadyReplicas
	return specReplicas == ss.Status.ReadyReplicas
}
