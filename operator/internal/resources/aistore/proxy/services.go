/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	ServiceLabelHeadless = "proxy-svc"
	ServiceLabelLB       = "proxy-lb"
)

func headlessSVCName(aisName string) string {
	return aisName + "-" + aisapc.Proxy
}

func HeadlessSVCNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      headlessSVCName(ais.Name),
		Namespace: ais.Namespace,
	}
}

func loadBalancerSVCName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aisapc.Proxy + "-lb"
}

func LoadBalancerSVCNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      loadBalancerSVCName(ais),
		Namespace: ais.Namespace,
	}
}

// NewProxyHeadlessSvc creates the apply config for the headless Service fronting proxy pods.
func NewProxyHeadlessSvc(ais *aisv1.AIStore) *corev1ac.ServiceApplyConfiguration {
	servicePort := ais.Spec.ProxySpec.ServicePort
	controlPort := ais.Spec.ProxySpec.IntraControlPort
	dataPort := ais.Spec.ProxySpec.IntraDataPort

	return corev1ac.Service(headlessSVCName(ais.Name), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithAnnotations(map[string]string{
			"prometheus.io/scrape": "true",
		}).
		WithLabels(cmn.NewServiceLabels(ais.Name, ServiceLabelHeadless)).
		WithSpec(corev1ac.ServiceSpec().
			WithClusterIP("None").
			WithPublishNotReadyAddresses(true).
			WithPorts(
				corev1ac.ServicePort().
					WithName("pub").
					WithProtocol(corev1.ProtocolTCP).
					WithPort(int32(servicePort.IntValue())).
					WithTargetPort(servicePort),
				corev1ac.ServicePort().
					WithName("control").
					WithProtocol(corev1.ProtocolTCP).
					WithPort(int32(controlPort.IntValue())).
					WithTargetPort(controlPort),
				corev1ac.ServicePort().
					WithName("data").
					WithProtocol(corev1.ProtocolTCP).
					WithPort(int32(dataPort.IntValue())).
					WithTargetPort(dataPort),
			).
			WithSelector(SelectorLabels(ais)),
		)
}

func NewProxyLoadBalancerSVC(ais *aisv1.AIStore) *corev1ac.ServiceApplyConfiguration {
	servicePort := ais.Spec.ProxySpec.ServicePort
	publicNetPort := ais.Spec.ProxySpec.PublicPort
	return corev1ac.Service(loadBalancerSVCName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithAnnotations(cmn.ExternalAccessLBAnnotations(ais.Spec.ProxySpec.ExternalAccess)).
		WithLabels(cmn.NewServiceLabels(ais.Name, ServiceLabelLB)).
		WithSpec(corev1ac.ServiceSpec().
			WithType(corev1.ServiceTypeLoadBalancer).
			WithPorts(
				corev1ac.ServicePort().
					WithName("pub").
					WithProtocol(corev1.ProtocolTCP).
					WithPort(int32(servicePort.IntValue())).
					WithTargetPort(publicNetPort),
			).
			WithSelector(SelectorLabels(ais)),
		)
}
