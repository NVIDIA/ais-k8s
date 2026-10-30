/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aistore

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/cmn/cos"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/aistore/target"
	mocks "github.com/ais-operator/internal/services/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

var _ = Describe("target removal wait reasons", func() {
	var (
		startingPod      = &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(false)}}}}
		startedPod       = &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}}}}
		readyPod         = &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}, ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}}}}
		unschedulablePod = &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable}}}}
		crashLoopPod     = &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}}
		decommissioned   = metav1.ObjectMeta{Annotations: map[string]string{decommissionedAnnotation: "t3"}}
		decommStartedPod = &corev1.Pod{ObjectMeta: decommissioned, Status: startedPod.Status}
		decommCrashPod   = &corev1.Pod{ObjectMeta: decommissioned, Status: crashLoopPod.Status}
	)

	DescribeTable("memberWaitReason",
		func(mode aisv1.ScaleDownMode, flags cos.BitFlags, pod *corev1.Pod, wait bool) {
			ais := &aisv1.AIStore{Spec: aisv1.AIStoreSpec{TargetSpec: aisv1.TargetSpec{ScaleDownMode: mode}}}
			node := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, Flags: flags}
			Expect(memberWaitReason(ais, node, pod) != "").To(Equal(wait))
		},
		Entry("ready pod", aisv1.ScaleDownModeDecommission, cos.BitFlags(0), readyPod, false),
		Entry("starting pod", aisv1.ScaleDownModeDecommission, cos.BitFlags(0), startingPod, true),
		Entry("started but unready pod", aisv1.ScaleDownModeDecommission, cos.BitFlags(0), startedPod, true),
		Entry("unschedulable pod", aisv1.ScaleDownModeDecommission, cos.BitFlags(0), unschedulablePod, false),
		Entry("CrashLoopBackOff pod", aisv1.ScaleDownModeDecommission, cos.BitFlags(0), crashLoopPod, false),
		Entry("maintenance without post-rebalance", aisv1.ScaleDownModeDecommission, aismeta.SnodeMaint, readyPod, true),
		Entry("maintenance with post-rebalance and starting pod", aisv1.ScaleDownModeDecommission, aismeta.SnodeMaint|aismeta.SnodeMaintPostReb, startingPod, false),
		Entry("retain, maintenance with starting pod", aisv1.ScaleDownModeRetain, aismeta.SnodeMaint, startingPod, false),
		Entry("retain, not in maintenance with starting pod", aisv1.ScaleDownModeRetain, cos.BitFlags(0), startingPod, true),
		Entry("discard, maintenance without post-rebalance", aisv1.ScaleDownModeDiscard, aismeta.SnodeMaint, readyPod, false),
		Entry("discard, not in maintenance with starting pod", aisv1.ScaleDownModeDiscard, cos.BitFlags(0), startingPod, true),
	)

	DescribeTable("nonMemberWaitReason",
		func(mode aisv1.ScaleDownMode, pod *corev1.Pod, wait bool) {
			ais := &aisv1.AIStore{Spec: aisv1.AIStoreSpec{TargetSpec: aisv1.TargetSpec{ScaleDownMode: mode}}}
			Expect(nonMemberWaitReason(ais, pod) != "").To(Equal(wait))
		},
		Entry("starting pod", aisv1.ScaleDownModeDecommission, startingPod, true),
		Entry("started pod not yet in the cluster map", aisv1.ScaleDownModeDecommission, startedPod, true),
		Entry("unschedulable pod", aisv1.ScaleDownModeDecommission, unschedulablePod, true),
		Entry("CrashLoopBackOff pod", aisv1.ScaleDownModeSafeDecommission, crashLoopPod, true),
		Entry("decommissioned started pod", aisv1.ScaleDownModeDecommission, decommStartedPod, false),
		Entry("decommissioned CrashLoopBackOff pod", aisv1.ScaleDownModeSafeDecommission, decommCrashPod, false),
		Entry("retain, unschedulable pod", aisv1.ScaleDownModeRetain, unschedulablePod, false),
		Entry("retain, CrashLoopBackOff pod", aisv1.ScaleDownModeRetain, crashLoopPod, false),
		Entry("retain, started pod not yet in the cluster map", aisv1.ScaleDownModeRetain, startedPod, true),
		Entry("discard, CrashLoopBackOff pod", aisv1.ScaleDownModeDiscard, crashLoopPod, false),
		Entry("discard, started pod not yet in the cluster map", aisv1.ScaleDownModeDiscard, startedPod, true),
		Entry("discard, decommissioned started pod", aisv1.ScaleDownModeDiscard, decommStartedPod, false),
	)
})

var _ = Describe("scaleDownMode", func() {
	var (
		r         *Reconciler
		ais       *aisv1.AIStore
		mockCtrl  *gomock.Controller
		apiClient *mocks.MockAIStoreClientInterface
		recorder  *events.FakeRecorder
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
		recorder = events.NewFakeRecorder(8)
		r = NewReconciler(tmpClient, recorder, ctrl.Log, clientManager)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("outgoingTargetsSettled", func() {
		It("waits when the outgoing pod is missing", func() {
			settled, err := r.outgoingTargetsSettled(ctx, ais, &aismeta.Smap{Tmap: aismeta.NodeMap{}}, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(settled).To(BeFalse())
		})

		It("proceeds when the outgoing target is a member with a ready pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-target-2", Namespace: namespace},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}

			settled, err := r.outgoingTargetsSettled(ctx, ais, &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(settled).To(BeTrue())
		})

		DescribeTable("for an unschedulable target that left the cluster map without a decommission",
			func(pvcPhases []corev1.PersistentVolumeClaimPhase, boundStatePVC, hostPath, wantSettled bool, wantEvent string) {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "ais-target-2", Namespace: namespace},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}},
					},
				}
				addPVC := func(volumeName string, phase corev1.PersistentVolumeClaimPhase) {
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{Name: volumeName + "-ais-target-2", Namespace: namespace},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
						},
					}
					Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
					pvc.Status.Phase = phase
					Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())
					pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
						Name:         volumeName,
						VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc.Name}},
					})
				}
				for i, phase := range pvcPhases {
					addPVC(fmt.Sprintf("data%d", i), phase)
				}
				if boundStatePVC {
					addPVC(cmn.StatePVCName(ais), corev1.ClaimBound)
				}
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
				pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable}}
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				ais.Spec.TargetSpec.Mounts[0].UseHostPath = &hostPath

				settled, err := r.outgoingTargetsSettled(ctx, ais, &aismeta.Smap{Tmap: aismeta.NodeMap{}}, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(settled).To(Equal(wantSettled))
				if wantEvent == "" {
					Expect(recorder.Events).To(BeEmpty())
				} else {
					Expect(recorder.Events).To(Receive(ContainSubstring("Warning " + wantEvent)))
				}
			},
			Entry("waits with a bound PVC", []corev1.PersistentVolumeClaimPhase{corev1.ClaimBound}, false, false, false, EventReasonWaiting),
			Entry("waits with a bound PVC beside a pending PVC",
				[]corev1.PersistentVolumeClaimPhase{corev1.ClaimPending, corev1.ClaimBound}, false, false, false, EventReasonWaiting),
			Entry("proceeds with a PVC that never bound", []corev1.PersistentVolumeClaimPhase{corev1.ClaimPending}, false, false, true, ""),
			Entry("proceeds with a warning on hostPath mounts", nil, false, true, true, EventReasonUnconfirmedRemoval),
			Entry("proceeds with a warning on hostPath mounts beside a bound state PVC", nil, true, true, true, EventReasonUnconfirmedRemoval),
		)
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

				// 2 targets in smap but specReplicas is 3, so safe to scale down by 1.
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

		DescribeTable("with one outgoing target",
			func(mode aisv1.ScaleDownMode, rebalanceEnabled *bool, flags cos.BitFlags, wantMaintenance bool, wantRmUserData *bool) {
				ais.Spec.TargetSpec.ScaleDownMode = mode
				if rebalanceEnabled != nil {
					ais.Spec.ConfigToUpdate = &aisv1.ConfigToUpdate{}
					ais.Spec.ConfigToUpdate.UpdateRebalanceEnabled(rebalanceEnabled)
				}
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())

				apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: flags}
				smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
				if wantMaintenance || wantRmUserData != nil {
					apiClient.EXPECT().IsRebalancing().Return(false, nil)
				}
				if wantMaintenance {
					apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(act.DaemonID).To(Equal("t3"))
						Expect(act.SkipRebalance).To(BeFalse())
						return "xid", nil
					})
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "ais-target-2", Namespace: namespace},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}}},
				}
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
				if wantRmUserData != nil {
					apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(act.DaemonID).To(Equal("t3"))
						Expect(act.RmUserData).To(Equal(*wantRmUserData))
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: namespace}, pod)).To(Succeed())
						Expect(pod.Annotations).To(HaveKeyWithValue(decommissionedAnnotation, "t3"))
						return "xid", nil
					})
				}
				Expect(r.prepareTargetsForScaleDown(ctx, ais, smap, 3)).To(Succeed())
				if wantRmUserData == nil {
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: namespace}, pod)).To(Succeed())
					Expect(pod.Annotations).NotTo(HaveKey(decommissionedAnnotation))
				}
				if !wantMaintenance && wantRmUserData == nil {
					Expect(recorder.Events).To(Receive(ContainSubstring("Warning " + EventReasonWaiting)))
				} else {
					Expect(recorder.Events).To(BeEmpty())
				}
			},
			Entry("decommission mode puts an active target in maintenance with rebalance",
				aisv1.ScaleDownModeDecommission, nil, cos.BitFlags(0), true, nil),
			Entry("safe_decommission mode puts an active target in maintenance with rebalance",
				aisv1.ScaleDownModeSafeDecommission, nil, cos.BitFlags(0), true, nil),
			Entry("defers an active target while the spec disables rebalance",
				aisv1.ScaleDownModeSafeDecommission, apc.Ptr(false), cos.BitFlags(0), false, nil),
			Entry("decommission mode decommissions a target that finished maintenance rebalance with RmUserData=true",
				aisv1.ScaleDownModeDecommission, nil, aismeta.SnodeMaint|aismeta.SnodeMaintPostReb, false, apc.Ptr(true)),
			Entry("safe_decommission mode decommissions a target that finished maintenance rebalance with RmUserData=false",
				aisv1.ScaleDownModeSafeDecommission, nil, aismeta.SnodeMaint|aismeta.SnodeMaintPostReb, false, apc.Ptr(false)),
			Entry("decommissions a target that finished maintenance rebalance while the spec disables rebalance",
				aisv1.ScaleDownModeSafeDecommission, apc.Ptr(false), aismeta.SnodeMaint|aismeta.SnodeMaintPostReb, false, apc.Ptr(false)),
		)

		It("refuses to decommission a target in maintenance without the post-rebalance flag", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())

			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint}
			smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
			err := r.prepareTargetsForScaleDown(ctx, ais, smap, 3)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("post-rebalance"))
		})

		errUnavailable := errors.New("unavailable")
		DescribeTable("with two outgoing targets",
			func(highestFlags cos.BitFlags, rebalancing *bool, rebalancingErr error, wantMaintenance string) {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
				apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
				t4 := &aismeta.Snode{DaeID: "t4", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-3"}, Flags: highestFlags}
				primary := &aismeta.Snode{DaeID: "p1", DaeType: apc.Proxy}
				smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3, "t4": t4}, Primary: primary}
				if rebalancing != nil {
					apiClient.EXPECT().IsRebalancing().Return(*rebalancing, rebalancingErr)
				}
				if wantMaintenance != "" {
					apiClient.EXPECT().GetSoftwareVersion(primary).Return("4.4", nil)
					apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(act.DaemonID).To(Equal(wantMaintenance))
						return "xid", nil
					})
				}
				err := r.prepareTargetsForScaleDown(ctx, ais, smap, 4)
				if rebalancingErr != nil {
					Expect(err).To(MatchError(rebalancingErr))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("puts only the highest target in maintenance before AIS v5.0", cos.BitFlags(0), apc.Ptr(false), nil, "t4"),
			Entry("defers while the highest target is still decommissioning", aismeta.SnodeDecomm, nil, nil, ""),
			Entry("defers while the highest target has maintenance and decommission flags", aismeta.SnodeMaint|aismeta.SnodeDecomm, nil, nil, ""),
			Entry("defers while a rebalance is running", cos.BitFlags(0), apc.Ptr(true), nil, ""),
			Entry("returns the error when the rebalance status is unavailable", cos.BitFlags(0), apc.Ptr(false), errUnavailable, ""),
		)

		postReb := aismeta.SnodeMaint | aismeta.SnodeMaintPostReb
		DescribeTable("with two outgoing targets and a reachable primary",
			func(version string, versionErr error, t3Flags, t4Flags cos.BitFlags, wantMaintenance, wantDecommission []string, wantErr string) {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDecommission
				Expect(k8sClient.Update(ctx, ais)).To(Succeed())
				apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
				primary := &aismeta.Snode{DaeID: "p1", DaeType: apc.Proxy}
				t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: t3Flags}
				t4 := &aismeta.Snode{DaeID: "t4", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-3"}, Flags: t4Flags}
				smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3, "t4": t4}, Primary: primary}
				for _, name := range []string{"ais-target-2", "ais-target-3"} {
					Expect(k8sClient.Create(ctx, &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}}},
					})).To(Succeed())
				}
				if wantErr == "" {
					apiClient.EXPECT().IsRebalancing().Return(false, nil)
					apiClient.EXPECT().GetSoftwareVersion(primary).Return(version, versionErr)
				}
				// AIS before v5.0 reads only DaemonID, so one node must not use DaemonIDs.
				removedIDs := func(act *apc.ActValRmNode) []string {
					if act.DaemonID != "" {
						Expect(act.DaemonIDs).To(BeEmpty())
						return []string{act.DaemonID}
					}
					Expect(len(act.DaemonIDs)).To(BeNumerically(">", 1))
					return act.DaemonIDs
				}
				if wantMaintenance != nil {
					apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(removedIDs(act)).To(ConsistOf(wantMaintenance))
						Expect(act.SkipRebalance).To(BeFalse())
						return "xid", nil
					})
				}
				if wantDecommission != nil {
					apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(removedIDs(act)).To(ConsistOf(wantDecommission))
						Expect(act.RmUserData).To(BeTrue())
						for name, id := range map[string]string{"ais-target-2": "t3", "ais-target-3": "t4"} {
							pod := &corev1.Pod{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pod)).To(Succeed())
							Expect(pod.Annotations).To(HaveKeyWithValue(decommissionedAnnotation, id))
						}
						return "xid", nil
					})
				}
				err := r.prepareTargetsForScaleDown(ctx, ais, smap, 4)
				if wantErr != "" {
					Expect(err).To(MatchError(ContainSubstring(wantErr)))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("puts every active target in maintenance in one request on AIS v5.0+",
				"5.0.abc", nil, cos.BitFlags(0), cos.BitFlags(0), []string{"t3", "t4"}, nil, ""),
			Entry("puts only the active target in maintenance on AIS v5.0+",
				"5.1", nil, cos.BitFlags(0), postReb, []string{"t3"}, nil, ""),
			Entry("decommissions every target in one request once all finish maintenance rebalance on AIS v5.0+",
				"5.0", nil, postReb, postReb, nil, []string{"t3", "t4"}, ""),
			Entry("refuses to decommission while a target in maintenance lacks the post-rebalance flag on AIS v5.0+",
				"5.0", nil, aismeta.SnodeMaint, postReb, nil, nil, "post-rebalance"),
			Entry("puts only the highest target in maintenance when the primary version is unavailable",
				"", errors.New("unavailable"), cos.BitFlags(0), cos.BitFlags(0), []string{"t4"}, nil, ""),
		)

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

				apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
					Expect(act.SkipRebalance).To(BeTrue())
					Expect(act.DaemonID).To(Equal("t3"))
					return "xid", nil
				})

				err := r.prepareTargetsForScaleDown(ctx, ais, smap, 3)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when scaleDownMode is discard", func() {
			BeforeEach(func() {
				ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeDiscard
			})

			DescribeTable("decommissions the target without a rebalance",
				func(rebalanceEnabled *bool, flags cos.BitFlags) {
					if rebalanceEnabled != nil {
						ais.Spec.ConfigToUpdate = &aisv1.ConfigToUpdate{}
						ais.Spec.ConfigToUpdate.UpdateRebalanceEnabled(rebalanceEnabled)
					}
					Expect(k8sClient.Update(ctx, ais)).To(Succeed())

					t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: flags}
					smap := &aismeta.Smap{Tmap: aismeta.NodeMap{"t3": t3}}
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "ais-target-2", Namespace: namespace},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}}},
					}
					Expect(k8sClient.Create(ctx, pod)).To(Succeed())
					apiClient.EXPECT().IsRebalancing().Return(false, nil)
					apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
						Expect(act.DaemonID).To(Equal("t3"))
						Expect(act.SkipRebalance).To(BeTrue())
						Expect(act.RmUserData).To(BeTrue())
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: namespace}, pod)).To(Succeed())
						Expect(pod.Annotations).To(HaveKeyWithValue(decommissionedAnnotation, "t3"))
						return "xid", nil
					})

					Expect(r.prepareTargetsForScaleDown(ctx, ais, smap, 3)).To(Succeed())
					Expect(recorder.Events).To(BeEmpty())
				},
				Entry("for an active target", nil, cos.BitFlags(0)),
				Entry("while the spec disables rebalance", apc.Ptr(false), cos.BitFlags(0)),
				Entry("for a target in maintenance without post-rebalance", nil, aismeta.SnodeMaint),
			)
		})
	})

	Describe("scaleDownTargets", func() {
		var ss *appsv1.StatefulSet

		readyStatus := func() corev1.PodStatus {
			return corev1.PodStatus{
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}},
			}
		}
		unreadyStatus := func() corev1.PodStatus {
			return corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}},
			}
		}
		crashLoopStatus := func() corev1.PodStatus {
			return corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  cmn.AISContainerName,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			}
		}
		setTargetPodStatus := func(idx int32, status corev1.PodStatus) {
			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: target.PodName(ais, idx), Namespace: namespace}, pod)).To(Succeed())
			pod.Status = status
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		}
		createTargetPod := func(idx int32, status corev1.PodStatus) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: target.PodName(ais, idx), Namespace: namespace, Labels: target.SelectorLabels(ais)},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}}},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			setTargetPodStatus(idx, status)
		}
		markTargetPodDecommissioned := func(idx int32, nodeID string) {
			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: target.PodName(ais, idx), Namespace: namespace}, pod)).To(Succeed())
			pod.Annotations = map[string]string{decommissionedAnnotation: nodeID}
			Expect(k8sClient.Update(ctx, pod)).To(Succeed())
		}

		BeforeEach(func() {
			ais.Spec.TargetSpec.Size = apc.Ptr[int32](-1)
			ais.Spec.TargetSpec.AutoScaleConf = &aisv1.AutoScaleConf{
				SizeLimit:      apc.Ptr[int32](3),
				MaxUnavailable: apc.Ptr[int32](1),
			}
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			ais.Status.AutoScaleStatus.ExpectedTargetNodes = []string{"n0", "n1"}

			ss = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: ais.TargetStatefulSetName(), Namespace: namespace},
				Spec: appsv1.StatefulSetSpec{
					Replicas:    apc.Ptr[int32](3),
					ServiceName: ais.TargetStatefulSetName(),
					Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ais"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ais"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "aisnode", Image: "node:latest"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ss)).To(Succeed())
			ss.Status.Replicas = 3
			ss.Status.ReadyReplicas = 2
			ss.Status.ObservedGeneration = ss.Generation
			Expect(k8sClient.Status().Update(ctx, ss)).To(Succeed())
			Expect(k8sClient.Get(ctx, target.StatefulSetNSName(ais), ss)).To(Succeed())

			createTargetPod(0, readyStatus())
			createTargetPod(1, readyStatus())
		})

		It("shrinks the StatefulSet once outgoing targets have left the cluster map", func() {
			ais.SetCondition(aisv1.ConditionReadyRebalance)
			Expect(k8sClient.Status().Update(ctx, ais)).To(Succeed())
			ais.Status.AutoScaleStatus.ExpectedTargetNodes = []string{"n0", "n1"}
			createTargetPod(2, crashLoopStatus())
			markTargetPodDecommissioned(2, "t3")

			t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
			t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
			apiClient.EXPECT().GetClusterMap().Return(&aismeta.Smap{Tmap: aismeta.NodeMap{"t1": t1, "t2": t2}}, nil).Times(2)
			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)

			result, err := r.scaleDownTargets(ctx, ais, ss)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			updated := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, target.StatefulSetNSName(ais), updated)).To(Succeed())
			Expect(*updated.Spec.Replicas).To(Equal(int32(2)))
		})

		It("does not start decommission when unavailable targets that the StatefulSet keeps are within maxUnavailable", func() {
			setTargetPodStatus(1, unreadyStatus())
			createTargetPod(2, readyStatus())

			t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
			t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
			apiClient.EXPECT().GetClusterMap().Return(&aismeta.Smap{Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3}}, nil)

			result, err := r.scaleDownTargets(ctx, ais, ss)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsZero()).To(BeTrue())

			updated := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, target.StatefulSetNSName(ais), updated)).To(Succeed())
			Expect(*updated.Spec.Replicas).To(Equal(int32(3)))
		})

		It("puts the next outgoing target in maintenance when only outgoing targets are unavailable", func() {
			ais.SetCondition(aisv1.ConditionReadyRebalance)
			Expect(k8sClient.Status().Update(ctx, ais)).To(Succeed())
			ais.Status.AutoScaleStatus.ExpectedTargetNodes = []string{"n0", "n1"}

			// The scale-down goes from 4 to 2 targets. ais-target-3 left the cluster map, and ais-target-2 is next.
			ss.Spec.Replicas = apc.Ptr[int32](4)
			Expect(k8sClient.Update(ctx, ss)).To(Succeed())
			ss.Status.Replicas = 4
			ss.Status.ReadyReplicas = 3
			ss.Status.ObservedGeneration = ss.Generation
			Expect(k8sClient.Status().Update(ctx, ss)).To(Succeed())
			Expect(k8sClient.Get(ctx, target.StatefulSetNSName(ais), ss)).To(Succeed())
			createTargetPod(2, readyStatus())
			createTargetPod(3, crashLoopStatus())
			markTargetPodDecommissioned(3, "t4")

			t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
			t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
			apiClient.EXPECT().GetClusterMap().Return(&aismeta.Smap{Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3}}, nil).Times(2)
			apiClient.EXPECT().SetClusterConfigUsingMsg(gomock.Any()).Return(nil)
			apiClient.EXPECT().IsRebalancing().Return(false, nil)
			apiClient.EXPECT().StartMaintenance(gomock.Any()).DoAndReturn(func(act *apc.ActValRmNode) (string, error) {
				Expect(act.DaemonID).To(Equal("t3"))
				return "xid", nil
			})

			result, err := r.scaleDownTargets(ctx, ais, ss)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())
		})

		It("shrinks in the same reconcile once retain-mode removal starts", func() {
			ais.Spec.TargetSpec.ScaleDownMode = aisv1.ScaleDownModeRetain
			Expect(k8sClient.Update(ctx, ais)).To(Succeed())
			ais.Status.AutoScaleStatus.ExpectedTargetNodes = []string{"n0", "n1"}
			ss.Status.ReadyReplicas = 3
			Expect(k8sClient.Status().Update(ctx, ss)).To(Succeed())

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ais-target-2", Namespace: namespace},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: cmn.AISContainerName, Image: "aisnode:test"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: cmn.AISContainerName, Started: apc.Ptr(true)}}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			t1 := &aismeta.Snode{DaeID: "t1", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-0"}}
			t2 := &aismeta.Snode{DaeID: "t2", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-1"}}
			t3 := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}}
			t3Maint := &aismeta.Snode{DaeID: "t3", DaeType: apc.Target, ControlNet: aismeta.NetInfo{Hostname: "ais-target-2"}, Flags: aismeta.SnodeMaint}
			gomock.InOrder(
				apiClient.EXPECT().GetClusterMap().Return(&aismeta.Smap{Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3}}, nil),
				apiClient.EXPECT().StartMaintenance(gomock.Any()).Return("xid", nil),
				apiClient.EXPECT().GetClusterMap().Return(&aismeta.Smap{Tmap: aismeta.NodeMap{"t1": t1, "t2": t2, "t3": t3Maint}}, nil),
			)

			_, err := r.scaleDownTargets(ctx, ais, ss)
			Expect(err).NotTo(HaveOccurred())

			updated := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, target.StatefulSetNSName(ais), updated)).To(Succeed())
			Expect(*updated.Spec.Replicas).To(Equal(int32(2)))
		})
	})

	Describe("isTargetScalingNeeded", func() {
		It("is true for an autoscale shrink even when unavailable replicas are within maxUnavailable", func() {
			ais.Spec.TargetSpec.Size = apc.Ptr[int32](-1)
			ais.Spec.TargetSpec.AutoScaleConf = &aisv1.AutoScaleConf{
				SizeLimit:      apc.Ptr[int32](3),
				MaxUnavailable: apc.Ptr[int32](1),
			}
			ais.Status.AutoScaleStatus.ExpectedTargetNodes = []string{"n0", "n1"}
			ss := &appsv1.StatefulSet{
				Spec:   appsv1.StatefulSetSpec{Replicas: apc.Ptr[int32](3)},
				Status: appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 2},
			}
			Expect(statefulsetScalingNeeded(ss, ais.GetTargetSize(), ais.GetTargetMaxUnavailable(), true)).To(BeFalse())
			Expect(isTargetScalingNeeded(ais, ss)).To(BeTrue())
		})
	})
})

var _ = Describe("handleTargetState", func() {
	var (
		r         *Reconciler
		ais       *aisv1.AIStore
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
				ProxySpec: aisv1.DaemonSpec{Size: apc.Ptr[int32](1)},
				TargetSpec: aisv1.TargetSpec{
					DaemonSpec: aisv1.DaemonSpec{
						Size: apc.Ptr[int32](1),
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

		mockCtrl := gomock.NewController(GinkgoT())
		clientManager := mocks.NewMockAISClientManagerInterface(mockCtrl)
		r = NewReconciler(aisclient.NewClient(k8sClient, k8sClient.Scheme()), events.NewFakeRecorder(8), ctrl.Log, clientManager)
	})

	It("should switch a RollingUpdate target statefulset to OnDelete before it updates the pod template", func() {
		reconcileTarget(ctx, ais, r)
		oldImg := ais.Spec.NodeImage

		By("Set the target statefulset to RollingUpdate")
		ss := getStatefulSet(ctx, ais, k8sClient, "ais-target")
		ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType}
		Expect(k8sClient.Update(ctx, &ss)).To(Succeed())
		ss.Status.Replicas = *ss.Spec.Replicas
		ss.Status.ObservedGeneration = ss.Generation
		Expect(k8sClient.Status().Update(ctx, &ss)).To(Succeed())

		By("Update node image in spec and reconcile targets")
		ais.Spec.NodeImage = "testNodeImage"
		Expect(k8sClient.Update(ctx, ais)).To(Succeed())
		reconcileTarget(ctx, ais, r)

		By("Expect the update strategy to change before the pod template")
		ss = getStatefulSet(ctx, ais, k8sClient, "ais-target")
		Expect(ss.Spec.UpdateStrategy.Type).To(Equal(appsv1.OnDeleteStatefulSetStrategyType))
		Expect(ss.Spec.Template.Spec.Containers[0].Image).To(Equal(oldImg))
	})
})
