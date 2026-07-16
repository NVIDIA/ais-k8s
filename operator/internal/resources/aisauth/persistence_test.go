/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Persistence", func() {
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
						Image: "docker.io/aistorage/authn:v4.5",
					},
				},
			},
		}
	})

	It("names the PVC after the CR", func() {
		Expect(authnres.PVCName(authn)).To(Equal("ais-authn-storage"))
		Expect(authnres.PVCNSName(authn)).To(Equal(types.NamespacedName{
			Name: "ais-authn-storage", Namespace: "ais",
		}))
	})

	Describe("PVC", func() {
		It("requests the default size with standard labels and owner reference", func() {
			sc := "openebs-hostpath"
			authn.Spec.Persistence.StorageClass = &sc

			pvc, err := authnres.NewPVC(authn)
			Expect(err).NotTo(HaveOccurred())

			Expect(pvc.Labels).To(Equal(map[string]string{
				"app.kubernetes.io/name":       "authn",
				"app.kubernetes.io/instance":   "ais-authn",
				"app.kubernetes.io/managed-by": "ais-operator",
			}))
			Expect(pvc.OwnerReferences).To(HaveLen(1))
			Expect(*pvc.OwnerReferences[0].Name).To(Equal(authn.Name))
			Expect(*pvc.OwnerReferences[0].Controller).To(BeTrue())

			Expect(pvc.Spec.AccessModes).To(Equal([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}))
			req := (*pvc.Spec.Resources.Requests)[corev1.ResourceStorage]
			Expect(req.Cmp(resource.MustParse("256Mi"))).To(Equal(0))
		})

		It("honors an explicit requested size", func() {
			size := resource.MustParse("1Gi")
			sc := "openebs-hostpath"
			authn.Spec.Persistence.Size = &size
			authn.Spec.Persistence.StorageClass = &sc

			pvc, err := authnres.NewPVC(authn)
			Expect(err).NotTo(HaveOccurred())
			req := (*pvc.Spec.Resources.Requests)[corev1.ResourceStorage]
			Expect(req.Cmp(size)).To(Equal(0))
		})

		It("uses the StorageClass for dynamic provisioning", func() {
			sc := "openebs-hostpath"
			authn.Spec.Persistence.StorageClass = &sc

			pvc, err := authnres.NewPVC(authn)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal("openebs-hostpath")))
			Expect(pvc.Spec.VolumeName).To(BeNil())
		})

		It("binds an existing volume by name and opts out of dynamic provisioning", func() {
			vol := "existing-pv"
			authn.Spec.Persistence.VolumeName = &vol

			pvc, err := authnres.NewPVC(authn)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.VolumeName).To(HaveValue(Equal("existing-pv")))
			Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal("")))
		})

		It("rejects persistence with no mode selected", func() {
			_, err := authnres.NewPVC(authn)
			Expect(err).To(MatchError("spec.persistence must set exactly one of storageClass or volumeName"))
		})
	})
})
