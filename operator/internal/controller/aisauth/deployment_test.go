/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
)

var _ = Describe("AIStoreAuth Deployment rollout", Label("short"), func() {
	DescribeTable("isRolloutComplete",
		func(deployment *appsv1ac.DeploymentApplyConfiguration, expected bool) {
			Expect(isRolloutComplete(deployment)).To(Equal(expected))
		},
		Entry("the rollout landed", rolledOut(1, 1, 1, 1, 1), true),
		Entry("the Deployment has not observed the current generation", rolledOut(2, 1, 1, 1, 1), false),
		Entry("the Deployment is ahead of the generation just applied", rolledOut(1, 2, 1, 1, 1), true),
		Entry("an old replica is still terminating", rolledOut(1, 1, 2, 1, 1), false),
		Entry("the new replica is not updated yet", rolledOut(1, 1, 1, 0, 1), false),
		Entry("the new replica is not available yet", rolledOut(1, 1, 1, 1, 0), false),
		Entry("the apply response carried no status", appsv1ac.Deployment("ais-authn", "ais"), false),
		Entry("the apply response carried no metadata", withoutMetadata(), false),
		Entry("the Deployment is scaled to zero", scaledTo(0), false),
		Entry("the live spec asks for more than the operator applies", scaledTo(3), true),
	)

	DescribeTable("isProgressDeadlineExceeded",
		func(deployment *appsv1ac.DeploymentApplyConfiguration, expected bool) {
			Expect(isProgressDeadlineExceeded(deployment)).To(Equal(expected))
		},
		Entry("no conditions reported yet", rolledOut(1, 1, 1, 1, 0), false),
		Entry("the rollout is still progressing",
			progressing(1, corev1.ConditionTrue, "ReplicaSetUpdated"), false),
		Entry("the Deployment gave up on the rollout",
			progressing(1, corev1.ConditionFalse, deploymentProgressDeadlineExceeded), true),
		Entry("not progressing for some other reason",
			progressing(1, corev1.ConditionFalse, "ReplicaSetCreateError"), false),
		Entry("the verdict predates the current generation",
			progressing(2, corev1.ConditionFalse, deploymentProgressDeadlineExceeded), false),
	)
})

// rolledOut builds the apply response for a Deployment desiring one replica, reporting the given
// generation and replica counts.
func rolledOut(generation, observed int64, replicas, updated, available int32) *appsv1ac.DeploymentApplyConfiguration {
	return scaled(1, generation, observed, replicas, updated, available)
}

// withoutMetadata builds an otherwise-complete rollout whose response has no metadata..
func withoutMetadata() *appsv1ac.DeploymentApplyConfiguration {
	deployment := rolledOut(1, 1, 1, 1, 1)
	deployment.ObjectMetaApplyConfiguration = nil
	return deployment
}

// scaledTo builds the apply response for a Deployment fully rolled out at the given replica count.
func scaledTo(desired int32) *appsv1ac.DeploymentApplyConfiguration {
	return scaled(desired, 1, 1, desired, desired, desired)
}

func scaled(
	desired int32, generation, observed int64, replicas, updated, available int32,
) *appsv1ac.DeploymentApplyConfiguration {
	return appsv1ac.Deployment("ais-authn", "ais").
		WithGeneration(generation).
		WithSpec(appsv1ac.DeploymentSpec().WithReplicas(desired)).
		WithStatus(appsv1ac.DeploymentStatus().
			WithObservedGeneration(observed).
			WithReplicas(replicas).
			WithUpdatedReplicas(updated).
			WithAvailableReplicas(available))
}

// progressing builds an apply response carrying the given Progressing condition, observed at
// generation 1.
func progressing(
	generation int64, status corev1.ConditionStatus, reason string,
) *appsv1ac.DeploymentApplyConfiguration {
	deployment := rolledOut(generation, 1, 1, 1, 0)
	deployment.Status.WithConditions(appsv1ac.DeploymentCondition().
		WithType(appsv1.DeploymentProgressing).
		WithStatus(status).
		WithReason(reason))
	return deployment
}
