/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"testing"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		reconciler *Reconciler
		authn      *authv1alpha1.AIStoreAuth
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(authv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(certmanagerv1.AddToScheme(scheme)).To(Succeed())

		sc := "openebs-hostpath"
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Persistence: authv1alpha1.PersistenceSpec{
					StorageClass: &sc,
				},
				Deployment: authv1alpha1.DeploymentSpec{
					Container: authv1alpha1.ContainerSpec{
						Image: "docker.io/aistorage/authn:v4.5",
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(authn).
			WithObjects(authn).
			Build()
		reconciler = &Reconciler{
			client: aisclient.NewClient(fakeClient, scheme),
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

	It("reconciles managed resources and service status through Reconcile", func() {
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: authn.Name, Namespace: authn.Namespace,
		}}
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{}
		Expect(reconciler.client.Get(ctx, authnres.ConfigMapNSName(authn), cm)).To(Succeed())

		service := &corev1.Service{}
		Expect(reconciler.client.Get(ctx, authnres.ServiceNSName(authn), service)).To(Succeed())
		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		Expect(stored.Status.ServiceURL).To(Equal(authnres.ServiceURL(authn)))
	})

	It("creates an owned PVC for dynamic storage", func() {
		Expect(reconciler.reconcilePersistence(ctx, authn)).To(Succeed())

		pvc := &corev1.PersistentVolumeClaim{}
		Expect(reconciler.client.Get(ctx, authnres.PVCNSName(authn), pvc)).To(Succeed())
		Expect(pvc.OwnerReferences).To(HaveLen(1))
		Expect(pvc.OwnerReferences[0].Name).To(Equal(authn.Name))
		Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal("openebs-hostpath")))
		Expect(pvc.Spec.VolumeName).To(BeEmpty())
	})

	It("creates a PVC bound to an existing volume by name", func() {
		vol := "existing-authn-pv"
		authn.Spec.Persistence = authv1alpha1.PersistenceSpec{VolumeName: &vol}

		Expect(reconciler.reconcilePersistence(ctx, authn)).To(Succeed())

		pvc := &corev1.PersistentVolumeClaim{}
		Expect(reconciler.client.Get(ctx, authnres.PVCNSName(authn), pvc)).To(Succeed())
		Expect(pvc.Spec.VolumeName).To(Equal(vol))
		Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal("")))
	})

	It("creates an owned Deployment", func() {
		Expect(reconciler.reconcileDeployment(ctx, authn)).To(Succeed())

		deployment := &appsv1.Deployment{}
		Expect(reconciler.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
		Expect(deployment.OwnerReferences).To(HaveLen(1))
		Expect(deployment.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
		Expect(deployment.OwnerReferences[0].Name).To(Equal(authn.Name))
		Expect(deployment.Spec.Replicas).To(HaveValue(Equal(int32(1))))
		Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
	})

	It("creates external Services when enabled and removes owned ones when disabled", func() {
		nodePort := int32(31001)
		authn.Spec.ExternalAccess = &authv1alpha1.ExternalAccessSpec{
			NodePort:     &authv1alpha1.NodePortSpec{Port: nodePort},
			LoadBalancer: &authv1alpha1.LoadBalancerSpec{Port: 52001},
		}
		Expect(reconciler.reconcileServices(ctx, authn)).To(Succeed())

		nodePortSvc := &corev1.Service{}
		Expect(reconciler.client.Get(ctx, authnres.NodePortServiceNSName(authn), nodePortSvc)).To(Succeed())
		Expect(metav1.IsControlledBy(nodePortSvc, authn)).To(BeTrue())
		lbSvc := &corev1.Service{}
		Expect(reconciler.client.Get(ctx, authnres.LoadBalancerServiceNSName(authn), lbSvc)).To(Succeed())
		Expect(metav1.IsControlledBy(lbSvc, authn)).To(BeTrue())

		authn.Spec.ExternalAccess = nil
		Expect(reconciler.reconcileServices(ctx, authn)).To(Succeed())

		svc := &corev1.Service{}
		Expect(k8serrors.IsNotFound(reconciler.client.Get(ctx, authnres.NodePortServiceNSName(authn), svc))).To(BeTrue())
		Expect(k8serrors.IsNotFound(reconciler.client.Get(ctx, authnres.LoadBalancerServiceNSName(authn), svc))).To(BeTrue())
		Expect(reconciler.client.Get(ctx, authnres.ServiceNSName(authn), svc)).To(Succeed())
	})

	It("does not delete a disabled external Service it does not own", func() {
		service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name:      authnres.NodePortServiceName(authn),
			Namespace: authn.Namespace,
		}}
		Expect(reconciler.client.Create(ctx, service)).To(Succeed())

		Expect(reconciler.reconcileServices(ctx, authn)).To(Succeed())
		Expect(reconciler.client.Get(ctx, authnres.NodePortServiceNSName(authn), service)).To(Succeed())
		Expect(service.OwnerReferences).To(BeEmpty())
	})

	It("reconciles and converges Deployment image and config changes", func() {
		req := ctrl.Request{NamespacedName: authnres.DeploymentNSName(authn)}
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{}
		Expect(reconciler.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
		originalChecksum := deployment.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		stored.Spec.Deployment.Container.Image = "docker.io/aistorage/authn:v4.8"
		level := int32(4)
		stored.Spec.Config = &authv1alpha1.ConfigSpec{Log: &authv1alpha1.LogSpec{Level: &level}}
		Expect(reconciler.client.Update(ctx, stored)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciler.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("docker.io/aistorage/authn:v4.8"))
		Expect(deployment.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]).NotTo(Equal(originalChecksum))
	})

})
