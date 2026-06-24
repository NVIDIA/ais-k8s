/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"context"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/target"
	mocks "github.com/ais-operator/pkg/services/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("isPodRolloutCompleted", func() {
	It("returns true when pod has a Ready condition", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		Expect(isPodRolloutCompleted(pod)).To(BeTrue())
	})

	It("returns true for an unschedulable pod so rollout can continue", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: corev1.PodReasonUnschedulable,
					},
				},
			},
		}

		Expect(isPodRolloutCompleted(pod)).To(BeTrue())
	})

	It("returns false for a generic not-ready pod", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		Expect(isPodRolloutCompleted(pod)).To(BeFalse())
	})

	It("returns false when pod is nil", func() {
		Expect(isPodRolloutCompleted(nil)).To(BeFalse())
	})
})

var _ = Describe("prepareTargetForRollout", func() {
	var (
		r         *AIStoreReconciler
		ais       *aisv1.AIStore
		mockCtrl  *gomock.Controller
		namespace string
		ctx       = context.TODO()
	)

	BeforeEach(func() {
		namespace = "ais-test-" + rand.String(10)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

		ais = &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "ais", Namespace: namespace}}

		tmpClient := aisclient.NewClient(k8sClient, k8sClient.Scheme())
		mockCtrl = gomock.NewController(GinkgoT())
		// No GetClient expectation: the controller fails the test if maintenance is attempted.
		clientManager := mocks.NewMockAISClientManagerInterface(mockCtrl)
		r = NewAISReconciler(tmpClient, &events.FakeRecorder{}, ctrl.Log, clientManager)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("skips maintenance for an unschedulable pod", func() {
		podName := target.PodName(ais, 0)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "aisnode", Image: "aisnode:test"}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		pod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable,
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		requeue, err := r.prepareTargetForRollout(ctx, ais, podName)
		Expect(err).NotTo(HaveOccurred())
		Expect(requeue).To(BeFalse())
	})
})
