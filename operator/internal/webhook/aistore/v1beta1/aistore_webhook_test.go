/*
 * Copyright (c) 2025-2026, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"errors"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// runTolerationUpdateScenarios exercises add/remove/modify toleration paths for proxy or target updates.
func runTolerationUpdateScenarios(
	t *testing.T,
	component string,
	validate func(prev, ais *aisv1.AIStore) error,
	setTolerations func(a *aisv1.AIStore, tols []corev1.Toleration),
) {
	t.Helper()

	toleration := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	t.Run("adding toleration to "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &aisv1.AIStore{}
		ais := &aisv1.AIStore{}
		setTolerations(ais, []corev1.Toleration{toleration})
		g.Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("removing toleration from "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &aisv1.AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &aisv1.AIStore{}
		g.Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("modifying toleration in "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &aisv1.AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &aisv1.AIStore{}
		modified := toleration
		modified.Effect = corev1.TaintEffectNoExecute
		setTolerations(ais, []corev1.Toleration{modified})
		g.Expect(validate(prev, ais)).To(Succeed())
	})
}

func TestValidateProxyUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Proxy, validateProxyUpdate, func(a *aisv1.AIStore, tols []corev1.Toleration) {
		a.Spec.ProxySpec.Tolerations = tols
	})
}

func TestValidateTargetUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Target, validateTargetUpdate, func(a *aisv1.AIStore, tols []corev1.Toleration) {
		a.Spec.TargetSpec.Tolerations = tols
	})
}

func TestValidateTargetUpdateToScaleDownMode(t *testing.T) {
	g := NewWithT(t)
	prev := &aisv1.AIStore{}
	ais := &aisv1.AIStore{}
	ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeRetain
	g.Expect(validateTargetUpdate(prev, ais)).To(Succeed())
}

func sarInterceptor(allowed bool, reviews *[]*authorizationv1.SubjectAccessReview) interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			sar, ok := obj.(*authorizationv1.SubjectAccessReview)
			if !ok {
				return nil
			}
			*reviews = append(*reviews, sar.DeepCopy())
			sar.Status.Allowed = allowed
			sar.Status.Denied = !allowed
			return nil
		},
	}
}

// newSARWebhook returns a webhook whose SubjectAccessReviews all resolve to allowed, along with
// the reviews it submitted. Optional objs are seeded into the fake client (e.g. AIStoreAuthProfiles).
func newSARWebhook(t *testing.T, allowed bool, objs ...client.Object) (*AIStoreWebhook, *[]*authorizationv1.SubjectAccessReview) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := authorizationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add authorization scheme: %v", err)
	}
	if err := authv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add aisauth scheme: %v", err)
	}
	reviews := &[]*authorizationv1.SubjectAccessReview{}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithInterceptorFuncs(sarInterceptor(allowed, reviews)).
		Build()
	return &AIStoreWebhook{Client: c}, reviews
}

const tenantNS = "tenant"

func admissionCtx() context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{Username: "alice"},
		},
	})
}

func authProfile(name string) *authv1alpha1.AIStoreAuthProfile {
	return &authv1alpha1.AIStoreAuthProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func secretAIS(secretName string, secretNS *string) *aisv1.AIStore {
	ais := &aisv1.AIStore{}
	ais.Name = "cluster"
	ais.Namespace = tenantNS
	ais.Spec.Auth = &aisv1.AuthSpec{
		UsernamePassword: &aisv1.UsernamePasswordAuth{
			SecretName:      secretName,
			SecretNamespace: secretNS,
		},
	}
	return ais
}

func profileAIS(profileName string) *aisv1.AIStore {
	ais := &aisv1.AIStore{}
	ais.Name = "cluster"
	ais.Namespace = tenantNS
	ais.Spec.Auth = &aisv1.AuthSpec{
		ProfileRef: &aisv1.AuthProfileRef{Name: profileName},
	}
	return ais
}

// profileObjs returns seeded AIStoreAuthProfile objects for any profileRef on ais.
func profileObjs(ais *aisv1.AIStore) []client.Object {
	if ais == nil || ais.Spec.Auth == nil || ais.Spec.Auth.ProfileRef == nil {
		return nil
	}
	return []client.Object{authProfile(ais.Spec.Auth.ProfileRef.Name)}
}

func TestValidateAuthSecret(t *testing.T) {
	ctx := admissionCtx()

	for _, tt := range []struct {
		name       string
		prev       *aisv1.AIStore
		ais        *aisv1.AIStore
		wantReview *authorizationv1.ResourceAttributes
	}{
		{
			name: "no auth on create",
			ais:  &aisv1.AIStore{},
		},
		{
			name: "auth on create",
			ais:  secretAIS("creds", aisapc.Ptr(tenantNS)),
			wantReview: &authorizationv1.ResourceAttributes{
				Verb: "get", Resource: "secrets", Namespace: tenantNS, Name: "creds",
			},
		},
		{
			name: "secret namespace defaults to the cluster namespace",
			ais:  secretAIS("creds", nil),
			wantReview: &authorizationv1.ResourceAttributes{
				Verb: "get", Resource: "secrets", Namespace: tenantNS, Name: "creds",
			},
		},
		{
			name: "auth added on update",
			prev: &aisv1.AIStore{},
			ais:  secretAIS("creds", aisapc.Ptr(tenantNS)),
			wantReview: &authorizationv1.ResourceAttributes{
				Verb: "get", Resource: "secrets", Namespace: tenantNS, Name: "creds",
			},
		},
		{
			name: "auth removed on update",
			prev: secretAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  &aisv1.AIStore{},
		},
		{
			name: "unchanged secret ref on update",
			prev: secretAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  secretAIS("creds", aisapc.Ptr(tenantNS)),
		},
		{
			name: "changed secret name on update",
			prev: secretAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  secretAIS("other-creds", aisapc.Ptr(tenantNS)),
			wantReview: &authorizationv1.ResourceAttributes{
				Verb: "get", Resource: "secrets", Namespace: tenantNS, Name: "other-creds",
			},
		},
		{
			name: "changed secret namespace on update",
			prev: secretAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  secretAIS("creds", aisapc.Ptr("other")),
			wantReview: &authorizationv1.ResourceAttributes{
				Verb: "get", Resource: "secrets", Namespace: "other", Name: "creds",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook, reviews := newSARWebhook(t, true)
			g.Expect(webhook.validateAuthSecret(ctx, tt.prev, tt.ais)).To(Succeed())
			if tt.wantReview == nil {
				g.Expect(*reviews).To(BeEmpty())
				return
			}
			g.Expect(*reviews).To(HaveLen(1))
			g.Expect((*reviews)[0].Spec.ResourceAttributes).To(Equal(tt.wantReview))
		})
	}
}

func TestValidateAuthProfile(t *testing.T) {
	ctx := admissionCtx()

	profileAttrs := func(name string) *authorizationv1.ResourceAttributes {
		return &authorizationv1.ResourceAttributes{
			Verb:     "use",
			Group:    "auth.ais.nvidia.com",
			Version:  "v1alpha1",
			Resource: "aistoreauthprofiles",
			Name:     name,
		}
	}

	for _, tt := range []struct {
		name       string
		prev       *aisv1.AIStore
		ais        *aisv1.AIStore
		wantReview *authorizationv1.ResourceAttributes
	}{
		{
			name: "no auth on create",
			ais:  &aisv1.AIStore{},
		},
		{
			name:       "profile ref on create",
			ais:        profileAIS("prod-authn"),
			wantReview: profileAttrs("prod-authn"),
		},
		{
			name:       "profile ref added on update",
			prev:       &aisv1.AIStore{},
			ais:        profileAIS("prod-authn"),
			wantReview: profileAttrs("prod-authn"),
		},
		{
			name: "profile ref removed on update",
			prev: profileAIS("prod-authn"),
			ais:  &aisv1.AIStore{},
		},
		{
			name: "unchanged profile ref on update",
			prev: profileAIS("prod-authn"),
			ais:  profileAIS("prod-authn"),
		},
		{
			name:       "changed profile ref on update",
			prev:       profileAIS("prod-authn"),
			ais:        profileAIS("staging-authn"),
			wantReview: profileAttrs("staging-authn"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook, reviews := newSARWebhook(t, true, profileObjs(tt.ais)...)
			g.Expect(webhook.validateAuthProfile(ctx, tt.prev, tt.ais)).To(Succeed())
			if tt.wantReview == nil {
				g.Expect(*reviews).To(BeEmpty())
				return
			}
			g.Expect(*reviews).To(HaveLen(1))
			g.Expect((*reviews)[0].Spec.ResourceAttributes).To(Equal(tt.wantReview))
		})
	}
}

func TestValidateAuthProfileExistence(t *testing.T) {
	ctx := context.Background()
	path := field.NewPath("spec", "auth", "profileRef")

	t.Run("existing profile is admitted", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, true, authProfile("prod-authn"))
		g.Expect(webhook.validateAuthProfileExistence(ctx, path, "cluster", "prod-authn")).To(Succeed())
	})

	t.Run("missing profile is rejected with a field error", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, true)
		err := webhook.validateAuthProfileExistence(ctx, path, "cluster", "missing-authn")
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring("spec.auth.profileRef")))
		g.Expect(err).To(MatchError(ContainSubstring("referenced AIStoreAuthProfile does not exist")))
		g.Expect(err).To(MatchError(ContainSubstring("missing-authn")))
	})

	t.Run("get failure is an internal error", func(t *testing.T) {
		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(authv1alpha1.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("apiserver unavailable")
			},
		}).Build()
		webhook := &AIStoreWebhook{Client: c}
		err := webhook.validateAuthProfileExistence(ctx, path, "cluster", "prod-authn")
		g.Expect(apierrors.IsInternalError(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring(`checking AIStoreAuthProfile "prod-authn"`)))
	})
}

func TestValidateAuthAccess(t *testing.T) {
	ctx := admissionCtx()

	t.Run("no auth requires no review", func(t *testing.T) {
		g := NewWithT(t)
		webhook, reviews := newSARWebhook(t, false)
		g.Expect(webhook.validateAuthAccess(ctx, nil, &aisv1.AIStore{})).To(Succeed())
		g.Expect(*reviews).To(BeEmpty())
	})

	t.Run("secret is allowed when authorized", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, true)
		g.Expect(webhook.validateAuthAccess(ctx, nil, secretAIS("creds", aisapc.Ptr(tenantNS)))).To(Succeed())
	})

	t.Run("secret is rejected when unauthorized", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, false)
		err := webhook.validateAuthAccess(ctx, nil, secretAIS("creds", aisapc.Ptr(tenantNS)))
		g.Expect(err).To(MatchError(ContainSubstring(`is not authorized to get secrets resource "creds"`)))
	})

	t.Run("changed secret ref on update requires SAR", func(t *testing.T) {
		g := NewWithT(t)
		prev := secretAIS("creds", aisapc.Ptr(tenantNS))
		ais := secretAIS("other-creds", aisapc.Ptr(tenantNS))
		webhook, _ := newSARWebhook(t, false)
		g.Expect(webhook.validateAuthAccess(ctx, prev, ais)).To(HaveOccurred())
	})

	t.Run("profile ref is rejected when unauthorized", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, false)
		err := webhook.validateAuthAccess(ctx, nil, profileAIS("prod-authn"))
		g.Expect(err).To(MatchError(ContainSubstring(`is not authorized to use aistoreauthprofiles resource "prod-authn"`)))
	})

	t.Run("both refs are reviewed", func(t *testing.T) {
		g := NewWithT(t)
		ais := secretAIS("creds", aisapc.Ptr(tenantNS))
		ais.Spec.Auth.ProfileRef = &aisv1.AuthProfileRef{Name: "prod-authn"}
		webhook, reviews := newSARWebhook(t, true, authProfile("prod-authn"))
		g.Expect(webhook.validateAuthAccess(ctx, nil, ais)).To(Succeed())
		g.Expect(*reviews).To(HaveLen(2))
		g.Expect((*reviews)[0].Spec.ResourceAttributes.Resource).To(Equal("aistoreauthprofiles"))
		g.Expect((*reviews)[1].Spec.ResourceAttributes.Resource).To(Equal("secrets"))
	})

	t.Run("missing profile is rejected after authorization", func(t *testing.T) {
		g := NewWithT(t)
		webhook, reviews := newSARWebhook(t, true)
		err := webhook.validateAuthAccess(ctx, nil, profileAIS("missing-authn"))
		g.Expect(*reviews).To(HaveLen(1))
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring("spec.auth.profileRef")))
		g.Expect(err).To(MatchError(ContainSubstring("referenced AIStoreAuthProfile does not exist")))
	})

	t.Run("profile ref denial short-circuits the secret review", func(t *testing.T) {
		g := NewWithT(t)
		ais := secretAIS("creds", aisapc.Ptr(tenantNS))
		ais.Spec.Auth.ProfileRef = &aisv1.AuthProfileRef{Name: "prod-authn"}
		webhook, reviews := newSARWebhook(t, false)
		g.Expect(webhook.validateAuthAccess(ctx, nil, ais)).To(HaveOccurred())
		g.Expect(*reviews).To(HaveLen(1))
		g.Expect((*reviews)[0].Spec.ResourceAttributes.Resource).To(Equal("aistoreauthprofiles"))
	})
}
