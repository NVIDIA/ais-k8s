/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete

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

	if !authn.GetDeletionTimestamp().IsZero() {
		return r.reconcileDeletion(ctx, authn)
	}

	if err := r.ensureFinalizer(ctx, authn); err != nil {
		msg := "Failed to add AIStoreAuth finalizer"
		logger.Error(err, msg)
		statusBase := authn.DeepCopy()
		r.recordError(authn, EventReasonFinalizerFailed, msg)
		if statusErr := r.updateStatus(ctx, statusBase, authn); statusErr != nil {
			logger.Error(statusErr, "Failed to update AIStoreAuth status")
		}
		return reconcile.Result{}, err
	}

	base := authn.DeepCopy()
	reconcileErr := r.reconcileResources(ctx, authn)
	if statusErr := r.updateStatus(ctx, base, authn); statusErr != nil {
		if reconcileErr == nil {
			return reconcile.Result{}, statusErr
		}
		// The reconcile error is returned instead, so this one is only visible if logged here.
		logger.Error(statusErr, "Failed to update AIStoreAuth status")
	}
	if reconcileErr != nil {
		return reconcile.Result{}, reconcileErr
	}

	logger.V(1).Info("Reconciled AIStoreAuth")
	return reconcile.Result{}, nil
}

// reconcileResources converges every operator-managed child object.
func (r *Reconciler) reconcileResources(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	logger := logf.FromContext(ctx)
	if err := r.reconcileConfigMap(ctx, authn); err != nil {
		msg := "Failed to reconcile ConfigMap"
		logger.Error(err, msg)
		r.recordError(authn, EventReasonConfigMapFailed, msg)
		return err
	}
	if err := r.reconcilePersistence(ctx, authn); err != nil {
		msg := "Failed to reconcile PersistentVolumeClaim"
		logger.Error(err, msg)
		r.recordError(authn, EventReasonPVCFailed, msg)
		return err
	}
	if err := r.reconcileServices(ctx, authn); err != nil {
		msg := "Failed to reconcile Services"
		logger.Error(err, msg)
		r.recordError(authn, EventReasonServicesFailed, msg)
		return err
	}
	if err := r.reconcileTLSCertificate(ctx, authn); err != nil {
		msg := "Failed to reconcile TLS Certificate"
		logger.Error(err, msg)
		r.recordError(authn, EventReasonCertificateFailed, msg)
		return err
	}
	if err := r.reconcileDeployment(ctx, authn); err != nil {
		msg := "Failed to reconcile Deployment"
		logger.Error(err, msg)
		r.recordError(authn, EventReasonDeploymentFailed, msg)
		return err
	}
	return nil
}

// recordError records a failed apply and sets the ready condition false.
func (r *Reconciler) recordError(authn *authv1alpha1.AIStoreAuth, eventReason, msg string) {
	r.recorder.Eventf(authn, nil, corev1.EventTypeWarning, eventReason, ActionReconcile, "%s", msg)
	setReadyCondition(authn, metav1.ConditionFalse, authv1alpha1.ReasonReconcileFailed, msg)
}

func (r *Reconciler) reconcileConfigMap(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	cm, err := authnres.NewConfigMap(authn)
	if err != nil {
		return err
	}
	if err := r.client.Apply(ctx, cm); err != nil {
		return err
	}
	logf.FromContext(ctx).V(1).Info("AuthN ConfigMap applied", "name", authnres.ConfigMapName(authn))
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
	logf.FromContext(ctx).V(1).Info("AuthN PVC applied", "name", authnres.PVCName(authn))
	return nil
}

// SetupWithManager registers the reconciler with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authv1alpha1.AIStoreAuth{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&certmanagerv1.Certificate{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("aistoreauth").
		Complete(r)
}
