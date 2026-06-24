/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/cmn"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SyncMode int

const (
	SyncModeIgnoreNone = iota
	SyncModeIgnoreRemovedEnv
	SyncModeIgnoreAddedEnv
)

const statefulsetRequeueDelay = time.Second

// Given a desired and current pod template spec, determine if we need to trigger a rollout to sync
// When the actual sync happens, specs will be updated
// In many cases we do not want every change to cause a restart, so those should be defined here
func shouldUpdatePodTemplate(desired, current *corev1.PodTemplateSpec) (bool, string) {
	// Define a series of functions to check
	// Each function returns whether we should trigger a rollout to sync along with a reason
	checks := []func(desired, current *corev1.PodTemplateSpec) (bool, string){
		shouldUpdateInitContainers,
		shouldUpdateContainers,
		shouldUpdatePodSecurityContext,
		shouldUpdateAnnotations,
		shouldUpdateLabels,
		shouldUpdateVolumes,
		shouldUpdatePriorityClass,
		shouldUpdateTolerations,
	}
	return shouldUpdate(desired, current, checks...)
}

// containerChecks declares which fields participate in rollout-trigger comparisons
// for a given container, scoped to user-controllable fields per container kind
// Name and Image are always compared.
// Operator-internal changes should not roll existing clusters on upgrade.
// Security Context comparison is an exception, enabled to force sync with the v3.0.0 release.
type containerChecks struct {
	env, resources, probes, securityContext bool
}

var (
	primaryContainerChecks = containerChecks{env: true, resources: true, probes: true, securityContext: true}
	sidecarContainerChecks = containerChecks{resources: true, securityContext: true}
	initContainerChecks    = containerChecks{securityContext: true}
)

// containerChecksFor returns the rollout-trigger policy for a container based on name
func containerChecksFor(name string) containerChecks {
	if name == cmn.AISContainerName {
		return primaryContainerChecks
	}
	return sidecarContainerChecks
}

func shouldUpdateInitContainers(desired, current *corev1.PodTemplateSpec) (bool, string) {
	desiredInit := desired.Spec.InitContainers
	currentInit := current.Spec.InitContainers
	if len(desiredInit) != len(currentInit) {
		return true, "updating desired init containers"
	}
	for i := range desiredInit {
		if update, reason := shouldUpdateContainerSpec(&desiredInit[i], &currentInit[i], initContainerChecks); update {
			return true, reason
		}
	}
	return false, ""
}

func shouldUpdateContainers(desired, current *corev1.PodTemplateSpec) (bool, string) {
	desiredCont := desired.Spec.Containers
	currentCont := current.Spec.Containers
	if len(desiredCont) != len(currentCont) {
		return true, "updating desired containers"
	}
	for i := range desiredCont {
		checks := containerChecksFor(desiredCont[i].Name)
		if update, reason := shouldUpdateContainerSpec(&desiredCont[i], &currentCont[i], checks); update {
			return true, reason
		}
	}
	return false, ""
}

func shouldUpdateContainerSpec(desired, current *corev1.Container, c containerChecks) (bool, string) {
	reason := func(detail string) string {
		return fmt.Sprintf("container %q: %s", desired.Name, detail)
	}
	if desired.Name != current.Name {
		return true, reason(fmt.Sprintf("renamed from %q", current.Name))
	}
	if desired.Image != current.Image {
		return true, reason("updating image")
	}
	if c.env && !equality.Semantic.DeepEqual(desired.Env, current.Env) {
		return true, reason("updating env variables")
	}
	if c.resources && !equality.Semantic.DeepEqual(desired.Resources, current.Resources) {
		return true, reason("updating resource requests/limits")
	}
	if c.probes && shouldUpdateProbes(desired, current) {
		return true, reason("updating health probes")
	}
	if c.securityContext && !equality.Semantic.DeepEqual(desired.SecurityContext, current.SecurityContext) {
		return true, reason("updating security context")
	}
	return false, ""
}

func shouldUpdateProbes(desired, current *corev1.Container) bool {
	return !equality.Semantic.DeepEqual(desired.LivenessProbe, current.LivenessProbe) ||
		!equality.Semantic.DeepEqual(desired.ReadinessProbe, current.ReadinessProbe) ||
		!equality.Semantic.DeepEqual(desired.StartupProbe, current.StartupProbe)
}

func shouldUpdatePodSecurityContext(desired, current *corev1.PodTemplateSpec) (bool, string) {
	// Pod-level securityContext
	// Both `desired.SecurityContext` and `current.SecurityContext` are
	// expected to be non-nil here as `SecurityContext` should be set by default.
	if !equality.Semantic.DeepEqual(desired.Spec.SecurityContext, current.Spec.SecurityContext) {
		return true, "updating security context for pod"
	}
	return false, ""
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

func shouldUpdateTolerations(desired, current *corev1.PodTemplateSpec) (bool, string) {
	if !equality.Semantic.DeepEqual(desired.Spec.Tolerations, current.Spec.Tolerations) {
		return true, "updating tolerations"
	}
	return false, ""
}

func syncPodTemplate(desired, current *corev1.PodTemplateSpec) (updated bool) {
	if syncContainers(desired.Spec.InitContainers, &current.Spec.InitContainers) {
		updated = true
	}
	if syncContainers(desired.Spec.Containers, &current.Spec.Containers) {
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

	if !equality.Semantic.DeepEqual(desired.Spec.Tolerations, current.Spec.Tolerations) {
		current.Spec.Tolerations = desired.Spec.Tolerations
		updated = true
	}

	return
}

func syncContainers(desired []corev1.Container, current *[]corev1.Container) bool {
	if len(desired) != len(*current) {
		*current = desired
		return true
	}
	updated := false
	for i := range desired {
		if syncContainer(&desired[i], &(*current)[i]) {
			updated = true
		}
	}
	return updated
}

func syncContainer(desired, current *corev1.Container) bool {
	if equality.Semantic.DeepEqual(*desired, *current) {
		return false
	}
	*current = *desired
	return true
}

// hostnameMatchesPod checks if a hostname belongs to the given pod by matching
// the pod name exactly or as the first DNS label of an FQDN (podName + ".").
// Plain HasPrefix is unsafe (e.g. "ais-target-1" prefixes "ais-target-10").
func hostnameMatchesPod(hostname, podName string) bool {
	return hostname == podName || strings.HasPrefix(hostname, podName+".")
}

func findAISNodeByPodName(nodeMap aismeta.NodeMap, podName string) (*aismeta.Snode, error) {
	for _, node := range nodeMap {
		if hostnameMatchesPod(node.ControlNet.Hostname, podName) {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no matching AIS node found for pod %q", podName)
}

// statefulsetScalingNeeded determines whether a daemon StatefulSet should be scaled.
func statefulsetScalingNeeded(ss *appsv1.StatefulSet, desired, maxUnavailable int32, autoScaling bool) bool {
	specReplicas := *ss.Spec.Replicas
	// Always scale UP the statefulset to match spec
	if desired > specReplicas {
		return true
	}
	// Already at the desired size
	if desired == specReplicas {
		return false
	}
	// Scaling down: a fixed-size cluster scales to exactly the desired size
	if !autoScaling {
		return true
	}
	// Autoscaling scale-down: only trust the status when it is settled, otherwise wait so
	// we don't act on a stale or in-flight pod count (e.g. a pod being recreated).
	if isRolloutInProgress(ss) || isScalingInProgress(ss) {
		return false
	}
	// Defer scale-down while unavailable pods are within the budget.
	unavailable := specReplicas - ss.Status.ReadyReplicas
	return unavailable == 0 || unavailable > maxUnavailable
}

// confirmScalingNeeded re-checks an autoscale scale-down against a non-cached read of the
// StatefulSet. The informer can lag a disruption (a pod going unready) and still show full
// readiness, which would let statefulsetScalingNeeded green-light a scale-down that
// decommissions a healthy daemon.
func (r *AIStoreReconciler) confirmScalingNeeded(ctx context.Context, key types.NamespacedName, cached *appsv1.StatefulSet, desired, maxUnavailable int32, autoScaling bool) (bool, error) {
	if !autoScaling || desired >= *cached.Spec.Replicas {
		return true, nil
	}
	fresh, err := r.k8sClient.GetStatefulSetDirect(ctx, key)
	if err != nil {
		return false, err
	}
	return statefulsetScalingNeeded(fresh, desired, maxUnavailable, autoScaling), nil
}

// isStatusCurrent returns true if the StatefulSet controller has processed the latest spec change.
// When metadata.generation > status.observedGeneration, status fields like UpdateRevision and
// CurrentRevision are stale and cannot be trusted.
func isStatusCurrent(ss *appsv1.StatefulSet) bool {
	return ss.Status.ObservedGeneration >= ss.Generation
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
	if pod == nil {
		return false
	}
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

// isStatefulSetFullyReady reports whether the StatefulSet is at the desired size with
// every pod updated to the latest revision and ready (no rollout or scaling in progress).
func (*AIStoreReconciler) isStatefulSetFullyReady(desiredSize int32, ss *appsv1.StatefulSet) bool {
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

// isStatefulSetReady reports whether a daemon StatefulSet is ready to proceed.
func (r *AIStoreReconciler) isStatefulSetReady(ss *appsv1.StatefulSet, desired, minReady int32, autoScaling bool) bool {
	if r.isStatefulSetFullyReady(desired, ss) {
		return true
	}
	if !autoScaling {
		return false
	}
	// When autoscaling enabled, tolerate configured amount of unavailable pods
	// if there is no ongoing rollout or scale operation.
	if isRolloutInProgress(ss) || isScalingInProgress(ss) {
		return false
	}
	return ss.Status.ReadyReplicas >= minReady
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

// toleratesTaints reports whether the given tolerations cover every NoSchedule
// / NoExecute taint on the node.
func toleratesTaints(ctx context.Context, tolerations []corev1.Toleration, node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		isTolerated := false
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(logf.FromContext(ctx), &taint, true) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false
		}
	}
	return true
}

// publicHostsForNodes returns host strings per publicNetDNSMode: node names for
// `publicNetDNSMode: Node`, primary IPs for `publicNetDNSMode: IP`, and no hosts
// for `publicNetDNSMode: Pod`. Caller is responsible for sort/dedup.
func publicHostsForNodes(nodes []corev1.Node, mode aisv1.PubNetDNSMode) []string {
	if mode == aisv1.PubNetDNSModePod {
		return nil
	}
	hosts := make([]string, 0, len(nodes))
	switch mode {
	case aisv1.PubNetDNSModeNode:
		for i := range nodes {
			hosts = append(hosts, nodes[i].Name)
		}
	case aisv1.PubNetDNSModeIP:
		for i := range nodes {
			if ip := aisclient.NodePrimaryIP(&nodes[i]); ip != "" {
				hosts = append(hosts, ip)
			}
		}
	}
	return hosts
}
