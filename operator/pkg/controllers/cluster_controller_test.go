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
			ctx = context.TODO()
			namespace = rand.String(10)
			c = k8sClient

			// Setup initial resources.
			err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
			Expect(err).NotTo(HaveOccurred())
			err = c.Create(ctx, &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			})
			Expect(client.IgnoreAlreadyExists(err)).ToNot(HaveOccurred())

			tmpClient := aisclient.NewClient(c, c.Scheme())
			Expect(tmpClient).NotTo(BeNil())

			r = NewAISReconciler(tmpClient, &record.FakeRecorder{}, ctrl.Log, false)
		})

		Describe("Reconcile", func() {
			BeforeEach(func() {
				err := c.Create(ctx, &aisv1.AIStore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ais",
						Namespace: namespace,
					},
					Spec: aisv1.AIStoreSpec{
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
					},
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should properly reconcile basic AIStore cluster", func() {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
				Expect(err).ToNot(HaveOccurred())

				var ais aisv1.AIStore
				err = c.Get(ctx, types.NamespacedName{Name: "ais", Namespace: namespace}, &ais)
				Expect(err).ToNot(HaveOccurred())
				Expect(ais.GetFinalizers()).To(HaveLen(1))
				Expect(ais.Status.State).To(Equal(aisv1.ConditionInitialized))

				var proxyService corev1.Service
				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
				Expect(err).ToNot(HaveOccurred())
				Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
				Expect(proxyService.Spec.Ports).To(HaveLen(3))

				var proxySS appsv1.StatefulSet
				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxySS)
				Expect(err).ToNot(HaveOccurred())
				Expect(*proxySS.Spec.Replicas).To(BeEquivalentTo(1))

				// Targets should be deployed after proxies come to live.
				var targetSS appsv1.StatefulSet
				err = c.Get(ctx, types.NamespacedName{Name: "ais-target", Namespace: namespace}, &targetSS)
				Expect(err).To(HaveOccurred())
			})

			It("should properly sync external edits to owned resources", func() {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
				Expect(err).ToNot(HaveOccurred())

				var proxyService corev1.Service
				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
				Expect(err).ToNot(HaveOccurred())
				Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
				Expect(proxyService.Spec.Ports).To(HaveLen(3))

				// Delete service
				err = c.Delete(ctx, &proxyService)
				Expect(err).NotTo(HaveOccurred())

				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
				Expect(err).To(HaveOccurred())
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

				// Reconcile
				_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "ais", Namespace: namespace}})
				Expect(err).ToNot(HaveOccurred())

				// Ensure service is recreated
				err = c.Get(ctx, types.NamespacedName{Name: "ais-proxy", Namespace: namespace}, &proxyService)
				Expect(err).ToNot(HaveOccurred())
				Expect(proxyService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(proxyService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
				Expect(proxyService.Spec.Ports).To(HaveLen(3))
			})
		})
	})
})
