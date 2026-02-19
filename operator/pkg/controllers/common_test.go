package controllers

import (
	"github.com/NVIDIA/aistore/api/apc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
func makeSS(specReplicas, statusReplicas, updatedReplicas, readyReplicas int32, currentRev, updateRev string, strategy appsv1.StatefulSetUpdateStrategyType) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas:       apc.Ptr(specReplicas),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: strategy},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        statusReplicas,
			UpdatedReplicas: updatedReplicas,
			ReadyReplicas:   readyReplicas,
			CurrentRevision: currentRev,
			UpdateRevision:  updateRev,
		},
	}
}

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

var _ = Describe("isStatefulSetReady", func() {
	r := &AIStoreReconciler{}
	DescribeTable("should correctly detect readiness",
		func(ss *appsv1.StatefulSet, desiredSize int32, expected bool) {
			Expect(r.isStatefulSetReady(desiredSize, ss)).To(Equal(expected))
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
