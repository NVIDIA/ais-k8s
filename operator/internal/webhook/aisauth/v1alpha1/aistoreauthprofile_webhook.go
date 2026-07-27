/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"context"
	"fmt"
	"net/url"

	authv1 "github.com/ais-operator/api/aisauth/v1alpha1"
	webhookcmn "github.com/ais-operator/internal/webhook"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AIStoreAuthProfileWebhook defines the validating webhook for AIStoreAuthProfile
// +kubebuilder:object:generate=false
type AIStoreAuthProfileWebhook struct {
	Client    client.Client
	APIReader client.Reader
}

// +kubebuilder:webhook:path=/validate-auth-ais-nvidia-com-v1alpha1-aistoreauthprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=auth.ais.nvidia.com,resources=aistoreauthprofiles,verbs=create;update,versions=v1alpha1,name=vaistoreauthprofile.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

var _ admission.Validator[*authv1.AIStoreAuthProfile] = &AIStoreAuthProfileWebhook{}

func (w *AIStoreAuthProfileWebhook) ValidateCreate(ctx context.Context, profile *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	warnings := securityWarnings(nil, profile)
	return warnings, w.validate(ctx, profile, &warnings)
}

func (w *AIStoreAuthProfileWebhook) ValidateUpdate(ctx context.Context, previous, profile *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	warnings := securityWarnings(previous, profile)
	return warnings, w.validate(ctx, profile, &warnings)
}

func (*AIStoreAuthProfileWebhook) ValidateDelete(_ context.Context, _ *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	return nil, nil
}

func (w *AIStoreAuthProfileWebhook) validate(
	ctx context.Context,
	profile *authv1.AIStoreAuthProfile,
	warnings *admission.Warnings,
) error {
	if err := profile.ValidateSpec(); err != nil {
		return err
	}
	// Skip reference checks while terminating so a stale reference cannot block finalizer removal
	if !profile.DeletionTimestamp.IsZero() {
		return nil
	}

	var allErrs field.ErrorList
	if profile.Spec.UsernamePassword != nil {
		fieldErrs, err := w.validateSecretRef(ctx, &profile.Spec.UsernamePassword.Secret, warnings)
		if err != nil {
			return err
		}
		allErrs = append(allErrs, fieldErrs...)
	}
	if profile.Spec.TLS != nil && profile.Spec.TLS.CAConfigMapRef != nil {
		fieldErrs, err := w.validateCAConfigMap(ctx, profile.Spec.TLS.CAConfigMapRef, warnings)
		if err != nil {
			return err
		}
		allErrs = append(allErrs, fieldErrs...)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		authv1.GroupVersion.WithKind("AIStoreAuthProfile").GroupKind(),
		profile.Name,
		allErrs,
	)
}

// validateSecretRef validates access to the credentials secret, then checks existence and internal fields
func (w *AIStoreAuthProfileWebhook) validateSecretRef(
	ctx context.Context,
	secretSpec *authv1.AuthProfileSecret,
	warnings *admission.Warnings,
) (field.ErrorList, error) {
	path := field.NewPath("spec", "usernamePassword", "secret")
	secretRef := client.ObjectKey{Namespace: secretSpec.Namespace, Name: secretSpec.Name}
	// Validate access first to avoid exposing secret existence to a user without "get" access
	fieldErr, err := webhookcmn.AuthorizeGet(ctx, w.Client, path, &authorizationv1.ResourceAttributes{
		Resource:  "secrets",
		Namespace: secretRef.Namespace,
		Name:      secretRef.Name,
	})
	if err != nil {
		return nil, err
	}
	if fieldErr != nil {
		return field.ErrorList{fieldErr}, nil
	}

	secret := &corev1.Secret{}
	if getErr := w.APIReader.Get(ctx, secretRef, secret); getErr != nil {
		return handleResourceGetError(ctx, getErr, path, "Secret", secretRef, warnings)
	}
	return validateSecretFields(secretSpec, secret), nil
}

func handleResourceGetError(
	ctx context.Context,
	err error,
	path *field.Path,
	kind string,
	ref client.ObjectKey,
	warnings *admission.Warnings,
) (field.ErrorList, error) {
	switch {
	case apierrors.IsNotFound(err):
		return field.ErrorList{field.Invalid(
			path,
			ref.Name,
			fmt.Sprintf("referenced %s does not exist in namespace %q", kind, ref.Namespace),
		)}, nil
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		logf.FromContext(ctx).Info("Skipping content validation of referenced object",
			"kind", kind, "name", ref.Name, "namespace", ref.Namespace, "reason", err)
		*warnings = append(*warnings, fmt.Sprintf(
			"the operator is not authorized to get %s %q in namespace %q; its contents were not validated",
			kind, ref.Name, ref.Namespace,
		))
		return nil, nil
	default:
		return nil, apierrors.NewInternalError(
			fmt.Errorf("checking %s %q in namespace %q: %w", kind, ref.Name, ref.Namespace, err),
		)
	}
}

func validateSecretFields(secretSpec *authv1.AuthProfileSecret, secret *corev1.Secret) field.ErrorList {
	path := field.NewPath("spec", "usernamePassword", "secret")
	var allErrs field.ErrorList
	keys := []struct {
		path *field.Path
		key  string
	}{
		{path: path.Child("userKey"), key: secretSpec.UserKeyOrDefault()},
		{path: path.Child("passKey"), key: secretSpec.PassKeyOrDefault()},
	}
	for _, required := range keys {
		if _, ok := secret.Data[required.key]; !ok {
			allErrs = append(allErrs, field.Invalid(required.path, required.key, "key does not exist in referenced Secret"))
		}
	}
	return allErrs
}

func (w *AIStoreAuthProfileWebhook) validateCAConfigMap(
	ctx context.Context,
	cmRef *authv1.AuthProfileCAConfigMapRef,
	warnings *admission.Warnings,
) (field.ErrorList, error) {
	path := field.NewPath("spec", "tls", "caConfigMapRef")
	configMapRef := client.ObjectKey{Namespace: cmRef.Namespace, Name: cmRef.Name}
	// Validate access first so admission cannot be used to probe which ConfigMaps and keys exist
	fieldErr, err := webhookcmn.AuthorizeGet(ctx, w.Client, path, &authorizationv1.ResourceAttributes{
		Resource:  "configmaps",
		Namespace: configMapRef.Namespace,
		Name:      configMapRef.Name,
	})
	if err != nil {
		return nil, err
	}
	if fieldErr != nil {
		return field.ErrorList{fieldErr}, nil
	}

	configMap := &corev1.ConfigMap{}
	if getErr := w.APIReader.Get(ctx, configMapRef, configMap); getErr != nil {
		return handleResourceGetError(ctx, getErr, path, "ConfigMap", configMapRef, warnings)
	}
	if _, ok := configMap.Data[cmRef.Key]; !ok {
		return field.ErrorList{
			field.Invalid(path.Child("key"), cmRef.Key, "key does not exist in referenced ConfigMap"),
		}, nil
	}
	return nil, nil
}

// securityWarnings returns admission warnings for changed settings that weaken TLS.
// Changes are only warnings since they may be intentional with TLS termination / service mesh
func securityWarnings(previous, profile *authv1.AIStoreAuthProfile) admission.Warnings {
	var warnings admission.Warnings
	if (previous == nil || previous.Spec.ServiceURL != profile.Spec.ServiceURL) &&
		isPlainHTTP(profile.Spec.ServiceURL) {
		warnings = append(warnings, "spec.serviceURL should use https unless TLS is terminated elsewhere (e.g. service mesh)")
	}
	if profile.Spec.TLS == nil || !profile.Spec.TLS.InsecureSkipVerify {
		return warnings
	}
	previousInsecureSkipVerify := previous != nil &&
		previous.Spec.TLS != nil &&
		previous.Spec.TLS.InsecureSkipVerify
	if !previousInsecureSkipVerify {
		warnings = append(warnings, "spec.tls.insecureSkipVerify is enabled; TLS certificate verification is disabled")
	}
	return warnings
}

// isPlainHTTP reports whether the URL uses the http scheme, which url.Parse normalizes to lowercase.
func isPlainHTTP(serviceURL string) bool {
	u, err := url.Parse(serviceURL)
	return err == nil && u.Scheme == "http"
}

// SetupAIStoreAuthProfileWebhookWithManager registers the AIStoreAuthProfile validating webhook with the manager.
func SetupAIStoreAuthProfileWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &authv1.AIStoreAuthProfile{}).
		WithValidator(&AIStoreAuthProfileWebhook{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
		}).
		Complete()
}
