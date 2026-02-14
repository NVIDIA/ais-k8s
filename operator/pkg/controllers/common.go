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

type SyncMode int

const (
	SyncModeIgnoreNone = iota
	SyncModeIgnoreRemovedEnv
	SyncModeIgnoreAddedEnv
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

	if desired.Spec.PriorityClassName != current.Spec.PriorityClassName {
		return true, "updating priority class name"
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
// TODO: Update in next major release to remove backwards compatible env var changes
func shouldUpdateEnv(name string, desired, current []corev1.EnvVar) bool {
	var ignored map[string]struct{}
	switch name {
	case cmn.AISContainerName:
		ignored = map[string]struct{}{cmn.EnvPublicHostname: {}}
		// Compare but don't sync if the only change is removing this env
		return compareEnvWithIgnored(desired, current, ignored, SyncModeIgnoreRemovedEnv)
	case cmn.InitContainerName:
		ignored = map[string]struct{}{cmn.EnvPublicDNSMode: {}, cmn.EnvHostIPS: {}}
		// Compare but don't sync if the only change is adding this env
		return compareEnvWithIgnored(desired, current, ignored, SyncModeIgnoreAddedEnv)
	default:
		return !equality.Semantic.DeepEqual(desired, current)
	}
}

// Compares the given slices of EnvVars and return true if there are changes to sync
// Ignores any that are solely added or removed depending on the provided mode
// Still sync on modifications of ignored variables that are present in both slices
func compareEnvWithIgnored(des, cur []corev1.EnvVar, ignored map[string]struct{}, mode SyncMode) bool {
	// convert to map for lookup
	desired := envSliceToMap(des)
	current := envSliceToMap(cur)

	// Deep copy a map of env vars but ignore those in the ignored set
	normalize := func(envMap map[string]string) map[string]string {
		out := make(map[string]string, len(envMap))
		for k, v := range envMap {
			if _, ok := ignored[k]; !ok {
				out[k] = v
			}
		}
		return out
	}
	// This checks if there are any changes to any other env vars besides those ignored.
	// If there are, we need to sync so return true
	// If ignoring removals, we remove from current but not des for comparison
	if !equality.Semantic.DeepEqual(normalize(desired), normalize(current)) {
		return true
	}
	// At this point the only changes that can be left are the ignored variables
	for env := range ignored {
		desVal, desOk := desired[env]
		curVal, curOk := current[env]
		// If ignoring removals and the env does not exist in desired, skip this env
		if mode == SyncModeIgnoreRemovedEnv && !desOk {
			continue
		}
		// If ignoring additions and the env does not exist in current, skip this env
		if mode == SyncModeIgnoreAddedEnv && !curOk {
			continue
		}
		// Sync if values are different
		if desVal != curVal {
			return true
		}
	}
	return false
}

func envSliceToMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		m[e.Name] = e.Value
	}
	return m
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

	if !equality.Semantic.DeepEqual(desired.Spec.Volumes, current.Spec.Volumes) {
		current.Spec.Volumes = desired.Spec.Volumes
		updated = true
	}

	if desired.Spec.PriorityClassName != current.Spec.PriorityClassName {
		current.Spec.PriorityClassName = desired.Spec.PriorityClassName
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
