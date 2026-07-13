/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"context"
	"errors"
	"slices"
	"testing"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
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

// newValidator builds a validator whose cached client already holds the named Secrets.
func newValidator(existing ...string) *AIStoreAuthCustomValidator {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	b := fake.NewClientBuilder().WithScheme(scheme)
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
		existingSecrets  []string
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
			wantFields: []string{"spec.hmacSecret"},
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
			name: "neither signing secret is admitted (external or unprotected RSA key)",
		},
		{
			name:            "rsa mode without a passphrase reference is admitted",
			admin:           secretRef("admin"),
			existingSecrets: []string{"admin"},
		},
		{
			name:       "rsa mode with a missing passphrase secret is rejected",
			rsa:        secretRef("rsa-pass"),
			wantFields: []string{"spec.rsaPassphraseSecret"},
		},
		{
			name:            "missing admin secret is rejected",
			admin:           secretRef("admin"),
			hmac:            secretRef("hmac"),
			existingSecrets: []string{"hmac"},
			wantFields:      []string{"spec.adminSecret"},
		},
		{
			name:  "empty-name references are treated as unset and admitted",
			admin: secretRef(""),
			hmac:  secretRef(""),
			rsa:   secretRef(""),
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newValidator(tc.existingSecrets...).ValidateCreate(context.Background(), newAuthN(tc.admin, tc.hmac, tc.rsa))
			assertResult(t, err, tc.wantFields)
		})
	}
}

// TestValidateUpdateAndDelete locks in that update reuses create's validation and
// that delete is a no-op.
func TestValidateUpdateAndDelete(t *testing.T) {
	authn := newAuthN(nil, secretRef("hmac"), nil) // referenced Secret does not exist
	v := newValidator()
	_, err := v.ValidateUpdate(context.Background(), authn, authn)
	assertResult(t, err, []string{"spec.hmacSecret"})
	if _, err := v.ValidateDelete(context.Background(), authn); err != nil {
		t.Errorf("expected delete to be a no-op, got %v", err)
	}
}
