/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */

package v1beta1

import (
	"context"
	"testing"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// runTolerationUpdateScenarios exercises add/remove/modify toleration paths for proxy or target updates.
func runTolerationUpdateScenarios(
	t *testing.T,
	component string,
	validate func(prev, ais *AIStore) error,
	setTolerations func(a *AIStore, tols []corev1.Toleration),
) {
	t.Helper()

	toleration := corev1.Toleration{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	t.Run("adding toleration to "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &AIStore{}
		ais := &AIStore{}
		setTolerations(ais, []corev1.Toleration{toleration})
		g.Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("removing toleration from "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &AIStore{}
		g.Expect(validate(prev, ais)).To(Succeed())
	})

	t.Run("modifying toleration in "+component+" spec is allowed", func(subT *testing.T) {
		g := NewWithT(subT)
		prev := &AIStore{}
		setTolerations(prev, []corev1.Toleration{toleration})
		ais := &AIStore{}
		modified := toleration
		modified.Effect = corev1.TaintEffectNoExecute
		setTolerations(ais, []corev1.Toleration{modified})
		g.Expect(validate(prev, ais)).To(Succeed())
	})
}

func TestUsesStateEmptyDir(t *testing.T) {
	tests := []struct {
		name     string
		emptyDir *StateEmptyDirConfig
		expected bool
	}{
		{"nil returns false", nil, false},
		{"set returns true", &StateEmptyDirConfig{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = &StateStorage{EmptyDir: tt.emptyDir}
			Expect(ais.Spec.UsesStateEmptyDir()).To(Equal(tt.expected))
		})
	}
}

func TestValidateStateStorage(t *testing.T) {
	tests := []struct {
		name           string
		stateStorage   *StateStorage
		hostpathPrefix *string
		storageClass   *string
		wantErr        bool
		wantWarning    bool
	}{
		{
			name:         "only emptyDir is valid",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
		},
		{
			name:         "only hostPath is valid",
			stateStorage: &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
		},
		{
			name:         "only pvc is valid",
			stateStorage: &StateStorage{PVC: &StatePVCConfig{StorageClass: "my-sc"}},
		},
		{
			name:           "stateStorage and legacy hostpathPrefix emits warning",
			stateStorage:   &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			hostpathPrefix: aisapc.Ptr("/mnt"),
			wantWarning:    true,
		},
		{
			name:           "hostpathPrefix and stateStorageClass emits legacy warning",
			hostpathPrefix: aisapc.Ptr("/mnt"),
			storageClass:   aisapc.Ptr("my-sc"),
			wantWarning:    true,
		},
		{
			name:         "emptyDir and hostPath errors",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}, HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			wantErr:      true,
		},
		{
			name:         "emptyDir and pvc errors",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}, PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			wantErr:      true,
		},
		{
			name:    "none set errors",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = tt.stateStorage
			ais.Spec.HostpathPrefix = tt.hostpathPrefix
			ais.Spec.StateStorageClass = tt.storageClass
			warns, err := ais.validateStateStorage()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			if tt.wantWarning {
				Expect(warns).ToNot(BeEmpty())
			} else {
				Expect(warns).To(BeEmpty())
			}
		})
	}
}

func TestValidateShutdownWithEmptyDir(t *testing.T) {
	tests := []struct {
		name            string
		stateStorage    *StateStorage
		shutdownCluster *bool
		wantErr         bool
	}{
		{
			name:            "emptyDir with shutdown enabled errors",
			stateStorage:    &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
			shutdownCluster: aisapc.Ptr(true),
			wantErr:         true,
		},
		{
			name:            "emptyDir with shutdown disabled is valid",
			stateStorage:    &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
			shutdownCluster: aisapc.Ptr(false),
		},
		{
			name:         "emptyDir with shutdown nil is valid",
			stateStorage: &StateStorage{EmptyDir: &StateEmptyDirConfig{}},
		},
		{
			name:            "hostPath with shutdown enabled is valid",
			stateStorage:    &StateStorage{HostPath: &StateHostPathConfig{Prefix: "/mnt"}},
			shutdownCluster: aisapc.Ptr(true),
		},
		{
			name:            "pvc with shutdown enabled is valid",
			stateStorage:    &StateStorage{PVC: &StatePVCConfig{StorageClass: "my-sc"}},
			shutdownCluster: aisapc.Ptr(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ais := &AIStore{}
			ais.Spec.StateStorage = tt.stateStorage
			ais.Spec.ShutdownCluster = tt.shutdownCluster
			_, err := ais.validateShutdownWithEmptyDir()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateProxyUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Proxy, validateProxyUpdate, func(a *AIStore, tols []corev1.Toleration) {
		a.Spec.ProxySpec.Tolerations = tols
	})
}

func TestValidateTargetUpdateTolerations(t *testing.T) {
	runTolerationUpdateScenarios(t, aisapc.Target, validateTargetUpdate, func(a *AIStore, tols []corev1.Toleration) {
		a.Spec.TargetSpec.Tolerations = tols
	})
}

func TestValidateTargetUpdateToScaleDownMode(t *testing.T) {
	g := NewWithT(t)
	prev := &AIStore{}
	ais := &AIStore{}
	ais.Spec.TargetSpec.ScaleDownMode = ScaleDownModeRetain
	g.Expect(validateTargetUpdate(prev, ais)).To(Succeed())
}

func sarInterceptor(allowed bool) interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			sar, ok := obj.(*authorizationv1.SubjectAccessReview)
			if !ok {
				return nil
			}
			sar.Status.Allowed = allowed
			sar.Status.Denied = !allowed
			return nil
		},
	}
}

func newSARWebhook(t *testing.T, allowed bool) *AIStoreWebhook {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := authorizationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add authorization scheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(sarInterceptor(allowed)).Build()
	return &AIStoreWebhook{Client: c}
}

func TestShouldVerifyAuthSecret(t *testing.T) {
	const tenantNS = "tenant"

	authAIS := func(secretName string, secretNS *string) *AIStore {
		ais := &AIStore{}
		ais.Namespace = tenantNS
		ais.Spec.Auth = &AuthSpec{
			UsernamePassword: &UsernamePasswordAuth{
				SecretName:      secretName,
				SecretNamespace: secretNS,
			},
		}
		return ais
	}

	for _, tt := range []struct {
		name string
		prev *AIStore
		ais  *AIStore
		want bool
	}{
		{
			name: "no auth on create",
			ais:  &AIStore{},
			want: false,
		},
		{
			name: "auth on create",
			ais:  authAIS("creds", aisapc.Ptr(tenantNS)),
			want: true,
		},
		{
			name: "auth added on update",
			prev: &AIStore{},
			ais:  authAIS("creds", aisapc.Ptr(tenantNS)),
			want: true,
		},
		{
			name: "auth removed on update",
			prev: authAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  &AIStore{},
			want: false,
		},
		{
			name: "unchanged secret ref on update",
			prev: authAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  authAIS("creds", aisapc.Ptr(tenantNS)),
			want: false,
		},
		{
			name: "changed secret name on update",
			prev: authAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  authAIS("other-creds", aisapc.Ptr(tenantNS)),
			want: true,
		},
		{
			name: "changed secret namespace on update",
			prev: authAIS("creds", aisapc.Ptr(tenantNS)),
			ais:  authAIS("creds", aisapc.Ptr("other")),
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(shouldVerifyAuthSecret(tt.prev, tt.ais)).To(Equal(tt.want))
		})
	}
}

func TestValidateAuthSecretAccess(t *testing.T) {
	const tenantNS = "tenant"
	ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{Username: "alice"},
		},
	})

	authAIS := func() *AIStore {
		ais := &AIStore{}
		ais.Namespace = tenantNS
		ais.Spec.Auth = &AuthSpec{
			UsernamePassword: &UsernamePasswordAuth{
				SecretName:      "creds",
				SecretNamespace: aisapc.Ptr(tenantNS),
			},
		}
		return ais
	}

	t.Run("secret is allowed when authorized", func(t *testing.T) {
		g := NewWithT(t)
		_, err := newSARWebhook(t, true).validateAuthSecretAccess(ctx, authAIS(), nil)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("secret is rejected when unauthorized", func(t *testing.T) {
		g := NewWithT(t)
		_, err := newSARWebhook(t, false).validateAuthSecretAccess(ctx, authAIS(), nil)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("changed secret ref on update requires SAR", func(t *testing.T) {
		g := NewWithT(t)
		prev := authAIS()
		ais := prev.DeepCopy()
		ais.Spec.Auth.UsernamePassword.SecretName = "other-creds"
		_, err := newSARWebhook(t, false).validateAuthSecretAccess(ctx, ais, prev)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestAIStoreValidateSize(t *testing.T) {
	tests := []struct {
		name       string // description of this test case
		want       admission.Warnings
		wantErr    bool
		proxySize  *int32
		targetSize *int32
		size       *int32
	}{
		{
			"Proxy size is -1 thus proxy autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](-1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"target size is -1 thus target autoscaling is true",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
			nil,
		},
		{
			" size is -1 thus autoscaling is true",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](-1),
		},
		{
			"autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-1),
		},
		{
			"not autoscaling",
			nil,
			false,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"not autoscaling with just size",
			nil,
			false,
			nil,
			nil,
			aisapc.Ptr[int32](1),
		},
		{
			"invalid size",
			nil,
			true,
			nil,
			nil,
			aisapc.Ptr[int32](-2),
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](-2),
			nil,
		},
		{
			"invalid proxy size",
			nil,
			true,
			aisapc.Ptr[int32](-2),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid proxy size;0",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](1),
			nil,
		},
		{
			"invalid target size;0",
			nil,
			true,
			aisapc.Ptr[int32](1),
			aisapc.Ptr[int32](0),
			nil,
		},
		{
			"invalid target size",
			nil,
			true,
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
			aisapc.Ptr[int32](0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var ais AIStore
			ais.Spec.ProxySpec.Size = tt.proxySize
			ais.Spec.TargetSpec.Size = tt.targetSize
			ais.Spec.Size = tt.size
			got, gotErr := ais.validateSize()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("validateSize() failed: %v for test %s", gotErr, tt.name)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("validateSize() succeeded unexpectedly")
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
