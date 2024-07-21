// Package controllers contains k8s controller logic for AIS cluster
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"context"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient := aisclient.NewClient(c, c.Scheme())
			Expect(k8sClient).NotTo(BeNil())

			r = NewAISReconciler(k8sClient, &record.FakeRecorder{}, ctrl.Log, false)
			ctx = context.TODO()
		})

		Describe("Reconcile", func() {
			BeforeEach(func() {
				err := c.Create(ctx, &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ais",
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
				})
				Expect(err).ToNot(HaveOccurred())

				err = c.Create(ctx, &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should properly reconcile basic AIStore cluster", func() {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais"}})
				Expect(err).ToNot(HaveOccurred())

				var ais aisv1.AIStore
				err = c.Get(ctx, types.NamespacedName{Name: "ais"}, &ais)
				Expect(err).ToNot(HaveOccurred())
				Expect(ais.GetFinalizers()).To(HaveLen(1))
				Expect(ais.Status.State).To(Equal(aisv1.ConditionInitialized))

				var proxySS appsv1.StatefulSet
				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy"}, &proxySS)
				Expect(err).ToNot(HaveOccurred())

				// Targets should be deployed after proxies come to live.
				var targetSS appsv1.StatefulSet
				err = c.Get(ctx, types.NamespacedName{Name: "ais-target"}, &targetSS)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
