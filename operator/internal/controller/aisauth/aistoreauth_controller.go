/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	eventReasonFailed = "Failed"
	actionReconcile   = "Reconciled"
)

// Reconciler reconciles an AIStoreAuth object.
type Reconciler struct {
	client   *aisclient.K8sClient
	scheme   *runtime.Scheme
	log      logr.Logger
	recorder events.EventRecorder
}

// NewReconcilerFromMgr builds a Reconciler from a controller manager.
func NewReconcilerFromMgr(mgr manager.Manager, logger logr.Logger) *Reconciler {
	return &Reconciler{
		client:   aisclient.NewClientFromMgr(mgr),
		scheme:   mgr.GetScheme(),
		log:      logger,
		recorder: mgr.GetEventRecorder("aistoreauth-controller"),
	}
}

// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auth.ais.nvidia.com,resources=aistoreauths/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	if err := r.reconcilePersistence(ctx, authn); err != nil {
		r.recordError(ctx, authn, err, "Failed to reconcile AuthN persistence")
		return reconcile.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, authn); err != nil {
		r.recordError(ctx, authn, err, "Failed to reconcile AuthN Deployment")
		return reconcile.Result{}, err
	}

	if err := r.reconcileServices(ctx, authn); err != nil {
		r.recordError(ctx, authn, err, "Failed to reconcile AuthN Services")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled AIStoreAuth")
	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcileConfigMap(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	cm, err := authnres.NewConfigMap(authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, cm); err != nil {
		return err
	}
	logf.FromContext(ctx).Info("AuthN ConfigMap applied", "name", authnres.ConfigMapName(authn))
	return nil
}

// reconcilePersistence creates the owned AuthN data PVC. PersistentVolumes are not
// managed by the operator, they are pre-provisioned with Helm (volumeName) or created
// by a StorageClass provisioner (storageClass).
//
// Changing an immutable PVC field (storageClassName, volumeName, or size on a
// non-expandable class) will fail server-side apply on every reconcile.
func (r *Reconciler) reconcilePersistence(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	pvc, err := authnres.NewPVC(authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, pvc); err != nil {
		return err
	}
	logf.FromContext(ctx).Info("AuthN PVC applied", "name", authnres.PVCName(authn))
	return nil
}

func (r *Reconciler) reconcileDeployment(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	deployment, err := authnres.NewDeployment(authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, deployment); err != nil {
		return err
	}
	logf.FromContext(ctx).Info("AuthN Deployment applied", "name", authnres.DeploymentName(authn))
	return nil
}

func (r *Reconciler) recordError(ctx context.Context, authn *authv1alpha1.AIStoreAuth, err error, msg string) {
	logf.FromContext(ctx).Error(err, msg)
	r.recorder.Eventf(authn, nil, corev1.EventTypeWarning, eventReasonFailed, actionReconcile,
		fmt.Sprintf("%s, err: %v", msg, err))
}

// SetupWithManager registers the reconciler with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authv1alpha1.AIStoreAuth{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("aistoreauth").
		Complete(r)
}
