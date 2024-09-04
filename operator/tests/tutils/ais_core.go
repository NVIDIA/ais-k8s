// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"

	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	"github.com/ais-operator/pkg/resources/proxy"
	. "github.com/onsi/gomega"
)

const urlTemplate = "http://%s:%s"

func GetProxyURL(ctx context.Context, client *aisclient.K8sClient, ais *aisv1.AIStore) (proxyURL string) {
	var ip string
	if ais.Spec.EnableExternalLB {
		ip = GetLoadBalancerIP(ctx, client, proxy.LoadBalancerSVCNSName(ais))
	} else {
		ip = GetRandomProxyIP(ctx, client, ais)
	}
	Expect(ip).NotTo(Equal(""))
	return fmt.Sprintf(urlTemplate, ip, ais.Spec.ProxySpec.ServicePort.String())
}

func GetAllProxyURLs(ctx context.Context, client *aisclient.K8sClient, ais *aisv1.AIStore) (proxyURLs []*string) {
	var proxyIPs []string
	if ais.Spec.EnableExternalLB {
		proxyIPs = []string{GetLoadBalancerIP(ctx, client, proxy.LoadBalancerSVCNSName(ais))}
	} else {
		proxyIPs = GetAllProxyIPs(ctx, client, ais)
	}
	for _, ip := range proxyIPs {
		proxyURL := fmt.Sprintf(urlTemplate, ip, ais.Spec.ProxySpec.ServicePort.String())
		proxyURLs = append(proxyURLs, &proxyURL)
	}
	return proxyURLs
}
