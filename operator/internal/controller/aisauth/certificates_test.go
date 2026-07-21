/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("TLS Certificate controller", Label("short"), func() {
	var (
		scheme *runtime.Scheme
		authn  *authv1alpha1.AIStoreAuth
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(authv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(certmanagerv1.AddToScheme(scheme)).To(Succeed())

		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
		}
	})

	newReconciler := func(objects ...clientpkg.Object) *Reconciler {
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
		return &Reconciler{client: aisclient.NewClient(client, scheme)}
	}

	enableManagedTLS := func() {
		authn.Spec.TLS = &authv1alpha1.TLSSpec{
			Certificate: &authv1alpha1.TLSCertificateConfig{
				IssuerRef: authv1alpha1.CertIssuerRef{Name: "ca-issuer"},
			},
		}
	}

	It("applies and deletes a managed Certificate", func(ctx context.Context) {
		enableManagedTLS()
		reconciler := newReconciler()

		Expect(reconciler.reconcileTLSCertificate(ctx, authn)).To(Succeed())
		certificate := authnres.TLSCertificate(authn)
		Expect(reconciler.client.Get(ctx, authnres.CertificateNSName(authn), certificate)).To(Succeed())

		authn.Spec.TLS = nil
		Expect(reconciler.reconcileTLSCertificate(ctx, authn)).To(Succeed())
		Expect(k8serrors.IsNotFound(
			reconciler.client.Get(ctx, authnres.CertificateNSName(authn), certificate),
		)).To(BeTrue())
	})

	Describe("LoadBalancer endpoints", func() {
		BeforeEach(func() {
			authn.Spec.TLS = &authv1alpha1.TLSSpec{
				Certificate: &authv1alpha1.TLSCertificateConfig{},
			}
			authn.Spec.ExternalAccess = &authv1alpha1.ExternalAccessSpec{
				LoadBalancer: &authv1alpha1.LoadBalancerSpec{},
			}
		})

		It("resolves ingress IPs and hostnames", func(ctx context.Context) {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      authnres.LoadBalancerServiceName(authn),
					Namespace: authn.Namespace,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{IP: "192.0.2.20"},
							{Hostname: "lb.authn.example.com"},
						},
					},
				},
			}
			reconciler := newReconciler(service)

			endpoints, err := reconciler.loadBalancerEndpoints(ctx, authn)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(Equal([]string{"192.0.2.20", "lb.authn.example.com"}))
		})

		It("returns no endpoints before the Service exists", func(ctx context.Context) {
			endpoints, err := newReconciler().loadBalancerEndpoints(ctx, authn)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(BeEmpty())
		})
	})
})
