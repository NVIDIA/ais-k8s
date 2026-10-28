/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aistore

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/aistore/api/apc"
	aismeta "github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	aisclient "github.com/ais-operator/internal/client"
	"github.com/ais-operator/internal/resources/aistore/proxy"
	mocks "github.com/ais-operator/internal/services/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	currentRevision = "rev-1"
	updateRevision  = "rev-2"
)

// scaleDownSize is the proxy count before scale down, so pods 0-2 exist and pod 2 is removed.
const scaleDownSize = 3

// settledSS returns a proxy StatefulSet with no rollout in progress.
func settledSS() *appsv1.StatefulSet {
	return makeSS(scaleDownSize, scaleDownSize, scaleDownSize, scaleDownSize, currentRevision, currentRevision, appsv1.OnDeleteStatefulSetStrategyType)
}

// rollingSS returns a proxy StatefulSet mid-rollout to updateRevision.
func rollingSS() *appsv1.StatefulSet {
	return makeSS(scaleDownSize, scaleDownSize, scaleDownSize-1, scaleDownSize, currentRevision, updateRevision, appsv1.OnDeleteStatefulSetStrategyType)
}

// unsizedAIS returns an AIStore with no desired proxy count, for helpers that are handed the
// current and desired sizes explicitly and only use the AIStore to derive pod names.
func unsizedAIS() *aisv1.AIStore {
	return &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "ais", Namespace: "ais-test"}}
}

// proxyAIS returns an AIStore whose desired proxy count, as reported by GetProxySize, is size.
func proxyAIS(size int32) *aisv1.AIStore {
	ais := unsizedAIS()
	ais.Spec.ProxySpec.Size = apc.Ptr(size)
	return ais
}

// proxySmap builds a cluster map holding proxies for pod indices [0, size), with primaryIdx as primary.
// Node IDs follow the "p<idx>" convention so assertions can name the expected daemon directly.
func proxySmap(ais *aisv1.AIStore, size, primaryIdx int32) *aismeta.Smap {
	smap := &aismeta.Smap{Pmap: aismeta.NodeMap{}}
	for idx := range size {
		node := &aismeta.Snode{
			DaeID:      fmt.Sprintf("p%d", idx),
			DaeType:    apc.Proxy,
			ControlNet: aismeta.NetInfo{Hostname: proxy.PodName(ais, idx)},
		}
		smap.Pmap[node.ID()] = node
		if idx == primaryIdx {
			smap.Primary = node
		}
	}
	return smap
}

// unknownPrimarySmap builds a cluster map whose primary matches none of the cluster's pods.
func unknownPrimarySmap(ais *aisv1.AIStore, size int32) *aismeta.Smap {
	smap := proxySmap(ais, size, -1)
	smap.Primary = &aismeta.Snode{DaeID: "px", ControlNet: aismeta.NetInfo{Hostname: "other-proxy-0"}}
	return smap
}

// expectDecommission expects one DecommissionNode call for daemonID and makes it return retErr.
// Wrap several in gomock.InOrder to pin down the order proxies are removed in.
func expectDecommission(apiClient *mocks.MockAIStoreClientInterface, daemonID string, retErr error) *gomock.Call {
	return apiClient.EXPECT().DecommissionNode(gomock.Any()).DoAndReturn(
		func(act *apc.ActValRmNode) (string, error) {
			Expect(act.DaemonID).To(Equal(daemonID))
			return "xid", retErr
		})
}

var _ = Describe("findPrimaryPodIdx", func() {
	ais := unsizedAIS()

	It("returns the pod index of the primary proxy", func() {
		idx, found := findPrimaryPodIdx(ais, proxySmap(ais, 3, 2), 3)
		Expect(found).To(BeTrue())
		Expect(idx).To(Equal(int32(2)))
	})

	It("reports not found when primary is nil", func() {
		_, found := findPrimaryPodIdx(ais, proxySmap(ais, 3, -1), 3)
		Expect(found).To(BeFalse())
	})

	It("reports not found when the primary is not one of the cluster's pods", func() {
		_, found := findPrimaryPodIdx(ais, unknownPrimarySmap(ais, 3), 3)
		Expect(found).To(BeFalse())
	})

	It("only searches pod indices below the current size", func() {
		_, found := findPrimaryPodIdx(ais, proxySmap(ais, 3, 2), 2)
		Expect(found).To(BeFalse())
	})
})

var _ = Describe("decommissionProxies", func() {
	var (
		ais       *aisv1.AIStore
		mockCtrl  *gomock.Controller
		apiClient *mocks.MockAIStoreClientInterface
		ctx       = context.TODO()
	)

	BeforeEach(func() {
		ais = unsizedAIS()
		mockCtrl = gomock.NewController(GinkgoT())
		apiClient = mocks.NewMockAIStoreClientInterface(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("decommissions only the pods being removed, highest index first", func() {
		gomock.InOrder(
			expectDecommission(apiClient, "p3", nil),
			expectDecommission(apiClient, "p2", nil),
		)

		decommissionProxies(ctx, ais, proxySmap(ais, 4, 0), apiClient, 4, 2)
	})

	It("continues past cluster map misses and decommission failures", func() {
		smap := proxySmap(ais, 4, 0)
		delete(smap.Pmap, "p3") // proxy-3 is absent from the cluster map, so it is skipped
		gomock.InOrder(
			expectDecommission(apiClient, "p2", errors.New("decommission failed")),
			expectDecommission(apiClient, "p1", nil),
		)

		decommissionProxies(ctx, ais, smap, apiClient, 4, 1)
	})
})

var _ = Describe("prepareProxyScaleDown", func() {
	var (
		ais           *aisv1.AIStore
		mockCtrl      *gomock.Controller
		apiClient     *mocks.MockAIStoreClientInterface
		clientManager *mocks.MockAISClientManagerInterface
		ctx           = context.TODO()
	)

	BeforeEach(func() {
		// Scaling 3 proxies down to 2, so pod 2 is the one being removed.
		ais = proxyAIS(2)
		mockCtrl = gomock.NewController(GinkgoT())
		apiClient = mocks.NewMockAIStoreClientInterface(mockCtrl)
		clientManager = mocks.NewMockAISClientManagerInterface(mockCtrl)
		clientManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(apiClient, nil).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	// proxyPod builds a proxy pod at the given index, ready or not, on the given revision.
	proxyPod := func(idx int32, ready bool, revision string) *corev1.Pod {
		status := corev1.PodStatus{Phase: corev1.PodRunning}
		if ready {
			status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxy.PodName(ais, idx),
				Namespace: ais.Namespace,
				Labels:    map[string]string{appsv1.ControllerRevisionHashLabelKey: revision},
			},
			Status: status,
		}
	}

	// reconcilerWithPods returns a Reconciler backed by an in-memory client holding the given pods.
	reconcilerWithPods := func(pods ...*corev1.Pod) *Reconciler {
		objs := make([]client.Object, 0, len(pods))
		for _, pod := range pods {
			objs = append(objs, pod)
		}
		c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objs...).Build()
		return &Reconciler{
			k8sClient:     aisclient.NewClient(c, scheme.Scheme),
			clientManager: clientManager,
		}
	}

	// reconcilerWithReadyPods returns a Reconciler holding ready proxy pods at the given indices,
	// all on the current revision. Any other pod index has no pod at all.
	reconcilerWithReadyPods := func(idxs ...int32) *Reconciler {
		pods := make([]*corev1.Pod, 0, len(idxs))
		for _, idx := range idxs {
			pods = append(pods, proxyPod(idx, true, currentRevision))
		}
		return reconcilerWithPods(pods...)
	}

	It("leaves a surviving primary alone", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 0), nil)
		expectDecommission(apiClient, "p2", nil)

		// No SetPrimaryProxy expectation: the mock fails the test if the primary is reassigned.
		Expect(reconcilerWithReadyPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("reassigns a primary that is being removed to the lowest ready pod", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p0", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithReadyPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("skips pods that are not ready when choosing a new primary", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		// Proxy 0 is running but not ready, so we expect a call to set p1 as primary
		apiClient.EXPECT().SetPrimaryProxy("p1", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		r := reconcilerWithPods(
			proxyPod(0, false, currentRevision),
			proxyPod(1, true, currentRevision),
			proxyPod(2, true, currentRevision),
		)
		Expect(r.prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("skips pods on the outgoing revision while a rollout is in progress", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p1", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		r := reconcilerWithPods(
			proxyPod(0, true, currentRevision),
			proxyPod(1, true, updateRevision),
			proxyPod(2, true, updateRevision),
		)
		Expect(r.prepareProxyScaleDown(ctx, ais, rollingSS())).To(Succeed())
	})

	It("skips pods in maintenance or decommission when choosing a new primary", func() {
		smap := proxySmap(ais, 3, 2)
		smap.Pmap["p0"].Flags = aismeta.SnodeMaint
		apiClient.EXPECT().GetClusterMap().Return(smap, nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p1", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithReadyPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("tries the next eligible pod when SetPrimaryProxy fails", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p0", gomock.Any(), true).Return(errors.New("set primary failed"))
		apiClient.EXPECT().SetPrimaryProxy("p1", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithReadyPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("reassigns a primary that matches none of the cluster's pods", func() {
		apiClient.EXPECT().GetClusterMap().Return(unknownPrimarySmap(ais, 3), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p0", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithReadyPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, settledSS())).To(Succeed())
	})

	It("decommissions nothing when no pod can take over as primary", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil)

		err := reconcilerWithReadyPods(2).prepareProxyScaleDown(ctx, ais, settledSS())
		Expect(err).To(MatchError(ContainSubstring("no pod found to set as primary")))
	})
})

var _ = Describe("handleProxyRollout", func() {
	type podState int
	const (
		ready podState = iota
		notReady
		unschedulable
	)
	type podSpec struct {
		state    podState
		revision string
	}

	var (
		ais           = unsizedAIS()
		mockCtrl      *gomock.Controller
		apiClient     *mocks.MockAIStoreClientInterface
		clientManager *mocks.MockAISClientManagerInterface
		ctx           = context.TODO()
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		apiClient = mocks.NewMockAIStoreClientInterface(mockCtrl)
		clientManager = mocks.NewMockAISClientManagerInterface(mockCtrl)
		clientManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(apiClient, nil).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	// rolloutPod builds a proxy pod at the given index, selectable by the proxy labels.
	rolloutPod := func(idx int32, spec podSpec) *corev1.Pod {
		labels := proxy.SelectorLabels(ais)
		labels[appsv1.ControllerRevisionHashLabelKey] = spec.revision
		status := corev1.PodStatus{Phase: corev1.PodRunning}
		switch spec.state {
		case ready:
			status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		case unschedulable:
			status.Phase = corev1.PodPending
			status.Conditions = []corev1.PodCondition{{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable,
			}}
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: proxy.PodName(ais, idx), Namespace: ais.Namespace, Labels: labels},
			Status:     status,
		}
	}

	// rolloutSS builds an OnDelete proxy StatefulSet with a pending update to updateRevision.
	rolloutSS := func(size int32) *appsv1.StatefulSet {
		ss := makeSS(size, size, 0, size, currentRevision, updateRevision, appsv1.OnDeleteStatefulSetStrategyType)
		ss.Name = ais.ProxyStatefulSetName()
		ss.Spec.Selector = &metav1.LabelSelector{MatchLabels: proxy.SelectorLabels(ais)}
		return ss
	}

	// expectRemaining checks that the proxy pods left are exactly those at the given indices.
	expectRemaining := func(r *Reconciler, wantRemaining []int32) {
		podList, err := r.k8sClient.ListPods(ctx, ais, proxy.SelectorLabels(ais))
		Expect(err).NotTo(HaveOccurred())
		remaining := make([]string, 0, len(podList.Items))
		for i := range podList.Items {
			remaining = append(remaining, podList.Items[i].Name)
		}
		want := make([]string, 0, len(wantRemaining))
		for _, idx := range wantRemaining {
			want = append(want, proxy.PodName(ais, idx))
		}
		Expect(remaining).To(ConsistOf(want))
	}

	DescribeTable("replaces the next outdated proxy",
		func(primaryIdx int32, specs []podSpec, newPrimary string, wantRemaining []int32, wantWarning bool) {
			size := int32(len(specs))
			apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, size, primaryIdx), nil).AnyTimes()
			// Without this expectation, the mock fails the test if the primary is reassigned.
			if newPrimary != "" {
				apiClient.EXPECT().SetPrimaryProxy(newPrimary, gomock.Any(), true).Return(nil)
			}
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
			for idx, spec := range specs {
				builder = builder.WithObjects(rolloutPod(int32(idx), spec))
			}
			recorder := events.NewFakeRecorder(8)
			r := &Reconciler{k8sClient: aisclient.NewClient(builder.Build(), scheme.Scheme), clientManager: clientManager, recorder: recorder}
			ss := rolloutSS(size)

			Expect(r.handleProxyRollout(ctx, ais, ss)).To(Succeed())

			expectRemaining(r, wantRemaining)
			if wantWarning {
				Expect(recorder.Events).To(Receive(ContainSubstring("Warning " + EventReasonNoPrimaryCandidate)))
			} else {
				Expect(recorder.Events).To(BeEmpty())
			}
		},
		Entry("replaces the lowest outdated proxy",
			int32(2), []podSpec{{ready, currentRevision}, {ready, currentRevision}, {ready, currentRevision}},
			"", []int32{1, 2}, false),
		Entry("waits while an updated proxy is not ready",
			int32(2), []podSpec{{notReady, updateRevision}, {ready, currentRevision}, {ready, currentRevision}},
			"", []int32{0, 1, 2}, false),
		Entry("does not wait on an unschedulable proxy",
			int32(2), []podSpec{{ready, updateRevision}, {unschedulable, updateRevision}, {ready, currentRevision}},
			"p0", []int32{0, 1, 2}, false),
		Entry("hands off the primary to the lowest ready proxy when the rollout reaches the primary",
			int32(0), []podSpec{{ready, currentRevision}, {ready, currentRevision}, {ready, currentRevision}},
			"p1", []int32{0, 1, 2}, false),
		Entry("replaces the primary without a handoff when no other proxy is ready",
			int32(0), []podSpec{{ready, currentRevision}, {unschedulable, updateRevision}},
			"", []int32{1}, true),
		Entry("replaces a single proxy",
			int32(0), []podSpec{{ready, currentRevision}},
			"", []int32{}, false),
	)

	DescribeTable("replaces an unready proxy without calling the AIS API",
		func(specs []podSpec, wantRemaining []int32) {
			// No GetClusterMap expectation: the mock fails the test if the AIS API is called.
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
			for idx, spec := range specs {
				builder = builder.WithObjects(rolloutPod(int32(idx), spec))
			}
			r := &Reconciler{k8sClient: aisclient.NewClient(builder.Build(), scheme.Scheme), clientManager: clientManager}
			ss := rolloutSS(int32(len(specs)))

			Expect(r.handleProxyRollout(ctx, ais, ss)).To(Succeed())

			expectRemaining(r, wantRemaining)
		},
		Entry("unschedulable proxy",
			[]podSpec{{unschedulable, currentRevision}, {ready, currentRevision}}, []int32{1}),
		Entry("single crashing proxy",
			[]podSpec{{notReady, currentRevision}}, []int32{}),
	)
})

var _ = Describe("handleProxyState", func() {
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
				ProxySpec: aisv1.DaemonSpec{
					Size: apc.Ptr[int32](1),
					ServiceSpec: aisv1.ServiceSpec{
						PublicPort:       apc.Ptr(intstr.FromInt32(51080)),
						IntraControlPort: apc.Ptr(intstr.FromInt32(51082)),
						IntraDataPort:    apc.Ptr(intstr.FromInt32(51083)),
					},
				},
				TargetSpec: aisv1.TargetSpec{
					DaemonSpec: aisv1.DaemonSpec{Size: apc.Ptr[int32](1)},
					Mounts:     []aisv1.Mount{{Path: "/data"}},
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

	It("should switch a RollingUpdate proxy statefulset to OnDelete before it updates the pod template", func() {
		expectOnDeleteBeforeTemplateUpdate(ctx, ais, r, "ais-proxy", reconcileProxy)
	})
})
