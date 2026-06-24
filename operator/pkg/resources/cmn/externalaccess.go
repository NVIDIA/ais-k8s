/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// ExternalAccessLBAnnotations returns annotations for an external LoadBalancer Service.
func ExternalAccessLBAnnotations(ea *aisv1.ExternalAccessSpec) map[string]string {
	ann := map[string]string{
		"prometheus.io/scrape": "true",
	}
	var user map[string]string
	if ea != nil {
		user = ea.Annotations
	}
	return mergeServiceAnnotations(ann, user)
}

func mergeServiceAnnotations(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// LoadBalancerIngressReady returns true when at least one ingress has an IP or hostname.
func LoadBalancerIngressReady(ingress []corev1.LoadBalancerIngress) bool {
	for i := range ingress {
		if ingress[i].IP != "" || ingress[i].Hostname != "" {
			return true
		}
	}
	return false
}
