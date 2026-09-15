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
						PublicPort:       apc.Ptr(intstr.FromInt32(51081)),
						IntraControlPort: apc.Ptr(intstr.FromInt32(51082)),
						IntraDataPort:    apc.Ptr(intstr.FromInt32(51083)),
					},
				},
				TargetSpec: aisv1.TargetSpec{
					DaemonSpec: aisv1.DaemonSpec{
						Size: apc.Ptr[int32](2),
						ServiceSpec: aisv1.ServiceSpec{
							PublicPort:       apc.Ptr(intstr.FromInt32(51081)),
							IntraControlPort: apc.Ptr(intstr.FromInt32(51082)),
							IntraDataPort:    apc.Ptr(intstr.FromInt32(51083)),
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

	Describe("targetsReadyForScaleDown", func() {
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
				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
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

				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeFalse())
			})

			It("delays scaling until every target being removed has left the cluster map", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)

				// Scaling from 4 to 2, so ais-target-2 leaving is not enough.
				ready, err := r.targetsReadyForScaleDown(ctx, ais, 4)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeFalse())
			})

			It("scales when an unrelated target lingers in the cluster map", func() {
				t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
				t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
				// Left in maintenance by an earlier retain-mode scale-down, so it never leaves the map.
				t4 := &aismeta.Snode{DaeID: "t4", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-3"}, Flags: aismeta.SnodeMaint}
				smap := &aismeta.Smap{
					Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t4": t4},
				}
				apiClient.EXPECT().GetClusterMap().Return(smap, nil)

				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeTrue())
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
				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
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
				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
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
				ready, err := r.targetsReadyForScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(ready).To(BeTrue())
			})
		})
	})

	Describe("prepareTargetsForScaleDown", func() {
		BeforeEach(func() {
			// Pre-set rebalance condition so enableRebalanceCondition is a no-op
			ais.SetCondition(aisv1.ConditionReadyRebalance)
			Expect(k8sClient.Status().Update(ctx, ais)).To(Succeed())
		})

		expectDecommission := func(mode aisv1.ScaleDownMode, rmUserData bool) {
			ais.Spec.TargetSpec.ScaleDownMode = mode
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
				Expect(act.RmUserData).To(Equal(rmUserData))
				return "xid", nil
			})
			Expect(r.prepareTargetsForScaleDown(ctx, ais, 3)).To(Succeed())
		}

		It("decommissions targets with RmUserData=true when scaleDownMode is decommission", func() {
			expectDecommission(aisv1.ScaleDownModeDecommission, true)
		})

		It("keeps target data with RmUserData=false when scaleDownMode is safe_decommission", func() {
			expectDecommission(aisv1.ScaleDownModeSafeDecommission, false)
		})

		It("decommissions a target that has finished maintenance rebalance", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint | aismeta.SnodeMaintPostReb}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
				Expect(act.DaemonID).To(Equal("t3"))
				Expect(act.RmUserData).To(BeTrue())
				return "xid", nil
			})
			Expect(r.prepareTargetsForScaleDown(ctx, ais, 3)).To(Succeed())
		})

		It("refuses to decommission a target in maintenance without the post-rebalance flag", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			err := r.prepareTargetsForScaleDown(ctx, ais, 3)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("post-rebalance"))
		})

		It("skips a target that is already decommissioning", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeDecomm}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			Expect(r.prepareTargetsForScaleDown(ctx, ais, 3)).To(Succeed())
		})

		It("skips a target that has both maintenance and decommission flags", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint | aismeta.SnodeDecomm}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			apiClient.EXPECT().GetClusterMap().Return(smap, nil)
			Expect(r.prepareTargetsForScaleDown(ctx, ais, 3)).To(Succeed())
		})

		Context("when scaleDownMode is retain", func() {
			BeforeEach(func() {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeRetain
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			})

			It("puts targets in maintenance with SkipRebalance=true", func() {
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

				err := r.prepareTargetsForScaleDown(ctx, ais, 3)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
