/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */

package client

import (
	"context"
	"time"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("K8sClient", func() {
	Describe("CreateOrUpdateResource", func() {
		var (
			c         client.Client
			k8sClient *K8sClient
			ais       *aisv1.AIStore
			ns        *corev1.Namespace

			ctx = context.TODO()
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient = NewClient(c, c.Scheme())
			Expect(k8sClient).NotTo(BeNil())

			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "somenamespace"}}
			err := c.Create(ctx, ns)
			Expect(err).NotTo(HaveOccurred())

			ais = &aisv1.AIStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ais",
					Namespace: ns.GetName(),
				},
				Spec: aisv1.AIStoreSpec{
					ProxySpec: aisv1.DaemonSpec{
						Size: apc.Ptr[int32](1),
					},
					TargetSpec: aisv1.TargetSpec{
						DaemonSpec: aisv1.DaemonSpec{
							Size: apc.Ptr[int32](1),
						},
					},
					StateStorage: &aisv1.StateStorage{
						HostPath: &aisv1.StateHostPathConfig{Prefix: "/ais"},
					},
				},
			}
			err = c.Create(ctx, ais)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create an object if not exists", func() {
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update the resource with diff", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			}
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm)
			Expect(err).NotTo(HaveOccurred())

			updatedCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore-updated"},
			}
			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, updatedCM)
			Expect(err).NotTo(HaveOccurred())

			fetchCM := &corev1.ConfigMap{}
			err = c.Get(ctx, client.ObjectKeyFromObject(cm), fetchCM)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchCM.Data).To(Equal(updatedCM.Data))
		})

		It("should be no-op if there is no change", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: ns.GetName(),
				},
				Data: map[string]string{"hello": "from-aistore"},
			}
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(err).NotTo(HaveOccurred())

			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, cm.DeepCopy())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not update resource when only status changes", func() {
			podObj := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-resource",
					Namespace: ns.GetName(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "something:tag",
						},
					},
				},
			}
			newRes := podObj.DeepCopy()
			_, err := k8sClient.CreateOrUpdateResource(ctx, ais, newRes)
			Expect(err).NotTo(HaveOccurred())

			podWithStatus := podObj.DeepCopy()
			podWithStatus.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
			}
			err = c.Status().Update(ctx, podWithStatus)
			Expect(err).NotTo(HaveOccurred())

			_, err = k8sClient.CreateOrUpdateResource(ctx, ais, podObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Apply", func() {
		const (
			cmName = "test-configmap"
			nsName = "apply-ns"
		)

		var (
			c         client.Client
			k8sClient *K8sClient
			cmKey     = client.ObjectKey{Namespace: nsName, Name: cmName}
			ctx       = context.TODO()
		)

		BeforeEach(func() {
			c = newFakeClient(nil)
			k8sClient = NewClient(c, c.Scheme())
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(c.Create(ctx, ns)).To(Succeed())
		})

		newCM := func() *corev1ac.ConfigMapApplyConfiguration {
			return corev1ac.ConfigMap(cmName, nsName).
				WithData(map[string]string{"test-key": "test-value"})
		}

		findManagedField := func(cm *corev1.ConfigMap, manager string) *metav1.ManagedFieldsEntry {
			for i := range cm.ManagedFields {
				if cm.ManagedFields[i].Manager == manager {
					return &cm.ManagedFields[i]
				}
			}
			return nil
		}

		It("should create the object if not exists", func() {
			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())
			Expect(fetchCM.Data).To(HaveKeyWithValue("test-key", "test-value"))
		})

		It("should record the operator field manager with operation Apply", func() {
			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())

			entry := findManagedField(fetchCM, FieldOwner)
			Expect(entry).NotTo(BeNil(), "expected a managedFields entry for %q", FieldOwner)
			Expect(entry.Operation).To(Equal(metav1.ManagedFieldsOperationApply))
		})

		It("should take over fields owned by another manager (ForceOwnership)", func() {
			const otherManager = "other-manager"
			otherApply := corev1ac.ConfigMap(cmName, nsName).
				WithData(map[string]string{"test-key": "other-value"})

			Expect(c.Apply(ctx, otherApply, client.FieldOwner(otherManager), client.ForceOwnership)).To(Succeed())

			Expect(k8sClient.Apply(ctx, newCM())).To(Succeed())

			fetchCM := &corev1.ConfigMap{}
			Expect(c.Get(ctx, cmKey, fetchCM)).To(Succeed())
			Expect(fetchCM.Data).To(HaveKeyWithValue("test-key", "test-value"))

			entry := findManagedField(fetchCM, FieldOwner)
			Expect(entry).NotTo(BeNil(), "expected operator field manager to have taken ownership")
		})
	})

	Describe("CreateServiceAccountToken", func() {
		const (
			saNamespace = "ais-operator-system"
			saName      = "ais-operator-controller-manager"
		)

		var (
			k8sClient *K8sClient
			request   *authenticationv1.TokenRequest
			minted    client.ObjectKey
			sa        = types.NamespacedName{Namespace: saNamespace, Name: saName}
			ctx       = context.TODO()
		)

		When("the ServiceAccount exists", func() {
			BeforeEach(func() {
				request = nil
				minted = client.ObjectKey{}
				account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: saNamespace}}
				c := newFakeClientBuilder([]runtime.Object{account}).WithInterceptorFuncs(interceptor.Funcs{
					// The fake client discards the TokenRequest spec, so record it on the way through
					SubResourceCreate: func(ctx context.Context, c client.Client, subResource string,
						obj, body client.Object, opts ...client.SubResourceCreateOption,
					) error {
						minted = client.ObjectKeyFromObject(obj)
						request = body.(*authenticationv1.TokenRequest).DeepCopy()
						return c.SubResource(subResource).Create(ctx, obj, body, opts...)
					},
				}).Build()
				k8sClient = NewClient(c, c.Scheme())
			})

			It("should request a token for the given ServiceAccount", func() {
				req, err := k8sClient.CreateServiceAccountToken(ctx, sa, "", 10*time.Minute)
				Expect(err).NotTo(HaveOccurred())
				Expect(req).NotTo(BeNil())
				Expect(minted).To(Equal(client.ObjectKey{Namespace: saNamespace, Name: saName}))
				Expect(request).NotTo(BeNil())
				Expect(request.Spec.Audiences).To(BeEmpty())
				Expect(request.Spec.ExpirationSeconds).To(HaveValue(Equal(int64(600))))
			})

			It("should bind the token to the requested audience", func() {
				_, err := k8sClient.CreateServiceAccountToken(ctx, sa, "ais-authn", 10*time.Minute)
				Expect(err).NotTo(HaveOccurred())
				Expect(minted).To(Equal(client.ObjectKey{Namespace: saNamespace, Name: saName}))
				Expect(request).NotTo(BeNil())
				Expect(request.Spec.Audiences).To(Equal([]string{"ais-authn"}))
			})
		})

		It("should fail when the ServiceAccount does not exist", func() {
			k8sClient = NewClient(newFakeClient(nil), scheme.Scheme)
			_, err := k8sClient.CreateServiceAccountToken(ctx, sa, "", 10*time.Minute)
			Expect(err).To(HaveOccurred())
		})
	})
})
