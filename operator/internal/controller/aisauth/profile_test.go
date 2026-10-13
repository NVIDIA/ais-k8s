/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"errors"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("AIStoreAuth admin AIStoreAuthProfile", Label("short"), func() {
	const profileName = "ais.ais-authn-admin"
	adminSecret := &corev1.LocalObjectReference{Name: "ais-authn-su-creds"}

	var (
		scheme     *runtime.Scheme
		reconciler *Reconciler
		authn      *authv1alpha1.AIStoreAuth
	)

	BeforeEach(func() {
		scheme = newTestScheme()
		authn = newTestAuthN()
		authn.Spec.AdminSecret = adminSecret
		reconciler, _ = newTestReconciler(scheme, authn)
	})

	reconcileOnce := func(ctx context.Context) error {
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: authnNSName(authn)})
		return err
	}

	DescribeTable("creates the profile only with an admin Secret",
		func(ctx context.Context, secret *corev1.LocalObjectReference, created bool) {
			authn.Spec.AdminSecret = secret

			Expect(reconciler.reconcileAuthProfile(ctx, authn)).To(Succeed())

			_, err := reconciler.client.GetAuthProfile(ctx, profileName)
			Expect(k8serrors.IsNotFound(err)).To(Equal(!created))
		},
		Entry("with an admin Secret", adminSecret, true),
		Entry("without an admin Secret", nil, false),
	)

	It("deletes the profile when the admin Secret is removed", func(ctx context.Context) {
		Expect(reconcileOnce(ctx)).To(Succeed())
		authn.Spec.AdminSecret = nil

		Expect(reconciler.reconcileAuthProfile(ctx, authn)).To(Succeed())

		_, err := reconciler.client.GetAuthProfile(ctx, profileName)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})

	otherOwner := ownerref.AIStoreAuthOwner(&authv1alpha1.AIStoreAuth{ObjectMeta: metav1.ObjectMeta{Namespace: "ais", Name: "other"}})

	DescribeTable("leaves a profile that this AIStoreAuth does not own",
		func(ctx context.Context, owner *ownerref.LabeledOwner, secret *corev1.LocalObjectReference, result gomegatypes.GomegaMatcher) {
			existing := &authv1alpha1.AIStoreAuthProfile{
				ObjectMeta: metav1.ObjectMeta{Name: profileName},
				Spec:       authv1alpha1.AIStoreAuthProfileSpec{ServiceURL: "https://auth.example.com"},
			}
			if owner != nil {
				ownerref.SetLabeledOwner(existing, *owner)
			}
			Expect(reconciler.client.Create(ctx, existing)).To(Succeed())
			authn.Spec.AdminSecret = secret

			Expect(reconciler.reconcileAuthProfile(ctx, authn)).To(result)

			profile, err := reconciler.client.GetAuthProfile(ctx, profileName)
			Expect(err).NotTo(HaveOccurred())
			Expect(profile.Spec.ServiceURL).To(Equal("https://auth.example.com"))
		},
		Entry("unlabeled, with an admin Secret", nil, adminSecret, MatchError(ContainSubstring(profileName))),
		Entry("unlabeled, without an admin Secret", nil, nil, Succeed()),
		Entry("owned by another AIStoreAuth, with an admin Secret", &otherOwner, adminSecret, MatchError(ContainSubstring(profileName))),
		Entry("owned by another AIStoreAuth, without an admin Secret", &otherOwner, nil, Succeed()),
	)

	DescribeTable("leaves a profile that changes owner after the ownership check",
		func(ctx context.Context, secret *corev1.LocalObjectReference) {
			relabel := false
			reconciler, _ = newTestReconciler(scheme, authn, interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
				) error {
					if err := c.Get(ctx, key, obj, opts...); err != nil || !relabel {
						return err
					}
					relabel = false
					current := obj.DeepCopyObject().(client.Object)
					current.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "helm"})
					return c.Update(ctx, current)
				},
			})
			Expect(reconciler.reconcileAuthProfile(ctx, authn)).To(Succeed())
			relabel = true
			authn.Spec.AdminSecret = secret

			Expect(reconciler.reconcileAuthProfile(ctx, authn)).To(HaveOccurred())

			profile, err := reconciler.client.GetAuthProfile(ctx, profileName)
			Expect(err).NotTo(HaveOccurred())
			Expect(profile.Labels).To(Equal(map[string]string{"app.kubernetes.io/managed-by": "helm"}))
		},
		Entry("on apply", adminSecret),
		Entry("on delete", nil),
	)

	It("deletes the profile on deletion of the AIStoreAuth", func(ctx context.Context) {
		Expect(reconcileOnce(ctx)).To(Succeed())

		deleteAuthN(ctx, reconciler, authn)
		Expect(reconcileOnce(ctx)).To(Succeed())

		_, err := reconciler.client.GetAuthProfile(ctx, profileName)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})

	It("holds the finalizer while the profile delete is rejected", func(ctx context.Context) {
		reconciler, _ = newTestReconciler(scheme, authn, interceptor.Funcs{
			Delete: func(
				ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption,
			) error {
				if _, isProfile := obj.(*authv1alpha1.AIStoreAuthProfile); isProfile {
					return errors.New("still referenced by AIStore")
				}
				return c.Delete(ctx, obj, opts...)
			},
		})
		Expect(reconcileOnce(ctx)).To(Succeed())

		deleteAuthN(ctx, reconciler, authn)
		Expect(reconcileOnce(ctx)).To(HaveOccurred())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, authnNSName(authn), stored)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(stored, authnFinalizer)).To(BeTrue())
		_, err := reconciler.client.GetAuthProfile(ctx, profileName)
		Expect(err).NotTo(HaveOccurred())
	})

	It("maps a profile event to the AIStoreAuth that owns the profile", func(ctx context.Context) {
		Expect(reconcileOnce(ctx)).To(Succeed())
		profile, err := reconciler.client.GetAuthProfile(ctx, profileName)
		Expect(err).NotTo(HaveOccurred())

		Expect(reconciler.findAIStoreAuthForProfile(ctx, profile)).To(Equal([]reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: "ais", Name: "ais-authn"}},
		}))

		profile.Labels["ais.nvidia.com/owner-name"] = "missing"
		Expect(reconciler.findAIStoreAuthForProfile(ctx, profile)).To(BeEmpty())

		profile.Labels = nil
		Expect(reconciler.findAIStoreAuthForProfile(ctx, profile)).To(BeEmpty())
	})
})
