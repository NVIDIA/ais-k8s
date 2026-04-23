// Package client contains wrapper for k8s client
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package client

import (
	"context"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("K8sClient", func() {
	Describe("CreateOrUpdateResource", func() {
		var (
			c         client.Client
			k8sClient *K8sClient
			ais       *aisv1.AIStore
			ns        *corev1.Namespace

			ctx = context.TODO()
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient = NewClient(c, c.Scheme())
			Expect(k8sClient).NotTo(BeNil())

			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "somenamespace"}}
			err := c.Create(ctx, ns)
			Expect(err).NotTo(HaveOccurred())

			ais = &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ais",
					Namespace: ns.GetName(),
				},
				Spec: aisv1.AIStoreSpec{
					ProxySpec: aisv1.DaemonSpec{
						Size: apc.Ptr[int32](1),
					},
					TargetSpec: aisv1.TargetSpec{
						DaemonSpec: aisv1.DaemonSpec{
							Size: apc.Ptr[int32](1),
						},
					},
					HostpathPrefix: apc.Ptr("/ais"),
				},
			}
			err = c.Create(ctx, ais)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create an object if not exists", func() {
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update the resource with diff", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			}
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm)
			Expect(err).NotTo(HaveOccurred())

			updatedCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore-updated"},
			}
			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, updatedCM)
			Expect(err).NotTo(HaveOccurred())

			fetchCM := &corev1.ConfigMap{}
			err = c.Get(ctx, client.ObjectKeyFromObject(cm), fetchCM)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchCM.Data).To(Equal(updatedCM.Data))
		})

		It("should be no-op if there is no change", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			}
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(err).NotTo(HaveOccurred())

			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not update resource when only status changes", func() {
			podObj := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-resource",
					Namespace: ns.GetName(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "something:tag",
						},
					},
				},
			}
			newRes := podObj.DeepCopy()
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, newRes)
			Expect(err).NotTo(HaveOccurred())

			podWithStatus := podObj.DeepCopy()
			podWithStatus.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
			}
			err = c.Status().Update(ctx, podWithStatus)
			Expect(err).NotTo(HaveOccurred())

			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, podObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Apply", func() {
		const (
			cmName = "test-configmap"
			nsName = "apply-ns"
		)

		var (
			c         client.Client
			k8sClient *K8sClient
			cmKey     = client.ObjectKey{Namespace: nsName, Name: cmName}
			ctx       = context.TODO()
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient = NewClient(c, c.Scheme())
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(c.Create(ctx, ns)).To(Succeed())
		})

		newCM := func() *corev1ac.ConfigMapApplyConfiguration {
			return corev1ac.ConfigMap(cmName, nsName).
				WithData(map[string]string{"test-key": "test-value"})
		}

		findManagedField := func(cm *corev1.ConfigMap, manager string) *metav1.ManagedFieldsEntry {
			for i := range cm.ManagedFields {
				if cm.ManagedFields[i].Manager == manager {
					return &cm.ManagedFields[i]
				}
			}
			return nil
		}

		It("should create the object if not exists", func() {
			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())
			Expect(fetchCM.Data).To(HaveKeyWithValue("test-key", "test-value"))
		})

		It("should record the operator field manager with operation Apply", func() {
			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())

			entry := findManagedField(fetchCM, FieldOwner)
			Expect(entry).NotTo(BeNil(), "expected a managedFields entry for %q", FieldOwner)
			Expect(entry.Operation).To(Equal(metav1.ManagedFieldsOperationApply))
		})

		It("should take over fields owned by another manager (ForceOwnership)", func() {
			const otherManager = "other-manager"
			otherApply := corev1ac.ConfigMap(cmName, nsName).
				WithData(map[string]string{"test-key": "other-value"})

			Expect(c.Apply(ctx, otherApply, client.FieldOwner(otherManager), client.ForceOwnership)).To(Succeed())

			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())
			Expect(fetchCM.Data).To(HaveKeyWithValue("test-key", "test-value"))

			entry := findManagedField(fetchCM, FieldOwner)
			Expect(entry).NotTo(BeNil(), "expected operator field manager to have taken ownership")
		})
	})
})
