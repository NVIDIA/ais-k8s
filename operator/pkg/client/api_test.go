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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("K8sClient", func() {
	Describe("CreateOrUpdateResource", func() {
		var (
			c         client.Client
			ctx       context.Context
			k8sClient *K8sClient
			ais       *aisv1.AIStore
			ns        *corev1.Namespace
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient = NewClient(c, c.Scheme())
			Expect(k8sClient).NotTo(BeNil())

			ctx = context.TODO()
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
			changed, err := k8sClient.CreateOrUpdateResource(ctx, ais, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			})
			Expect(changed).To(BeTrue())
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
			changed, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm)
			Expect(changed).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())

			updatedCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore-updated"},
			}
			changed, err = k8sClient.CreateOrUpdateResource(ctx, ais, updatedCM)
			Expect(changed).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())

			fetchCM := &corev1.ConfigMap{}
			err = c.Get(ctx, client.ObjectKeyFromObject(cm), fetchCM)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchCM).To(Equal(updatedCM))
		})

		It("should be no-op if there is no change", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			}
			changed, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(changed).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())

			changed, err = k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(changed).To(BeFalse())
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
			changed, err := k8sClient.CreateOrUpdateResource(ctx, ais, newRes)
			Expect(changed).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())

			podWithStatus := podObj.DeepCopy()
			podWithStatus.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
			}
			err = c.Status().Update(ctx, podWithStatus)
			Expect(err).NotTo(HaveOccurred())

			changed, err = k8sClient.CreateOrUpdateResource(ctx, ais, podObj)
			Expect(changed).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip updating when unspecified fields are updated", func() {
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
					ServiceAccountName: "default", // assumed to be set by some controller.
				},
			}

			changed, err := k8sClient.CreateOrUpdateResource(ctx, ais, podObj)
			Expect(changed).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())

			newObj := &corev1.Pod{
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
					// missing sa name.
				},
			}
			changed, err = k8sClient.CreateOrUpdateResource(ctx, ais, newObj)
			Expect(changed).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			comparePod := &corev1.Pod{}
			err = c.Get(ctx, client.ObjectKeyFromObject(podObj), comparePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(comparePod).To(Equal(podObj))
		})
	})
})
