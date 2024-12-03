// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import corev1 "k8s.io/api/core/v1"

// Environment variables used by AIS init&daemon containers
const (
	EnvNodeName    = "MY_NODE"    // Hostname of node in which pod is deployed
	EnvPodName     = "MY_POD"     // Pod name to which the container belongs to
	EnvNS          = "K8S_NS"     // K8s Namespace where `pod` is deployed
	EnvServiceName = "MY_SERVICE" // K8s service associated with Pod

	EnvPublicHostname       = "AIS_PUBLIC_HOSTNAME"
	EnvDefaultPrimaryPod    = "AIS_DEFAULT_PRIMARY"      // Default Primary pod name
	EnvCIDR                 = "AIS_CLUSTER_CIDR"         // CIDR to use
	EnvClusterDomain        = "AIS_K8S_CLUSTER_DOMAIN"   // K8s cluster DNS domain
	EnvConfigFilePath       = "AIS_CONF_FILE"            // Path to AIS config file
	EnvLocalConfigFilePath  = "AIS_LOCAL_CONF_FILE"      // Path to AIS local config file
	EnvEnablePrometheus     = "AIS_PROMETHEUS"           // Enable prometheus exporter
	EnvUseHTTPS             = "AIS_USE_HTTPS"            // Use HTTPS endpoints
	EnvStatsDConfig         = "STATSD_CONF_FILE"         // Path to StatsD config json
	EnvNumTargets           = "TARGETS"                  // Expected target count // TODO: Add AIS_ prefix
	EnvEnableExternalAccess = "ENABLE_EXTERNAL_ACCESS"   // Bool flag to indicate AIS daemon is exposed using LoadBalancer
	EnvShutdownMarkerPath   = "AIS_SHUTDOWN_MARKER_PATH" // Path where node shutdown marker will be located

	//nolint:gosec // This is not really credential.
	EnvGCPCredsPath = "GOOGLE_APPLICATION_CREDENTIALS" // Path to GCP credentials

	EnvHostNetwork = "HOST_NETWORK" // Bool flag to indicate if host network is enabled for target

	// AuthN related environment variables
	EnvAuthNSecretKey = "SIGNING-KEY" // Key for secret signing key in the K8s secret
)

func CommonEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		EnvFromFieldPath(EnvNodeName, "spec.nodeName"),
		EnvFromFieldPath(EnvPodName, "metadata.name"),
		EnvFromFieldPath(EnvNS, "metadata.namespace"),
	}
}
