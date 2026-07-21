/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("buildCertificateSANs", func() {
	It("generates sorted unique DNS names and IP addresses", func() {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: aisv1.AIStoreSpec{
				TLS: &aisv1.TLSSpec{
					Certificate: &aisv1.TLSCertificateConfig{
						AdditionalDNSNames: []string{"test-additional-dns-name", "test-additional-dns-name"},
					},
				},
				HostnameMap: map[string]string{
					"test-worker-1": "test-worker-1, 127.0.0.1",
					"test-worker-2": "test-worker-2, 127.0.0.2",
				},
			},
		}
		nodeNames := []string{
			"test-target-node-1", "test-target-node-2",
			"test-proxy-node-1", "test-proxy-node-2", "test-proxy-node-1", "127.0.0.1",
		}

		dnsNames, ipAddresses := buildCertificateSANs(ais, nodeNames)

		Expect(dnsNames).To(Equal([]string{
			"*.test-cluster-proxy.test-ns.svc.cluster.local",
			"*.test-cluster-target.test-ns.svc.cluster.local",
			"test-additional-dns-name",
			"test-cluster-proxy",
			"test-cluster-proxy.test-ns",
			"test-cluster-proxy.test-ns.svc.cluster.local",
			"test-cluster-target",
			"test-cluster-target.test-ns",
			"test-cluster-target.test-ns.svc.cluster.local",
			"test-proxy-node-1",
			"test-proxy-node-2",
			"test-target-node-1",
			"test-target-node-2",
			"test-worker-1",
			"test-worker-2",
		}))
		Expect(ipAddresses).To(Equal([]string{"127.0.0.1", "127.0.0.2"}))
	})
})
