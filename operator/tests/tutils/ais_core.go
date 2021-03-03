// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package tutils

import (
	"context"

	. "github.com/onsi/gomega"

	aisv1 "github.com/ais-operator/api/v1alpha1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
)

func GetProxyURL(ctx context.Context, client *aisclient.K8sClient, ais *aisv1.AIStore) (proxyURL string) {
	var ip string
	if ais.Spec.EnableExternalLB {
		ip = GetLoadBalancerIP(ctx, client, proxy.LoadBalancerSVCNSName(ais))
	} else {
		ip = GetRandomProxyIP(ctx, client, ais)
	}
	Expect(ip).NotTo(Equal(""))
	return "http://" + ip + ":" + ais.Spec.ProxySpec.ServicePort.String()
}
