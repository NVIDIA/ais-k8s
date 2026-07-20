/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// reconcileServices applies and converges services for AIStoreAuth.
func (r *Reconciler) reconcileServices(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	if err := r.client.Apply(ctx, authnres.NewService(authn)); err != nil {
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

	if err := r.updateServiceURL(ctx, authn); err != nil {
		return fmt.Errorf("update service URL status: %w", err)
	}
	logf.FromContext(ctx).Info("AuthN Services reconciled")
	return nil
}

func (r *Reconciler) applyOrDeleteService(
	ctx context.Context,
	authn *authv1alpha1.AIStoreAuth,
	service *corev1ac.ServiceApplyConfiguration,
	name types.NamespacedName,
) error {
	if service != nil {
		return r.client.Apply(ctx, service)
	}
	return r.deleteOwnedService(ctx, authn, name)
}

// deleteOwnedService removes a disabled optional Service only when this CR controls it.
func (r *Reconciler) deleteOwnedService(
	ctx context.Context,
	authn *authv1alpha1.AIStoreAuth,
	name types.NamespacedName,
) error {
	service := &corev1.Service{}
	if err := r.client.Get(ctx, name, service); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(service, authn) {
		logf.FromContext(ctx).V(1).Info("Leaving non-owned disabled Service unchanged", "service", name)
		return nil
	}
	uid := service.UID
	_, err := r.client.DeleteResourceIfExists(ctx, service, client.Preconditions{UID: &uid})
	return err
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
