/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// reconcileServices applies and converges services for AIStoreAuth.
func (r *Reconciler) reconcileServices(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if err := r.client.ApplyService(ctx, authnres.NewService(authn)); err != nil {
		return fmt.Errorf("apply in-cluster Service: %w", err)
	}

	if err := r.applyOrDeleteService(
		ctx, authn, authnres.NewNodePortService(authn), authnres.NodePortServiceNSName(authn),
	); err != nil {
		return fmt.Errorf("reconcile NodePort Service: %w", err)
	}
	if err := r.applyOrDeleteService(
		ctx, authn, authnres.NewLoadBalancerService(authn), authnres.LoadBalancerServiceNSName(authn),
	); err != nil {
		return fmt.Errorf("reconcile LoadBalancer Service: %w", err)
	}

	logf.FromContext(ctx).V(1).Info("AuthN Services reconciled")
	return nil
}

func (r *Reconciler) applyOrDeleteService(
	ctx context.Context,
	authn *authv1alpha1.AIStoreAuth,
	service *corev1ac.ServiceApplyConfiguration,
	name types.NamespacedName,
) error {
	if service != nil {
		return r.client.ApplyService(ctx, service)
	}
	return r.client.DeleteOwnedService(ctx, name, authn)
}
