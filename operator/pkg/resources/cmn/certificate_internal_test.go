// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("buildCertificateSANs", func() {
	It("Should correctly generate DNS names and IP addresses", func() {
		ais := &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: aisv1.AIStoreSpec{
				TLSCertificate: &aisv1.TLSCertificateConfig{
					AdditionalDNSNames: []string{"test-additional-dns-name"},
				},
				HostnameMap: map[string]string{
					"test-worker-1": "test-worker-1, 127.0.0.1",
				},
			},
			Status: aisv1.AIStoreStatus{
				AutoScaleStatus: aisv1.AutoScaleStatus{
					ExpectedTargetNodes: []string{"test-target-node-1", "test-target-node-2"},
					ExpectedProxyNodes:  []string{"test-proxy-node-1", "test-proxy-node-2"},
				},
			},
		}

		dnsNames, ipAddresses := buildCertificateSANs(ais)

		Expect(dnsNames).To(ContainElement("test-cluster-proxy"))
		Expect(dnsNames).To(ContainElement("test-cluster-proxy.test-ns"))
		Expect(dnsNames).To(ContainElement("test-cluster-proxy.test-ns.svc.cluster.local"))
		Expect(dnsNames).To(ContainElement("test-cluster-target"))
		Expect(dnsNames).To(ContainElement("test-cluster-target.test-ns"))
		Expect(dnsNames).To(ContainElement("test-cluster-target.test-ns.svc.cluster.local"))
		Expect(dnsNames).To(ContainElement("*.test-cluster-proxy.test-ns.svc.cluster.local"))
		Expect(dnsNames).To(ContainElement("*.test-cluster-target.test-ns.svc.cluster.local"))
		Expect(dnsNames).To(ContainElement("test-additional-dns-name"))
		Expect(dnsNames).To(ContainElement("test-worker-1"))
		Expect(dnsNames).To(ContainElement("test-target-node-1"))
		Expect(dnsNames).To(ContainElement("test-target-node-2"))
		Expect(dnsNames).To(ContainElement("test-proxy-node-1"))
		Expect(dnsNames).To(ContainElement("test-proxy-node-2"))
		Expect(ipAddresses).To(ContainElement("127.0.0.1"))
	})
})
