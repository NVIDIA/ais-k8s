// Package v1alpha1 contains admission webhooks for the auth.ais.nvidia.com/v1alpha1 API group.
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package v1alpha1

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
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

var _ admission.Validator[*authv1alpha1.AIStoreAuth] = &AIStoreAuthCustomValidator{}

// ValidateCreate implements admission.Validator.
func (*AIStoreAuthCustomValidator) ValidateCreate(_ context.Context, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate create")
	return nil, nil
}

// ValidateUpdate implements admission.Validator.
func (*AIStoreAuthCustomValidator) ValidateUpdate(_ context.Context, _, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate update")
	return nil, nil
}

// ValidateDelete implements admission.Validator.
func (*AIStoreAuthCustomValidator) ValidateDelete(_ context.Context, authn *authv1alpha1.AIStoreAuth) (admission.Warnings, error) {
	webhooklog.WithValues("name", authn.Name, "namespace", authn.Namespace).Info("Validate delete")
	return nil, nil
}

// SetupAIStoreAuthWebhookWithManager registers the AIStoreAuth validating webhook with the manager.
func SetupAIStoreAuthWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &authv1alpha1.AIStoreAuth{}).
		WithValidator(&AIStoreAuthCustomValidator{Client: mgr.GetClient()}).
		Complete()
}
