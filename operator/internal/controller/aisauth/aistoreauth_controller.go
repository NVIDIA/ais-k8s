/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	eventReasonFailed = "Failed"
	actionReconcile   = "Reconciled"
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
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch

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

	if err := r.reconcileConfigMap(ctx, authn); err != nil {
		r.recordError(ctx, authn, err, "Failed to reconcile AuthN ConfigMap")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled AIStoreAuth")
	return reconcile.Result{}, nil
}

func (r *AIStoreAuthReconciler) reconcileConfigMap(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	cm, err := authnres.NewConfigMap(authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, cm, client.FieldOwner(aisclient.FieldOwner), client.ForceOwnership); err != nil {
		return err
	}
	logf.FromContext(ctx).Info("AuthN ConfigMap applied", "name", authnres.ConfigMapName(authn))
	return nil
}

func (r *AIStoreAuthReconciler) recordError(ctx context.Context, authn *authv1alpha1.AIStoreAuth, err error, msg string) {
	logf.FromContext(ctx).Error(err, msg)
	r.recorder.Eventf(authn, nil, corev1.EventTypeWarning, eventReasonFailed, actionReconcile,
		fmt.Sprintf("%s, err: %v", msg, err))
}

// SetupWithManager registers the reconciler with the manager.
func (r *AIStoreAuthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authv1alpha1.AIStoreAuth{}).
		Owns(&corev1.ConfigMap{}).
		Named("aistoreauth").
		Complete(r)
}
