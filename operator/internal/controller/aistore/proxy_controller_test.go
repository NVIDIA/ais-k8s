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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

	// reconcilerWithPods returns a Reconciler backed by an in-memory client holding proxy pods in
	// the given phases, keyed by pod index. Any index not listed has no pod at all.
	reconcilerWithPods := func(phases map[int32]corev1.PodPhase) *Reconciler {
		pods := make([]client.Object, 0, len(phases))
		for idx, phase := range phases {
			pods = append(pods, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: proxy.PodName(ais, idx), Namespace: ais.Namespace},
				Status:     corev1.PodStatus{Phase: phase},
			})
		}
		c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(pods...).Build()
		return &Reconciler{
			k8sClient:     aisclient.NewClient(c, scheme.Scheme),
			clientManager: clientManager,
		}
	}

	// reconcilerWithRunningPods returns a Reconciler holding running proxy pods at the given
	// indices. Any other pod index has no pod at all, so it looks unready to the reconciler.
	reconcilerWithRunningPods := func(idxs ...int32) *Reconciler {
		phases := make(map[int32]corev1.PodPhase, len(idxs))
		for _, idx := range idxs {
			phases[idx] = corev1.PodRunning
		}
		return reconcilerWithPods(phases)
	}

	It("leaves a surviving primary alone", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 0), nil)
		expectDecommission(apiClient, "p2", nil)

		// No SetPrimaryProxy expectation: the mock fails the test if the primary is reassigned.
		Expect(reconcilerWithRunningPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, 3)).To(Succeed())
	})

	It("reassigns a primary that is being removed to the lowest ready pod", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p0", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithRunningPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, 3)).To(Succeed())
	})

	It("skips pods that are not running when choosing a new primary", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil).AnyTimes()
		// Proxy 0 exists but has not started running, so we expect a call to set p1 as primary
		apiClient.EXPECT().SetPrimaryProxy("p1", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		r := reconcilerWithPods(map[int32]corev1.PodPhase{
			0: corev1.PodPending,
			1: corev1.PodRunning,
			2: corev1.PodRunning,
		})
		Expect(r.prepareProxyScaleDown(ctx, ais, 3)).To(Succeed())
	})

	It("reassigns a primary that matches none of the cluster's pods", func() {
		apiClient.EXPECT().GetClusterMap().Return(unknownPrimarySmap(ais, 3), nil).AnyTimes()
		apiClient.EXPECT().SetPrimaryProxy("p0", gomock.Any(), true).Return(nil)
		expectDecommission(apiClient, "p2", nil)

		Expect(reconcilerWithRunningPods(0, 1, 2).prepareProxyScaleDown(ctx, ais, 3)).To(Succeed())
	})

	It("decommissions nothing when no pod can take over as primary", func() {
		apiClient.EXPECT().GetClusterMap().Return(proxySmap(ais, 3, 2), nil)

		err := reconcilerWithRunningPods(2).prepareProxyScaleDown(ctx, ais, 3)
		Expect(err).To(MatchError(ContainSubstring("no pod found to set as primary")))
	})
})
