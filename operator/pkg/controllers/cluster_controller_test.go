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
	"k8s.io/apimachinery/pkg/api/equality"
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
					reconcileProxy(ctx, ais, r)

					By("Ensure that proxy StatefulSet has been created")
					getStatefulSet(ctx, ais, c, "ais-proxy")
				})

				It("should create target StatefulSet when it was removed", func() {
					var ss appsv1.StatefulSet

					By("Check that target StatefulSet does not exist")
					err := c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &ss)
					Expect(err).To(HaveOccurred())

					By("Reconcile to create target StatefulSet")
					reconcileTarget(ctx, ais, r)

					By("Ensure that target StatefulSet has been created")
					getStatefulSet(ctx, ais, c, "ais-target")
				})

				It("should properly handle config update", func() {
					By("Update CRD")
					ais.Spec.ConfigToUpdate.Features = apc.Ptr("2568")
					expectedConfig, err := cmn.GenerateConfigToSet(ais)
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

				It("should reconcile new init image in spec", func() {
					newImg := "testInitImage"
					createStatefulSets(ctx, c, ais, r)

					By("Update init image in spec and reconcile")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil).Times(1)
					ais.Spec.InitImage = newImg
					err := c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					reconcileProxy(ctx, ais, r)
					reconcileTarget(ctx, ais, r)

					By("Expect statefulset spec to update")
					Eventually(statefulSetsImagesLatest(ctx, c, ais), 30*time.Second, 2*time.Second).Should(Succeed())
				})

				It("should reconcile new aisnode image in spec", func() {
					newImg := "testNodeImage"
					createStatefulSets(ctx, c, ais, r)

					By("Update node image in spec and reconcile")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil).Times(1)
					ais.Spec.NodeImage = newImg
					err := c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					reconcileProxy(ctx, ais, r)
					reconcileTarget(ctx, ais, r)

					By("Expect statefulset spec to update")
					Eventually(statefulSetsImagesLatest(ctx, c, ais), 30*time.Second, 2*time.Second).Should(Succeed())
				})

				It("should reconcile changed container resources", func() {
					createStatefulSets(ctx, c, ais, r)

					By("Update container resources and reconcile")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil).Times(1)
					ais.Spec.ProxySpec.Resources = corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					}
					ais.Spec.TargetSpec.Resources = corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					}
					err := c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					reconcileProxy(ctx, ais, r)
					reconcileTarget(ctx, ais, r)

					By("Expect statefulset spec to update")
					Eventually(func(g Gomega) {
						for _, stsType := range []string{"ais-proxy", "ais-target"} {
							ss := getStatefulSet(ctx, ais, c, stsType)
							g.Expect(ss.Spec.Template.Spec.Containers[0].Resources.Requests).To(HaveLen(2))
							g.Expect(ss.Spec.Template.Spec.Containers[0].Resources.Limits).To(HaveLen(2))
						}
					}, 30*time.Second, 2*time.Second).Should(Succeed())
				})

				It("should reconcile changed container env variables", func() {
					createStatefulSets(ctx, c, ais, r)

					By("Update container resources and reconcile")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil).Times(1)
					ais.Spec.ProxySpec.Env = []corev1.EnvVar{{Name: "key", Value: "value"}}
					ais.Spec.TargetSpec.Env = []corev1.EnvVar{{Name: "key", Value: "value"}}
					err := c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					reconcileProxy(ctx, ais, r)
					reconcileTarget(ctx, ais, r)

					By("Expect statefulset spec to update")
					Eventually(func(g Gomega) {
						for _, stsType := range []string{"ais-proxy", "ais-target"} {
							ss := getStatefulSet(ctx, ais, c, stsType)
							// Custom env variable should be first in the list.
							env := ss.Spec.Template.Spec.Containers[0].Env[0]
							g.Expect(env.Name).To(Equal("key"))
							g.Expect(env.Value).To(Equal("value"))
						}
					}, 30*time.Second, 2*time.Second).Should(Succeed())
				})

				It("should reconcile changed annotations", func() {
					createStatefulSets(ctx, c, ais, r)

					By("Update container resources and reconcile")
					apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil).Times(1)
					ais.Spec.ProxySpec.Annotations = map[string]string{"key": "value"}
					ais.Spec.TargetSpec.Annotations = map[string]string{"key": "value"}
					err := c.Update(ctx, ais)
					Expect(err).ToNot(HaveOccurred())
					reconcileProxy(ctx, ais, r)
					reconcileTarget(ctx, ais, r)

					By("Expect statefulset spec to update")
					Eventually(func(g Gomega) {
						for _, stsType := range []string{"ais-proxy", "ais-target"} {
							ss := getStatefulSet(ctx, ais, c, stsType)
							g.Expect(ss.Spec.Template.Annotations).To(HaveLen(1))
						}
					}, 30*time.Second, 2*time.Second).Should(Succeed())
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
					proxySS := getStatefulSet(ctx, &ais, c, "ais-proxy")
					Expect(*proxySS.Spec.Replicas).To(BeEquivalentTo(1))

					By("Waiting for proxies to come up")
					Eventually(proxiesReady(ctx, c, &ais), 2*time.Minute, 5*time.Second).Should(Succeed())

					result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())
					Expect(result.Requeue).To(BeTrue())

					By("Ensure that target Service has been created")
					var targetService corev1.Service
					err = c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &targetService)
					Expect(err).ToNot(HaveOccurred())
					Expect(targetService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
					Expect(targetService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
					Expect(targetService.Spec.Ports).To(HaveLen(3))

					By("Ensure that target StatefulSet has been created")
					targetSS := getStatefulSet(ctx, &ais, c, "ais-target")
					Expect(*targetSS.Spec.Replicas).To(BeEquivalentTo(1))

					By("Waiting for targets to come up")
					Eventually(targetsReady(ctx, c, &ais), 2*time.Minute, 5*time.Second).Should(Succeed())

					result, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
					Expect(err).ToNot(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
				})
			})
		})
	})

	Describe("shouldUpdatePodTemplate & syncPodTemplate", func() {
		DescribeTable("should correctly compare pod templates", func(desiredPodTemplate, currentPodTemplate *corev1.PodTemplateSpec, expectedResult bool) {
			needsUpdate, _ := shouldUpdatePodTemplate(desiredPodTemplate, currentPodTemplate)
			Expect(needsUpdate).To(Equal(expectedResult))

			// Also make sure that when syncing we will correctly update the template.
			synced := syncPodTemplate(desiredPodTemplate, currentPodTemplate)
			Expect(synced).To(Equal(expectedResult))
			equal := equality.Semantic.DeepEqual(desiredPodTemplate, currentPodTemplate)
			Expect(equal).To(BeTrue())
		},
			Entry("different init image",
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers:     []corev1.Container{{Image: "test:latest"}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:old"}},
						Containers:     []corev1.Container{{Image: "test:latest"}},
					},
				},
				true,
			),
			Entry("different node image",
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers:     []corev1.Container{{Image: "test:latest"}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers:     []corev1.Container{{Image: "test:old"}},
					},
				},
				true,
			),
			Entry("different resources (empty vs non-empty)",
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers:     []corev1.Container{{Image: "test:latest"}},
					},
				},
				true,
			),
			Entry("different resources (different values)",
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("200m"),
							}},
						}},
					},
				},
				true,
			),
			Entry("different resources (different types)",
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("200m"),
							}},
						}},
					},
				},
				true,
			),
			Entry("different env",
				&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Env:   []corev1.EnvVar{{Name: "key", Value: "value"}},
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				true,
			),
			Entry("different annotations",
				&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Env:   []corev1.EnvVar{{Name: "key", Value: "value"}},
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Env:   []corev1.EnvVar{{Name: "key", Value: "value"}},
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				true,
			),
			Entry("no update needed",
				&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Env:   []corev1.EnvVar{{Name: "key", Value: "value"}},
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "test:latest"}},
						Containers: []corev1.Container{{
							Image: "test:latest",
							Env:   []corev1.EnvVar{{Name: "key", Value: "value"}},
							Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							}},
						}},
					},
				},
				false,
			),
		)
	})
})

func statefulSetsImagesLatest(ctx context.Context, c client.Client, ais *aisv1.AIStore) func(g Gomega) {
	return func(g Gomega) {
		for _, stsType := range []string{"ais-proxy", "ais-target"} {
			ss := getStatefulSet(ctx, ais, c, stsType)
			g.Expect(ss.Spec.Template.Spec.InitContainers[0].Image).To(BeEquivalentTo(ais.Spec.InitImage))
			g.Expect(ss.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(ais.Spec.NodeImage))
		}
	}
}

func proxiesReady(ctx context.Context, c client.Client, ais *aisv1.AIStore) func(g Gomega) {
	return func(g Gomega) {
		ss := getStatefulSet(ctx, ais, c, "ais-proxy")
		g.Expect(ss.Spec.Template.Spec.InitContainers[0].Image).To(BeEquivalentTo(ais.Spec.InitImage))
		g.Expect(ss.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(ais.Spec.NodeImage))
		g.Expect(ss.Status.Replicas).To(BeEquivalentTo(1))
		g.Expect(ss.Status.ReadyReplicas).To(BeEquivalentTo(1), "%v", ss.Status.Conditions)
	}
}

func targetsReady(ctx context.Context, c client.Client, ais *aisv1.AIStore) func(g Gomega) {
	return func(g Gomega) {
		ss := getStatefulSet(ctx, ais, c, "ais-target")
		g.Expect(ss.Spec.Template.Spec.InitContainers[0].Image).To(BeEquivalentTo(ais.Spec.InitImage))
		g.Expect(ss.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(ais.Spec.NodeImage))
		g.Expect(ss.Status.Replicas).To(BeEquivalentTo(1))
		g.Expect(ss.Status.ReadyReplicas).To(BeEquivalentTo(1), "%v", ss.Status.Conditions)
	}
}

func createStatefulSets(ctx context.Context, c client.Client, ais *aisv1.AIStore, r *AIStoreReconciler) {
	By("Reconcile to create StatefulSets")
	reconcileProxy(ctx, ais, r)
	reconcileTarget(ctx, ais, r)

	By("Ensure that StatefulSets have been created")
	getStatefulSet(ctx, ais, c, "ais-proxy")
	getStatefulSet(ctx, ais, c, "ais-target")
}

func getStatefulSet(ctx context.Context, ais *aisv1.AIStore, c client.Client, ssName string) (ss appsv1.StatefulSet) {
	err := c.Get(ctx, types.NamespacedName{Name: ssName, Namespace: ais.Namespace}, &ss)
	Expect(err).ToNot(HaveOccurred())
	return
}

func reconcileTarget(ctx context.Context, ais *aisv1.AIStore, r *AIStoreReconciler) {
	By("Reconcile targets")
	result, err := r.handleTargetState(ctx, ais)
	Expect(err).ToNot(HaveOccurred())
	Expect(result.RequeueAfter).To(Not(BeNil()))
}

func reconcileProxy(ctx context.Context, ais *aisv1.AIStore, r *AIStoreReconciler) {
	By("Reconcile proxies")
	result, err := r.handleProxyState(ctx, ais)
	Expect(err).ToNot(HaveOccurred())
	Expect(result.Requeue).To(BeTrue())
}
