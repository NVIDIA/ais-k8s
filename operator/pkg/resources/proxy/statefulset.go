// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

import (
	"strconv"

	apiv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	aiscmn "github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1alpha1"
	"github.com/ais-operator/pkg/resources/cmn"
)

func statefulSetName(ais *aisv1.AIStore) string {
	return ais.Name + "-" + aiscmn.Proxy
}

func StatefulSetNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      statefulSetName(ais),
		Namespace: ais.Namespace,
	}
}

func NewProxyStatefulSet(ais *aisv1.AIStore, size int32) *apiv1.StatefulSet {
	ls := podLabels(ais)
	proxySpec := proxyPodSpec(ais)
	return &apiv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName(ais),
			Namespace: ais.Namespace,
			Labels:    ls,
		},
		Spec: apiv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			ServiceName:         headlessSVCName(ais),
			PodManagementPolicy: apiv1.ParallelPodManagement,
			Replicas:            &size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: proxySpec,
			},
		},
	}
}

/////////////////
//   helpers  //
////////////////
func proxyPodSpec(ais *aisv1.AIStore) corev1.PodSpec {
	return corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name:            "populate-env",
				Image:           ais.Spec.InitImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env: []corev1.EnvVar{
					cmn.EnvFromFieldPath("MY_NODE", "spec.nodeName"),
					cmn.EnvFromFieldPath("MY_POD", "metadata.name"),
					cmn.EnvFromValue("K8S_NS", ais.Namespace),
					cmn.EnvFromValue("MY_SERVICE", headlessSVCName(ais)),
					cmn.EnvFromValue("AIS_NODE_ROLE", aiscmn.Proxy),
					cmn.EnvFromValue("CLUSTERIP_PROXY_SERVICE_HOSTNAME", ais.Name+"-"+aiscmn.Proxy),
					cmn.EnvFromValue("CLUSTERIP_PROXY_SERVICE_PORT", ais.Spec.ProxySpec.ServicePort.String()),
					cmn.EnvFromValue("AIS_DEFAULT_PRIMARY", statefulSetName(ais)+"-0"),
				},
				Args:         []string{"-c", "/bin/bash /var/ais_config_template/set_initial_primary_proxy_env.sh"},
				Command:      []string{"/bin/bash"},
				VolumeMounts: cmn.NewInitVolumeMounts(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "ais-node",
				Image:           ais.Spec.NodeImage,
				ImagePullPolicy: corev1.PullAlways,
				Env: []corev1.EnvVar{
					cmn.EnvFromFieldPath("MY_POD", "metadata.name"),
					cmn.EnvFromValue("K8S_NS", ais.Namespace),
					cmn.EnvFromValue("AIS_CLUSTER_CIDR", ""),
					cmn.EnvFromValue("AIS_CONF_FILE", "/var/ais_config/ais.json"),
					cmn.EnvFromValue("STATSD_CONF_FILE", "/var/statsd_config/statsd.json"),
					cmn.EnvFromValue("AIS_NODE_ROLE", aiscmn.Proxy),
					cmn.EnvFromValue("TARGETS", strconv.Itoa(int(ais.Spec.Size))),
					cmn.EnvFromValue("CLUSTERIP_PROXY_SERVICE_HOSTNAME", ais.Name+"-"+aiscmn.Proxy),
					cmn.EnvFromValue("CLUSTERIP_PROXY_SERVICE_PORT", ais.Spec.ProxySpec.ServicePort.String()),
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: int32(ais.Spec.ProxySpec.ServicePort.IntValue()),
						Protocol:      corev1.ProtocolTCP,
					},
				},
				SecurityContext: ais.Spec.ProxySpec.ContainerSecurity,
				VolumeMounts:    cmn.NewAISVolumeMounts(),
				Lifecycle:       cmn.NewAISNodeLifecycle(),
				LivenessProbe:   cmn.NewAISLivenessProbe(ais.Spec.ProxySpec.ServicePort),
				ReadinessProbe:  readinessProbe(ais.Spec.ProxySpec.ServicePort),
			},
		},
		Affinity:           cmn.NewAISPodAffinity(ais, ais.Spec.ProxySpec.Affinity, podLabels(ais)),
		ServiceAccountName: cmn.ServiceAccountName(ais),
		SecurityContext:    ais.Spec.ProxySpec.SecurityContext,
		Volumes:            cmn.NewAISVolumes(ais, aiscmn.Proxy),
		Tolerations:        ais.Spec.ProxySpec.Tolerations,
	}
}

func podLabels(ais *aisv1.AIStore) map[string]string {
	return map[string]string{
		"app":       ais.Name,
		"component": aiscmn.Proxy,
		"function":  "gateway",
	}
}

func readinessProbe(port intstr.IntOrString) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"/ais_readiness.sh"},
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
	}
}
