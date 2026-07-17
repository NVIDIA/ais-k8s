/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	"crypto/sha256"
	"encoding/hex"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

var _ = Describe("Deployment", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		storageClass := "openebs-hostpath"
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Persistence: authv1alpha1.PersistenceSpec{StorageClass: &storageClass},
				Deployment: authv1alpha1.DeploymentSpec{
					Container: authv1alpha1.ContainerSpec{
						Image:           "docker.io/aistorage/authn:v4.8",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
			},
		}
	})

	It("uses the CR name and namespace", func() {
		Expect(authnres.DeploymentName(authn)).To(Equal("ais-authn"))
		Expect(authnres.DeploymentNSName(authn)).To(Equal(types.NamespacedName{
			Name: "ais-authn", Namespace: "ais",
		}))
	})

	It("builds an owned, single-replica Recreate Deployment with stable selectors", func() {
		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())

		Expect(deployment.Labels).To(Equal(map[string]string{
			"app.kubernetes.io/name":       "authn",
			"app.kubernetes.io/instance":   "ais-authn",
			"app.kubernetes.io/managed-by": "ais-operator",
		}))
		Expect(deployment.OwnerReferences).To(HaveLen(1))
		Expect(deployment.OwnerReferences[0].Name).To(HaveValue(Equal(authn.Name)))
		Expect(deployment.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
		Expect(deployment.Spec.Replicas).To(HaveValue(Equal(int32(1))))
		Expect(deployment.Spec.Strategy.Type).To(HaveValue(Equal(appsv1.RecreateDeploymentStrategyType)))
		Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{
			"app.kubernetes.io/name":     "authn",
			"app.kubernetes.io/instance": "ais-authn",
		}))
		Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "ais-authn"))
	})

	It("runs the configured image and pull policy on the AuthN listen port", func() {
		port := int32(53001)
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{HTTP: &authv1alpha1.HTTPConfSpec{Port: &port}},
		}
		container := newContainer(authn)

		Expect(container.Name).To(HaveValue(Equal("authn")))
		Expect(container.Image).To(HaveValue(Equal("docker.io/aistorage/authn:v4.8")))
		Expect(container.ImagePullPolicy).To(HaveValue(Equal(corev1.PullIfNotPresent)))
		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].Name).To(HaveValue(Equal("http")))
		Expect(container.Ports[0].ContainerPort).To(HaveValue(Equal(port)))
		Expect(container.Ports[0].Protocol).To(HaveValue(Equal(corev1.ProtocolTCP)))
	})

	It("checksums the exact rendered config and changes the checksum when config changes", func() {
		configMap, err := authnres.NewConfigMap(authn)
		Expect(err).NotTo(HaveOccurred())
		expected := sha256.Sum256([]byte(configMap.Data[authnres.AuthnJSONKey]))

		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		checksum := deployment.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]
		Expect(checksum).To(Equal(hex.EncodeToString(expected[:])))

		level := int32(4)
		authn.Spec.Config = &authv1alpha1.ConfigSpec{Log: &authv1alpha1.LogSpec{Level: &level}}
		updated, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation]).NotTo(Equal(checksum))
	})

	It("leaves optional container fields unset", func() {
		container := newContainer(authn)

		Expect(container.Resources).To(BeNil())
		Expect(container.SecurityContext).To(BeNil())
		Expect(container.LivenessProbe).To(BeNil())
		Expect(container.ReadinessProbe).To(BeNil())
	})

	It("renders optional container fields from spec.deployment.container", func() {
		runAsNonRoot := true
		authn.Spec.Deployment.Container.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
		}
		authn.Spec.Deployment.Container.SecurityContext = &corev1.SecurityContext{RunAsNonRoot: &runAsNonRoot}
		authn.Spec.Deployment.Container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("http")}},
		}
		authn.Spec.Deployment.Container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health", Port: intstr.FromString("http"), Scheme: corev1.URISchemeHTTPS,
			}},
		}

		container := newContainer(authn)

		Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
		Expect(container.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
		Expect(container.LivenessProbe.TCPSocket.Port).To(HaveValue(Equal(intstr.FromString("http"))))
		Expect(container.ReadinessProbe.HTTPGet.Scheme).To(HaveValue(Equal(corev1.URISchemeHTTPS)))
	})

	It("leaves optional pod fields unset", func() {
		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		podSpec := deployment.Spec.Template.Spec

		Expect(podSpec.SecurityContext).To(BeNil())
		Expect(podSpec.NodeSelector).To(BeEmpty())
		Expect(podSpec.Tolerations).To(BeEmpty())
		Expect(podSpec.Affinity).To(BeNil())
		Expect(podSpec.ImagePullSecrets).To(BeEmpty())
	})

	It("renders optional pod fields from spec.deployment.pod", func() {
		fsGroup := int64(2000)
		authn.Spec.Deployment.Pod = &authv1alpha1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{FSGroup: &fsGroup},
			NodeSelector:    map[string]string{"node-pool": "authn"},
			Tolerations: []corev1.Toleration{{
				Key: "dedicated", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule,
			}},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "kubernetes.io/os", Operator: corev1.NodeSelectorOpIn, Values: []string{"linux"},
							}},
						}},
					},
				},
			},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-creds"}},
		}

		deployment, err := authnres.NewDeployment(authn)
		Expect(err).NotTo(HaveOccurred())
		podSpec := deployment.Spec.Template.Spec

		Expect(podSpec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(2000))))
		Expect(podSpec.NodeSelector).To(Equal(map[string]string{"node-pool": "authn"}))
		Expect(podSpec.Tolerations).To(HaveLen(1))
		Expect(podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeNil())
		Expect(podSpec.ImagePullSecrets).To(HaveLen(1))
		Expect(podSpec.ImagePullSecrets[0].Name).To(HaveValue(Equal("registry-creds")))
	})

})

func newContainer(authn *authv1alpha1.AIStoreAuth) corev1ac.ContainerApplyConfiguration {
	GinkgoHelper()
	deployment, err := authnres.NewDeployment(authn)
	Expect(err).NotTo(HaveOccurred())
	spec := deployment.Spec.Template.Spec
	Expect(spec.Containers).To(HaveLen(1))
	return spec.Containers[0]
}
