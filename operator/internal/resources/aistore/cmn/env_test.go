/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/opinfo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Env", Label("short"), func() {
	Describe("CommonEnv", func() {
		It("returns the node, pod, namespace and cluster name vars", func() {
			ais := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "ais"}}

			Expect(CommonEnv(ais)).To(Equal([]corev1.EnvVar{
				EnvFromFieldPath(EnvNodeName, "spec.nodeName"),
				EnvFromFieldPath(EnvPodName, "metadata.name"),
				EnvFromFieldPath(EnvNS, "metadata.namespace"),
				EnvFromValue(EnvCluster, "test-cluster"),
			}))
		})

		It("sources the cluster name from the resource name", func() {
			ais := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name", Namespace: "ns-name"}}
			env := CommonEnv(ais)
			Expect(env).To(ContainElement(corev1.EnvVar{Name: EnvCluster, Value: "cluster-name"}))
		})

		It("sets the pod ID source only when annotated", func() {
			annotated := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PodDerivedNodeIDsAnnotation: "true"}}}
			Expect(CommonEnv(annotated)).To(ContainElement(EnvFromValue(EnvDaemonIDSource, "k8s-pod")))
			Expect(CommonEnv(&aisv1.AIStore{})).NotTo(ContainElement(HaveField("Name", EnvDaemonIDSource)))
		})
	})
	Describe("MergeEnvVars", func() {
		It("keeps the default node ID source over an override", func() {
			merged := MergeEnvVars(
				[]corev1.EnvVar{EnvFromValue(EnvDaemonIDSource, "k8s-pod")},
				[]corev1.EnvVar{EnvFromValue(EnvDaemonIDSource, "random")},
			)
			Expect(merged).To(ConsistOf(EnvFromValue(EnvDaemonIDSource, "k8s-pod")))
		})

		It("keeps an override of the node ID source when no default sets it", func() {
			override := EnvFromValue(EnvDaemonIDSource, "random")
			Expect(MergeEnvVars(nil, []corev1.EnvVar{override})).To(ConsistOf(override))
		})
	})
	Describe("CommonInitEnv", func() {
		It("returns the host IPs and cluster domain vars and the common vars", func() {
			ais := &aisv1.AIStore{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}
			Expect(CommonInitEnv(ais)).To(Equal(append([]corev1.EnvVar{
				EnvFromFieldPath(EnvHostIPS, "status.hostIPs"),
				EnvFromValue(EnvClusterDomain, opinfo.ClusterDomain()),
			}, CommonEnv(ais)...)))
		})
	})

})
