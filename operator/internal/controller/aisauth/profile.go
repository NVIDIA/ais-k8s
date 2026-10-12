/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileAuthProfile applies the admin AIStoreAuthProfile while AuthN has an admin Secret and deletes it otherwise.
func (r *Reconciler) reconcileAuthProfile(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	profile, err := authnres.NewAuthProfile(authn)
	if err != nil {
		return err
	}
	if profile == nil {
		return r.deleteAuthProfile(ctx, authn)
	}
	existing, err := r.getAuthProfile(ctx, authn)
	if err != nil {
		return err
	}
	if existing != nil && !isAuthProfileOf(existing, authn) {
		return fmt.Errorf("AIStoreAuthProfile %q exists and is not managed by this AIStoreAuth", existing.Name)
	}
	if err := r.client.ApplyAuthProfile(ctx, profile); err != nil {
		return err
	}
	logf.FromContext(ctx).V(1).Info("AuthN admin AIStoreAuthProfile applied", "name", authnres.AuthProfileName(authn))
	return nil
}

// deleteAuthProfile removes the admin AIStoreAuthProfile if this AIStoreAuth manages it.
func (r *Reconciler) deleteAuthProfile(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	existing, err := r.getAuthProfile(ctx, authn)
	if err != nil || existing == nil || !isAuthProfileOf(existing, authn) {
		return err
	}
	uid := existing.UID
	_, err = r.client.DeleteResourceIfExists(ctx, existing, client.Preconditions{UID: &uid})
	return err
}

// getAuthProfile returns the admin AIStoreAuthProfile or nil if it does not exist.
func (r *Reconciler) getAuthProfile(
	ctx context.Context, authn *authv1alpha1.AIStoreAuth,
) (*authv1alpha1.AIStoreAuthProfile, error) {
	profile, err := r.client.GetAuthProfile(ctx, authnres.AuthProfileName(authn))
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	return profile, err
}

// isAuthProfileOf reports whether authn manages the profile.
func isAuthProfileOf(profile *authv1alpha1.AIStoreAuthProfile, authn *authv1alpha1.AIStoreAuth) bool {
	owner, managed := authnres.AuthProfileOwner(profile)
	return managed && owner == types.NamespacedName{Namespace: authn.Namespace, Name: authn.Name}
}

// findAIStoreAuthForProfile maps an AIStoreAuthProfile event to the AIStoreAuth that manages the profile.
func (*Reconciler) findAIStoreAuthForProfile(_ context.Context, obj client.Object) []reconcile.Request {
	profile, ok := obj.(*authv1alpha1.AIStoreAuthProfile)
	if !ok {
		return nil
	}
	owner, managed := authnres.AuthProfileOwner(profile)
	if !managed {
		return nil
	}
	return []reconcile.Request{{NamespacedName: owner}}
}
