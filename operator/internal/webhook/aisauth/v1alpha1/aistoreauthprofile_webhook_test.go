/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1alpha1

import (
	"context"
	"testing"
	"time"

	authv1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// sarInterceptor answers every SubjectAccessReview with the given decision.
func sarInterceptor(allowed bool) interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			sar, ok := obj.(*authorizationv1.SubjectAccessReview)
			if !ok {
				return nil
			}
			sar.Status.Allowed = allowed
			sar.Status.Denied = !allowed
			return nil
		},
	}
}

// authorContext carries the identity of the profile author for SubjectAccessReview.
func authorContext() context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{Username: "alice"},
		},
	})
}

func TestAIStoreAuthProfileWebhook(t *testing.T) {
	webhook := newFakeWebhook(t, true, nil)
	ctx := authorContext()

	t.Run("accepts https without warnings", func(t *testing.T) {
		g := NewWithT(t)
		warnings, err := webhook.ValidateCreate(ctx, &authv1.AIStoreAuthProfile{
			Spec: authv1.AIStoreAuthProfileSpec{
				ServiceURL:    "https://auth.example.com",
				TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
			},
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})

	t.Run("rejects invalid spec", func(t *testing.T) {
		g := NewWithT(t)
		_, err := webhook.ValidateCreate(ctx, &authv1.AIStoreAuthProfile{})
		g.Expect(err).To(HaveOccurred())
	})

	for _, tt := range []struct {
		name        string
		previous    *authv1.AIStoreAuthProfile
		profile     *authv1.AIStoreAuthProfile
		wantWarning string
	}{
		{
			name: "http serviceURL",
			previous: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "https://auth.example.com",
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			profile: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "http://auth.example.com",
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			wantWarning: "spec.serviceURL should use https",
		},
		{
			name: "uppercase http serviceURL",
			previous: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "https://auth.example.com",
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			profile: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "HTTP://auth.example.com",
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			wantWarning: "spec.serviceURL should use https",
		},
		{
			name: "insecureSkipVerify",
			previous: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "https://auth.example.com",
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			profile: &authv1.AIStoreAuthProfile{
				Spec: authv1.AIStoreAuthProfileSpec{
					ServiceURL:    "https://auth.example.com",
					TLS:           &authv1.AuthProfileTLSConfig{InsecureSkipVerify: true},
					TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
				},
			},
			wantWarning: "spec.tls.insecureSkipVerify is enabled",
		},
	} {
		t.Run("warns on changed "+tt.name, func(t *testing.T) {
			g := NewWithT(t)
			warnings, err := webhook.ValidateCreate(ctx, tt.profile)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(warnings).To(ContainElement(ContainSubstring(tt.wantWarning)))

			warnings, err = webhook.ValidateUpdate(ctx, tt.profile, tt.profile)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())

			warnings, err = webhook.ValidateUpdate(ctx, tt.previous, tt.profile)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(warnings).To(ContainElement(ContainSubstring(tt.wantWarning)))
		})
	}
}

const (
	profileNamespace = "ais-authn"
	// profileName is the name of the profile every test helper builds.
	profileName = "profile"
)

// newFakeWebhook builds a webhook over a fake client seeded with objects, where every
// SubjectAccessReview is answered with the given decision. When reader is nil, the fake
// client is also used for content GETs.
func newFakeWebhook(t *testing.T, allowed bool, reader client.Reader, objects ...client.Object) *AIStoreAuthProfileWebhook {
	t.Helper()
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(authorizationv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(aisv1.AddToScheme(scheme)).To(Succeed())
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithInterceptorFuncs(sarInterceptor(allowed)).
		Build()
	if reader == nil {
		reader = fakeClient
	}
	return &AIStoreAuthProfileWebhook{
		Client:    fakeClient,
		APIReader: reader,
	}
}

// forbiddenReader returns Forbidden for every Get, simulating an operator without access.
type forbiddenReader struct{}

func (forbiddenReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	resource := "secrets"
	if _, ok := obj.(*corev1.ConfigMap); ok {
		resource = "configmaps"
	}
	return apierrors.NewForbidden(schema.GroupResource{Resource: resource}, key.Name, nil)
}

func (forbiddenReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	panic("unexpected List")
}

// unavailableReader fails every List, simulating an unreachable API server.
type unavailableReader struct{}

func (unavailableReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	panic("unexpected Get")
}

func (unavailableReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return apierrors.NewServiceUnavailable("API server is unreachable")
}

// validationCase validates profile on create, or on update against previous when it is set.
type validationCase struct {
	name string
	// objects seed the fake client the webhook reads references from.
	objects []client.Object
	// unauthorized denies the author "get" on the referenced Secret and ConfigMap.
	unauthorized bool
	// operatorUnauthorized makes content GETs return Forbidden so contents are skipped with a warning.
	operatorUnauthorized bool
	// ctx overrides the default context carrying the admission request
	ctx      context.Context
	previous *authv1.AIStoreAuthProfile
	profile  *authv1.AIStoreAuthProfile
	// wantErr matches the returned error, or expects success when nil.
	wantErr types.GomegaMatcher
	// wantWarning matches a returned warning substring when set.
	wantWarning types.GomegaMatcher
}

func runValidationCases(t *testing.T, cases []validationCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := tc.ctx
			if ctx == nil {
				ctx = authorContext()
			}
			var reader client.Reader
			if tc.operatorUnauthorized {
				reader = forbiddenReader{}
			}
			webhook := newFakeWebhook(t, !tc.unauthorized, reader, tc.objects...)

			var (
				warnings admission.Warnings
				err      error
			)
			if tc.previous != nil {
				warnings, err = webhook.ValidateUpdate(ctx, tc.previous, tc.profile)
			} else {
				warnings, err = webhook.ValidateCreate(ctx, tc.profile)
			}

			if tc.wantErr == nil {
				g.Expect(err).NotTo(HaveOccurred())
			} else {
				g.Expect(err).To(MatchError(tc.wantErr))
			}
			if tc.wantWarning == nil {
				g.Expect(warnings).To(BeEmpty())
				return
			}
			g.Expect(warnings).To(ContainElement(tc.wantWarning))
		})
	}
}

func usernamePasswordProfile(secretName string) *authv1.AIStoreAuthProfile {
	return &authv1.AIStoreAuthProfile{
		ObjectMeta: metav1.ObjectMeta{Name: profileName},
		Spec: authv1.AIStoreAuthProfileSpec{
			ServiceURL: "https://auth.example.com",
			UsernamePassword: &authv1.AuthProfileUsernamePassword{
				Secret: authv1.AuthProfileSecret{Name: secretName, Namespace: profileNamespace},
			},
		},
	}
}

func terminatingProfile(secretName string) *authv1.AIStoreAuthProfile {
	profile := usernamePasswordProfile(secretName)
	profile.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	profile.Finalizers = []string{"example.com/hold"}
	return profile
}

func caConfigMapProfile(configMapName, key string) *authv1.AIStoreAuthProfile {
	return &authv1.AIStoreAuthProfile{
		ObjectMeta: metav1.ObjectMeta{Name: profileName},
		Spec: authv1.AIStoreAuthProfileSpec{
			ServiceURL:    "https://auth.example.com",
			TokenExchange: &authv1.AuthProfileTokenExchange{Endpoint: "/token"},
			TLS: &authv1.AuthProfileTLSConfig{
				CAConfigMapRef: &authv1.AuthProfileCAConfigMapRef{
					Name: configMapName, Namespace: profileNamespace, Key: key,
				},
			},
		},
	}
}

func credentialsSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: profileNamespace},
		Data:       data,
	}
}

func caConfigMap(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "ca", Namespace: profileNamespace},
		Data:       data,
	}
}

func TestAIStoreAuthProfileWebhookSecretReferences(t *testing.T) {
	populatedSecret := credentialsSecret(map[string][]byte{
		authv1.DefaultAuthProfileUserKey: []byte("user"),
		authv1.DefaultAuthProfilePassKey: []byte("pass"),
	})

	customKeysProfile := usernamePasswordProfile("credentials")
	customKeysProfile.Spec.UsernamePassword.Secret.UserKey = "username"
	customKeysProfile.Spec.UsernamePassword.Secret.PassKey = "password"

	insecureProfile := usernamePasswordProfile("missing")
	insecureProfile.Spec.TLS = &authv1.AuthProfileTLSConfig{InsecureSkipVerify: true}

	finalizerRemoved := terminatingProfile("missing")
	finalizerRemoved.Finalizers = nil

	rerouted := terminatingProfile("credentials")
	rerouted.Spec.ServiceURL = "https://other.example.com"

	// staleSpec references a missing Secret and sets two auth methods, so it fails every check.
	staleSpec := func() *authv1.AIStoreAuthProfile {
		profile := usernamePasswordProfile("credentials")
		profile.Spec.TokenExchange = &authv1.AuthProfileTokenExchange{Endpoint: "/token"}
		return profile
	}

	runValidationCases(t, []validationCase{
		{
			name:    "accepts existing Secret with required keys",
			objects: []client.Object{populatedSecret},
			profile: usernamePasswordProfile("credentials"),
		},
		{
			name:    "rejects missing Secret",
			profile: usernamePasswordProfile("missing"),
			wantErr: ContainSubstring("referenced Secret does not exist"),
		},
		{
			name:    "rejects missing Secret keys",
			objects: []client.Object{credentialsSecret(nil)},
			profile: usernamePasswordProfile("credentials"),
			wantErr: And(
				ContainSubstring("spec.usernamePassword.secret.userKey"),
				ContainSubstring("spec.usernamePassword.secret.passKey"),
			),
		},
		{
			name: "accepts configured Secret keys in place of the defaults",
			objects: []client.Object{credentialsSecret(map[string][]byte{
				"username": []byte("user"),
				"password": []byte("pass"),
			})},
			profile: customKeysProfile,
		},
		{
			name:         "rejects Secret the author cannot read",
			objects:      []client.Object{populatedSecret},
			unauthorized: true,
			profile:      usernamePasswordProfile("credentials"),
			wantErr:      ContainSubstring(`user "alice" is not authorized to get secrets resource "credentials"`),
		},
		{
			name:         "reports denial without disclosing whether the Secret exists",
			unauthorized: true,
			profile:      usernamePasswordProfile("missing"),
			wantErr: And(
				ContainSubstring("is not authorized to get secrets"),
				Not(ContainSubstring("does not exist")),
			),
		},
		{
			name:    "requires an admission request to authorize",
			ctx:     context.Background(),
			profile: usernamePasswordProfile("credentials"),
			wantErr: ContainSubstring(`cannot authorize secrets resource "credentials" in namespace "ais-authn"`),
		},
		{
			name:     "accepts an unchanged spec that no longer passes validation",
			previous: staleSpec(),
			profile:  staleSpec(),
		},
		{
			name:     "validates a changed Secret reference on update",
			previous: usernamePasswordProfile("credentials"),
			profile:  usernamePasswordProfile("missing"),
			wantErr:  ContainSubstring("referenced Secret does not exist"),
		},
		{
			name:                 "warns when the operator cannot get the Secret",
			operatorUnauthorized: true,
			profile:              usernamePasswordProfile("credentials"),
			wantWarning:          ContainSubstring("the operator is not authorized to get Secret"),
		},
		{
			name:        "returns warnings alongside a rejection",
			profile:     insecureProfile,
			wantErr:     ContainSubstring("referenced Secret does not exist"),
			wantWarning: ContainSubstring("spec.tls.insecureSkipVerify is enabled"),
		},
		{
			name:         "accepts finalizer removal from a terminating profile with an unreadable Secret",
			unauthorized: true,
			previous:     terminatingProfile("missing"),
			profile:      finalizerRemoved,
		},
		{
			name:         "rejects an unauthorized serviceURL change on a terminating profile",
			unauthorized: true,
			previous:     terminatingProfile("credentials"),
			profile:      rerouted,
			wantErr:      ContainSubstring(`user "alice" is not authorized to get secrets resource "credentials"`),
		},
	})
}

func referencingAIStore(namespace, name, ref string) *aisv1.AIStore {
	ais := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if ref != "" {
		ais.Spec.Auth = &aisv1.AuthSpec{ProfileRef: &aisv1.AuthProfileRef{Name: ref}}
	}
	return ais
}

func TestAIStoreAuthProfileWebhookDelete(t *testing.T) {
	for _, tc := range []struct {
		name string
		// objects seed the AIStore resources the webhook reads references from.
		objects []client.Object
		// unavailable makes the reference lookup fail.
		unavailable bool
		wantErr     types.GomegaMatcher
	}{
		{
			name: "accepts a profile no AIStore references",
			objects: []client.Object{
				referencingAIStore("ais", "no-auth", ""),
				referencingAIStore("ais", "other-profile", "staging-authn"),
			},
		},
		{
			name:    "rejects a referenced profile",
			objects: []client.Object{referencingAIStore("ais", "prod", profileName)},
			wantErr: ContainSubstring(`still referenced by AIStore ais/prod`),
		},
		{
			name: "rejects a profile referenced from any namespace",
			objects: []client.Object{
				referencingAIStore("ais", "no-auth", ""),
				referencingAIStore("tenant", "prod", profileName),
			},
			wantErr: ContainSubstring(`still referenced by AIStore tenant/prod`),
		},
		{
			name:        "rejects deletion when references cannot be read",
			unavailable: true,
			wantErr:     ContainSubstring(`checking AIStore references to "profile"`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var reader client.Reader
			if tc.unavailable {
				reader = unavailableReader{}
			}
			webhook := newFakeWebhook(t, true, reader, tc.objects...)

			warnings, err := webhook.ValidateDelete(authorContext(), usernamePasswordProfile("credentials"))
			g.Expect(warnings).To(BeEmpty())
			if tc.wantErr == nil {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			g.Expect(err).To(MatchError(tc.wantErr))
		})
	}
}

func TestAIStoreAuthProfileWebhookCAConfigMapReferences(t *testing.T) {
	runValidationCases(t, []validationCase{
		{
			name:    "accepts existing ConfigMap key",
			objects: []client.Object{caConfigMap(map[string]string{"ca.crt": "certificate"})},
			profile: caConfigMapProfile("ca", "ca.crt"),
		},
		{
			name:    "rejects missing ConfigMap",
			profile: caConfigMapProfile("missing-ca", "ca.crt"),
			wantErr: ContainSubstring("referenced ConfigMap does not exist"),
		},
		{
			name:    "rejects missing ConfigMap key",
			objects: []client.Object{caConfigMap(map[string]string{"ca.crt": "certificate"})},
			profile: caConfigMapProfile("ca", "missing-ca.crt"),
			wantErr: ContainSubstring("key does not exist in referenced ConfigMap"),
		},
		{
			name:         "rejects ConfigMap the author cannot read",
			objects:      []client.Object{caConfigMap(map[string]string{"ca.crt": "certificate"})},
			unauthorized: true,
			profile:      caConfigMapProfile("ca", "ca.crt"),
			wantErr:      ContainSubstring(`user "alice" is not authorized to get configmaps resource "ca"`),
		},
		{
			name:         "reports denial without disclosing whether the ConfigMap exists",
			unauthorized: true,
			profile:      caConfigMapProfile("missing-ca", "ca.crt"),
			wantErr: And(
				ContainSubstring("is not authorized to get configmaps"),
				Not(ContainSubstring("does not exist")),
			),
		},
		{
			name:     "validates a changed ConfigMap key on update",
			objects:  []client.Object{caConfigMap(map[string]string{"ca.crt": "certificate"})},
			previous: caConfigMapProfile("ca", "ca.crt"),
			profile:  caConfigMapProfile("ca", "missing-ca.crt"),
			wantErr:  ContainSubstring("key does not exist in referenced ConfigMap"),
		},
		{
			name:                 "warns when the operator cannot get the ConfigMap",
			operatorUnauthorized: true,
			profile:              caConfigMapProfile("ca", "ca.crt"),
			wantWarning:          ContainSubstring("the operator is not authorized to get ConfigMap"),
		},
	})
}
