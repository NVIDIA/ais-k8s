// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

// Environment variables used by AIS init&daemon containers
const (
	EnvNodeName    = "MY_NODE"    // Hostname of node in which pod is deployed
	EnvPodName     = "MY_POD"     // Pod name to which the container belongs to
	EnvNS          = "K8S_NS"     // K8s Namespace where `pod` is deployed
	EnvServiceName = "MY_SERVICE" // K8s service associated with Pod

	EnvDaemonRole           = "AIS_NODE_ROLE"                    // Role of AIS daemon (Proxy or Target)
	EnvProxyServiceName     = "CLUSTERIP_PROXY_SERVICE_HOSTNAME" // Service name of Proxy StatefulSets
	EnvProxyServicePort     = "CLUSTERIP_PROXY_SERVICE_PORT"     // Port used by Proxy Service
	EnvDefaultPrimaryPod    = "AIS_DEFAULT_PRIMARY"              // Default Primary pod name
	EnvCIDR                 = "AIS_CLUSTER_CIDR"                 // CIDR to use
	EnvClusterDomain        = "AIS_K8S_CLUSTER_DOMAIN"           // K8s cluster DNS domain
	ENVConfigFilePath       = "AIS_CONF_FILE"                    // Path to AIS config file
	ENVLocalConfigFilePath  = "AIS_LOCAL_CONF_FILE"              // Path to AIS local config file
	EnvStatsDConfig         = "STATSD_CONF_FILE"                 // Path to StatsD config json
	EnvNumTargets           = "TARGETS"                          // Expected target count // TODO: Add AIS_ prefix
	EnvEnableExternalAccess = "ENABLE_EXTERNAL_ACCESS"           // Bool flag to indicate AIS daemon is exposed using LoadBalancer

	// Benchmark see: https://github.com/NVIDIA/aistore/blob/master/docs/howto_benchmark.md#dry-run-performance-tests
	EnvNoDiskIO   = "AIS_NO_DISK_IO"
	EnvDryObjSize = "AIS_DRY_OBJ_SIZE"
)
