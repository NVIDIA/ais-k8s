/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Ready condition messages for the AuthN Deployment.
const (
	msgAvailable      = "AuthN deployment is available"
	msgUnavailable    = "Waiting for the AuthN deployment to become available"
	msgNotProgressing = "The AuthN deployment rollout is no longer progressing"
)

// deploymentProgressDeadlineExceeded is the reason the Deployment controller sets on its
// Progressing condition once a rollout exceeds spec.progressDeadlineSeconds.
const deploymentProgressDeadlineExceeded = "ProgressDeadlineExceeded"

func (r *Reconciler) reconcileDeployment(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	deployment, err := authnres.NewDeployment(ctx, authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, deployment); err != nil {
		return err
	}
	r.setReadyFromDeployment(authn, deployment)
	logf.FromContext(ctx).V(1).Info("AuthN Deployment applied", "name", authnres.DeploymentName(authn))
	return nil
}

// setReadyFromDeployment records Ready from the Deployment the API server returned for our apply.
func (r *Reconciler) setReadyFromDeployment(
	authn *authv1alpha1.AIStoreAuth, deployment *appsv1ac.DeploymentApplyConfiguration,
) {
	related := deploymentRef(authn)
	if isRolloutComplete(deployment) {
		if !isReady(authn) {
			r.recorder.Eventf(authn, related, corev1.EventTypeNormal, EventReasonReady,
				ActionReconcile, msgAvailable)
		}
		setReadyCondition(authn, metav1.ConditionTrue,
			authv1alpha1.ReasonAvailable, msgAvailable)
		return
	}

	reason, message := rolloutPending(deployment)
	if reason == authv1alpha1.ReasonProgressDeadlineExceeded && !hasReadyReason(authn, reason) {
		r.recorder.Eventf(authn, related, corev1.EventTypeWarning, EventReasonRolloutStalled,
			ActionReconcile, "%s", message)
	}
	setReadyCondition(authn, metav1.ConditionFalse, reason, message)
}

// deploymentRef identifies the Deployment an event is about.
func deploymentRef(authn *authv1alpha1.AIStoreAuth) *appsv1.Deployment {
	name := authnres.DeploymentNSName(authn)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
	}
}

// rolloutPending reports why the Deployment is not available yet.
func rolloutPending(deployment *appsv1ac.DeploymentApplyConfiguration) (authv1alpha1.ConditionReason, string) {
	if isProgressDeadlineExceeded(deployment) {
		return authv1alpha1.ReasonProgressDeadlineExceeded, msgNotProgressing
	}
	return authv1alpha1.ReasonDeploymentUnavailable, msgUnavailable
}

// isProgressDeadlineExceeded reports whether the Deployment has given up on its current rollout.
func isProgressDeadlineExceeded(
	deployment *appsv1ac.DeploymentApplyConfiguration,
) bool {
	if deployment == nil || deployment.ObjectMetaApplyConfiguration == nil ||
		deployment.Status == nil ||
		value(deployment.Status.ObservedGeneration) < value(deployment.Generation) {
		return false
	}
	for i := range deployment.Status.Conditions {
		condition := &deployment.Status.Conditions[i]
		if value(condition.Type) == appsv1.DeploymentProgressing &&
			value(condition.Status) == corev1.ConditionFalse &&
			value(condition.Reason) == deploymentProgressDeadlineExceeded {
			return true
		}
	}
	return false
}

// isRolloutComplete reports whether the Deployment has finished rolling out its current generation.
func isRolloutComplete(deployment *appsv1ac.DeploymentApplyConfiguration) bool {
	if deployment == nil || deployment.ObjectMetaApplyConfiguration == nil ||
		deployment.Spec == nil || deployment.Status == nil {
		return false
	}
	desired := value(deployment.Spec.Replicas)
	status := deployment.Status
	return desired > 0 &&
		value(status.ObservedGeneration) >= value(deployment.Generation) &&
		value(status.UpdatedReplicas) == desired &&
		value(status.Replicas) == value(status.UpdatedReplicas) &&
		value(status.AvailableReplicas) == value(status.UpdatedReplicas)
}

// value dereferences a pointer field of an apply configuration.
func value[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
