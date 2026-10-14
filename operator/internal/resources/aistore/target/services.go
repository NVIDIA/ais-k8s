/*
 * Copyright (c) 2021-2025, NVIDIA CORPORATION. All rights reserved.
 */

package target

import (
	"fmt"

	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/cmn"
	"github.com/ais-operator/internal/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	ServiceLabelHeadless = "target-svc"
	ServiceLabelLB       = "target-lb"
)

func loadBalancerSVCName(ais *aisv1.AIStore, index int32) string {
	return PodName(ais, index)
}

func LoadBalancerSVCNSName(ais *aisv1.AIStore, index int32) types.NamespacedName {
	return types.NamespacedName{
		Name:      loadBalancerSVCName(ais, index),
		Namespace: ais.Namespace,
	}
}

func PodName(ais *aisv1.AIStore, index int32) string {
	return fmt.Sprintf("%s-%d", ais.TargetStatefulSetName(), index)
}

func NewTargetHeadlessSvc(ais *aisv1.AIStore) *corev1ac.ServiceApplyConfiguration {
	publicPort := ais.TargetPublicPort()
	controlPort := ais.TargetIntraControlPort()
	dataPort := ais.TargetIntraDataPort()
	svc := ais.TargetHeadlessSVCNSName()
	return corev1ac.Service(svc.Name, svc.Namespace).
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
					WithPort(int32(publicPort.IntValue())).
					WithTargetPort(publicPort),
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

func NewTargetLoadBalancerSVC(ais *aisv1.AIStore, targetIndex int32) *corev1ac.ServiceApplyConfiguration {
	externalPort := ais.TargetExternalPort()
	publicNetPort := ais.TargetPublicPort()
	selectors := SelectorLabels(ais)
	selectors["statefulset.kubernetes.io/pod-name"] = PodName(ais, targetIndex)
	svcName := loadBalancerSVCName(ais, targetIndex)
	annotations := cmn.ExternalAccessLBAnnotations(ais.Spec.TargetSpec.ExternalAccess)
	if dns := ais.TargetPublicDNS(); dns != nil && dns.ExternalDNSAnnotation {
		annotations[aisv1.ExternalDNSHostnameAnnotation] = svcName + "." + dns.Domain
	}
	return corev1ac.Service(svcName, ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithAnnotations(annotations).
		WithLabels(cmn.NewServiceLabels(ais.Name, ServiceLabelLB)).
		WithSpec(corev1ac.ServiceSpec().
			WithType(corev1.ServiceTypeLoadBalancer).
			WithPorts(
				corev1ac.ServicePort().
					WithName("pub").
					WithProtocol(corev1.ProtocolTCP).
					WithPort(int32(externalPort.IntValue())).
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
