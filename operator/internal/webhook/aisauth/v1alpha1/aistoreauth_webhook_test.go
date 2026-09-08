/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"errors"
	"slices"
	"testing"
	"time"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "ais"

func secretRef(name string) *corev1.LocalObjectReference {
	return &corev1.LocalObjectReference{Name: name}
}

// newValidator builds a validator whose cached client already holds the named Secrets and
// answers every SubjectAccessReview with the given decision.
func newValidator(allowed bool, existing ...string) *AIStoreAuthCustomValidator {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := authorizationv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	b := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(sarInterceptor(allowed))
	for _, n := range existing {
		b = b.WithObjects(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: n}})
	}
	return &AIStoreAuthCustomValidator{Client: b.Build()}
}

func newAuthN(admin, hmac, rsa *corev1.LocalObjectReference) *authv1alpha1.AIStoreAuth {
	return &authv1alpha1.AIStoreAuth{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "authn"},
		Spec:       authv1alpha1.AIStoreAuthSpec{AdminSecret: admin, HMACSecret: hmac, RSAPassphraseSecret: rsa},
	}
}

// invalidFields returns the spec field paths named by an Invalid admission error.
func invalidFields(err error) []string {
	statusErr := &apierrors.StatusError{}
	if !errors.As(err, &statusErr) || statusErr.ErrStatus.Details == nil {
		return nil
	}
	fields := make([]string, 0, len(statusErr.ErrStatus.Details.Causes))
	for _, c := range statusErr.ErrStatus.Details.Causes {
		fields = append(fields, c.Field)
	}
	return fields
}

// assertResult admits when wantFields is empty, otherwise requires an Invalid admission error.
func assertResult(t *testing.T, err error, wantFields []string) {
	t.Helper()
	if len(wantFields) == 0 {
		if err != nil {
			t.Fatalf("expected admission, got error: %v", err)
		}
		return
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected an Invalid error, got %v", err)
	}
	got := invalidFields(err)
	slices.Sort(got)
	want := slices.Clone(wantFields)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("fields: got %v want %v", got, want)
	}
}

func TestValidateSecretRefs(t *testing.T) {
	tests := []struct {
		name             string
		admin, hmac, rsa *corev1.LocalObjectReference
		podAnnotations   map[string]string
		existingSecrets  []string
		denyAccess       bool
		wantFields       []string // empty means the spec is admitted
	}{
		{
			name:            "rsa mode with existing admin and passphrase secrets is admitted",
			admin:           secretRef("admin"),
			rsa:             secretRef("rsa-pass"),
			existingSecrets: []string{"admin", "rsa-pass"},
		},
		{
			name:            "hmac mode with existing admin and signing secrets is admitted",
			admin:           secretRef("admin"),
			hmac:            secretRef("hmac"),
			existingSecrets: []string{"admin", "hmac"},
		},
		{
			name:       "hmac mode with a missing signing secret is rejected",
			hmac:       secretRef("hmac"),
			wantFields: []string{"spec.adminSecret", "spec.hmacSecret"},
		},
		{
			name:            "setting both hmac and rsa passphrase secrets is rejected",
			admin:           secretRef("admin"),
			hmac:            secretRef("hmac"),
			rsa:             secretRef("rsa-pass"),
			existingSecrets: []string{"admin", "hmac", "rsa-pass"},
			wantFields:      []string{"spec.rsaPassphraseSecret"},
		},
		{
			name:       "neither signing secret is rejected only for the missing admin secret (external or unprotected RSA key)",
			wantFields: []string{"spec.adminSecret"},
		},
		{
			name:            "rsa mode without a passphrase reference is admitted",
			admin:           secretRef("admin"),
			existingSecrets: []string{"admin"},
		},
		{
			name:       "rsa mode with a missing passphrase secret is rejected",
			rsa:        secretRef("rsa-pass"),
			wantFields: []string{"spec.adminSecret", "spec.rsaPassphraseSecret"},
		},
		{
			name:            "missing admin secret is rejected",
			admin:           secretRef("admin"),
			hmac:            secretRef("hmac"),
			existingSecrets: []string{"hmac"},
			wantFields:      []string{"spec.adminSecret"},
		},
		{
			name:           "empty-name references are treated as unset and admitted via pod annotations",
			admin:          secretRef(""),
			hmac:           secretRef(""),
			rsa:            secretRef(""),
			podAnnotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
		},
		{
			name:       "missing admin and passphrase secrets are reported together",
			admin:      secretRef("admin"),
			rsa:        secretRef("rsa-pass"),
			wantFields: []string{"spec.adminSecret", "spec.rsaPassphraseSecret"},
		},
		{
			name:       "missing admin and HMAC secrets are reported together",
			admin:      secretRef("admin"),
			hmac:       secretRef("hmac"),
			wantFields: []string{"spec.adminSecret", "spec.hmacSecret"},
		},
		{
			name:       "no admin secret and no pod annotations is rejected",
			wantFields: []string{"spec.adminSecret"},
		},
		{
			name:            "admin secret unset but pod annotations present is admitted",
			podAnnotations:  map[string]string{"vault.hashicorp.com/agent-inject": "true"},
			existingSecrets: nil,
		},
		{
			name:           "empty pod annotations map does not satisfy the admin credential requirement",
			podAnnotations: map[string]string{},
			wantFields:     []string{"spec.adminSecret"},
		},
		{
			name:            "a user without get access to the referenced Secrets is rejected",
			admin:           secretRef("admin"),
			hmac:            secretRef("hmac"),
			existingSecrets: []string{"admin", "hmac"},
			denyAccess:      true,
			wantFields:      []string{"spec.adminSecret", "spec.hmacSecret"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authn := newAuthN(tc.admin, tc.hmac, tc.rsa)
			if tc.podAnnotations != nil {
				authn.Spec.Deployment.Pod = &authv1alpha1.PodSpec{Annotations: tc.podAnnotations}
			}
			v := newValidator(!tc.denyAccess, tc.existingSecrets...)
			_, err := v.ValidateCreate(authorContext(), authn)
			assertResult(t, err, tc.wantFields)
		})
	}
}

// TestValidateConfig covers the AuthN config constraints the CRD schema cannot express.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *authv1alpha1.ConfigSpec
		wantFields []string // empty means the spec is admitted
	}{
		{
			name: "resource without config is admitted",
		},
		{
			name: "token lifetime within the maximum token age is admitted",
			config: &authv1alpha1.ConfigSpec{
				Auth: &authv1alpha1.ServerConfSpec{
					ExpirationTime: &metav1.Duration{Duration: 24 * time.Hour},
					MaxTokenAge:    &metav1.Duration{Duration: 72 * time.Hour},
				},
			},
		},
		{
			name: "token lifetime beyond the maximum token age is rejected",
			config: &authv1alpha1.ConfigSpec{
				Auth: &authv1alpha1.ServerConfSpec{
					ExpirationTime: &metav1.Duration{Duration: 48 * time.Hour},
					MaxTokenAge:    &metav1.Duration{Duration: 24 * time.Hour},
				},
			},
			wantFields: []string{"spec.config"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authn := newAuthN(secretRef("admin"), nil, nil)
			authn.Spec.Config = tc.config
			_, err := newValidator(true, "admin").ValidateCreate(authorContext(), authn)
			assertResult(t, err, tc.wantFields)
		})
	}
}

// TestValidateUpdateAndDelete locks in that update reuses create's validation and
// that delete is a no-op.
func TestValidateUpdateAndDelete(t *testing.T) {
	authn := newAuthN(nil, secretRef("hmac"), nil) // referenced Secret does not exist
	v := newValidator(true)
	ctx := authorContext()
	_, err := v.ValidateUpdate(ctx, authn, authn)
	assertResult(t, err, []string{"spec.adminSecret", "spec.hmacSecret"})
	if _, err := v.ValidateDelete(ctx, authn); err != nil {
		t.Errorf("expected delete to be a no-op, got %v", err)
	}
}
