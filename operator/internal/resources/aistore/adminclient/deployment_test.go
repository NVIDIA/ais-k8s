/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package adminclient

import (
	"context"
	"crypto/tls"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	"github.com/ais-operator/internal/services"
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

			deployment := NewClientDeployment(ais, nil)
			podSpec := deployment.Spec.Template.Spec

			Expect(podSpec.ServiceAccountName).To(Equal("default"))
			Expect(podSpec.AutomountServiceAccountToken).To(HaveValue(BeFalse()))
			Expect(podSpec.ImagePullSecrets).To(Equal(ais.Spec.ImagePullSecrets))
		})

		It("should reconcile service account security settings", func() {
			ais := baseAIS()
			ais.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "registry-creds"}}
			desired := NewClientDeployment(ais, nil)
			current := desired.DeepCopy()
			current.Spec.Template.Spec.ServiceAccountName = "test-ais-sa"
			current.Spec.Template.Spec.AutomountServiceAccountToken = apc.Ptr(true)
			current.Spec.Template.Spec.ImagePullSecrets = nil

			changed, reason := SyncDeployment(desired, current)

			Expect(changed).To(BeTrue())
			Expect(reason).To(ContainSubstring("serviceAccountName"))
			Expect(reason).To(ContainSubstring("automountServiceAccountToken"))
			Expect(reason).To(ContainSubstring("imagePullSecrets"))
			Expect(current.Spec.Template.Spec).To(Equal(desired.Spec.Template.Spec))
		})
	})

	Describe("NewClientDeployment AuthN env", func() {
		It("should set AIS_AUTHN_URL from the resolved service URL", func() {
			ais := baseAIS()
			deploy := NewClientDeployment(ais, &fakeAuthConfig{serviceURL: "https://authn.test:52001"})
			Expect(deploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "AIS_AUTHN_URL",
				Value: "https://authn.test:52001",
			}))
		})

		It("should not include authn env vars when auth is nil", func() {
			ais := baseAIS()
			deploy := NewClientDeployment(ais, nil)
			for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
				Expect(e.Name).NotTo(Equal("AIS_AUTHN_URL"))
			}
		})
	})
})

// fakeAuthConfig is a test double for services.AuthConfig.
type fakeAuthConfig struct {
	serviceURL string
}

func (f *fakeAuthConfig) GetServiceURL() string                     { return f.serviceURL }
func (*fakeAuthConfig) IsTokenExchange() bool                       { return false }
func (*fakeAuthConfig) GetTokenPath() string                        { return "" }
func (*fakeAuthConfig) GetSubjectTokenAudience() string             { return "" }
func (*fakeAuthConfig) GetTokenExchangeEndpoint() string            { return "" }
func (*fakeAuthConfig) GetOAuthLoginConf() *services.OAuthLoginConf { return nil }
func (*fakeAuthConfig) GetSecretName() string                       { return "" }
func (*fakeAuthConfig) GetSecretNamespace() string                  { return "" }
func (*fakeAuthConfig) GetUserKey() string                          { return "" }
func (*fakeAuthConfig) GetPassKey() string                          { return "" }

func (*fakeAuthConfig) GetTLSConfig(context.Context) (*tls.Config, error) { return nil, nil }
