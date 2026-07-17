/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// reconcileServices applies and converges services for AIStoreAuth.
func (r *Reconciler) reconcileServices(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if err := r.client.Apply(ctx, authnres.NewService(authn)); err != nil {
		return fmt.Errorf("apply in-cluster Service: %w", err)
	}

	if err := r.updateServiceURL(ctx, authn); err != nil {
		return fmt.Errorf("update service URL status: %w", err)
	}
	logf.FromContext(ctx).Info("AuthN Services reconciled")
	return nil
}

func (r *Reconciler) updateServiceURL(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	serviceURL := authnres.ServiceURL(authn)
	if authn.Status.ServiceURL == serviceURL {
		return nil
	}
	base := authn.DeepCopy()
	authn.Status.ServiceURL = serviceURL
	return r.client.Status().Patch(ctx, authn, client.MergeFrom(base))
}
