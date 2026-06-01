// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"github.com/NVIDIA/aistore/api/apc"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	"github.com/ais-operator/pkg/resources/cmn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("compareEnvWithIgnored", func() {
	makeEnv := func(k, v string) corev1.EnvVar {
		return corev1.EnvVar{Name: k, Value: v}
	}

	It("returns false when env slices are identical and nothing ignored", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		ignored := map[string]struct{}{}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeFalse())
	})

	It("returns true when non-ignored env differs", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "DIFF")}
		ignored := map[string]struct{}{"IGNORED": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeTrue())
	})

	It("ignores changes to removed env when mode is IgnoreRemovedEnv", func() {
		des := []corev1.EnvVar{makeEnv("A", "1")} // B removed from desired
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		ignored := map[string]struct{}{"B": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreRemovedEnv)

		Expect(changed).To(BeFalse())
	})

	It("ignores changes to added env when mode is IgnoreAddedEnv", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1")} // B only in desired
		ignored := map[string]struct{}{"B": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreAddedEnv)

		Expect(changed).To(BeFalse())
	})

	It("detects value changes for ignored env when present in both", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "y")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeTrue())
	})

	It("skips ignored env missing from desired in IgnoreRemovedEnv mode", func() {
		des := []corev1.EnvVar{makeEnv("A", "1")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreRemovedEnv)

		Expect(changed).To(BeFalse())
	})

	It("skips ignored env missing from current in IgnoreAddedEnv mode", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		cur := []corev1.EnvVar{makeEnv("A", "1")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreAddedEnv)

		Expect(changed).To(BeFalse())
	})
})

// makeSS is a helper that builds a StatefulSet with the given spec/status fields.
// Generation and ObservedGeneration both default to 1 (status is current).
func makeSS(specReplicas, statusReplicas, updatedReplicas, readyReplicas int32, currentRev, updateRev string, strategy appsv1.StatefulSetUpdateStrategyType) *appsv1.StatefulSet {
	ss := makeSSWithGeneration(specReplicas, statusReplicas, updatedReplicas, readyReplicas, currentRev, updateRev, strategy, 1, 1)
	return ss
}

// makeSSWithGeneration extends makeSS with explicit generation and observedGeneration values.
func makeSSWithGeneration(specReplicas, statusReplicas, updatedReplicas, readyReplicas int32, currentRev, updateRev string, strategy appsv1.StatefulSetUpdateStrategyType, generation, observedGeneration int64) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: generation,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       apc.Ptr(specReplicas),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: strategy},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: observedGeneration,
			Replicas:           statusReplicas,
			UpdatedReplicas:    updatedReplicas,
			ReadyReplicas:      readyReplicas,
			CurrentRevision:    currentRev,
			UpdateRevision:     updateRev,
		},
	}
}

var _ = Describe("isStatusCurrent", func() {
	DescribeTable("should detect whether status is current",
		func(generation, observedGeneration int64, expected bool) {
			ss := makeSSWithGeneration(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType, generation, observedGeneration)
			Expect(isStatusCurrent(ss)).To(Equal(expected))
		},
		Entry("generation matches observed (current)", int64(1), int64(1), true),
		Entry("generation ahead of observed (stale)", int64(2), int64(1), false),
		Entry("both zero (fresh object)", int64(0), int64(0), true),
	)
})

var _ = Describe("isRolloutInProgress", func() {
	check := func(ss *appsv1.StatefulSet, expected bool) {
		Expect(isRolloutInProgress(ss)).To(Equal(expected))
	}

	Context("RollingUpdate (proxy)", func() {
		DescribeTable("should correctly detect rollout state", check,
			Entry("fresh SS with no revisions",
				makeSS(3, 3, 0, 3, "", "", appsv1.RollingUpdateStatefulSetStrategyType),
				false,
			),
			Entry("revisions match and rollout complete",
				makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
				false,
			),
			Entry("revisions differ, partial update",
				makeSS(3, 3, 1, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType),
				true,
			),
			Entry("revisions differ, all pods updated",
				makeSS(3, 3, 3, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType),
				true,
			),
		)
	})

	Context("OnDelete (target)", func() {
		DescribeTable("should correctly detect rollout state", check,
			Entry("fresh SS with no revisions",
				makeSS(3, 3, 0, 3, "", "", appsv1.OnDeleteStatefulSetStrategyType),
				false,
			),
			Entry("revisions match and rollout complete",
				makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.OnDeleteStatefulSetStrategyType),
				false,
			),
			Entry("revisions differ, all pods updated",
				makeSS(3, 3, 3, 3, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
				false,
			),
			Entry("revisions differ, partial update",
				makeSS(3, 3, 1, 3, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
				true,
			),
			Entry("revisions differ, no pods updated",
				makeSS(3, 3, 0, 3, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
				true,
			),
			Entry("scale-down with terminating pod should not false-positive as rollout",
				// Spec=2, Status=3 (terminating pod), Updated=2 (terminating excluded)
				makeSS(2, 3, 2, 2, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
				false,
			),
			Entry("scale-up with new pods starting should not false-positive as rollout",
				// Spec=4, Status=2 (new pods not yet created), Updated=2
				makeSS(4, 2, 2, 2, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
				false,
			),
		)
	})
})

var _ = Describe("isScalingInProgress", func() {
	DescribeTable("should correctly detect scaling state",
		func(ss *appsv1.StatefulSet, expected bool) {
			Expect(isScalingInProgress(ss)).To(Equal(expected))
		},
		Entry("status matches spec (no scaling)",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			false,
		),
		Entry("status < spec (scaling up)",
			makeSS(5, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			true,
		),
		Entry("status > spec (scaling down)",
			makeSS(3, 5, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			true,
		),
		Entry("status != spec but RollingUpdate rollout in progress (not scaling)",
			makeSS(3, 4, 1, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType),
			false,
		),
		Entry("status != spec, OnDelete rollout done (scaling)",
			makeSS(3, 4, 4, 4, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
			true,
		),
		Entry("status != spec, OnDelete rollout in progress (not scaling)",
			makeSS(3, 4, 1, 3, "rev-1", "rev-2", appsv1.OnDeleteStatefulSetStrategyType),
			false,
		),
		Entry("fresh SS with zero status replicas",
			makeSS(3, 0, 0, 0, "", "", appsv1.RollingUpdateStatefulSetStrategyType),
			true,
		),
		Entry("scaled to zero (status matches)",
			makeSS(0, 0, 0, 0, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			false,
		),
	)
})

var _ = Describe("isPodUnschedulable", func() {
	It("returns true when pod has Unschedulable condition", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: corev1.PodReasonUnschedulable,
					},
				},
			},
		}
		Expect(isPodUnschedulable(pod)).To(BeTrue())
	})

	It("returns false when pod is scheduled", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		Expect(isPodUnschedulable(pod)).To(BeFalse())
	})

	It("returns false when pod has no conditions", func() {
		pod := &corev1.Pod{}
		Expect(isPodUnschedulable(pod)).To(BeFalse())
	})

	It("returns false when pod is nil", func() {
		Expect(isPodUnschedulable(nil)).To(BeFalse())
	})

	It("returns false when PodScheduled is false for a different reason", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: "SomeOtherReason",
					},
				},
			},
		}
		Expect(isPodUnschedulable(pod)).To(BeFalse())
	})
})

var _ = Describe("isPodInCrashLoopBackOff", func() {
	It("returns true when a container is in CrashLoopBackOff", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
					{
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff",
							},
						},
					},
				},
			},
		}
		Expect(isPodInCrashLoopBackOff(pod)).To(BeTrue())
	})

	It("returns false when all containers are running", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		}
		Expect(isPodInCrashLoopBackOff(pod)).To(BeFalse())
	})

	It("returns false when pod has no container statuses", func() {
		pod := &corev1.Pod{}
		Expect(isPodInCrashLoopBackOff(pod)).To(BeFalse())
	})

	It("returns false when container is waiting for a different reason", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "ImagePullBackOff",
							},
						},
					},
				},
			},
		}
		Expect(isPodInCrashLoopBackOff(pod)).To(BeFalse())
	})
})

var _ = Describe("findAISNodeByPodName", func() {
	makeNode := func(id, hostname string) *aismeta.Snode {
		return &aismeta.Snode{
			DaeID:      id,
			ControlNet: aismeta.NetInfo{Hostname: hostname},
		}
	}

	It("returns the node whose hostname exactly matches the pod name", func() {
		target1 := makeNode("t1", "ais-target-1")
		target10 := makeNode("t10", "ais-target-10")
		nodeMap := aismeta.NodeMap{"t1": target1, "t10": target10}

		node, err := findAISNodeByPodName(nodeMap, "ais-target-1")

		Expect(err).NotTo(HaveOccurred())
		Expect(node).To(Equal(target1))
	})

	It("does not match a longer hostname that shares the pod name as a prefix", func() {
		// Reproduces the rollout bug: with a plain HasPrefix check, looking up
		// "ais-target-1" could return the node for ais-target-10 (or ..-100),
		// which caused the operator to put the wrong target into maintenance.
		target10 := makeNode("t10", "ais-target-10")
		target100 := makeNode("t100", "ais-target-100")
		nodeMap := aismeta.NodeMap{"t10": target10, "t100": target100}

		node, err := findAISNodeByPodName(nodeMap, "ais-target-1")

		Expect(err).To(HaveOccurred())
		Expect(node).To(BeNil())
	})

	It("matches an FQDN whose first label is the pod name", func() {
		target1 := makeNode("t1", "ais-target-1.ais-target.ais.svc.cluster.local")
		target10 := makeNode("t10", "ais-target-10.ais-target.ais.svc.cluster.local")
		nodeMap := aismeta.NodeMap{"t1": target1, "t10": target10}

		node, err := findAISNodeByPodName(nodeMap, "ais-target-1")

		Expect(err).NotTo(HaveOccurred())
		Expect(node).To(Equal(target1))
	})

	It("returns an error when no node matches", func() {
		nodeMap := aismeta.NodeMap{"t2": makeNode("t2", "ais-target-2")}

		node, err := findAISNodeByPodName(nodeMap, "ais-target-1")

		Expect(err).To(HaveOccurred())
		Expect(node).To(BeNil())
	})
})

var _ = Describe("shouldUpdateContainers", func() {
	secCtx := func(nonRoot bool) *corev1.SecurityContext {
		return &corev1.SecurityContext{RunAsNonRoot: apc.Ptr(nonRoot)}
	}
	resources := func(cpu string) corev1.ResourceRequirements {
		return corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)}}
	}
	probe := func(period int32) *corev1.Probe {
		return &corev1.Probe{PeriodSeconds: period}
	}

	makePodTemplate := func(containers []corev1.Container) *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: containers}}
	}

	It("returns false when both templates are identical", func() {
		base := []corev1.Container{
			{Name: cmn.AISContainerName, Image: "node:latest", SecurityContext: secCtx(true)},
			{Name: "ais-logs", Image: "logs:latest", SecurityContext: secCtx(false)},
		}
		update, _ := shouldUpdateContainers(makePodTemplate(base), makePodTemplate(base))
		Expect(update).To(BeFalse())
	})

	It("returns true when container length differs", func() {
		desired := makePodTemplate([]corev1.Container{
			{Name: "old-node", Image: "test:latest"},
			{Name: "new-node", Image: "test:latest"},
		})
		current := makePodTemplate([]corev1.Container{{Name: "old-node", Image: "test:latest"}})
		update, reason := shouldUpdateContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`updating desired containers`))
	})

	It("returns true when a container is renamed at the same index", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "new-node", Image: "test:latest"}})
		current := makePodTemplate([]corev1.Container{{Name: "old-node", Image: "test:latest"}})
		update, reason := shouldUpdateContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "new-node": renamed from "old-node"`))
	})

	Describe("primary container (all user-controllable fields trigger rollout)", func() {
		It("returns true when the image differs", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Image: "node:new"}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Image: "node:old"}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating image`))
		})

		It("returns true when env differs", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Env: []corev1.EnvVar{{Name: "A", Value: "1"}}}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Env: []corev1.EnvVar{{Name: "A", Value: "2"}}}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating env variables`))
		})

		It("returns true when resources differ", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Resources: resources("200m")}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, Resources: resources("100m")}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating resource requests/limits`))
		})

		It("returns true when a probe differs", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, LivenessProbe: probe(5)}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, LivenessProbe: probe(10)}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating health probes`))
		})

		It("returns true when the security context differs", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, SecurityContext: secCtx(true)}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, SecurityContext: secCtx(false)}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating security context`))
		})

		It("returns true when the security context is added", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, SecurityContext: secCtx(true)}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating security context`))
		})

		It("returns true when the security context is removed", func() {
			desired := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName}})
			current := makePodTemplate([]corev1.Container{{Name: cmn.AISContainerName, SecurityContext: secCtx(true)}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-node": updating security context`))
		})

		Describe("AIS primary container security context values", func() {
			defaultSC := cmn.DefaultAISContainerSecurityContext()
			makePrimary := func(sc *corev1.SecurityContext) *corev1.PodTemplateSpec {
				return makePodTemplate([]corev1.Container{{
					Name:            cmn.AISContainerName,
					Image:           "node:latest",
					SecurityContext: sc,
				}})
			}

			It("returns false when both templates use the default security context", func() {
				update, _ := shouldUpdateContainers(makePrimary(defaultSC), makePrimary(defaultSC))
				Expect(update).To(BeFalse())
			})

			It("returns true when AISContainerSecurityContext overrides the default", func() {
				custom := &corev1.SecurityContext{
					RunAsUser:                apc.Ptr(int64(123)),
					RunAsNonRoot:             apc.Ptr(true),
					AllowPrivilegeEscalation: apc.Ptr(false),
				}
				update, reason := shouldUpdateContainers(makePrimary(custom), makePrimary(defaultSC))
				Expect(update).To(BeTrue())
				Expect(reason).To(Equal(`container "ais-node": updating security context`))
			})

			It("returns false when custom security context matches on both sides", func() {
				custom := &corev1.SecurityContext{
					RunAsUser:                apc.Ptr(int64(123)),
					RunAsNonRoot:             apc.Ptr(true),
					AllowPrivilegeEscalation: apc.Ptr(false),
				}
				update, _ := shouldUpdateContainers(makePrimary(custom), makePrimary(custom))
				Expect(update).To(BeFalse())
			})
		})
	})

	Describe("sidecar (only image/resources/securityContext trigger sync)", func() {
		primary := corev1.Container{Name: cmn.AISContainerName, Image: "node:latest"}

		It("returns true when the sidecar image differs", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Image: "logs:new"}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Image: "logs:old"}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-logs": updating image`))
		})

		It("returns true when the sidecar resources differ", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Resources: resources("200m")}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Resources: resources("100m")}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-logs": updating resource requests/limits`))
		})

		It("returns false when only the sidecar env differs (operator-internal)", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Env: []corev1.EnvVar{{Name: "A", Value: "1"}}}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", Env: []corev1.EnvVar{{Name: "A", Value: "2"}}}})
			update, _ := shouldUpdateContainers(desired, current)
			Expect(update).To(BeFalse())
		})

		It("returns false when only the sidecar probes differ (operator-internal)", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", LivenessProbe: probe(5)}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", LivenessProbe: probe(10)}})
			update, _ := shouldUpdateContainers(desired, current)
			Expect(update).To(BeFalse())
		})

		It("returns true when the sidecar security context differs", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", SecurityContext: secCtx(true)}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", SecurityContext: secCtx(false)}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-logs": updating security context`))
		})

		It("returns true when the sidecar security context is added", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", SecurityContext: secCtx(true)}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs"}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-logs": updating security context`))
		})

		It("returns true when the sidecar security context is removed", func() {
			desired := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs"}})
			current := makePodTemplate([]corev1.Container{primary, {Name: "ais-logs", SecurityContext: secCtx(true)}})
			update, reason := shouldUpdateContainers(desired, current)
			Expect(update).To(BeTrue())
			Expect(reason).To(Equal(`container "ais-logs": updating security context`))
		})
	})
})

var _ = Describe("shouldUpdateInitContainers", func() {
	secCtx := func(nonRoot bool) *corev1.SecurityContext {
		return &corev1.SecurityContext{RunAsNonRoot: apc.Ptr(nonRoot)}
	}
	resources := func(cpu string) corev1.ResourceRequirements {
		return corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)}}
	}

	makePodTemplate := func(initContainers []corev1.Container) *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{Spec: corev1.PodSpec{InitContainers: initContainers}}
	}

	It("returns true when init container length differs", func() {
		desired := makePodTemplate([]corev1.Container{
			{Name: "old-init", Image: "test:latest"},
			{Name: "new-init", Image: "test:latest"},
		})
		current := makePodTemplate([]corev1.Container{{Name: "old-init", Image: "test:latest"}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`updating desired init containers`))
	})

	It("returns true when an init container is renamed at the same index", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "new-init", Image: "test:latest"}})
		current := makePodTemplate([]corev1.Container{{Name: "old-init", Image: "test:latest"}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "new-init": renamed from "old-init"`))
	})

	It("returns true when an init container image differs", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "init", Image: "init:new"}})
		current := makePodTemplate([]corev1.Container{{Name: "init", Image: "init:old"}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "init": updating image`))
	})

	// Operator-internal fields that are not part of the per-kind init policy.
	// These must not cause rollouts on operator upgrades.
	DescribeTable("returns false when only an operator-internal init container field differs",
		func(desired, current corev1.Container) {
			desired.Name = "init"
			current.Name = "init"
			update, _ := shouldUpdateInitContainers(
				makePodTemplate([]corev1.Container{desired}),
				makePodTemplate([]corev1.Container{current}),
			)
			Expect(update).To(BeFalse())
		},
		Entry("env",
			corev1.Container{Env: []corev1.EnvVar{{Name: "A", Value: "1"}}},
			corev1.Container{Env: []corev1.EnvVar{{Name: "A", Value: "2"}}},
		),
		Entry("resources",
			corev1.Container{Resources: resources("200m")},
			corev1.Container{Resources: resources("100m")},
		),
	)

	It("returns true when an init container security context differs", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "init", SecurityContext: secCtx(true)}})
		current := makePodTemplate([]corev1.Container{{Name: "init", SecurityContext: secCtx(false)}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "init": updating security context`))
	})

	It("returns true when an init container security context is added", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "init", SecurityContext: secCtx(true)}})
		current := makePodTemplate([]corev1.Container{{Name: "init"}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "init": updating security context`))
	})

	It("returns true when an init container security context is removed", func() {
		desired := makePodTemplate([]corev1.Container{{Name: "init"}})
		current := makePodTemplate([]corev1.Container{{Name: "init", SecurityContext: secCtx(true)}})
		update, reason := shouldUpdateInitContainers(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal(`container "init": updating security context`))
	})
})

var _ = Describe("syncContainers", func() {
	It("replaces the slice when lengths differ", func() {
		desired := []corev1.Container{
			{Name: cmn.AISContainerName, Image: "node:latest"},
			{Name: "ais-logs", Image: "logs:latest"},
		}
		current := []corev1.Container{{Name: cmn.AISContainerName, Image: "node:latest"}}
		Expect(syncContainers(desired, &current)).To(BeTrue())
		Expect(current).To(Equal(desired))
	})

	It("syncs an individual container when it differs", func() {
		desired := []corev1.Container{
			{Name: cmn.AISContainerName, Image: "node:latest"},
			{Name: "ais-logs", Image: "logs:new"},
		}
		current := []corev1.Container{
			{Name: cmn.AISContainerName, Image: "node:latest"},
			{Name: "ais-logs", Image: "logs:old"},
		}
		Expect(syncContainers(desired, &current)).To(BeTrue())
		Expect(current[1].Image).To(Equal("logs:new"))
	})

	It("syncs primary container security context when it differs from default", func() {
		custom := &corev1.SecurityContext{
			RunAsUser:                apc.Ptr(int64(123)),
			RunAsNonRoot:             apc.Ptr(true),
			AllowPrivilegeEscalation: apc.Ptr(false),
		}
		desired := []corev1.Container{{
			Name:            cmn.AISContainerName,
			Image:           "node:latest",
			SecurityContext: custom,
		}}
		current := []corev1.Container{{
			Name:            cmn.AISContainerName,
			Image:           "node:latest",
			SecurityContext: cmn.DefaultAISContainerSecurityContext(),
		}}
		Expect(syncContainers(desired, &current)).To(BeTrue())
		Expect(current[0].SecurityContext).To(Equal(custom))
	})
})

var _ = Describe("shouldUpdateTolerations", func() {
	makePodTemplate := func(tolerations []corev1.Toleration) *corev1.PodTemplateSpec {
		pt := &corev1.PodTemplateSpec{}
		pt.Spec.Tolerations = tolerations
		return pt
	}

	It("returns false when both have no tolerations", func() {
		desired := makePodTemplate(nil)
		current := makePodTemplate(nil)
		update, _ := shouldUpdateTolerations(desired, current)
		Expect(update).To(BeFalse())
	})

	It("returns true when a toleration is added", func() {
		tol := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
		desired := makePodTemplate([]corev1.Toleration{tol})
		current := makePodTemplate(nil)
		update, reason := shouldUpdateTolerations(desired, current)
		Expect(update).To(BeTrue())
		Expect(reason).To(Equal("updating tolerations"))
	})

	It("returns true when a toleration is removed", func() {
		tol := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
		desired := makePodTemplate(nil)
		current := makePodTemplate([]corev1.Toleration{tol})
		update, _ := shouldUpdateTolerations(desired, current)
		Expect(update).To(BeTrue())
	})

	It("returns true when a toleration is modified", func() {
		tol := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
		modified := tol
		modified.Effect = corev1.TaintEffectNoExecute
		desired := makePodTemplate([]corev1.Toleration{modified})
		current := makePodTemplate([]corev1.Toleration{tol})
		update, _ := shouldUpdateTolerations(desired, current)
		Expect(update).To(BeTrue())
	})

	It("returns false when tolerations are identical", func() {
		tol := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
		desired := makePodTemplate([]corev1.Toleration{tol})
		current := makePodTemplate([]corev1.Toleration{tol})
		update, _ := shouldUpdateTolerations(desired, current)
		Expect(update).To(BeFalse())
	})
})

var _ = Describe("hostnameMatchesPod", func() {
	DescribeTable("matches only when the hostname's first label equals the pod name",
		func(hostname, podName string, expected bool) {
			Expect(hostnameMatchesPod(hostname, podName)).To(Equal(expected))
		},
		Entry("exact match", "ais-target-1", "ais-target-1", true),
		Entry("FQDN first label match", "ais-target-1.ais-target.ais.svc.cluster.local", "ais-target-1", true),
		Entry("rooted FQDN first label match", "ais-target-1.ais-target.ais.svc.cluster.local.", "ais-target-1", true),
		Entry("longer ordinal must not match (bare)", "ais-target-10", "ais-target-1", false),
		Entry("longer ordinal must not match (FQDN)", "ais-target-10.ais-target.ais.svc.cluster.local", "ais-target-1", false),
		Entry("unrelated hostname", "ais-proxy-1", "ais-target-1", false),
		Entry("empty hostname", "", "ais-target-1", false),
	)
})

var _ = Describe("isStatefulSetFullyReady", func() {
	r := &AIStoreReconciler{}
	DescribeTable("should correctly detect readiness",
		func(ss *appsv1.StatefulSet, desiredSize int32, expected bool) {
			Expect(r.isStatefulSetFullyReady(desiredSize, ss)).To(Equal(expected))
		},
		Entry("all conditions met",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			true,
		),
		Entry("spec != desired",
			makeSS(5, 5, 5, 5, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			false,
		),
		Entry("not all replicas ready",
			makeSS(3, 3, 3, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			false,
		),
		Entry("status.Replicas != spec (terminating pods)",
			makeSS(3, 4, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			false,
		),
		Entry("update revision set but not all updated",
			makeSS(3, 3, 2, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			false,
		),
		Entry("scaling in progress",
			makeSS(5, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType), int32(5),
			false,
		),
		Entry("no update revision (ready if counts match)",
			makeSS(3, 3, 0, 3, "", "", appsv1.RollingUpdateStatefulSetStrategyType), int32(3),
			true,
		),
	)
})

var _ = Describe("statefulsetScalingNeeded", func() {
	DescribeTable("should decide whether scaling is needed",
		func(ss *appsv1.StatefulSet, desired, maxUnavailable int32, autoScaling, expected bool) {
			Expect(statefulsetScalingNeeded(ss, desired, maxUnavailable, autoScaling)).To(Equal(expected))
		},
		Entry("fixed: scale up to desired",
			makeSS(1, 1, 1, 1, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(0), false, true,
		),
		Entry("fixed: scale down to desired",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(1), int32(0), false, true,
		),
		Entry("fixed: already at desired",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(0), false, false,
		),
		Entry("auto: always scale up to desired",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(5), int32(1), true, true,
		),
		Entry("auto: unavailable within tolerance, no scale down",
			makeSS(3, 3, 3, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(2), int32(1), true, false,
		),
		Entry("auto: unavailable exceeds tolerance, scale down",
			makeSS(3, 3, 3, 1, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(2), int32(1), true, true,
		),
		Entry("auto: healthy cluster scales down to desired",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(2), int32(1), true, true,
		),
		Entry("auto: scaling in flight defers scale down",
			makeSS(3, 2, 2, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(2), int32(1), true, false,
		),
		Entry("auto: rollout in flight defers scale down",
			makeSS(3, 3, 2, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(2), int32(1), true, false,
		),
	)
})

var _ = Describe("statefulsetReady", func() {
	r := &AIStoreReconciler{}
	DescribeTable("should decide readiness with autoscaling tolerance",
		func(ss *appsv1.StatefulSet, desired, minReady int32, autoScaling, expected bool) {
			Expect(r.isStatefulSetReady(ss, desired, minReady, autoScaling)).To(Equal(expected))
		},
		Entry("fully ready",
			makeSS(3, 3, 3, 3, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(3), false, true,
		),
		Entry("fixed: not fully ready is not ready",
			makeSS(3, 3, 3, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(3), false, false,
		),
		Entry("auto: not fully ready but meets minReady",
			makeSS(3, 3, 3, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(2), true, true,
		),
		Entry("auto: not fully ready and below minReady",
			makeSS(3, 3, 3, 1, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(2), true, false,
		),
		Entry("auto: meets minReady but rollout in flight is not ready",
			makeSS(3, 3, 2, 3, "rev-1", "rev-2", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(2), true, false,
		),
		Entry("auto: meets minReady but scaling in flight is not ready",
			makeSS(3, 2, 2, 2, "rev-1", "rev-1", appsv1.RollingUpdateStatefulSetStrategyType),
			int32(3), int32(2), true, false,
		),
	)
})
