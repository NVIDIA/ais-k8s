/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"testing"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestAIStoreAuthController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIStoreAuth controller suite")
}

var _ = Describe("AIStoreAuthReconciler", Label("short"), func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		reconciler *AIStoreAuthReconciler
		authn      *authv1alpha1.AIStoreAuth
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(authv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Deployment: authv1alpha1.DeploymentSpec{
					Image: "docker.io/aistorage/authn:v4.5",
				},
			},
		}

		reconciler = &AIStoreAuthReconciler{
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(authn).Build(),
			scheme: scheme,
			log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}
	})

	It("creates an owned ConfigMap", func() {
		Expect(reconciler.reconcileConfigMap(ctx, authn)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(reconciler.client.Get(ctx, authnres.ConfigMapNSName(authn), cm)).To(Succeed())
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].Controller).NotTo(BeNil())
		Expect(*cm.OwnerReferences[0].Controller).To(BeTrue())
		Expect(cm.OwnerReferences[0].Name).To(Equal(authn.Name))
	})

	It("reconciles the ConfigMap through Reconcile", func() {
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      authn.Name,
				Namespace: authn.Namespace,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{}
		Expect(reconciler.client.Get(ctx, authnres.ConfigMapNSName(authn), cm)).To(Succeed())
	})
})
