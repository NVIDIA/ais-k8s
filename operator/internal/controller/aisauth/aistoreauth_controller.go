// Package aisauth contains k8s controller logic for the AuthN authentication server.
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AIStoreAuthReconciler reconciles an AIStoreAuth object.
type AIStoreAuthReconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	recorder events.EventRecorder
}

// NewAIStoreAuthReconcilerFromMgr builds an AIStoreAuthReconciler from a controller manager.
func NewAIStoreAuthReconcilerFromMgr(mgr manager.Manager, logger logr.Logger) *AIStoreAuthReconciler {
	return &AIStoreAuthReconciler{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		log:      logger,
		recorder: mgr.GetEventRecorder("aistoreauth-controller"),
	}
}

// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths/finalizers,verbs=update

func (r *AIStoreAuthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.log.WithValues("namespace", req.Namespace, "name", req.Name)
	ctx = logf.IntoContext(ctx, logger)

	authn := &authv1alpha1.AIStoreAuth{}
	if err := r.client.Get(ctx, req.NamespacedName, authn); err != nil {
		if k8serrors.IsNotFound(err) {
			// CR was deleted; owned objects are garbage collected via ownerRefs.
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Unable to fetch AIStoreAuth")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciling AIStoreAuth")
	return reconcile.Result{}, nil
}

// SetupWithManager registers the reconciler with the manager.
func (r *AIStoreAuthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authv1alpha1.AIStoreAuth{}).
		Named("aistoreauth").
		Complete(r)
}
