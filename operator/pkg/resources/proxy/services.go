// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

import (
	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func headlessSVCName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aiscmn.Proxy
}

func HeadlessSVCNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      headlessSVCName(ais),
		Namespace: ais.Namespace,
	}
}

func loadBalancerSVCName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aiscmn.Proxy + "-lb"
}

func LoadBalancerSVCNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      loadBalancerSVCName(ais),
		Namespace: ais.Namespace,
	}
}

// NewProxyHeadlessSvc returns a headless k8s services associated with `proxies`
func NewProxyHeadlessSvc(ais *aisv1.AIStore) *corev1.Service {
	servicePort := ais.Spec.ProxySpec.ServicePort
	controlPort := ais.Spec.ProxySpec.IntraControlPort
	dataPort := ais.Spec.ProxySpec.IntraDataPort

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessSVCName(ais),
			Namespace: ais.Namespace,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
			Labels: map[string]string{
				"app": ais.Name,
			},
		},
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
			Selector: podLabels(ais),
		},
	}
}

func NewProxyLoadBalancerSVC(ais *aisv1.AIStore) *corev1.Service {
	servicePort := ais.Spec.ProxySpec.ServicePort
	publicNetPort := ais.Spec.ProxySpec.PublicPort
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      loadBalancerSVCName(ais),
			Namespace: ais.Namespace,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
			Labels: map[string]string{
				"app": ais.Name,
			},
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
			Selector: podLabels(ais),
		},
	}
}
