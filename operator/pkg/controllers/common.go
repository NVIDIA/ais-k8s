// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
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

// Given a desired and current pod template spec, determine if we need to trigger a rollout to sync
// When the actual sync happens, specs will be updated
// In many cases we do not want every change to cause a restart, so those should be defined here
func shouldUpdatePodTemplate(desired, current *corev1.PodTemplateSpec) (bool, string) {
	// Define a series of functions to check
	// Each function returns whether we should trigger a rollout to sync along with a reason
	checks := []func(desired, current *corev1.PodTemplateSpec) (bool, string){
		shouldUpdateContainerList,
		shouldUpdateInitContainer,
		shouldUpdatePrimaryContainer,
		shouldUpdateSecurityContext,
		shouldUpdateAnnotations,
		shouldUpdateSidecars,
		shouldUpdateLabels,
		shouldUpdateVolumes,
		shouldUpdatePriorityClass,
	}
	return shouldUpdate(desired, current, checks...)
}

func shouldUpdateInitContainer(desired, current *corev1.PodTemplateSpec) (bool, string) {
	desiredInit := &desired.Spec.InitContainers[0]
	currentInit := &current.Spec.InitContainers[0]
	// Init container resources are hardcoded operator defaults and not user-specified.
	// Changes should not trigger a rollout and thus can skip the resource comparison.
	return shouldUpdateContainerSpec(desiredInit, currentInit, true)
}

func shouldUpdateContainerList(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if len(desired.Spec.Containers) != len(current.Spec.Containers) {
		return true, "updating desired containers"
	}
	return false, ""
}

func shouldUpdatePrimaryContainer(desired, current *corev1.PodTemplateSpec) (bool, string) {
	return shouldUpdateContainerSpec(&desired.Spec.Containers[0], &current.Spec.Containers[0], false)
}

func shouldUpdateSidecars(desired, current *corev1.PodTemplateSpec) (bool, string) {
	// Assuming 0 is primary container
	if len(desired.Spec.Containers) <= 1 {
		return false, ""
	}
	currentByName := make(map[string]corev1.Container, len(current.Spec.Containers))
	for i := range current.Spec.Containers {
		c := current.Spec.Containers[i]
		currentByName[c.Name] = c
	}
	// compare each desired sidecar to the current one with the same name
	for i := 1; i < len(desired.Spec.Containers); i++ {
		d := desired.Spec.Containers[i]
		c, ok := currentByName[d.Name]
		// should not happen (Spec.Containers length check previously)
		// but this means spec differs, so we should sync
		if !ok {
			return true, fmt.Sprintf("adding sidecar container: %q", d.Name)
		}
		// currently only checking image for triggering rollout
		if d.Image != c.Image {
			return true, fmt.Sprintf("updating image for %q container", d.Name)
		}
	}
	return false, ""
}

func shouldUpdateContainerSpec(desired, current *corev1.Container, skipRes bool) (bool, string) {
	if desired.Image != current.Image {
		return true, fmt.Sprintf("updating image for %q container", desired.Name)
	}
	if shouldUpdateEnv(desired.Name, desired.Env, current.Env) {
		return true, fmt.Sprintf("updating env variables for %q container", desired.Name)
	}
	if !skipRes && shouldUpdateResources(&desired.Resources, &current.Resources) {
		return true, fmt.Sprintf("updating resource requests/limits for %q container", desired.Name)
	}
	if shouldUpdateProbes(desired, current) {
		return true, "updating health probes"
	}
	return false, ""
}

func shouldUpdateProbes(desired, current *corev1.Container) bool {
	return !equality.Semantic.DeepEqual(desired.LivenessProbe, current.LivenessProbe) ||
		!equality.Semantic.DeepEqual(desired.ReadinessProbe, current.ReadinessProbe) ||
		!equality.Semantic.DeepEqual(desired.StartupProbe, current.StartupProbe)
}

func shouldUpdateSecurityContext(desired, current *corev1.PodTemplateSpec) (bool, string) {
	desiredSpec := &desired.Spec
	currentSpec := &current.Spec
	// Pod-level securityContext
	// Both `desired.SecurityContext` and `current.SecurityContext` are
	// expected to be non-nil here as `SecurityContext` should be set by default.
	if !equality.Semantic.DeepEqual(desiredSpec.SecurityContext, currentSpec.SecurityContext) {
		return true, "updating security context for pod"
	}

	// Only sync securityContext for primary container -- do not restart cluster to add to sidecar or init
	// TODO: sync on all in next major version
	if !equality.Semantic.DeepEqual(desiredSpec.Containers[0].SecurityContext, currentSpec.Containers[0].SecurityContext) {
		return true, fmt.Sprintf("updating security context for container %s", desiredSpec.Containers[0].Name)
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

func shouldUpdateAnnotations(desired, current *corev1.PodTemplateSpec) (bool, string) {
	desiredAnn := desired.Annotations
	currentAnn := current.Annotations
	if equality.Semantic.DeepDerivative(desiredAnn, currentAnn) {
		return false, ""
	}
	reason := "updating annotations"
	restartHash, exists := desiredAnn[cmn.RestartConfigHashAnnotation]
	// At this point annotations are not equal -- If the restart hash does not exist, trigger sync
	if !exists {
		return true, reason
	}
	// If the hash is different and NOT initial, trigger sync
	nonInitial := !strings.HasSuffix(restartHash, cmn.RestartConfigHashInitial)
	if nonInitial && restartHash != currentAnn[cmn.RestartConfigHashAnnotation] {
		return true, "updating annotations due to changed restart config hash"
	}
	// Compare the desired to current WITHOUT the restart hash and trigger if not equivalent
	desiredCopy := make(map[string]string)
	for k, v := range desiredAnn {
		desiredCopy[k] = v
	}
	delete(desiredCopy, cmn.RestartConfigHashAnnotation)
	return !equality.Semantic.DeepDerivative(desiredCopy, currentAnn), reason
}

func shouldUpdateLabels(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if !equality.Semantic.DeepEqual(desired.Labels, current.Labels) {
		return true, "updating labels"
	}
	return false, ""
}

func shouldUpdateVolumes(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if len(desired.Spec.Volumes) > len(current.Spec.Volumes) {
		return true, "updating volumes (new volumes)"
	}
	if len(desired.Spec.Volumes) < len(current.Spec.Volumes) {
		return true, "updating volumes (removed volumes)"
	}
	currentMap := make(map[string]corev1.Volume, len(current.Spec.Volumes))
	for i := range current.Spec.Volumes {
		currentMap[current.Spec.Volumes[i].Name] = current.Spec.Volumes[i]
	}
	for i := range desired.Spec.Volumes {
		dv := desired.Spec.Volumes[i]
		cv, exists := currentMap[dv.Name]
		if !exists || !equality.Semantic.DeepEqual(dv, cv) {
			return true, fmt.Sprintf("updating volumes (%s changed)", dv.Name)
		}
	}
	return false, ""
}

func shouldUpdatePriorityClass(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if desired.Spec.PriorityClassName != current.Spec.PriorityClassName {
		return true, "updating priority class name"
	}
	return false, ""
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
			// Account for env var removals with a deep equal check
			if equality.Semantic.DeepEqual(daemon.desiredContainer.Env, daemon.currentContainer.Env) {
				continue
			}
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

// isScalingNeeded returns true if the StatefulSet spec replicas differ from the
// desired count specified in the CR, indicating a scaling operation should be initiated.
func isScalingNeeded(ss *appsv1.StatefulSet, desired int32) bool {
	return *ss.Spec.Replicas != desired
}

// isRolloutInProgress returns true if a StatefulSet has an active rollout.
//   - RollingUpdate: K8s only bumps CurrentRevision when all pods are updated AND ready,
//     so CurrentRevision != UpdateRevision naturally gates on readiness.
//   - OnDelete: K8s never bumps CurrentRevision. Fall back to checking whether any
//     existing pods still carry a stale revision via UpdatedReplicas (no readiness guarantee).
//     Compare against min(Spec, Status) because UpdatedReplicas excludes terminating pods
//     while Status.Replicas includes them. Using Status.Replicas alone would false-positive
//     during scale-down (e.g. Updated=2, Status=3 with a terminating pod looks like a rollout).
//     Using Spec.Replicas alone would false-positive during scale-up (e.g. Updated=2, Spec=4
//     with new pods starting looks like a rollout).
func isRolloutInProgress(ss *appsv1.StatefulSet) bool {
	if ss.Status.UpdateRevision == "" {
		return false
	}
	if ss.Status.CurrentRevision == ss.Status.UpdateRevision {
		return false
	}
	if ss.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return ss.Status.UpdatedReplicas < min(*ss.Spec.Replicas, ss.Status.Replicas)
	}
	return true
}

// isScalingInProgress returns true if pods are actively being created or terminated
// to match the spec replica count. Returns false during a rollout to avoid confusing
// pod churn during rollout with scaling.
func isScalingInProgress(ss *appsv1.StatefulSet) bool {
	if isRolloutInProgress(ss) {
		return false
	}
	return ss.Status.Replicas != *ss.Spec.Replicas
}

func isPodUnschedulable(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled &&
			cond.Status == corev1.ConditionFalse &&
			cond.Reason == corev1.PodReasonUnschedulable {
			return true
		}
	}
	return false
}

func isPodInCrashLoopBackOff(pod *corev1.Pod) bool {
	for i := range pod.Status.ContainerStatuses {
		cs := &pod.Status.ContainerStatuses[i]
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			return true
		}
	}
	return false
}

func shouldUpdatePVCRetentionPolicy(desired, current *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy) bool {
	// If desired is unset then we want current to be either unset or the default value.
	if desired == nil {
		return current != nil && (current.WhenDeleted != appsv1.RetainPersistentVolumeClaimRetentionPolicyType || current.WhenScaled != appsv1.RetainPersistentVolumeClaimRetentionPolicyType)
	}

	return !equality.Semantic.DeepEqual(desired, current)
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

func shouldUpdate(desired, current *corev1.PodTemplateSpec, funcs ...func(desired, current *corev1.PodTemplateSpec) (bool, string)) (bool, string) {
	for _, f := range funcs {
		update, reason := f(desired, current)
		if update {
			return update, reason
		}
	}
	return false, ""
}
