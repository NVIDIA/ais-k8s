// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"strconv"

	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// Environment variables used by AIS init&daemon containers
const (
	EnvHostIPS     = "HOST_IPS"   // Host IPs of the node in which pod is deployed
	EnvNodeName    = "MY_NODE"    // Hostname of the node in which pod is deployed
	EnvPodName     = "MY_POD"     // Pod name to which the container belongs to
	EnvNS          = "K8S_NS"     // K8s Namespace where `pod` is deployed
	EnvServiceName = "MY_SERVICE" // K8s service associated with Pod

	EnvPublicHostname       = "AIS_PUBLIC_HOSTNAME"
	EnvPublicDNSMode        = "AIS_PUBLIC_DNS_MODE"    // Determines what DNS name to use for the public network
	EnvClusterDomain        = "AIS_K8S_CLUSTER_DOMAIN" // K8s cluster DNS domain
	EnvEnableExternalAccess = "ENABLE_EXTERNAL_ACCESS" // Bool flag to indicate AIS daemon is exposed using LoadBalancer

	EnvHostNetwork = "HOST_NETWORK" // Bool flag to indicate if host network is enabled for target

	// AuthN related environment variables
	EnvAuthNSecretKey = "SIGNING-KEY" // Key for secret signing key in the K8s secret

	// Cloud provider variables
	//nolint:gosec // This is a path, not credential
	EnvGoogleCreds = "GOOGLE_APPLICATION_CREDENTIALS"
	EnvOCIConfig   = "OCI_CLI_CONFIG_FILE"
)

// CommonEnv provides environment variables for all containers (target/proxy, init/aisnode)
func CommonEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		EnvFromFieldPath(EnvNodeName, "spec.nodeName"),
		EnvFromFieldPath(EnvPodName, "metadata.name"),
		EnvFromFieldPath(EnvNS, "metadata.namespace"),
	}
}

// CommonInitEnv provides environment variables used by init containers for both proxy and target pods
func CommonInitEnv(ais *aisv1.AIStore) []corev1.EnvVar {
	initEnv := []corev1.EnvVar{
		EnvFromFieldPath(EnvHostIPS, "status.hostIPs"),
		EnvFromValue(EnvClusterDomain, ais.GetClusterDomain()),
		EnvFromValue(
			EnvEnableExternalAccess,
			strconv.FormatBool(ais.Spec.EnableExternalLB),
		),
	}
	if ais.Spec.PublicNetDNSMode != nil {
		initEnv = append(initEnv, EnvFromValue(EnvPublicDNSMode, string(*ais.Spec.PublicNetDNSMode)))
	}
	return append(initEnv, CommonEnv()...)
}
