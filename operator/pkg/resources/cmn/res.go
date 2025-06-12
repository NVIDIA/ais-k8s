// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// probe constants
	// TODO: obtain probe specs from AIStore custom resource spec.
	defaultProbePeriodSeconds        = 5
	defaultProbeTimeoutSeconds       = 5
	defaultReadinessFailureThreshold = 5

	defaultStartupPeriodSeconds    = 5
	defaultStartupFailureThreshold = 30

	defaultLivenessFailureThreshold    = 10
	defaultLivenessInitialDelaySeconds = 60

	probeLivenessEndpoint  = "/v1/health"
	probeReadinessEndpoint = probeLivenessEndpoint + "?readiness=true"
)

func newHTTPProbeHandle(ais *aisv1.AIStore, daemonRole, probeEndpoint string) corev1.ProbeHandler {
	var (
		httpPort  intstr.IntOrString
		uriScheme = corev1.URISchemeHTTP
	)

	if ais.UseHTTPS() {
		uriScheme = corev1.URISchemeHTTPS
	}

	switch daemonRole {
	case aisapc.Proxy:
		httpPort = ais.Spec.ProxySpec.PublicPort
	case aisapc.Target:
		httpPort = ais.Spec.TargetSpec.PublicPort
	}
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Scheme: uriScheme,
			Path:   probeEndpoint,
			Port:   httpPort,
		},
	}
}

func NewLivenessProbe(ais *aisv1.AIStore, daemonRole string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        newHTTPProbeHandle(ais, daemonRole, probeLivenessEndpoint),
		InitialDelaySeconds: defaultLivenessInitialDelaySeconds,
		PeriodSeconds:       defaultProbePeriodSeconds,
		// liveness looks for the AIS daemon to successfully join the cluster.
		// Cluster join sequence could take a bit long, so add some initial delay to
		// ensure K8s doesn't kill the aisnode container prematurely.
		FailureThreshold: defaultLivenessFailureThreshold,
		TimeoutSeconds:   defaultProbeTimeoutSeconds,
	}
}

func NewReadinessProbe(ais *aisv1.AIStore, daemonRole string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:     newHTTPProbeHandle(ais, daemonRole, probeReadinessEndpoint),
		PeriodSeconds:    defaultProbePeriodSeconds,
		FailureThreshold: defaultReadinessFailureThreshold,
		TimeoutSeconds:   defaultProbeTimeoutSeconds,
	}
}

func NewStartupProbe(ais *aisv1.AIStore, daemonRole string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: newHTTPProbeHandle(ais, daemonRole, probeReadinessEndpoint),
		// For startup probe, which is a one-time probe we are more aggressive in checking for readiness.
		// We leave up-to 30secs for the daemon to start responding to HTTP request.
		// NOTE: Success here only means that the HTTP server is up and running, that doesn't imply AIS daemon is
		// ready in terms of the AIStore cluster.
		PeriodSeconds:    defaultStartupPeriodSeconds,
		FailureThreshold: defaultStartupFailureThreshold,
		TimeoutSeconds:   defaultProbeTimeoutSeconds,
	}
}

func NewDaemonPorts(spec *aisv1.DaemonSpec) []corev1.ContainerPort {
	var hostPort int32
	if spec.HostPort != nil {
		hostPort = *spec.HostPort
	}
	return []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: int32(spec.ServicePort.IntValue()),
			Protocol:      corev1.ProtocolTCP,
			HostPort:      hostPort,
		},
	}
}

func CreateAISAffinity(affinity *corev1.Affinity, basicLabels map[string]string) *corev1.Affinity {
	// If we have no affinity defined in spec, define an empty one
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}

	// If we have an affinity but no specific PodAntiAffinity, set it
	if affinity.PodAntiAffinity == nil {
		affinity.PodAntiAffinity = createPodAntiAffinity(basicLabels)
	}

	return affinity
}

func createPodAntiAffinity(basicLabels map[string]string) *corev1.PodAntiAffinity {
	// Pods matching basicLabels may not be scheduled on the same hostname
	labelAffinity := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: basicLabels,
		},
		TopologyKey: corev1.LabelHostname,
	}

	return &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			labelAffinity,
		},
	}
}

// Generate PVC claim ref for a specific namespace and cluster
func getStatePVCName(ais *aisv1.AIStore) string {
	return fmt.Sprintf("%s-%s-%s", ais.Namespace, ais.Name, "state")
}

// DefineStatePVC Define a PVC to use for pod state using dynamically configured volumes
func DefineStatePVC(ais *aisv1.AIStore, storageClass *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: getStatePVCName(ais),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
			StorageClassName: storageClass,
		},
	}
}
