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
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			r   *AIStoreReconciler
			c   client.Client
			ctx context.Context

			namespace string
		)

		BeforeEach(func() {
			// Skip checking DNS entry because in existing cluster we might not be able to access Service(s).
			checkDNSEntry = func(context.Context, *aisv1.AIStore) error {
				return nil
			}

			ctx = context.TODO()
			namespace = "ais-test-" + rand.String(10)
			By(fmt.Sprintf("Using %q namespace", namespace))
			c = k8sClient

			// Setup initial resources.
			err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
			Expect(err).NotTo(HaveOccurred())

			tmpClient := aisclient.NewClient(c, c.Scheme())
			Expect(tmpClient).NotTo(BeNil())

			r = NewAISReconciler(tmpClient, &record.FakeRecorder{}, ctrl.Log, false)
		})

		Describe("Validation", func() {
			DescribeTable("should reject AIStore definition", func(ais *aisv1.AIStore, expectedMessage string) {
				// Extra setup.
				ais.ObjectMeta = metav1.ObjectMeta{
					Name:      "ais",
					Namespace: namespace,
				}

				err := c.Create(ctx, ais)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedMessage))
			},
				Entry(
					"not defined nodeImage",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{InitImage: "", NodeImage: ""}},
					"spec.initImage in body should be at least 1 chars long",
				),
				Entry(
					"not defined initImage",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{InitImage: "init-image:tag", NodeImage: ""}},
					"spec.nodeImage in body should be at least 1 chars long",
				),
				Entry(
					"not defined targetSpec.mounts",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
					}},
					"spec.targetSpec.mounts: Required value",
				),
				Entry(
					"not defined size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: aisv1.DaemonSpec{},
						TargetSpec: aisv1.TargetSpec{
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"not defined targetSpec.size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: aisv1.DaemonSpec{
							Size: apc.Ptr[int32](1),
						},
						TargetSpec: aisv1.TargetSpec{
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"not defined proxySpec.size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: aisv1.DaemonSpec{},
						TargetSpec: aisv1.TargetSpec{
							DaemonSpec: aisv1.DaemonSpec{
								Size: apc.Ptr[int32](1),
							},
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"invalid value for size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						Size:      apc.Ptr[int32](-1),
						ProxySpec: aisv1.DaemonSpec{},
						TargetSpec: aisv1.TargetSpec{
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"invalid value for targetSize.size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						Size:      apc.Ptr[int32](1),
						ProxySpec: aisv1.DaemonSpec{},
						TargetSpec: aisv1.TargetSpec{
							DaemonSpec: aisv1.DaemonSpec{
								Size: apc.Ptr[int32](-1),
							},
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"spec.targetSpec.size in body should be greater than or equal to 0",
				),
				Entry(
					"invalid value for proxySize.size",
					&aisv1.AIStore{Spec: aisv1.AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: aisv1.DaemonSpec{
							Size: apc.Ptr[int32](-1),
						},
						TargetSpec: aisv1.TargetSpec{
							DaemonSpec: aisv1.DaemonSpec{
								Size: apc.Ptr[int32](1),
							},
							Mounts: []aisv1.Mount{{Path: "/mnt"}},
						},
					}},
					"spec.proxySpec.size in body should be greater than or equal to 0",
				),
			)
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
						InitImage: tutils.InitImage,
						NodeImage: tutils.NodeImage,
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
