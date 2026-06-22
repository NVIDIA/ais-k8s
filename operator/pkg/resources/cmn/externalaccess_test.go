// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"testing"

	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func TestLoadBalancerIngressReady(t *testing.T) {
	if LoadBalancerIngressReady(nil) {
		t.Fatal("expected false for nil ingress")
	}
	if LoadBalancerIngressReady([]corev1.LoadBalancerIngress{{}}) {
		t.Fatal("expected false for empty ingress entry")
	}
	if !LoadBalancerIngressReady([]corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}) {
		t.Fatal("expected true for IP")
	}
	if !LoadBalancerIngressReady([]corev1.LoadBalancerIngress{{Hostname: "lb.example.com"}}) {
		t.Fatal("expected true for hostname")
	}
}

func TestExternalAccessLBAnnotations_merge(t *testing.T) {
	ea := &aisv1.ExternalAccessSpec{
		Annotations: map[string]string{
			"custom": "value",
		},
	}
	ann := ExternalAccessLBAnnotations(ea)
	if ann["prometheus.io/scrape"] != "true" {
		t.Fatal("expected prometheus scrape annotation")
	}
	if ann["custom"] != "value" {
		t.Fatal("expected user annotation to be merged")
	}
}
