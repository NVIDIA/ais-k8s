/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	"fmt"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// ServiceName returns the name of the in-cluster AuthN Service.
func ServiceName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name
}

// ServiceNSName returns the namespaced name of the in-cluster AuthN Service.
func ServiceNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: ServiceName(authn), Namespace: authn.Namespace}
}

// NodePortServiceName returns the name of the optional AuthN NodePort Service.
func NodePortServiceName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name + "-nodeport"
}

// NodePortServiceNSName returns the namespaced name of the optional AuthN NodePort Service.
func NodePortServiceNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: NodePortServiceName(authn), Namespace: authn.Namespace}
}

// LoadBalancerServiceName returns the name of the optional AuthN LoadBalancer Service.
func LoadBalancerServiceName(authn *authv1alpha1.AIStoreAuth) string {
	return authn.Name + "-lb"
}

// LoadBalancerServiceNSName returns the namespaced name of the optional AuthN LoadBalancer Service.
func LoadBalancerServiceNSName(authn *authv1alpha1.AIStoreAuth) types.NamespacedName {
	return types.NamespacedName{Name: LoadBalancerServiceName(authn), Namespace: authn.Namespace}
}

// ServiceURL returns the stable in-cluster endpoint published in AIStoreAuth status.
func ServiceURL(authn *authv1alpha1.AIStoreAuth) string {
	scheme := "http"
	if authn.HasTLSEnabled() {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s.%s.svc:%d", scheme, ServiceName(authn), authn.Namespace, authn.ListenPort())
}

// NewService builds the always-present ClusterIP Service used by in-cluster clients.
func NewService(authn *authv1alpha1.AIStoreAuth) *corev1ac.ServiceApplyConfiguration {
	return baseService(authn, ServiceName(authn)).
		WithSpec(baseServiceSpecWithPort(authn, servicePort(authn.ListenPort())).
			WithType(corev1.ServiceTypeClusterIP))
}

// NewNodePortService builds the optional Service used for node-level external access.
func NewNodePortService(authn *authv1alpha1.AIStoreAuth) *corev1ac.ServiceApplyConfiguration {
	if authn.Spec.ExternalAccess == nil || authn.Spec.ExternalAccess.NodePort == nil {
		return nil
	}
	nodePort := authn.Spec.ExternalAccess.NodePort
	port := servicePort(authn.ListenPort()).WithNodePort(nodePort.Port)
	return baseService(authn, NodePortServiceName(authn)).
		WithSpec(baseServiceSpecWithPort(authn, port).
			WithType(corev1.ServiceTypeNodePort))
}

// NewLoadBalancerService builds the optional Service used for load-balanced external access.
func NewLoadBalancerService(authn *authv1alpha1.AIStoreAuth) *corev1ac.ServiceApplyConfiguration {
	if authn.Spec.ExternalAccess == nil || authn.Spec.ExternalAccess.LoadBalancer == nil {
		return nil
	}
	lb := authn.Spec.ExternalAccess.LoadBalancer
	return baseService(authn, LoadBalancerServiceName(authn)).
		WithAnnotations(lb.Annotations).
		WithSpec(baseServiceSpecWithPort(authn, servicePort(lb.Port)).
			WithType(corev1.ServiceTypeLoadBalancer))
}

func baseService(authn *authv1alpha1.AIStoreAuth, name string) *corev1ac.ServiceApplyConfiguration {
	return corev1ac.Service(name, authn.Namespace).
		WithOwnerReferences(ownerref.NewAIStoreAuthControllerRef(authn)).
		WithLabels(resourceLabels(authn))
}

func baseServiceSpecWithPort(
	authn *authv1alpha1.AIStoreAuth,
	port *corev1ac.ServicePortApplyConfiguration,
) *corev1ac.ServiceSpecApplyConfiguration {
	return corev1ac.ServiceSpec().
		WithSelector(selectorLabels(authn)).
		WithPorts(port)
}

func servicePort(port int32) *corev1ac.ServicePortApplyConfiguration {
	return corev1ac.ServicePort().
		WithName(portName).
		WithProtocol(corev1.ProtocolTCP).
		WithPort(port).
		WithTargetPort(intstr.FromString(portName))
}
