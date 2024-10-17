// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/cmn"
	mocks "github.com/ais-operator/pkg/services/mocks"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AIStoreController", func() {
	Describe("Reconcile", func() {
		var (
			r         *AIStoreReconciler
			c         client.Client
			apiClient *mocks.MockAIStoreClientInterface

			namespace string
			ctx       = context.TODO()
		)

		BeforeEach(func() {
			// Skip checking DNS entry because in existing cluster we might not be able to access Service(s).
			checkDNSEntry = func(context.Context, *aisv1.AIStore) error {
				return nil
			}

			namespace = "ais-test-" + rand.String(10)
			By(fmt.Sprintf("Using %q namespace", namespace))
			c = k8sClient

			// Setup initial resources.
			err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
			Expect(err).NotTo(HaveOccurred())

			tmpClient := aisclient.NewClient(c, c.Scheme())
			Expect(tmpClient).NotTo(BeNil())

			// Mock the client for AIS API calls
			mockCtrl := gomock.NewController(GinkgoT())
			apiClient = mocks.NewMockAIStoreClientInterface(mockCtrl)
			// Mock the client manager to return the mock client
			clientManager := mocks.NewMockAISClientManagerInterface(mockCtrl)
			clientManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(apiClient, nil).AnyTimes()

			r = NewAISReconciler(tmpClient, &record.FakeRecorder{}, ctrl.Log, clientManager)
		})

		Describe("Reconcile", func() {
			var (
				ais *aisv1.AIStore
			)

			BeforeEach(func() {
				ais = &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ais",
						Namespace: namespace,
					},
					Spec: aisv1.AIStoreSpec{
						InitImage: tutils.DefaultInitImage,
						NodeImage: tutils.DefaultNodeImage,
						ProxySpec: aisv1.DaemonSpec{
							Size: apc.Ptr[int32](1),
							ServiceSpec: aisv1.ServiceSpec{
								ServicePort:      intstr.FromInt32(51080),
								PublicPort:       intstr.FromInt32(51081),
								IntraControlPort: intstr.FromInt32(51082),
								IntraDataPort:    intstr.FromInt32(51083),
							},
						},
						TargetSpec: aisv1.TargetSpec{
							DaemonSpec: aisv1.DaemonSpec{
								Size: apc.Ptr[int32](1),
								ServiceSpec: aisv1.ServiceSpec{
									ServicePort:      intstr.FromInt32(51080),
									PublicPort:       intstr.FromInt32(51081),
									IntraControlPort: intstr.FromInt32(51082),
									IntraDataPort:    intstr.FromInt32(51083),
								},
							},
							Mounts: []aisv1.Mount{
								{Path: "/data", Size: resource.MustParse("10Gi")},
							},
						},
						HostpathPrefix: apc.Ptr("/ais"),
						ConfigToUpdate: &aisv1.ConfigToUpdate{
							Log: &aisv1.LogConfToUpdate{
								ToStderr: apc.Ptr(true),
							},
						},
					},
				}
				err := c.Create(ctx, ais)
				Expect(err).ToNot(HaveOccurred())
			})

			Describe("Without existing cluster", func() {
				It("should properly sync external edits to owned resources", func() {
					_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())

					By("Ensure that proxy Service has been created")
					var proxyService corev1.Service
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
					Expect(err).ToNot(HaveOccurred())
					Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
					Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
					Expect(proxyService.Spec.Ports).To(HaveLen(3))

					By("Delete proxy Service")
					err = c.Delete(ctx, &proxyService)
					Expect(err).NotTo(HaveOccurred())

					By("Ensure that Service is gone")
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
					Expect(err).To(HaveOccurred())
					Expect(k8serrors.IsNotFound(err)).To(BeTrue())

					By("Reconcile to recreate Service")
					_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())

					By("Ensure that proxy Service has been recreated")
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
					Expect(err).ToNot(HaveOccurred())
					Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
					Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
					Expect(proxyService.Spec.Ports).To(HaveLen(3))
				})

				It("should create proxy StatefulSet when it was removed", func() {
					var ss appsv1.StatefulSet

					By("Check that proxy StatefulSet does not exist")
					err := c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &ss)
					Expect(err).To(HaveOccurred())

					By("Reconcile to create proxy StatefulSet")
					ready, err := r.handleProxyState(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					Expect(ready).To(BeFalse())

					By("Ensure that proxy StatefulSet has been created")
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &ss)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should create target StatefulSet when it was removed", func() {
					var ss appsv1.StatefulSet

					By("Check that target StatefulSet does not exist")
					err := c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &ss)
					Expect(err).To(HaveOccurred())

					By("Reconcile to create target StatefulSet")
					ready, err := r.handleTargetState(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					Expect(ready).To(BeFalse())

					By("Ensure that target StatefulSet has been created")
					err = c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &ss)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should properly handle config update", func() {
					By("Update CRD")
					ais.Spec.ConfigToUpdate.Features = apc.Ptr("2568")
					expectedConfig, err := cmn.GenerateConfigToSet(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					expectedHash, err := cmn.HashConfigToSet(expectedConfig)
					Expect(err).ToNot(HaveOccurred())
					err = c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())

					By("Reconcile to propagate config")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Times(1)
					err = r.handleConfigState(ctx, ais, true /*force*/)
					Expect(err).ToNot(HaveOccurred())

					By("Ensure that config update is propagated to proxies/targets")
					err = c.Get(ctx, types.NamespacedName{Name: ais.Name, Namespace: namespace}, ais)
					Expect(err).ToNot(HaveOccurred())
					Expect(ais.Annotations[configHashAnnotation]).To(Equal(expectedHash))

					By("Ensure that a repeat with the same config does not result in an API call")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Times(0)
					err = r.handleConfigState(ctx, ais, false /*force*/)
					Expect(err).ToNot(HaveOccurred())

					By("Ensure that a repeat with the same config and force DOES result in an API call")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Times(1)
					err = r.handleConfigState(ctx, ais, true /*force*/)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Describe("With existing cluster", func() {
				BeforeEach(func() {
					if existingCluster, _ := cos.ParseBool(os.Getenv("USE_EXISTING_CLUSTER")); !existingCluster {
						Skip("Skipping tests which require existing cluster")
					}
				})

				It("should properly reconcile basic AIStore cluster", func() {
					_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())

					var ais aisv1.AIStore
					err = c.Get(ctx, types.NamespacedName{Name: "ais", Namespace: namespace}, &ais)
					Expect(err).ToNot(HaveOccurred())
					Expect(ais.GetFinalizers()).To(HaveLen(1))
					Expect(ais.Status.State).To(Equal(aisv1.ClusterInitialized))

					By("Ensure that proxy Service has been created")
					var proxyService corev1.Service
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
					Expect(err).ToNot(HaveOccurred())
					Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
					Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
					Expect(proxyService.Spec.Ports).To(HaveLen(3))

					By("Ensure that proxy StatefulSet has been created")
					var proxySS appsv1.StatefulSet
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxySS)
					Expect(err).ToNot(HaveOccurred())
					Expect(*proxySS.Spec.Replicas).To(BeEquivalentTo(1))

					By("Waiting for proxies to come up")
					Eventually(func(g Gomega) {
						var proxySS appsv1.StatefulSet
						err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxySS)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(proxySS.Status.Replicas).To(BeEquivalentTo(1))
						g.Expect(proxySS.Status.ReadyReplicas).To(BeEquivalentTo(1), "%v", proxySS.Status.Conditions)
					}, 2*time.Minute, 5*time.Second).Should(Succeed())

					result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())
					Expect(result.Requeue).To(BeTrue())

					By("Ensure that target Service has been created")
					var targetService corev1.Service
					err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &targetService)
					Expect(err).ToNot(HaveOccurred())
					Expect(targetService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
					Expect(targetService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
					Expect(targetService.Spec.Ports).To(HaveLen(3))

					By("Ensure that target StatefulSet has been created")
					var targetSS appsv1.StatefulSet
					err = c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &targetSS)
					Expect(err).ToNot(HaveOccurred())
					Expect(*targetSS.Spec.Replicas).To(BeEquivalentTo(1))

					By("Waiting for targets to come up")
					Eventually(func(g Gomega) {
						var targetSS appsv1.StatefulSet
						err = c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &targetSS)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(targetSS.Status.Replicas).To(BeEquivalentTo(1))
						g.Expect(targetSS.Status.ReadyReplicas).To(BeEquivalentTo(1), "%v", targetSS.Status.Conditions)
					}, 2*time.Minute, 5*time.Second).Should(Succeed())

					result, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
				})
			})
		})
	})
})
