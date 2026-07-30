/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"errors"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("AIStoreAuth cleanup", Label("short"), func() {
	var (
		scheme     *runtime.Scheme
		reconciler *Reconciler
		authn      *authv1alpha1.AIStoreAuth
	)

	BeforeEach(func() {
		scheme = newTestScheme()
		authn = newTestAuthN()
		reconciler, _ = newTestReconciler(scheme, authn)
	})

	reconcileOnce := func(ctx context.Context) error {
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: authnNSName(authn)})
		return err
	}

	// deleteCR removes the CR, which only stamps a deletion timestamp while the finalizer is held.
	deleteCR := func(ctx context.Context) {
		GinkgoHelper()
		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, authnNSName(authn), stored)).To(Succeed())
		_, err := reconciler.client.DeleteResourceIfExists(ctx, stored)
		Expect(err).NotTo(HaveOccurred())
	}

	It("adds the finalizer on first reconcile", func(ctx context.Context) {
		Expect(reconcileOnce(ctx)).To(Succeed())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, authnNSName(authn), stored)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(stored, authnFinalizer)).To(BeTrue())
	})

	It("releases the finalizer on deletion, leaving the ConfigMap to garbage collection", func(ctx context.Context) {
		Expect(reconcileOnce(ctx)).To(Succeed())

		deleteCR(ctx)
		Expect(reconcileOnce(ctx)).To(Succeed())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(k8serrors.IsNotFound(reconciler.client.Get(ctx, authnNSName(authn), stored))).To(BeTrue())
		cm := &corev1.ConfigMap{}
		Expect(reconciler.client.Get(ctx, authnres.ConfigMapNSName(authn), cm)).To(Succeed())
		Expect(metav1.IsControlledBy(cm, authn)).To(BeTrue())
	})

	It("leaves the owned TLS Certificate to garbage collection on deletion", func(ctx context.Context) {
		authn.Spec.TLS = &authv1alpha1.TLSSpec{
			Certificate: &authv1alpha1.TLSCertificateConfig{
				IssuerRef: authv1alpha1.CertIssuerRef{Name: "ca-issuer"},
			},
		}
		reconciler, _ = newTestReconciler(scheme, authn)
		Expect(reconcileOnce(ctx)).To(Succeed())

		deleteCR(ctx)
		Expect(reconcileOnce(ctx)).To(Succeed())

		certificate := &certmanagerv1.Certificate{}
		Expect(reconciler.client.Get(ctx, authnres.CertificateNSName(authn), certificate)).To(Succeed())
		Expect(metav1.IsControlledBy(certificate, authn)).To(BeTrue())
	})

	It("holds the finalizer and reports the failure on Ready when cleanup fails", func(ctx context.Context) {
		var recorder *events.FakeRecorder
		reconciler, recorder = newTestReconciler(scheme, authn, interceptor.Funcs{
			Patch: func(
				ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption,
			) error {
				if _, isPVC := obj.(*corev1.PersistentVolumeClaim); isPVC {
					return errors.New("patch rejected")
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		})
		Expect(reconcileOnce(ctx)).To(Succeed())

		deleteCR(ctx)
		Expect(reconcileOnce(ctx)).To(HaveOccurred())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, authnNSName(authn), stored)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(stored, authnFinalizer)).To(BeTrue())
		ready := meta.FindStatusCondition(stored.Status.Conditions, string(authv1alpha1.ConditionReady))
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(authv1alpha1.ReasonCleanupFailed)))
		Expect(recorder.Events).To(Receive(ContainSubstring("Warning PVCRetentionFailed")))

		Expect(reconcileOnce(ctx)).To(HaveOccurred())
		Expect(recorder.Events).To(BeEmpty())
	})

	DescribeTable("AuthN data PVC on deletion",
		func(ctx context.Context, policy authv1alpha1.PersistenceDeletionPolicy, ownerReferences OmegaMatcher) {
			Expect(reconcileOnce(ctx)).To(Succeed())
			stored := &authv1alpha1.AIStoreAuth{}
			Expect(reconciler.client.Get(ctx, authnNSName(authn), stored)).To(Succeed())
			stored.Spec.Persistence.DeletionPolicy = policy
			Expect(reconciler.client.Update(ctx, stored)).To(Succeed())

			deleteCR(ctx)
			Expect(reconcileOnce(ctx)).To(Succeed())

			pvc := &corev1.PersistentVolumeClaim{}
			Expect(reconciler.client.Get(ctx, authnres.PVCNSName(authn), pvc)).To(Succeed())
			Expect(pvc.OwnerReferences).To(ownerReferences)
		},
		Entry("kept on Retain, preserving the users and RSA keys",
			authv1alpha1.PersistenceRetain, BeEmpty()),
		Entry("handed to garbage collection on Delete",
			authv1alpha1.PersistenceDelete, Not(BeEmpty())),
	)

	It("tolerates deletion when the finalizer is already absent", func(ctx context.Context) {
		bare := &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: authn.Namespace},
		}
		result, err := reconciler.reconcileDeletion(ctx, bare)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())
	})
})
