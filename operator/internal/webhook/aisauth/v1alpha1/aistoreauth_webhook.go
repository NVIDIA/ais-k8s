/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package v1alpha1 contains admission webhooks for the auth.ais.nvidia.com/v1alpha1 API group.
package v1alpha1

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// webhooklog is for logging in this package.
var webhooklog = logf.Log.WithName("aistoreauth-resource")

// +kubebuilder:object:generate=false

// AIStoreAuthCustomValidator validates AIStoreAuth resources on admission.
type AIStoreAuthCustomValidator struct {
	Client client.Client
}

// +kubebuilder:webhook:path=/validate-auth-ais-nvidia-com-v1alpha1-aistoreauth,mutating=false,failurePolicy=fail,sideEffects=None,groups=auth.ais.nvidia.com,resources=aistoreauths,verbs=create;update,versions=v1alpha1,name=vaistoreauth.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

var _ admission.Validator[*authv1alpha1.AIStoreAuth] = &AIStoreAuthCustomValidator{}

// ValidateCreate implements admission.Validator.
func (v *AIStoreAuthCustomValidator) ValidateCreate(ctx context.Context, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate create")
	return nil, v.validate(ctx, authn)
}

// ValidateUpdate implements admission.Validator.
func (v *AIStoreAuthCustomValidator) ValidateUpdate(ctx context.Context, _, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate update")
	return nil, v.validate(ctx, authn)
}

// ValidateDelete implements admission.Validator.
func (*AIStoreAuthCustomValidator) ValidateDelete(_ context.Context, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate delete")
	return nil, nil
}

func (v *AIStoreAuthCustomValidator) validate(ctx context.Context, authn *authv1alpha1.AIStoreAuth) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if name := secretRefName(authn.Spec.AdminSecret); name != "" {
		fieldErr, err := v.requireSecret(ctx, authn.Namespace, name, specPath.Child("adminSecret"))
		if err != nil {
			return err
		}
		if fieldErr != nil {
			allErrs = append(allErrs, fieldErr)
		}
	}

	rsaPath := specPath.Child("rsaPassphraseSecret")
	if rsaName := secretRefName(authn.Spec.RSAPassphraseSecret); rsaName != "" {
		if secretRefName(authn.Spec.HMACSecret) != "" {
			allErrs = append(allErrs, field.Invalid(rsaPath, rsaName,
				"must not be set together with spec.hmacSecret"))
		} else {
			fieldErr, err := v.requireSecret(ctx, authn.Namespace, rsaName, rsaPath)
			if err != nil {
				return err
			}
			if fieldErr != nil {
				allErrs = append(allErrs, fieldErr)
			}
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		authv1alpha1.GroupVersion.WithKind("AIStoreAuth").GroupKind(), authn.Name, allErrs)
}

// requireSecret checks that the named Secret exists. A missing Secret yields a
// field error (the spec references something that isn't there). Any other lookup
// failure is returned as an internal error.
func (v *AIStoreAuthCustomValidator) requireSecret(ctx context.Context, namespace, name string, path *field.Path) (*field.Error, error) {
	secret := &corev1.Secret{}
	if err := v.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return field.Invalid(path, name,
				fmt.Sprintf("referenced Secret does not exist in namespace %q", namespace)), nil
		}
		return nil, apierrors.NewInternalError(
			fmt.Errorf("checking Secret %q in namespace %q: %w", name, namespace, err))
	}
	return nil, nil
}

// secretRefName returns the referenced Secret name, treating a nil reference or
// an empty name as "unset" by returning "".
func secretRefName(ref *corev1.LocalObjectReference) string {
	if ref == nil {
		return ""
	}
	return ref.Name
}

// SetupAIStoreAuthWebhookWithManager registers the AIStoreAuth validating webhook with the manager.
func SetupAIStoreAuthWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &authv1alpha1.AIStoreAuth{}).
		WithValidator(&AIStoreAuthCustomValidator{Client: mgr.GetClient()}).
		Complete()
}
