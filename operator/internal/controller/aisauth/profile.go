/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"github.com/ais-operator/internal/resources/ownerref"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileAuthProfile applies the admin AIStoreAuthProfile while AuthN has an admin Secret and deletes it otherwise.
func (r *Reconciler) reconcileAuthProfile(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	profile := authnres.NewAuthProfile(authn)
	if profile == nil {
		return r.client.DeleteAuthProfile(ctx, authnres.AuthProfileName(authn), authn)
	}
	if err := r.client.ApplyAuthProfile(ctx, profile); err != nil {
		return err
	}
	logf.FromContext(ctx).V(1).Info("AuthN admin AIStoreAuthProfile applied", "name", profile.Name)
	return nil
}

// findAIStoreAuthForProfile maps an AIStoreAuthProfile event to the AIStoreAuth that owns the profile.
func (r *Reconciler) findAIStoreAuthForProfile(ctx context.Context, obj client.Object) []reconcile.Request {
	profile, ok := obj.(*authv1alpha1.AIStoreAuthProfile)
	if !ok {
		return nil
	}
	owner, ok := ownerref.GetLabeledOwner(profile)
	if !ok || owner.Kind != ownerref.AIStoreAuthKind {
		return nil
	}
	// The cache rejects namespaces this operator does not watch.
	if err := r.client.Get(ctx, owner.NamespacedName, &authv1alpha1.AIStoreAuth{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			logf.FromContext(ctx).Info("Skipping profile event for an AIStoreAuth outside the cache", "owner", owner, "error", err.Error())
		}
		return nil
	}
	return []reconcile.Request{{NamespacedName: owner.NamespacedName}}
}
