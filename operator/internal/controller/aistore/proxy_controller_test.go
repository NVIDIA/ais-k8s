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
	"k8s.io/client-go/kubernetes/scheme"
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
	return makeSS(scaleDownSize, scaleDownSize, scaleDownSize, scaleDownSize, currentRevision, currentRevision, appsv1.RollingUpdateStatefulSetStrategyType)
}

// rollingSS returns a proxy StatefulSet mid-rollout to updateRevision.
func rollingSS() *appsv1.StatefulSet {
	return makeSS(scaleDownSize, scaleDownSize, scaleDownSize-1, scaleDownSize, currentRevision, updateRevision, appsv1.RollingUpdateStatefulSetStrategyType)
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
