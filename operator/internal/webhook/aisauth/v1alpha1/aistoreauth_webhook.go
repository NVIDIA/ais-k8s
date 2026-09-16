/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

// Package v1alpha1 contains admission webhooks for the auth.ais.nvidia.com/v1alpha1 API group.
package v1alpha1

import (
	"context"
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
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

// webhooklog is for logging in this package.
var webhooklog = logf.Log.WithName("aistoreauth-resource")

// +kubebuilder:object:generate=false

// AIStoreAuthCustomValidator validates AIStoreAuth resources on admission.
type AIStoreAuthCustomValidator struct {
	Client client.Client
}

// +kubebuilder:webhook:path=/validate-auth-ais-nvidia-com-v1alpha1-aistoreauth,mutating=false,failurePolicy=fail,sideEffects=None,groups=auth.ais.nvidia.com,resources=aistoreauths,verbs=create;update,versions=v1alpha1,name=vaistoreauth.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

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

	if secretRefName(authn.Spec.AdminSecret) == "" && !hasPodAnnotations(authn) {
		allErrs = append(allErrs, field.Required(specPath.Child("adminSecret"),
			"must set spec.adminSecret or spec.deployment.pod.annotations "+
				"(e.g. to inject admin credentials via an external mechanism)"))
	}

	hmacName := secretRefName(authn.Spec.HMACSecret)
	if rsaName := secretRefName(authn.Spec.RSAPassphraseSecret); rsaName != "" && hmacName != "" {
		allErrs = append(allErrs, field.Invalid(specPath.Child("rsaPassphraseSecret"), rsaName,
			"must not be set together with spec.hmacSecret"))
	}

	for _, ref := range userSecretRefs(authn) {
		fieldErr, err := v.reviewSecret(ctx, authn.Namespace, ref)
		if err != nil {
			return err
		}
		if fieldErr != nil {
			allErrs = append(allErrs, fieldErr)
		}
	}

	if err := authnres.ValidateConfig(authn); err != nil {
		allErrs = append(allErrs, field.Invalid(specPath.Child("config"), field.OmitValueType{}, err.Error()))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		authv1alpha1.GroupVersion.WithKind("AIStoreAuth").GroupKind(), authn.Name, allErrs)
}

// userSecretRef pairs a referenced Secret name with the spec field that names it.
type userSecretRef struct {
	path *field.Path
	name string
	// mustExist marks a Secret that AuthN cannot start without.
	mustExist bool
}

// userSecretRefs lists the user-provided Secrets that the spec references in the CR namespace.
// Each Secret appears once, under the first field that names it.
func userSecretRefs(authn *authv1alpha1.AIStoreAuth) []userSecretRef {
	specPath := field.NewPath("spec")
	var refs []userSecretRef
	// Reviewing a name once keeps a Secret that several fields share from costing an API call each.
	seen := make(map[string]int)
	add := func(path *field.Path, name string, mustExist bool) {
		if name == "" {
			return
		}
		if i, ok := seen[name]; ok {
			refs[i].mustExist = refs[i].mustExist || mustExist
			return
		}
		seen[name] = len(refs)
		refs = append(refs, userSecretRef{path: path, name: name, mustExist: mustExist})
	}

	add(specPath.Child("adminSecret"), secretRefName(authn.Spec.AdminSecret), true)
	add(specPath.Child("hmacSecret"), secretRefName(authn.Spec.HMACSecret), true)
	add(specPath.Child("rsaPassphraseSecret"), secretRefName(authn.Spec.RSAPassphraseSecret), true)
	if authn.UseTLSSecret() {
		add(specPath.Child("tls", "secretName"), *authn.Spec.TLS.SecretName, false)
	}
	if pod := authn.Spec.Deployment.Pod; pod != nil {
		pullPath := specPath.Child("deployment", "pod", "imagePullSecrets")
		for i := range pod.ImagePullSecrets {
			add(pullPath.Index(i).Child("name"), pod.ImagePullSecrets[i].Name, false)
		}
	}
	return refs
}

// reviewSecret checks that the submitting user may get the referenced Secret, and that a Secret
// the reference requires exists.
func (v *AIStoreAuthCustomValidator) reviewSecret(
	ctx context.Context,
	namespace string,
	ref userSecretRef,
) (*field.Error, error) {
	fieldErr, err := webhookcmn.AuthorizeGet(ctx, v.Client, ref.path, &authorizationv1.ResourceAttributes{
		Resource:  "secrets",
		Namespace: namespace,
		Name:      ref.name,
	})
	if err != nil || fieldErr != nil || !ref.mustExist {
		return fieldErr, err
	}

	secret := &corev1.Secret{}
	if err := v.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return field.Invalid(ref.path, ref.name,
				fmt.Sprintf("referenced Secret does not exist in namespace %q", namespace)), nil
		}
		return nil, apierrors.NewInternalError(
			fmt.Errorf("checking Secret %q in namespace %q: %w", ref.name, namespace, err))
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

// hasPodAnnotations reports whether the spec sets any AuthN pod template annotations.
func hasPodAnnotations(authn *authv1alpha1.AIStoreAuth) bool {
	pod := authn.Spec.Deployment.Pod
	return pod != nil && len(pod.Annotations) > 0
}

// SetupAIStoreAuthWebhookWithManager registers the AIStoreAuth validating webhook with the manager.
func SetupAIStoreAuthWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &authv1alpha1.AIStoreAuth{}).
		WithValidator(&AIStoreAuthCustomValidator{Client: mgr.GetClient()}).
		Complete()
}
