/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aistore

import (
	"context"

	"github.com/NVIDIA/aistore/api/apc"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/resources/aistore/target"
	mocks "github.com/ais-operator/internal/services/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
		r         *Reconciler
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
		r = NewReconciler(tmpClient, &events.FakeRecorder{}, ctrl.Log, clientManager)
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

var _ = Describe("scaleDownMode", func() {
	var (
		r         *Reconciler
		ais       *aisv1.AIStore
		mockCtrl  *gomock.Controller
		apiClient *mocks.MockAIStoreClientInterface
		namespace string
		ctx       = context.TODO()
	)

	BeforeEach(func() {
		namespace = "ais-test-" + rand.String(10)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

		ais = &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{Name: "ais", Namespace: namespace},
			Spec: aisv1.AIStoreSpec{
				InitImage: "init:latest",
				NodeImage: "node:latest",
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
						Size: apc.Ptr[int32](2),
						ServiceSpec: aisv1.ServiceSpec{
							ServicePort:      intstr.FromInt32(51080),
							PublicPort:       intstr.FromInt32(51081),
							IntraControlPort: intstr.FromInt32(51082),
							IntraDataPort:    intstr.FromInt32(51083),
						},
					},
					Mounts: []aisv1.Mount{{Path: "/data"}},
				},
				StateStorage: &aisv1.StateStorage{
					HostPath: &aisv1.StateHostPathConfig{Prefix: "/ais"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ais)).To(Succeed())

		tmpClient := aisclient.NewClient(k8sClient, k8sClient.Scheme())
		mockCtrl = gomock.NewController(GinkgoT())
		apiClient = mocks.NewMockAIStoreClientInterface(mockCtrl)
		clientManager := mocks.NewMockAISClientManagerInterface(mockCtrl)
		clientManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(apiClient, nil).AnyTimes()
		r = NewReconciler(tmpClient, &events.FakeRecorder{}, ctrl.Log, clientManager)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	makeSS := func(replicas int32) *appsv1.StatefulSet {
		return &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
		}
	}

	Describe("isReadyToScaleDown", func() {
		Context("when scaleDownMode is decommission", func() {
			BeforeEach(func() {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			})

			It("scales when all targets are ready", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)

				// 2 targets in smap but currentSize is 3, so safe to scale down by 1.
				ready, err := r.isReadyToScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeTrue())
			})

			It("delays scaling when all targets are still active", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)

				ready, err := r.isReadyToScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeFalse())
			})
		})

		Context("when scaleDownMode is retain", func() {
			BeforeEach(func() {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeRetain
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			})

			It("scales when the target being removed is in maintenance", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)
				ready, err := r.isReadyToScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeTrue())
			})

			It("delays scaling when the target being removed is not yet in maintenance", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)
				ready, err := r.isReadyToScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeFalse())
			})

			It("scales when the target being removed is absent from the cluster map", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)
				ready, err := r.isReadyToScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeTrue())
			})
		})
	})

	Describe("startTargetScaling", func() {
		BeforeEach(func() {
			// Pre-set rebalance condition so enableRebalanceCondition is a no-op
			ais.SetCondition(aisv1.ConditionReadyRebalance)
			Expect(k8sClient.Status().Update(ctx, ais)).To(Succeed())
		})

		expectDecommission := func(mode aisv1.ScaleDownMode, rmUserData bool) {
			ais.Spec.TargetSpec.ScaleDownMode = mode
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			ss := makeSS(3)
			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any(), false).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
				Expect(act.RmUserData).To(Equal(rmUserData))
				return "xid", nil
			})
			Expect(r.startTargetScaling(ctx, ais, ss)).To(Succeed())
		}

		It("decommissions targets with RmUserData=true when scaleDownMode is decommission", func() {
			expectDecommission(aisv1.ScaleDownModeDecommission, true)
		})

		It("keeps target data with RmUserData=false when scaleDownMode is safe_decommission", func() {
			expectDecommission(aisv1.ScaleDownModeSafeDecommission, false)
		})

		Context("when scaleDownMode is retain", func() {
			BeforeEach(func() {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeRetain
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			})

			It("puts targets in maintenance with SkipRebalance=true", func() {
				ss := makeSS(3)

				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t3": t3},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)

				apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
					Expect(act.SkipRebalance).To(BeTrue())
					Expect(act.DaemonID).To(Equal("t3"))
					return "xid", nil
				})

				err := r.startTargetScaling(ctx, ais, ss)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
