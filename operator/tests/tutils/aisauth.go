/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package tutils

import (
	"context"
	"fmt"
	"os"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	aisclient "github.com/ais-operator/internal/client"
	authnres "github.com/ais-operator/internal/resources/aisauth"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultAuthNImage = "docker.io/aistorage/authn:v4.8"

	authNAdminNameKey = "SU-NAME"
	authNAdminPassKey = "SU-PASS"

	authNAdminUser   = "admin"
	authNStorageSize = "256Mi"
	authNLogLevel    = int32(3)

	authNPortName = "http"
)

type AuthNSpecArgs struct {
	Name            string
	Namespace       string
	Image           string
	StorageClass    string
	AdminSecretName string
	AdminPassword   string
	TLS             *TLSArgs
	DeletionPolicy  authv1alpha1.PersistenceDeletionPolicy
}

func NewAuthNSpecArgs(testCfg *AISTestCfg, namespace string) *AuthNSpecArgs {
	name := "ais-authn-test-" + strings.ToLower(aiscos.CryptoRandS(6))
	return &AuthNSpecArgs{
		Name:            name,
		Namespace:       namespace,
		Image:           testCfg.AuthNImage,
		StorageClass:    testCfg.StateStorageClass,
		AdminSecretName: name + "-su-creds",
		AdminPassword:   aiscos.CryptoRandS(16),
	}
}

func NewAuthNAdminSecret(args *AuthNSpecArgs) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.AdminSecretName,
			Namespace: args.Namespace,
		},
		StringData: map[string]string{
			authNAdminNameKey: authNAdminUser,
			authNAdminPassKey: args.AdminPassword,
		},
	}
}

func NewAIStoreAuth(args *AuthNSpecArgs) *authv1alpha1.AIStoreAuth {
	storageSize := resource.MustParse(authNStorageSize)
	spec := authv1alpha1.AIStoreAuthSpec{
		AdminSecret: &corev1.LocalObjectReference{Name: args.AdminSecretName},
		Config: &authv1alpha1.ConfigSpec{
			Log: &authv1alpha1.LogSpec{Level: aisapc.Ptr(authNLogLevel)},
		},
		Persistence: authv1alpha1.PersistenceSpec{
			Size:           &storageSize,
			StorageClass:   aisapc.Ptr(args.StorageClass),
			DeletionPolicy: args.DeletionPolicy,
		},
		Deployment: authv1alpha1.DeploymentSpec{
			Container: authv1alpha1.ContainerSpec{
				Image:           args.Image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString(authNPortName)},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       5,
				},
			},
		},
	}
	if args.TLS != nil {
		spec.TLS = buildAuthNTLSSpec(args.TLS)
	}
	return &authv1alpha1.AIStoreAuth{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.Name,
			Namespace: args.Namespace,
		},
		Spec: spec,
	}
}

func buildAuthNTLSSpec(args *TLSArgs) *authv1alpha1.TLSSpec {
	if args.SecretName != "" {
		return &authv1alpha1.TLSSpec{SecretName: &args.SecretName}
	}
	kind := args.IssuerKind
	if kind == "" {
		kind = "ClusterIssuer"
	}
	mode := authv1alpha1.TLSCertificateModeSecret
	if args.Mode == "csi" {
		mode = authv1alpha1.TLSCertificateModeCSI
	}
	return &authv1alpha1.TLSSpec{
		Certificate: &authv1alpha1.TLSCertificateConfig{
			IssuerRef: authv1alpha1.CertIssuerRef{
				Name: args.IssuerName,
				Kind: kind,
			},
			Mode: mode,
		},
	}
}

// GetAIStoreAuthCR fetches an AIStoreAuth resource by name.
func GetAIStoreAuthCR(ctx context.Context, client *aisclient.K8sClient,
	name types.NamespacedName,
) (*authv1alpha1.AIStoreAuth, error) {
	authn := &authv1alpha1.AIStoreAuth{}
	if err := client.Get(ctx, name, authn); err != nil {
		return nil, err
	}
	return authn, nil
}

func authNReadyCondition(authn *authv1alpha1.AIStoreAuth) *metav1.Condition {
	return meta.FindStatusCondition(authn.Status.Conditions, string(authv1alpha1.ConditionReady))
}

func WaitForAuthNToBeReady(ctx context.Context, client *aisclient.K8sClient,
	name types.NamespacedName, generation int64, intervals ...interface{},
) {
	Eventually(func(ctx context.Context) error {
		authn, err := GetAIStoreAuthCR(ctx, client, name)
		if err != nil {
			return err
		}
		cond := authNReadyCondition(authn)
		switch {
		case cond == nil:
			return fmt.Errorf("AIStoreAuth %q has no Ready condition yet", name)
		case cond.ObservedGeneration < generation:
			return fmt.Errorf("AIStoreAuth %q Ready condition is stale: observed generation %d, want %d",
				name, cond.ObservedGeneration, generation)
		case cond.Status != metav1.ConditionTrue:
			return fmt.Errorf("AIStoreAuth %q is not Ready: %s: %s", name, cond.Reason, cond.Message)
		}
		return nil
	}, intervals...).WithContext(ctx).Should(Succeed())
}

// cleanupAuthN destroys AIStoreAuth resources left behind in a test namespace.
func cleanupAuthN(ctx context.Context, c *aisclient.K8sClient, namespace string) {
	authns := &authv1alpha1.AIStoreAuthList{}
	if err := c.List(ctx, authns, clientpkg.InNamespace(namespace)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch existing AIStoreAuth resources in namespace %q; err %v\n", namespace, err)
		return
	}
	for i := range authns.Items {
		authn := &authns.Items[i]
		fmt.Fprintf(os.Stdout, "Destroying old AIStoreAuth '%s' in namespace '%s'\n", authn.Name, namespace)
		DestroyResource(ctx, c, authn)
		// Delete the admin Secret and any retained PVC explicitly (not swept by the CR finalizer).
		if authn.Spec.AdminSecret != nil {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      authn.Spec.AdminSecret.Name,
					Namespace: authn.Namespace,
				},
			}
			if _, err := c.DeleteResourceIfExists(ctx, secret); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete admin Secret %q for AIStoreAuth %q: %v\n",
					authn.Spec.AdminSecret.Name, authn.Name, err)
			}
		}
		pvcRef := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      authnres.PVCNSName(authn).Name,
				Namespace: authn.Namespace,
			},
		}
		if _, err := c.DeleteResourceIfExists(ctx, pvcRef); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete PVC for AIStoreAuth %q: %v\n", authn.Name, err)
		}
	}
}
