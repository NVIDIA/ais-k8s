/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"context"
	"strings"

	authv1 "github.com/ais-operator/api/aisauth/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:object:generate=false
type AIStoreAuthProfileWebhook struct{}

// +kubebuilder:webhook:path=/validate-auth-ais-nvidia-com-v1alpha1-aistoreauthprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=auth.ais.nvidia.com,resources=aistoreauthprofiles,verbs=create;update,versions=v1alpha1,name=vaistoreauthprofile.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Validator[*authv1.AIStoreAuthProfile] = &AIStoreAuthProfileWebhook{}

func (*AIStoreAuthProfileWebhook) ValidateCreate(_ context.Context, profile *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	if err := profile.ValidateSpec(); err != nil {
		return nil, err
	}
	return securityWarnings(profile), nil
}

func (*AIStoreAuthProfileWebhook) ValidateUpdate(_ context.Context, _, profile *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	return nil, profile.ValidateSpec()
}

func (*AIStoreAuthProfileWebhook) ValidateDelete(_ context.Context, _ *authv1.AIStoreAuthProfile) (admission.Warnings, error) {
	return nil, nil
}

// securityWarnings returns create-time admission warnings for credential-bearing
// configs that weaken TLS.
func securityWarnings(profile *authv1.AIStoreAuthProfile) admission.Warnings {
	var warnings admission.Warnings
	// Warn only -- may be intentional with TLS termination / service mesh.
	if strings.HasPrefix(profile.Spec.ServiceURL, "http://") {
		warnings = append(warnings, "spec.serviceURL should use https unless TLS is terminated elsewhere (e.g. service mesh)")
	}
	if profile.Spec.TLS != nil && profile.Spec.TLS.InsecureSkipVerify {
		warnings = append(warnings, "spec.tls.insecureSkipVerify is enabled; TLS certificate verification is disabled")
	}
	return warnings
}

// SetupAIStoreAuthProfileWebhookWithManager registers the AIStoreAuthProfile validating webhook with the manager.
func SetupAIStoreAuthProfileWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &authv1.AIStoreAuthProfile{}).
		WithValidator(&AIStoreAuthProfileWebhook{}).
		Complete()
}
