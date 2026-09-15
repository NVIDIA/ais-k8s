/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package adminclient_test

import (
	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/resources/aistore/adminclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Admin Client Deployment", Label("short"), func() {
	baseAIS := func() *aisv1.AIStore {
		return &aisv1.AIStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ais",
				Namespace: "test-ns",
			},
			Spec: aisv1.AIStoreSpec{
				Size: apc.Ptr(int32(1)),
				AdminClient: &aisv1.AdminClientSpec{
					Enabled: apc.Ptr(true),
				},
			},
		}
	}

	Describe("NewClientDeployment", func() {
		It("should use the default service account without mounting its token", func() {
			ais := baseAIS()
			ais.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "registry-creds"}}

			deployment := adminclient.NewClientDeployment(ais, adminclient.Config{})
			podSpec := deployment.Spec.Template.Spec

			Expect(podSpec.ServiceAccountName).To(Equal("default"))
			Expect(podSpec.AutomountServiceAccountToken).To(HaveValue(BeFalse()))
			Expect(podSpec.ImagePullSecrets).To(Equal(ais.Spec.ImagePullSecrets))
		})

		It("should reconcile service account security settings", func() {
			ais := baseAIS()
			ais.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "registry-creds"}}
			desired := adminclient.NewClientDeployment(ais, adminclient.Config{})
			current := desired.DeepCopy()
			current.Spec.Template.Spec.ServiceAccountName = "test-ais-sa"
			current.Spec.Template.Spec.AutomountServiceAccountToken = apc.Ptr(true)
			current.Spec.Template.Spec.ImagePullSecrets = nil

			changed, reason := adminclient.SyncDeployment(desired, current)

			Expect(changed).To(BeTrue())
			Expect(reason).To(ContainSubstring("serviceAccountName"))
			Expect(reason).To(ContainSubstring("automountServiceAccountToken"))
			Expect(reason).To(ContainSubstring("imagePullSecrets"))
			Expect(current.Spec.Template.Spec).To(Equal(desired.Spec.Template.Spec))
		})

		It("should project a profile-scoped service account token", func() {
			ais := baseAIS()
			config := adminclient.Config{SubjectTokenAudience: "token-service"}

			deployment := adminclient.NewClientDeployment(ais, config)
			podSpec := deployment.Spec.Template.Spec

			Expect(podSpec.ServiceAccountName).To(Equal(ais.AdminClientName()))
			Expect(podSpec.AutomountServiceAccountToken).To(HaveValue(BeFalse()))
			Expect(podSpec.Volumes).To(HaveLen(1))
			projection := podSpec.Volumes[0].Projected.Sources[0].ServiceAccountToken
			Expect(projection.Audience).To(Equal("token-service"))
			Expect(projection.Path).To(Equal("token"))
			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name: "auth-subject-token", MountPath: "/var/run/secrets/ais/auth", ReadOnly: true,
			}))
		})
	})

	Describe("ServiceAccount", func() {
		It("should use the admin client name and standard resource labels", func() {
			ais := baseAIS()
			serviceAccount := adminclient.ServiceAccount(ais)

			Expect(serviceAccount.Name).To(Equal(ais.AdminClientName()))
			Expect(serviceAccount.Labels).To(SatisfyAll(
				HaveKeyWithValue("app.kubernetes.io/name", ais.AdminClientName()),
				HaveKeyWithValue("app.kubernetes.io/component", "client"),
				HaveKeyWithValue("app.kubernetes.io/managed-by", "ais-operator"),
			))
		})
	})

	Describe("NewClientDeployment AuthN env", func() {
		It("should set AIS_AUTHN_URL from the resolved service URL", func() {
			ais := baseAIS()
			deploy := adminclient.NewClientDeployment(ais, adminclient.Config{ServiceURL: "https://authn.test:52001"})
			Expect(deploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "AIS_AUTHN_URL",
				Value: "https://authn.test:52001",
			}))
		})

		It("should not include authn env vars when no auth service is resolved", func() {
			ais := baseAIS()
			deploy := adminclient.NewClientDeployment(ais, adminclient.Config{})
			for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
				Expect(e.Name).NotTo(Equal("AIS_AUTHN_URL"))
			}
		})
	})
})
