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
	"k8s.io/apimachinery/pkg/util/intstr"
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

func TestValidateDeprecatedStateStorage(t *testing.T) {
	tests := []struct {
		name    string
		ais     *aisv1.AIStore
		wantErr string
	}{
		{
			name: "no deprecated options",
			ais:  &aisv1.AIStore{},
		},
		{
			name: "migrated to stateStorage",
			ais: &aisv1.AIStore{Spec: aisv1.AIStoreSpec{
				StateStorage: &aisv1.StateStorage{HostPath: &aisv1.StateHostPathConfig{Prefix: "/mnt"}},
			}},
		},
		{
			name:    "hostpathPrefix",
			ais:     &aisv1.AIStore{Spec: aisv1.AIStoreSpec{HostpathPrefix: aisapc.Ptr("/mnt")}},
			wantErr: "spec.hostpathPrefix is no longer accepted, use spec.stateStorage.hostPath.prefix",
		},
		{
			name:    "stateStorageClass",
			ais:     &aisv1.AIStore{Spec: aisv1.AIStoreSpec{StateStorageClass: aisapc.Ptr("my-sc")}},
			wantErr: "spec.stateStorageClass is no longer accepted, use spec.stateStorage.pvc.storageClass",
		},
		{
			name: "both",
			ais: &aisv1.AIStore{Spec: aisv1.AIStoreSpec{
				HostpathPrefix:    aisapc.Ptr("/mnt"),
				StateStorageClass: aisapc.Ptr("my-sc"),
			}},
			wantErr: "spec.hostpathPrefix is no longer accepted, use spec.stateStorage.hostPath.prefix; " +
				"spec.stateStorageClass is no longer accepted, use spec.stateStorage.pvc.storageClass",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			err := validateDeprecatedStateStorage(tt.ais)
			if tt.wantErr == "" {
				g.Expect(err).ToNot(HaveOccurred())
				return
			}
			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
		})
	}
}

func TestValidateSpecRejectsDeprecatedStateStorage(t *testing.T) {
	tests := []struct {
		name    string
		spec    aisv1.AIStoreSpec
		wantErr string
	}{
		{
			name:    "hostpathPrefix",
			spec:    aisv1.AIStoreSpec{HostpathPrefix: aisapc.Ptr("/mnt")},
			wantErr: "spec.hostpathPrefix is no longer accepted",
		},
		{
			name:    "stateStorageClass",
			spec:    aisv1.AIStoreSpec{StateStorageClass: aisapc.Ptr("my-sc")},
			wantErr: "spec.stateStorageClass is no longer accepted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			aisw := &AIStoreWebhook{}
			prev := &aisv1.AIStore{Spec: tt.spec}
			ais := prev.DeepCopy()
			ais.Spec.Size = aisapc.Ptr[int32](3)

			_, err := aisw.validateSpec(context.Background(), prev, ais)
			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
		})
	}
}

// An unchanged spec must be admitted even when it would no longer pass validation, so that the
// operator can still patch metadata on a cluster deployed under an older release.
func TestValidateSpecUnchangedSpec(t *testing.T) {
	// missing size fails AIStore.ValidateSpec
	legacy := &aisv1.AIStore{Spec: aisv1.AIStoreSpec{HostpathPrefix: aisapc.Ptr("/mnt")}}

	t.Run("metadata-only update is admitted with a warning", func(subT *testing.T) {
		g := NewWithT(subT)
		aisw := &AIStoreWebhook{}
		ais := legacy.DeepCopy()
		ais.Finalizers = []string{"finalize.ais"}

		warnings, err := aisw.validateSpec(context.Background(), legacy, ais)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warnings).To(HaveLen(1))
	})

	t.Run("changed spec is still validated", func(subT *testing.T) {
		g := NewWithT(subT)
		aisw := &AIStoreWebhook{}

		_, err := aisw.validateSpec(context.Background(), legacy, &aisv1.AIStore{})
		g.Expect(err).To(MatchError(ContainSubstring("cluster size is not specified")))
	})
}

func TestValidateUpdatePorts(t *testing.T) {
	proxyAIS := func(spec aisv1.ServiceSpec) *aisv1.AIStore {
		return &aisv1.AIStore{Spec: aisv1.AIStoreSpec{ProxySpec: aisv1.DaemonSpec{ServiceSpec: spec}}}
	}
	targetAIS := func(spec aisv1.ServiceSpec) *aisv1.AIStore {
		return &aisv1.AIStore{Spec: aisv1.AIStoreSpec{
			TargetSpec: aisv1.TargetSpec{DaemonSpec: aisv1.DaemonSpec{ServiceSpec: spec}},
		}}
	}

	daemons := []struct {
		name string
		// otherPublic is the peer daemon's public default, which must not satisfy this daemon's
		otherPublic int32
		public      int32
		newAIS      func(aisv1.ServiceSpec) *aisv1.AIStore
		validate    func(prev, ais *aisv1.AIStore) error
	}{
		{"proxy", 51081, 51080, proxyAIS, validateProxyUpdate},
		{"target", 51080, 51081, targetAIS, validateTargetUpdate},
	}

	for _, d := range daemons {
		explicitDefaults := aisv1.ServiceSpec{
			PublicPort:       aisapc.Ptr(intstr.FromInt32(d.public)),
			IntraControlPort: aisapc.Ptr(intstr.FromInt32(51082)),
			IntraDataPort:    aisapc.Ptr(intstr.FromInt32(51083)),
		}
		legacy := explicitDefaults
		legacy.ServicePort = aisapc.Ptr(intstr.FromInt32(d.public)) //nolint:staticcheck // exercising the migration off the deprecated option
		tests := []struct {
			name    string
			prev    *aisv1.AIStore
			ais     *aisv1.AIStore
			wantErr bool
		}{
			{
				name: "dropping ports that match the defaults is allowed",
				prev: d.newAIS(explicitDefaults),
				ais:  d.newAIS(aisv1.ServiceSpec{}),
			},
			{
				name:    "changing the deprecated servicePort is rejected",
				prev:    d.newAIS(legacy),
				ais:     d.newAIS(aisv1.ServiceSpec{ServicePort: aisapc.Ptr(intstr.FromInt(51090))}),
				wantErr: true,
			},
			{
				name: "dropping the deprecated servicePort is allowed",
				prev: d.newAIS(legacy),
				ais:  d.newAIS(aisv1.ServiceSpec{}),
			},
			{
				name:    "changing the public port is rejected",
				prev:    d.newAIS(explicitDefaults),
				ais:     d.newAIS(aisv1.ServiceSpec{PublicPort: aisapc.Ptr(intstr.FromInt32(51099))}),
				wantErr: true,
			},
			{
				name:    "dropping a public port matching the peer daemon default is rejected",
				prev:    d.newAIS(aisv1.ServiceSpec{PublicPort: aisapc.Ptr(intstr.FromInt32(d.otherPublic))}),
				ais:     d.newAIS(aisv1.ServiceSpec{}),
				wantErr: true,
			},
			{
				name:    "changing the intra-control port is rejected",
				prev:    d.newAIS(explicitDefaults),
				ais:     d.newAIS(aisv1.ServiceSpec{IntraControlPort: aisapc.Ptr(intstr.FromInt32(51099))}),
				wantErr: true,
			},
			{
				name:    "dropping an intra-data port that does not match the default is rejected",
				prev:    d.newAIS(aisv1.ServiceSpec{IntraDataPort: aisapc.Ptr(intstr.FromInt32(51099))}),
				ais:     d.newAIS(aisv1.ServiceSpec{}),
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(d.name+"/"+tt.name, func(subT *testing.T) {
				g := NewWithT(subT)
				err := d.validate(tt.prev, tt.ais)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
			})
		}
	}
}

func TestValidateProxyUpdateExternalAccess(t *testing.T) {
	proxy := func(ea *aisv1.ExternalAccessSpec) *aisv1.AIStore {
		return &aisv1.AIStore{Spec: aisv1.AIStoreSpec{ProxySpec: aisv1.DaemonSpec{ExternalAccess: ea}}}
	}

	t.Run("retuning an enabled LoadBalancer is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		g.Expect(validateProxyUpdate(
			proxy(&aisv1.ExternalAccessSpec{}),
			proxy(&aisv1.ExternalAccessSpec{
				LoadBalancer: &aisv1.LoadBalancerSpec{Port: aisapc.Ptr[int32](443)},
			}),
		)).To(Succeed())
	})

	t.Run("removing the LoadBalancer block is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		g.Expect(validateProxyUpdate(
			proxy(&aisv1.ExternalAccessSpec{
				LoadBalancer: &aisv1.LoadBalancerSpec{Port: aisapc.Ptr[int32](443)},
			}),
			proxy(&aisv1.ExternalAccessSpec{}),
		)).To(Succeed())
	})

	t.Run("enabling external access is rejected", func(subT *testing.T) {
		g := NewWithT(subT)
		g.Expect(validateProxyUpdate(proxy(nil), proxy(&aisv1.ExternalAccessSpec{}))).To(HaveOccurred())
	})

	t.Run("disabling external access is rejected", func(subT *testing.T) {
		g := NewWithT(subT)
		g.Expect(validateProxyUpdate(proxy(&aisv1.ExternalAccessSpec{}), proxy(nil))).To(HaveOccurred())
	})
}

func TestValidateStateStorageUpdate(t *testing.T) {
	legacyClass := func(class string) *aisv1.AIStore {
		return &aisv1.AIStore{Spec: aisv1.AIStoreSpec{StateStorageClass: aisapc.Ptr(class)}}
	}
	statePVC := func(class string) *aisv1.AIStore {
		return &aisv1.AIStore{Spec: aisv1.AIStoreSpec{
			StateStorage: &aisv1.StateStorage{PVC: &aisv1.StatePVCConfig{StorageClass: class}},
		}}
	}

	tests := []struct {
		name    string
		prev    *aisv1.AIStore
		ais     *aisv1.AIStore
		wantErr bool
	}{
		{
			name: "moving the deprecated storage class to stateStorage is allowed",
			prev: legacyClass("my-sc"),
			ais:  statePVC("my-sc"),
		},
		{
			name:    "changing the storage class while migrating is rejected",
			prev:    legacyClass("my-sc"),
			ais:     statePVC("other-sc"),
			wantErr: true,
		},
		{
			name:    "changing the storage class is rejected",
			prev:    statePVC("my-sc"),
			ais:     statePVC("other-sc"),
			wantErr: true,
		},
		{
			name:    "migrating from emptyDir to a pvc is rejected",
			prev:    &aisv1.AIStore{Spec: aisv1.AIStoreSpec{StateStorage: &aisv1.StateStorage{EmptyDir: &aisv1.StateEmptyDirConfig{}}}},
			ais:     statePVC("my-sc"),
			wantErr: true,
		},
		{
			name: "migrating from a pvc to emptyDir is allowed",
			prev: statePVC("my-sc"),
			ais:  &aisv1.AIStore{Spec: aisv1.AIStoreSpec{StateStorage: &aisv1.StateStorage{EmptyDir: &aisv1.StateEmptyDirConfig{}}}},
		},
		{
			name: "moving the deprecated hostpath prefix to stateStorage is allowed",
			prev: &aisv1.AIStore{Spec: aisv1.AIStoreSpec{HostpathPrefix: aisapc.Ptr("/mnt")}},
			ais: &aisv1.AIStore{Spec: aisv1.AIStoreSpec{
				StateStorage: &aisv1.StateStorage{HostPath: &aisv1.StateHostPathConfig{Prefix: "/mnt"}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			g := NewWithT(subT)
			err := validateStateStorageUpdate(tt.prev, tt.ais)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
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

const (
	tenantNS    = "tenant"
	profileName = "prod-authn"
	// The audience of the cluster built by profileAIS
	ownAud = tenantNS + "/cluster"
)

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

func tokenExchangeProfile() *authv1alpha1.AIStoreAuthProfile {
	prof := authProfile(profileName)
	prof.Spec.TokenExchange = &authv1alpha1.AuthProfileTokenExchange{}
	return prof
}

func profileAIS() *aisv1.AIStore {
	ais := &aisv1.AIStore{}
	ais.Name = "cluster"
	ais.Namespace = tenantNS
	ais.Spec.Auth = &aisv1.AuthSpec{
		ProfileRef: &aisv1.AuthProfileRef{Name: profileName},
	}
	return ais
}

// audAIS returns a profileAIS cluster with auth.required_claims.aud set, leaving the
// enclosing config unset when aud is nil.
func audAIS(aud *[]string) *aisv1.AIStore {
	ais := profileAIS()
	if aud != nil {
		ais.Spec.ConfigToUpdate = &aisv1.ConfigToUpdate{
			Auth: &aisv1.AuthConfToUpdate{
				RequiredClaims: &aisv1.RequiredClaimsConfToUpdate{Aud: aud},
			},
		}
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
		ais        *aisv1.AIStore
		wantReview *authorizationv1.ResourceAttributes
	}{
		{
			name: "no auth",
			ais:  &aisv1.AIStore{},
		},
		{
			name:       "profile ref",
			ais:        profileAIS(),
			wantReview: profileAttrs(profileName),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook, reviews := newSARWebhook(t, true, profileObjs(tt.ais)...)
			g.Expect(webhook.validateAuthProfile(ctx, tt.ais)).To(Succeed())
			if tt.wantReview == nil {
				g.Expect(*reviews).To(BeEmpty())
				return
			}
			g.Expect(*reviews).To(HaveLen(1))
			g.Expect((*reviews)[0].Spec.ResourceAttributes).To(Equal(tt.wantReview))
		})
	}

	t.Run("profile ref is rejected when unauthorized", func(t *testing.T) {
		g := NewWithT(t)
		webhook, reviews := newSARWebhook(t, false)
		err := webhook.validateAuthProfile(ctx, profileAIS())
		g.Expect(*reviews).To(HaveLen(1))
		g.Expect((*reviews)[0].Spec.ResourceAttributes).To(Equal(profileAttrs(profileName)))
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring(`is not authorized to use aistoreauthprofiles resource "prod-authn"`)))
	})
}

func TestGetAuthProfile(t *testing.T) {
	ctx := context.Background()
	path := field.NewPath("spec", "auth", "profileRef")
	ais := profileAIS()

	t.Run("existing profile is returned", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, true, authProfile(profileName))
		prof, err := webhook.getAuthProfile(ctx, ais, path, profileName)
		g.Expect(err).To(Succeed())
		g.Expect(prof.Name).To(Equal("prod-authn"))
	})

	t.Run("missing profile is rejected with a field error", func(t *testing.T) {
		g := NewWithT(t)
		webhook, _ := newSARWebhook(t, true)
		_, err := webhook.getAuthProfile(ctx, ais, path, "missing-authn")
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
		_, err := webhook.getAuthProfile(ctx, ais, path, profileName)
		g.Expect(apierrors.IsInternalError(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring(`checking AIStoreAuthProfile "prod-authn"`)))
	})
}

func TestValidateTokenExchangeAud(t *testing.T) {
	for _, tt := range []struct {
		name string
		aud  *[]string
		prof *authv1alpha1.AIStoreAuthProfile
	}{
		{
			name: "no token exchange",
			aud:  &[]string{"other-ns/other"},
			prof: authProfile(profileName),
		},
		{
			name: "aud unset",
			prof: tokenExchangeProfile(),
		},
		{
			name: "aud empty",
			aud:  &[]string{},
			prof: tokenExchangeProfile(),
		},
		{
			name: "aud is the cluster's own",
			aud:  &[]string{ownAud},
			prof: tokenExchangeProfile(),
		},
		{
			name: "aud includes the cluster's own",
			aud:  &[]string{"other-ns/other", ownAud},
			prof: tokenExchangeProfile(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(validateTokenExchangeAud(audAIS(tt.aud), tt.prof)).To(Succeed())
		})
	}

	t.Run("aud without the cluster's own is rejected", func(t *testing.T) {
		g := NewWithT(t)
		ais := audAIS(&[]string{"other-ns/other"})
		err := validateTokenExchangeAud(ais, tokenExchangeProfile())
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring("spec.configToUpdate.auth.required_claims.aud")))
		g.Expect(err).To(MatchError(ContainSubstring(`must include "tenant/cluster"`)))
		g.Expect(err).To(MatchError(ContainSubstring("other-ns/other")))
	})

	t.Run("a profile using token exchange rejects a non-compliant spec on admission", func(t *testing.T) {
		g := NewWithT(t)
		ais := audAIS(&[]string{"other-ns/other"})
		webhook, _ := newSARWebhook(t, true, tokenExchangeProfile())
		err := webhook.validateAuthProfile(admissionCtx(), ais)
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring(`must include "tenant/cluster"`)))
	})
}

func TestValidateRequiredAudiences(t *testing.T) {
	for _, tt := range []struct {
		name string
		aud  *[]string
	}{
		{name: "aud unset"},
		{name: "aud empty", aud: &[]string{}},
		{name: "aud is the cluster's own", aud: &[]string{ownAud}},
		{name: "aud has no namespace", aud: &[]string{ownAud, "shared-fleet"}},
		{name: "aud is a URL", aud: &[]string{ownAud, "https://idp.example.com/tenant/cluster"}},
		{name: "aud is not a namespaced name", aud: &[]string{ownAud, "Other_NS/other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(validateRequiredAudiences(audAIS(tt.aud))).To(Succeed())
		})
	}

	// No cluster exists under either audience, so admission cannot report whether one does.
	t.Run("an audience in another namespace is rejected", func(t *testing.T) {
		g := NewWithT(t)
		err := validateRequiredAudiences(audAIS(&[]string{ownAud, "other-ns/other"}))
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring("spec.configToUpdate.auth.required_claims.aud[1]")))
		g.Expect(err).To(MatchError(ContainSubstring(`Only "tenant/cluster" is allowed`)))
	})

	t.Run("an audience in the cluster's own namespace is rejected", func(t *testing.T) {
		g := NewWithT(t)
		err := validateRequiredAudiences(audAIS(&[]string{tenantNS + "/other"}))
		g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
		g.Expect(err).To(MatchError(ContainSubstring("tenant/other")))
	})
}
