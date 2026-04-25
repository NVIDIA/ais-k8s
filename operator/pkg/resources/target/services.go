// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/cmn"
	"github.com/ais-operator/pkg/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	ServiceLabelHeadless = "target-svc"
	ServiceLabelLB       = "target-lb"
)

func headlessSVCName(aisName string) string {
	return aisName + "-" + aisapc.Target
}

func HeadlessSVCNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      headlessSVCName(ais.Name),
		Namespace: ais.Namespace,
	}
}

func loadBalancerSVCName(ais *aisv1.AIStore, index int32) string {
	return fmt.Sprintf("%s-%d", statefulSetName(ais), index)
}

func LoadBalancerSVCNSName(ais *aisv1.AIStore, index int32) types.NamespacedName {
	return types.NamespacedName{
		Name:      loadBalancerSVCName(ais, index),
		Namespace: ais.Namespace,
	}
}

func PodName(ais *aisv1.AIStore, index int32) string {
	return fmt.Sprintf("%s-%d", statefulSetName(ais), index)
}

func ServiceSelectorLabels(aisName string) map[string]string {
	return map[string]string{
		cmn.LabelApp:       aisName,
		cmn.LabelComponent: aisapc.Target,
	}
}

func NewTargetHeadlessSvc(ais *aisv1.AIStore) *corev1ac.ServiceApplyConfiguration {
	servicePort := ais.Spec.TargetSpec.ServicePort
	controlPort := ais.Spec.TargetSpec.IntraControlPort
	dataPort := ais.Spec.TargetSpec.IntraDataPort
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
			WithSelector(ServiceSelectorLabels(ais.Name)),
		)
}

func NewTargetLoadBalancerSVC(ais *aisv1.AIStore, targetIndex int32) *corev1ac.ServiceApplyConfiguration {
	servicePort := ais.Spec.TargetSpec.ServicePort
	publicNetPort := ais.Spec.TargetSpec.PublicPort
	selectors := ServiceSelectorLabels(ais.Name)
	selectors["statefulset.kubernetes.io/pod-name"] = fmt.Sprintf("%s-%d", statefulSetName(ais), targetIndex)
	return corev1ac.Service(loadBalancerSVCName(ais, targetIndex), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithAnnotations(map[string]string{
			"prometheus.io/scrape": "true",
		}).
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
			WithSelector(selectors),
		)
}

func NewLoadBalancerSVCList(ais *aisv1.AIStore) []*corev1ac.ServiceApplyConfiguration {
	return LoadBalancerSVCList(ais, 0, ais.GetTargetSize())
}

func LoadBalancerSVCList(ais *aisv1.AIStore, first, size int32) []*corev1ac.ServiceApplyConfiguration {
	svcs := make([]*corev1ac.ServiceApplyConfiguration, 0, size)
	for i := first; i < first+size; i++ {
		svcs = append(svcs, NewTargetLoadBalancerSVC(ais, i))
	}
	return svcs
}
