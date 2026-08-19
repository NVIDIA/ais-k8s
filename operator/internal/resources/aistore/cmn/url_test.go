/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/opinfo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	urlTestDomain       = "k8s.example.com"
	urlTestPublicPort   = 51080
	urlTestControlPort  = 51082
	urlTestScheme       = "http"
	urlTestSecureScheme = "https"
)

func newURLTestAIS() *aisv1.AIStore {
	return &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{Name: "ais", Namespace: "ais-ns"},
		Spec: aisv1.AIStoreSpec{
			ProxySpec: aisv1.DaemonSpec{
				ServiceSpec: aisv1.ServiceSpec{
					PublicPort:       intstr.FromInt32(urlTestPublicPort),
					IntraControlPort: intstr.FromInt32(urlTestControlPort),
				},
			},
		},
	}
}

var _ = Describe("ClusterDomain", Label("short"), func() {
	It("should fall back to the domain of the cluster the operator runs in", func() {
		Expect(ClusterDomain(newURLTestAIS())).To(Equal(opinfo.ClusterDomain()))
	})

	It("should prefer the domain from the spec", func() {
		ais := newURLTestAIS()
		ais.Spec.ClusterDomain = aisapc.Ptr(urlTestDomain)

		Expect(ClusterDomain(ais)).To(Equal(urlTestDomain))
	})
})

var _ = Describe("Proxy URLs", Label("short"), func() {
	var ais *aisv1.AIStore

	BeforeEach(func() {
		ais = newURLTestAIS()
		ais.Spec.ClusterDomain = aisapc.Ptr(urlTestDomain)
	})

	It("should address the primary proxy pod on the intra-control port", func() {
		Expect(DefaultProxyURL(ais)).To(Equal(fmt.Sprintf("%s://%s.%s.%s.svc.%s:%d",
			urlTestScheme, ais.DefaultPrimaryName(), ais.ProxyStatefulSetName(), ais.Namespace, urlTestDomain, urlTestControlPort)))
	})

	It("should address the proxy service on the intra-control port", func() {
		Expect(DiscoveryProxyURL(ais)).To(Equal(fmt.Sprintf("%s://%s.%s.svc.%s:%d",
			urlTestScheme, ais.ProxyStatefulSetName(), ais.Namespace, urlTestDomain, urlTestControlPort)))
	})

	It("should address the proxy service on the public port", func() {
		Expect(IntraClusterURL(ais)).To(Equal(fmt.Sprintf("%s://%s.%s.svc.%s:%d",
			urlTestScheme, ais.ProxyStatefulSetName(), ais.Namespace, urlTestDomain, urlTestPublicPort)))
	})

	It("should use https when the cluster does", func() {
		ais.Spec.ConfigToUpdate = &aisv1.ConfigToUpdate{
			Net: &aisv1.NetConfToUpdate{
				HTTP: &aisv1.HTTPConfToUpdate{UseHTTPS: aisapc.Ptr(true)},
			},
		}

		Expect(IntraClusterURL(ais)).To(HavePrefix(urlTestSecureScheme + "://"))
	})
})
