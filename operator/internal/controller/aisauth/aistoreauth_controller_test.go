/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"context"
	"errors"
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
		recorder   *events.FakeRecorder
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
			WithStatusSubresource(authn, &appsv1.Deployment{}).
			WithObjects(authn).
			Build()
		recorder = events.NewFakeRecorder(8)
		reconciler = &Reconciler{
			client:   aisclient.NewClient(fakeClient, scheme),
			scheme:   scheme,
			log:      zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			recorder: recorder,
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

	It("tracks readiness across the Deployment lifecycle", func() {
		req := ctrl.Request{NamespacedName: authnres.DeploymentNSName(authn)}

		// Applied, but the rollout has not landed yet.
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		Expect(stored.Status.ObservedGeneration).To(Equal(stored.Generation))
		Expect(stored.Status.ServiceURL).To(Equal(authnres.ServiceURL(authn)))
		Expect(stored.Status.Conditions).To(HaveLen(1))
		ready := meta.FindStatusCondition(stored.Status.Conditions, string(authv1alpha1.ConditionReady))
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(authv1alpha1.ReasonDeploymentUnavailable)))
		Expect(ready.Message).To(Equal("Waiting for the AuthN deployment to become available"))
		Expect(ready.ObservedGeneration).To(Equal(stored.Generation))

		// The rollout lands.
		markDeploymentAvailable(ctx, reconciler, authn)
		_, err = reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(stored.Status.Conditions, string(authv1alpha1.ConditionReady))).To(BeTrue())
		Expect(recorder.Events).To(Receive(ContainSubstring("Normal Ready")))

		// The deployment loses its ready replica.
		deployment := &appsv1.Deployment{}
		Expect(reconciler.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
		deployment.Status.AvailableReplicas = 0
		deployment.Status.ReadyReplicas = 0
		Expect(reconciler.client.Status().Update(ctx, deployment)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		Expect(meta.IsStatusConditionFalse(stored.Status.Conditions, string(authv1alpha1.ConditionReady))).To(BeTrue())
	})

	It("reports the Deployment's verdict on a wedged rollout", func() {
		req := ctrl.Request{NamespacedName: authnres.DeploymentNSName(authn)}
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{}
		Expect(reconciler.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
		deployment.Status.Conditions = []appsv1.DeploymentCondition{{
			Type:    appsv1.DeploymentProgressing,
			Status:  corev1.ConditionFalse,
			Reason:  "ProgressDeadlineExceeded",
			Message: `ReplicaSet "ais-authn-7d9f" has timed out progressing.`,
		}}
		Expect(reconciler.client.Status().Update(ctx, deployment)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		ready := meta.FindStatusCondition(stored.Status.Conditions, string(authv1alpha1.ConditionReady))
		Expect(ready.Reason).To(Equal(string(authv1alpha1.ReasonProgressDeadlineExceeded)))
		Expect(ready.Message).To(Equal(msgNotProgressing))
	})

	It("reports an apply failure on Ready", func() {
		reconciler.client = clientWithApplyError(scheme, authn, errors.New("apply conflict"))
		req := ctrl.Request{NamespacedName: authnNSName(authn)}

		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())

		stored := &authv1alpha1.AIStoreAuth{}
		Expect(reconciler.client.Get(ctx, req.NamespacedName, stored)).To(Succeed())
		ready := meta.FindStatusCondition(stored.Status.Conditions, string(authv1alpha1.ConditionReady))
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(authv1alpha1.ReasonReconcileFailed)))
		Expect(ready.Message).To(Equal("Failed to reconcile ConfigMap"))
		Expect(ready.ObservedGeneration).To(Equal(stored.Generation))
		// The endpoint is derived from spec, so a failed apply does not withhold it.
		Expect(stored.Status.ServiceURL).To(Equal(authnres.ServiceURL(authn)))
	})

})

// markDeploymentAvailable makes the AuthN Deployment report a fully rolled out replica.
// The fake client does not assign a generation on server-side apply, so it is set here.
func markDeploymentAvailable(ctx context.Context, r *Reconciler, authn *authv1alpha1.AIStoreAuth) {
	GinkgoHelper()
	deployment := &appsv1.Deployment{}
	Expect(r.client.Get(ctx, authnres.DeploymentNSName(authn), deployment)).To(Succeed())
	deployment.Generation = 1
	Expect(r.client.Update(ctx, deployment)).To(Succeed())
	deployment.Status.ObservedGeneration = deployment.Generation
	deployment.Status.Replicas = authnres.DeploymentReplicas
	deployment.Status.UpdatedReplicas = authnres.DeploymentReplicas
	deployment.Status.ReadyReplicas = authnres.DeploymentReplicas
	deployment.Status.AvailableReplicas = authnres.DeploymentReplicas
	Expect(r.client.Status().Update(ctx, deployment)).To(Succeed())
}

func authnNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: authn.Name, Namespace: authn.Namespace}
}

// clientWithApplyError fails every server-side apply. Status writes go through the subresource
// interceptor.
func clientWithApplyError(
	scheme *runtime.Scheme,
	authn *authv1alpha1.AIStoreAuth,
	applyErr error,
) *aisclient.K8sClient {
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(authn, &appsv1.Deployment{}).
		WithObjects(authn).
		WithInterceptorFuncs(interceptor.Funcs{
			Apply: func(
				_ context.Context,
				_ client.WithWatch,
				_ runtime.ApplyConfiguration,
				_ ...client.ApplyOption,
			) error {
				return applyErr
			},
		}).
		Build()
	return aisclient.NewClient(fakeClient, scheme)
}
