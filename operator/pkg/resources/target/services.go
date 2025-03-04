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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func NewTargetHeadlessSvc(ais *aisv1.AIStore) *corev1.Service {
	servicePort := ais.Spec.TargetSpec.ServicePort
	controlPort := ais.Spec.TargetSpec.IntraControlPort
	dataPort := ais.Spec.TargetSpec.IntraDataPort
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessSVCName(ais.Name),
			Namespace: ais.Namespace,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
			Labels: cmn.NewServiceLabels(ais.Name, ServiceLabelHeadless),
		},
		// TODO: in re G115 "integer overflow conversion int", see:
		// https://github.com/kubernetes/apimachinery/blob/master/pkg/util/intstr/intstr.go#L32-L36
		// once resolved in apimachinery, remove G115 exclusion from golangci yaml
		Spec: corev1.ServiceSpec{
			ClusterIP: "None", // headless
			Ports: []corev1.ServicePort{
				{
					Name:       "pub",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(servicePort.IntValue()),
					TargetPort: servicePort,
				},
				{
					Name:       "control",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(controlPort.IntValue()),
					TargetPort: controlPort,
				},
				{
					Name:       "data",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(dataPort.IntValue()),
					TargetPort: dataPort,
				},
			},
			Selector: ServiceSelectorLabels(ais.Name),
		},
	}
}

func NewTargetLoadBalancerSVC(ais *aisv1.AIStore, targetIndex int32) *corev1.Service {
	servicePort := ais.Spec.TargetSpec.ServicePort
	publicNetPort := ais.Spec.TargetSpec.PublicPort
	selectors := ServiceSelectorLabels(ais.Name)
	selectors["statefulset.kubernetes.io/pod-name"] = fmt.Sprintf("%s-%d", statefulSetName(ais), targetIndex)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      loadBalancerSVCName(ais, targetIndex),
			Namespace: ais.Namespace,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
			Labels: cmn.NewServiceLabels(ais.Name, ServiceLabelLB),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:       "pub",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(servicePort.IntValue()),
					TargetPort: publicNetPort,
				},
			},
			Selector: selectors,
		},
	}
}

func NewLoadBalancerSVCList(ais *aisv1.AIStore) []*corev1.Service {
	return LoadBalancerSVCList(ais, 0, ais.GetTargetSize())
}

func LoadBalancerSVCList(ais *aisv1.AIStore, first, size int32) []*corev1.Service {
	svcs := make([]*corev1.Service, 0, size)
	for i := first; i < first+size; i++ {
		svcs = append(svcs, NewTargetLoadBalancerSVC(ais, i))
	}
	return svcs
}
