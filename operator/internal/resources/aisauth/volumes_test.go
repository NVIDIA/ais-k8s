/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	"context"
	"time"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	csiapisv1 "github.com/cert-manager/csi-driver/pkg/apis/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

var _ = Describe("Volumes", func() {
	var authn *authv1alpha1.AIStoreAuth

	BeforeEach(func() {
		authn = &authv1alpha1.AIStoreAuth{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ais-authn",
				Namespace: "ais",
				UID:       types.UID("test-uid"),
			},
			Spec: authv1alpha1.AIStoreAuthSpec{
				Deployment: authv1alpha1.DeploymentSpec{
					Container: authv1alpha1.ContainerSpec{
						Image: "docker.io/aistorage/authn:v4.8",
					},
				},
			},
		}
	})

	It("mounts the data PVC and startup config", func() {
		spec := newPodSpec(authn)

		Expect(spec.Volumes).To(HaveLen(2))
		Expect(spec.Volumes[0].Name).To(HaveValue(Equal("storage")))
		Expect(spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(HaveValue(Equal("ais-authn-storage")))
		Expect(spec.Volumes[1].Name).To(HaveValue(Equal("config")))
		Expect(spec.Volumes[1].ConfigMap.Name).To(HaveValue(Equal("ais-authn-config")))

		mounts := spec.Containers[0].VolumeMounts
		Expect(mounts).To(HaveLen(2))
		Expect(mounts[0].MountPath).To(HaveValue(Equal("/etc/ais/authn")))
		Expect(mounts[1].MountPath).To(HaveValue(Equal("/etc/ais/authn/authn.json")))
		Expect(mounts[1].SubPath).To(HaveValue(Equal("authn.json")))
		Expect(mounts[1].ReadOnly).To(HaveValue(BeTrue()))
	})

	It("mounts an existing TLS Secret", func() {
		secretName := "authn-tls" //nolint:gosec // Secret name, not a credential.
		authn.Spec.TLS = &authv1alpha1.TLSSpec{SecretName: &secretName}
		spec := newPodSpec(authn)

		Expect(spec.Volumes).To(HaveLen(3))
		Expect(spec.Volumes[2].Name).To(HaveValue(Equal("tls-certs")))
		Expect(spec.Volumes[2].Secret.SecretName).To(HaveValue(Equal(secretName)))
		Expect(spec.Containers[0].VolumeMounts[2].MountPath).To(HaveValue(Equal("/var/certs")))
		Expect(spec.Containers[0].VolumeMounts[2].ReadOnly).To(HaveValue(BeTrue()))
	})

	It("mounts the Secret created by an operator-managed Certificate", func() {
		authn.Spec.TLS = &authv1alpha1.TLSSpec{
			Certificate: &authv1alpha1.TLSCertificateConfig{
				IssuerRef: authv1alpha1.CertIssuerRef{Name: "issuer"},
			},
		}
		spec := newPodSpec(authn)
		Expect(spec.Volumes[2].Secret.SecretName).To(HaveValue(Equal("ais-authn-authn-tls")))
	})

	It("requests a per-pod certificate from the cert-manager CSI driver", func() {
		duration := metav1.Duration{Duration: 90 * 24 * time.Hour}
		renewBefore := metav1.Duration{Duration: 15 * 24 * time.Hour}
		externalURL := "https://authn.example.com:52001"
		authn.Spec.Config = &authv1alpha1.ConfigSpec{
			Net: &authv1alpha1.NetSpec{ExternalURL: &externalURL},
		}
		authn.Spec.TLS = &authv1alpha1.TLSSpec{
			Certificate: &authv1alpha1.TLSCertificateConfig{
				IssuerRef:          authv1alpha1.CertIssuerRef{Name: "issuer", Kind: "Issuer"},
				AdditionalDNSNames: []string{"extra.example.com"},
				Duration:           &duration,
				RenewBefore:        &renewBefore,
				Mode:               authv1alpha1.TLSCertificateModeCSI,
			},
		}

		spec := newPodSpec(authn)
		volume := spec.Volumes[2]
		Expect(volume.Secret).To(BeNil())
		Expect(volume.CSI.Driver).To(HaveValue(Equal("csi.cert-manager.io")))
		Expect(volume.CSI.ReadOnly).To(HaveValue(BeTrue()))
		Expect(volume.CSI.VolumeAttributes).To(Equal(map[string]string{
			csiapisv1.IssuerNameKey:  "issuer",
			csiapisv1.IssuerKindKey:  "Issuer",
			csiapisv1.CommonNameKey:  "ais-authn.ais",
			csiapisv1.DNSNamesKey:    "ais-authn,ais-authn.ais,ais-authn.ais.svc,ais-authn.ais.svc.cluster.local,authn.example.com,extra.example.com,localhost",
			csiapisv1.DurationKey:    "2160h0m0s",
			csiapisv1.RenewBeforeKey: "360h0m0s",
			csiapisv1.KeyUsagesKey:   "digital signature,key encipherment,server auth",
		}))
		Expect(spec.Containers[0].VolumeMounts[2].MountPath).To(HaveValue(Equal("/var/certs")))
		Expect(spec.Containers[0].VolumeMounts[2].ReadOnly).To(HaveValue(BeTrue()))
	})
})

func newPodSpec(authn *authv1alpha1.AIStoreAuth) corev1ac.PodSpecApplyConfiguration {
	GinkgoHelper()
	deployment, err := authnres.NewDeployment(context.Background(), authn)
	Expect(err).NotTo(HaveOccurred())
	return *deployment.Spec.Template.Spec
}
