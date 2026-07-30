/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// authnFinalizer gates AIStoreAuth deletion.
const authnFinalizer = "auth.ais.nvidia.com/finalizer"

// ensureFinalizer adds the AuthN finalizer on first reconcile.
func (r *Reconciler) ensureFinalizer(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if controllerutil.ContainsFinalizer(authn, authnFinalizer) {
		return nil
	}
	base := authn.DeepCopy()
	controllerutil.AddFinalizer(authn, authnFinalizer)
	return r.client.Patch(ctx, authn, client.MergeFrom(base))
}

// reconcileDeletion runs finalizer-driven cleanup and then releases the finalizer.
func (r *Reconciler) reconcileDeletion(ctx context.Context, authn *authv1alpha1.AIStoreAuth) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(authn, authnFinalizer) {
		return reconcile.Result{}, nil
	}

	logger.V(1).Info("Cleaning up AIStoreAuth resources")
	if err := r.cleanup(ctx, authn); err != nil {
		return reconcile.Result{}, err
	}
	r.recorder.Eventf(authn, nil, corev1.EventTypeNormal, EventReasonCleanupCompleted, ActionDelete,
		"AIStoreAuth resources cleaned up")

	base := authn.DeepCopy()
	if controllerutil.RemoveFinalizer(authn, authnFinalizer) {
		if err := r.client.PatchIfExists(ctx, authn, client.MergeFrom(base)); err != nil {
			msg := "Failed to remove AIStoreAuth finalizer"
			logger.Error(err, msg)
			r.recordCleanupFailure(ctx, authn, EventReasonFinalizerRemovalFailed, msg)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// recordCleanupFailure surfaces a stuck deletion on Ready.
func (r *Reconciler) recordCleanupFailure(
	ctx context.Context, authn *authv1alpha1.AIStoreAuth, eventReason, msg string,
) {
	if !hasReadyFailure(authn, msg) {
		r.recorder.Eventf(authn, nil, corev1.EventTypeWarning, eventReason, ActionDelete, "%s", msg)
	}
	base := authn.DeepCopy()
	setReadyCondition(authn, metav1.ConditionFalse, authv1alpha1.ReasonCleanupFailed, msg)
	if err := r.updateStatus(ctx, base, authn); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update AIStoreAuth status")
	}
}

// hasReadyFailure reports whether Ready already carries this exact cleanup failure.
func hasReadyFailure(authn *authv1alpha1.AIStoreAuth, msg string) bool {
	condition := meta.FindStatusCondition(authn.Status.Conditions, string(authv1alpha1.ConditionReady))
	return condition != nil &&
		condition.Reason == string(authv1alpha1.ReasonCleanupFailed) &&
		condition.Message == msg
}

// cleanup handles the operator-created resources that garbage collection cannot resolve on
// its own.
func (r *Reconciler) cleanup(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if err := r.retainPVC(ctx, authn); err != nil {
		msg := "Failed to retain AuthN data PVC"
		logf.FromContext(ctx).Error(err, msg)
		r.recordCleanupFailure(ctx, authn, EventReasonPVCRetentionFailed, msg)
		return err
	}
	return nil
}

// retainPVC drops the controller owner reference from the AuthN data PVC.
func (r *Reconciler) retainPVC(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if authn.Spec.Persistence.ShouldDeletePVC() {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.client.Get(ctx, authnres.PVCNSName(authn), pvc); err != nil {
		return client.IgnoreNotFound(err)
	}
	// Already released, or adopted by something else that is now responsible for it.
	if !metav1.IsControlledBy(pvc, authn) {
		return nil
	}
	base := pvc.DeepCopy()
	if err := controllerutil.RemoveControllerReference(authn, pvc, r.scheme); err != nil {
		return err
	}
	return r.client.PatchIfExists(ctx, pvc, client.MergeFrom(base))
}
