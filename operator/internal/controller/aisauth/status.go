/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// updateStatus persists the status the reconcile steps built up in memory.
func (r *Reconciler) updateStatus(ctx context.Context, base, authn *authv1alpha1.AIStoreAuth) error {
	authn.Status.ObservedGeneration = authn.GetGeneration()
	authn.Status.ServiceURL = authnres.ServiceURL(authn)

	if equality.Semantic.DeepEqual(base.Status, authn.Status) {
		return nil
	}
	return client.IgnoreNotFound(r.client.Status().Patch(ctx, authn, client.MergeFrom(base)))
}

// setReadyCondition sets Ready, stamping the generation it was evaluated against.
func setReadyCondition(
	authn *authv1alpha1.AIStoreAuth,
	status metav1.ConditionStatus,
	reason authv1alpha1.ConditionReason,
	message string,
) {
	meta.SetStatusCondition(&authn.Status.Conditions, metav1.Condition{
		Type:               string(authv1alpha1.ConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: authn.GetGeneration(),
	})
}

// isReady reports whether Ready is currently True.
func isReady(authn *authv1alpha1.AIStoreAuth) bool {
	return meta.IsStatusConditionTrue(authn.Status.Conditions, string(authv1alpha1.ConditionReady))
}

// hasReadyReason reports whether Ready already carries the given reason.
func hasReadyReason(authn *authv1alpha1.AIStoreAuth, reason authv1alpha1.ConditionReason) bool {
	condition := meta.FindStatusCondition(authn.Status.Conditions, string(authv1alpha1.ConditionReady))
	return condition != nil && condition.Reason == string(reason)
}
