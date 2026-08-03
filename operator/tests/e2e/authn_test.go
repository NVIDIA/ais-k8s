/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package e2e

import (
	"context"
	"fmt"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"github.com/ais-operator/tests/tutils"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Run AIStoreAuth Controller", func() {
	var authnArgs *tutils.AuthNSpecArgs

	BeforeEach(func() {
		authnArgs = tutils.NewAuthNSpecArgs(AISTestCfg, WorkerCfg.TestNSName)
	})

	Context("Deploy and reconfigure", Ordered, func() {
		var as *authnServer

		BeforeAll(func(ctx context.Context) {
			as = newAuthNServer(AISTestCfg, WorkerCfg.K8sClient, authnArgs)
			as.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			as.printLogs(ctx)
			as.destroyAndCleanup()
		})

		It("Should create every managed resource and publish the in-cluster service URL", func(ctx context.Context) {
			as.waitForResources(ctx)

			By("Verifying the AuthN Deployment runs a single replica of the configured image")
			deployment, err := as.getDeployment(ctx)
			Expect(err).To(BeNil())
			Expect(deployment.Spec.Replicas).To(HaveValue(Equal(authnres.DeploymentReplicas)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(AISTestCfg.AuthNImage))

			By("Verifying the AuthN data PVC is bound")
			Eventually(func(ctx context.Context) (corev1.PersistentVolumeClaimPhase, error) {
				pvc := &corev1.PersistentVolumeClaim{}
				if err := as.k8sClient.Get(ctx, authnres.PVCNSName(as.authn), pvc); err != nil {
					return "", err
				}
				return pvc.Status.Phase, nil
			}, authnUpdateTimeout, authnUpdateInterval).WithContext(ctx).Should(Equal(corev1.ClaimBound))

			By("Verifying the published status")
			as.fetchLatest(ctx)
			Expect(as.authn.Status.ServiceURL).To(Equal(
				fmt.Sprintf("http://%s.%s.svc:%d", as.authn.Name, as.authn.Namespace, as.authn.ListenPort())))
			Expect(as.authn.Status.ObservedGeneration).To(Equal(as.authn.Generation))
		})

		It("Should restore an out-of-band edit of the rendered config without rolling the pod", func(ctx context.Context) {
			generation, err := as.deploymentGeneration(ctx)
			Expect(err).To(BeNil())
			cm, err := as.getConfigMap(ctx)
			Expect(err).To(BeNil())
			rendered := cm.Data[authnres.AuthnJSONKey]
			Expect(rendered).NotTo(BeEmpty())

			By("Overwriting the rendered AuthN config out of band")
			patch := clientpkg.MergeFrom(cm.DeepCopy())
			cm.Data[authnres.AuthnJSONKey] = "{}"
			Expect(as.k8sClient.Patch(ctx, cm, patch)).To(Succeed())

			By("Waiting for the operator to restore the rendered config")
			Eventually(as.renderedConfig, authnUpdateTimeout, authnUpdateInterval).
				WithContext(ctx).Should(Equal(rendered))

			By("Verifying restoring the config did not roll the AuthN Deployment")
			Consistently(as.deploymentGeneration, authnStableDuration, authnUpdateInterval).
				WithContext(ctx).Should(Equal(generation))
		})

		It("Should roll the AuthN pod when the rendered config changes", func(ctx context.Context) {
			checksum, err := as.configChecksum(ctx)
			Expect(err).To(BeNil())
			Expect(checksum).NotTo(BeEmpty())

			By("Raising the AuthN log level")
			as.patchSpec(ctx, func(spec *authv1alpha1.AIStoreAuthSpec) {
				spec.Config.Log.Level = aisapc.Ptr(int32(4))
			})

			By("Verifying the pod template picked up a new config checksum")
			Eventually(as.configChecksum, authnUpdateTimeout, authnUpdateInterval).
				WithContext(ctx).ShouldNot(Equal(checksum))

			By("Verifying every AuthN pod rolled onto the new config checksum")
			newChecksum, err := as.configChecksum(ctx)
			Expect(err).To(BeNil())
			Expect(newChecksum).NotTo(Equal(checksum))
			Eventually(func(ctx context.Context) error {
				return as.podsMatchChecksum(ctx, newChecksum)
			}, authnUpdateTimeout, authnUpdateInterval).WithContext(ctx).Should(Succeed())
		})

		It("Should add and remove the NodePort Service as external access is toggled", func(ctx context.Context) {
			svcName := authnres.NodePortServiceNSName(as.authn)

			By("Enabling external access")
			nodePort := authnNodePort()
			as.patchSpec(ctx, func(spec *authv1alpha1.AIStoreAuthSpec) {
				spec.ExternalAccess = &authv1alpha1.ExternalAccessSpec{
					NodePort: &authv1alpha1.NodePortSpec{Port: nodePort},
				}
			})

			tutils.EventuallyServiceExists(ctx, as.k8sClient, svcName, BeTrue(), authnUpdateTimeout, authnUpdateInterval)
			svc, err := as.k8sClient.GetService(ctx, svcName)
			Expect(err).To(BeNil())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].NodePort).To(Equal(nodePort))

			By("Disabling external access again")
			as.patchSpec(ctx, func(spec *authv1alpha1.AIStoreAuthSpec) {
				spec.ExternalAccess = nil
			})

			tutils.EventuallyServiceExists(ctx, as.k8sClient, svcName,
				BeFalse(), authnDestroyTimeout, authnDestroyInterval)
			By("Verifying the in-cluster Service is left in place")
			tutils.EventuallyServiceExists(ctx, as.k8sClient, authnres.ServiceNSName(as.authn),
				BeTrue(), authnUpdateTimeout, authnUpdateInterval)
		})
	})

	Context("TLS Certificate", Ordered, func() {
		var (
			issuer *certmanagerv1.Issuer
			as     *authnServer
		)

		BeforeAll(func(ctx context.Context) {
			issuer = &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-authn-selfsigned-issuer",
					Namespace: authnArgs.Namespace,
				},
				Spec: certmanagerv1.IssuerSpec{
					IssuerConfig: certmanagerv1.IssuerConfig{
						SelfSigned: &certmanagerv1.SelfSignedIssuer{},
					},
				},
			}
			_, err := WorkerCfg.K8sClient.CreateResourceIfNotExists(ctx, nil, issuer)
			Expect(err).To(BeNil())

			authnArgs.TLS = &tutils.TLSArgs{IssuerName: issuer.Name, IssuerKind: "Issuer"}
			as = newAuthNServer(AISTestCfg, WorkerCfg.K8sClient, authnArgs)
			as.create(ctx)
		})

		AfterAll(func(ctx context.Context) {
			as.printLogs(ctx)
			as.destroyAndCleanup()
			_, err := WorkerCfg.K8sClient.DeleteResourceIfExists(ctx, issuer)
			Expect(err).To(BeNil())
		})

		It("Should serve AuthN over TLS from an operator-managed certificate", func(ctx context.Context) {
			By("Verifying the managed Certificate and the Secret cert-manager issues for it exist")
			tutils.EventuallyResourceExists(ctx, as.k8sClient, authnres.TLSCertificate(as.authn),
				BeTrue(), authnUpdateTimeout, authnUpdateInterval)
			secretName := types.NamespacedName{
				Name:      as.authn.GetTLSSecretName(),
				Namespace: as.authn.Namespace,
			}
			tutils.EventuallySecretExists(ctx, as.k8sClient, secretName, BeTrue(), authnUpdateTimeout, authnUpdateInterval)

			By("Verifying the AuthN pod mounts the issued certificate")
			Eventually(as.tlsVolumeSecret, authnUpdateTimeout, authnUpdateInterval).
				WithContext(ctx).Should(Equal(secretName.Name))

			By("Verifying the published service URL switched to https")
			as.fetchLatest(ctx)
			Expect(as.authn.Status.ServiceURL).To(Equal(
				fmt.Sprintf("https://%s.%s.svc:%d", as.authn.Name, as.authn.Namespace, as.authn.ListenPort())))
		})

		It("Should remove the managed Certificate when TLS is disabled", func(ctx context.Context) {
			as.patchSpec(ctx, func(spec *authv1alpha1.AIStoreAuthSpec) {
				spec.TLS = nil
			})

			tutils.EventuallyResourceExists(ctx, as.k8sClient, authnres.TLSCertificate(as.authn),
				BeFalse(), authnDestroyTimeout, authnDestroyInterval)

			By("Verifying the AuthN pod no longer mounts a certificate")
			Eventually(as.tlsVolumeSecret, authnUpdateTimeout, authnUpdateInterval).
				WithContext(ctx).Should(BeEmpty())
			as.fetchLatest(ctx)
			Expect(as.authn.Status.ServiceURL).To(HavePrefix("http://"))
		})
	})

	Context("Destroy", func() {
		DescribeTable("AuthN data PVC on deletion",
			func(ctx context.Context, policy authv1alpha1.PersistenceDeletionPolicy, retained bool) {
				authnArgs.DeletionPolicy = policy
				as := newAuthNServer(AISTestCfg, WorkerCfg.K8sClient, authnArgs)
				defer as.destroyAndCleanup()

				as.createCR(ctx)
				tutils.EventuallyResourceExists(ctx, as.k8sClient, as.pvcRef(), BeTrue(),
					authnUpdateTimeout, authnUpdateInterval)

				as.destroy(ctx)

				By("Verifying owned resources are garbage collected")
				as.waitForResourceDeletion(ctx)

				By("Verifying the AuthN data PVC honors the deletion policy")
				tutils.EventuallyResourceExists(ctx, as.k8sClient, as.pvcRef(), Equal(retained),
					authnDestroyTimeout, authnDestroyInterval)

				if retained {
					By("Verifying the surviving PVC was released, so garbage collection cannot claim it later")
					Eventually(as.pvcOwnerReferences, authnDestroyTimeout, authnDestroyInterval).
						WithContext(ctx).Should(BeEmpty())
				}
			},
			Entry("is retained and released by default", authv1alpha1.PersistenceRetain, true),
			Entry("is deleted when deletionPolicy is Delete", authv1alpha1.PersistenceDelete, false),
		)
	})
})
