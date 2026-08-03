/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authnReadyInterval   = 5 * time.Second
	authnUpdateTimeout   = 2 * time.Minute
	authnUpdateInterval  = 2 * time.Second
	authnDestroyTimeout  = 2 * time.Minute
	authnDestroyInterval = 2 * time.Second

	authnStableDuration = 15 * time.Second
	authnNodePortBase   = int32(30500)

	authnContainerName = "authn"
	authnTLSVolumeName = "tls-certs"
)

type authnServer struct {
	aisCfg    *tutils.AISTestCfg
	k8sClient *aisclient.K8sClient
	authn     *authv1alpha1.AIStoreAuth
	secret    *corev1.Secret
}

func newAuthNServer(aisCfg *tutils.AISTestCfg, k8sClient *aisclient.K8sClient, args *tutils.AuthNSpecArgs) *authnServer {
	return &authnServer{
		aisCfg:    aisCfg,
		k8sClient: k8sClient,
		authn:     tutils.NewAIStoreAuth(args),
		secret:    tutils.NewAuthNAdminSecret(args),
	}
}

// authnNodePort returns a NodePort reserved for the current parallel Ginkgo process.
func authnNodePort() int32 {
	return authnNodePortBase + int32(GinkgoParallelProcess())
}

func (as *authnServer) createCR(ctx context.Context) {
	By(fmt.Sprintf("Creating AIStoreAuth %q", as.authn.Name))
	Expect(as.k8sClient.Create(ctx, as.secret)).To(Succeed())
	Expect(as.k8sClient.Create(ctx, as.authn)).To(Succeed())
}

func (as *authnServer) create(ctx context.Context) {
	as.createCR(ctx)
	as.waitForReady(ctx)
}

func (as *authnServer) waitForReady(ctx context.Context) {
	tutils.WaitForAuthNToBeReady(ctx, as.k8sClient, as.namespacedName(), as.authn.Generation,
		as.aisCfg.GetClusterCreateTimeout(), authnReadyInterval)
}

func (as *authnServer) namespacedName() types.NamespacedName {
	return types.NamespacedName{Name: as.authn.Name, Namespace: as.authn.Namespace}
}

func (as *authnServer) fetchLatest(ctx context.Context) {
	authn, err := tutils.GetAIStoreAuthCR(ctx, as.k8sClient, as.namespacedName())
	Expect(err).To(BeNil())
	as.authn = authn
}

// patchSpec applies a spec mutation and waits for the operator to reconcile it to Ready.
func (as *authnServer) patchSpec(ctx context.Context, mutate func(spec *authv1alpha1.AIStoreAuthSpec)) {
	as.fetchLatest(ctx)
	patch := clientpkg.MergeFrom(as.authn.DeepCopy())
	mutate(&as.authn.Spec)
	Expect(as.k8sClient.Patch(ctx, as.authn, patch)).To(Succeed())
	as.waitForReady(ctx)
}

func (as *authnServer) getDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	return as.k8sClient.GetDeployment(ctx, authnres.DeploymentNSName(as.authn))
}

func (as *authnServer) getConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	return as.k8sClient.GetConfigMap(ctx, authnres.ConfigMapNSName(as.authn))
}

func (as *authnServer) deploymentGeneration(ctx context.Context) (int64, error) {
	deployment, err := as.getDeployment(ctx)
	if err != nil {
		return 0, err
	}
	return deployment.Generation, nil
}

// renderedConfig returns the AuthN config the operator rendered into the ConfigMap.
func (as *authnServer) renderedConfig(ctx context.Context) (string, error) {
	cm, err := as.getConfigMap(ctx)
	if err != nil {
		return "", err
	}
	return cm.Data[authnres.AuthnJSONKey], nil
}

// configChecksum returns the pod template annotation that rolls AuthN on config changes.
func (as *authnServer) configChecksum(ctx context.Context) (string, error) {
	deployment, err := as.getDeployment(ctx)
	if err != nil {
		return "", err
	}
	return deployment.Spec.Template.Annotations[authnres.ConfigChecksumAnnotation], nil
}

// tlsVolumeSecret returns the Secret backing the AuthN certificate volume, or "" when TLS is off.
func (as *authnServer) tlsVolumeSecret(ctx context.Context) (string, error) {
	deployment, err := as.getDeployment(ctx)
	if err != nil {
		return "", err
	}
	volumes := deployment.Spec.Template.Spec.Volumes
	for i := range volumes {
		if volumes[i].Name != authnTLSVolumeName {
			continue
		}
		if secret := volumes[i].Secret; secret != nil {
			return secret.SecretName, nil
		}
	}
	return "", nil
}

// pvcOwnerReferences returns the owner references of the AuthN data PVC, or nil when it is gone.
func (as *authnServer) pvcOwnerReferences(ctx context.Context) ([]metav1.OwnerReference, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := as.k8sClient.Get(ctx, authnres.PVCNSName(as.authn), pvc)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return pvc.OwnerReferences, nil
}

// podsMatchChecksum reports whether the running AuthN pods carry the given config checksum.
func (as *authnServer) podsMatchChecksum(ctx context.Context, checksum string) error {
	pods, err := as.listPods(ctx)
	if err != nil {
		return err
	}
	if len(pods.Items) != int(authnres.DeploymentReplicas) {
		return fmt.Errorf("found %d AuthN pods, want %d", len(pods.Items), authnres.DeploymentReplicas)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if got := pod.Annotations[authnres.ConfigChecksumAnnotation]; got != checksum {
			return fmt.Errorf("pod %s has config checksum %q, want %q", pod.Name, got, checksum)
		}
	}
	return nil
}

// listPods returns the AuthN pods, selected the way the managed Deployment selects them.
func (as *authnServer) listPods(ctx context.Context) (*corev1.PodList, error) {
	deployment, err := as.getDeployment(ctx)
	if err != nil {
		return nil, err
	}
	pods := &corev1.PodList{}
	err = as.k8sClient.List(ctx, pods,
		clientpkg.InNamespace(as.authn.Namespace),
		clientpkg.MatchingLabels(deployment.Spec.Selector.MatchLabels),
	)
	if err != nil {
		return nil, err
	}
	return pods, nil
}

// waitForResources waits for every resource the operator manages for a plain HTTP deployment.
func (as *authnServer) waitForResources(ctx context.Context) {
	timeout, interval := authnUpdateTimeout, authnUpdateInterval
	tutils.EventuallyCMExists(ctx, as.k8sClient, authnres.ConfigMapNSName(as.authn), BeTrue(), timeout, interval)
	tutils.EventuallyResourceExists(ctx, as.k8sClient, as.pvcRef(), BeTrue(), timeout, interval)
	tutils.EventuallyServiceExists(ctx, as.k8sClient, authnres.ServiceNSName(as.authn), BeTrue(), timeout, interval)
	tutils.EventuallyDeploymentExists(ctx, as.k8sClient, authnres.DeploymentNSName(as.authn), BeTrue(), timeout, interval)
}

// waitForResourceDeletion waits for the resources garbage collection removes with the CR.
func (as *authnServer) waitForResourceDeletion(ctx context.Context) {
	timeout, interval := authnDestroyTimeout, authnDestroyInterval
	tutils.EventuallyDeploymentExists(ctx, as.k8sClient, authnres.DeploymentNSName(as.authn), BeFalse(), timeout, interval)
	tutils.EventuallyServiceExists(ctx, as.k8sClient, authnres.ServiceNSName(as.authn), BeFalse(), timeout, interval)
	tutils.EventuallyCMExists(ctx, as.k8sClient, authnres.ConfigMapNSName(as.authn), BeFalse(), timeout, interval)
}

// pvcRef returns a reference to the AuthN data PVC for existence checks.
func (as *authnServer) pvcRef() *corev1.PersistentVolumeClaim {
	name := authnres.PVCNSName(as.authn)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
	}
}

// destroy deletes the AIStoreAuth resource and waits for finalizer-driven cleanup to finish.
func (as *authnServer) destroy(ctx context.Context) {
	By(fmt.Sprintf("Destroying AIStoreAuth %q", as.authn.Name))
	tutils.DestroyResource(ctx, as.k8sClient, as.authn, authnDestroyTimeout, authnDestroyInterval)
}

// destroyAndCleanup tears down everything the deployment created.
func (as *authnServer) destroyAndCleanup() {
	ctx := context.Background()
	as.destroy(ctx)
	_, err := as.k8sClient.DeleteResourceIfExists(ctx, as.pvcRef())
	Expect(err).To(BeNil())
	_, err = as.k8sClient.DeleteResourceIfExists(ctx, as.secret)
	Expect(err).To(BeNil())
}

// printLogs dumps the AuthN container logs.
func (as *authnServer) printLogs(ctx context.Context) {
	cs, err := tutils.NewClientset()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating clientset: %v\n", err)
		return
	}
	pods, err := as.listPods(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing pods for AIStoreAuth %s: %v\n", as.authn.Name, err)
		return
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		fmt.Printf("Logs for pod %s of AIStoreAuth %s:\n", pod.Name, as.authn.Name)
		if err = printPodLogs(ctx, cs, pod, authnContainerName); err != nil {
			fmt.Fprintf(os.Stderr, "error printing logs for pod %s: %v\n", pod.Name, err)
		}
		fmt.Println("---------------------------------------------------")
	}
}
