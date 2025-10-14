// Package contains declaration of AIS Kubernetes Custom Resource Definitions
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package v1beta1

import (
	"context"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("AIStore", func() {
	Describe("Validation", func() {
		Describe("OpenAPI", func() {
			var namespace string

			BeforeEach(func() {
				namespace = "ais-test-" + rand.String(10)

				err := k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
				Expect(err).ToNot(HaveOccurred())
			})

			DescribeTable("should reject AIStore definition", func(ais *AIStore, expectedMessage string) {
				// Extra setup.
				ais.ObjectMeta = metav1.ObjectMeta{
					Name:      "ais",
					Namespace: namespace,
				}

				err := k8sClient.Create(context.Background(), ais)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedMessage))
			},
				Entry(
					"not defined nodeImage",
					&AIStore{Spec: AIStoreSpec{InitImage: "", NodeImage: ""}},
					"spec.initImage in body should be at least 1 chars long",
				),
				Entry(
					"not defined initImage",
					&AIStore{Spec: AIStoreSpec{InitImage: "init-image:tag", NodeImage: ""}},
					"spec.nodeImage in body should be at least 1 chars long",
				),
				Entry(
					"not defined targetSpec.mounts",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
					}},
					"spec.targetSpec.mounts: Required value",
				),
				Entry(
					"not defined size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: DaemonSpec{},
						TargetSpec: TargetSpec{
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"not defined targetSpec.size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: DaemonSpec{
							Size: aisapc.Ptr[int32](1),
						},
						TargetSpec: TargetSpec{
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"not defined proxySpec.size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: DaemonSpec{},
						TargetSpec: TargetSpec{
							DaemonSpec: DaemonSpec{
								Size: aisapc.Ptr[int32](1),
							},
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid cluster size, it is either not specified or value is not valid",
				),
				Entry(
					"invalid value for size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						Size:      aisapc.Ptr[int32](-2),
						ProxySpec: DaemonSpec{},
						TargetSpec: TargetSpec{
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid value: -2: spec.size in body should be greater than or equal to -1",
				),
				Entry(
					"invalid value for targetSize.size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						Size:      aisapc.Ptr[int32](1),
						ProxySpec: DaemonSpec{},
						TargetSpec: TargetSpec{
							DaemonSpec: DaemonSpec{
								Size: aisapc.Ptr[int32](-2),
							},
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"spec.targetSpec.size in body should be greater than or equal to -1",
				),
				Entry(
					"invalid value for proxySize.size",
					&AIStore{Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: DaemonSpec{
							Size: aisapc.Ptr[int32](-2),
						},
						TargetSpec: TargetSpec{
							DaemonSpec: DaemonSpec{
								Size: aisapc.Ptr[int32](1),
							},
							Mounts: []Mount{{Path: "/mnt"}},
						},
					}},
					"Invalid value: -2: spec.proxySpec.size in body should be greater than or equal to -1",
				),
			)

			It("should pass AIStore definition", func() {
				ais := &AIStore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ais",
						Namespace: namespace,
					},
					Spec: AIStoreSpec{
						InitImage: "init-image:tag",
						NodeImage: "node-image:tag",
						ProxySpec: DaemonSpec{
							Size: aisapc.Ptr[int32](1),
						},
						TargetSpec: TargetSpec{
							DaemonSpec: DaemonSpec{
								Size: aisapc.Ptr[int32](1),
							},
							Mounts: []Mount{{Path: "/mnt"}},
						},
					},
				}

				err := k8sClient.Create(context.Background(), ais)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Describe("Custom validation", func() {
			DescribeTable("should fail AIStore validation", func(ais AIStore, expectedMessage string) {
				_, err := ais.ValidateSpec(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedMessage))
			},
				Entry(
					"empty size",
					AIStore{},
					"cluster size is not specified",
				),
				Entry(
					"invalid size",
					AIStore{Spec: AIStoreSpec{Size: aisapc.Ptr[int32](-2)}},
					"invalid cluster size -2, should be at least 1 or -1 for autoScaling",
				),
				Entry(
					"hostpathPrefix and stateStorageClass empty",
					AIStore{Spec: AIStoreSpec{Size: aisapc.Ptr[int32](1)}},
					"AIS spec does not define hostpathPrefix or stateStorageClass",
				),
				Entry(
					"invalid proxy serviceSpec",
					AIStore{
						Spec: AIStoreSpec{
							Size:           aisapc.Ptr[int32](1),
							HostpathPrefix: aisapc.Ptr("/mnt"),
						},
					},
					"spec.proxySpec.servicePort: Invalid value: 0: must be between 1 and 65535",
				),
				Entry(
					"invalid target serviceSpec",
					AIStore{
						Spec: AIStoreSpec{
							Size:           aisapc.Ptr[int32](1),
							HostpathPrefix: aisapc.Ptr("/mnt"),
							ProxySpec: DaemonSpec{
								ServiceSpec: ServiceSpec{
									ServicePort:      intstr.FromInt32(51080),
									PublicPort:       intstr.FromInt32(51080),
									IntraControlPort: intstr.FromInt32(51081),
									IntraDataPort:    intstr.FromInt32(51082),
								},
							},
						},
					},
					"spec.targetSpec.servicePort: Invalid value: 0: must be between 1 and 65535",
				),
			)

			It("should pass AIStore validation", func() {
				ais := AIStore{
					Spec: AIStoreSpec{
						Size:           aisapc.Ptr[int32](1),
						HostpathPrefix: aisapc.Ptr("/mnt"),
						ProxySpec: DaemonSpec{
							ServiceSpec: ServiceSpec{
								ServicePort:      intstr.FromInt32(51080),
								PublicPort:       intstr.FromInt32(51080),
								IntraControlPort: intstr.FromInt32(51081),
								IntraDataPort:    intstr.FromInt32(51082),
							},
						},
						TargetSpec: TargetSpec{
							DaemonSpec: DaemonSpec{
								ServiceSpec: ServiceSpec{
									ServicePort:      intstr.FromInt32(51080),
									PublicPort:       intstr.FromInt32(51080),
									IntraControlPort: intstr.FromInt32(51081),
									IntraDataPort:    intstr.FromInt32(51082),
								},
							},
						},
					},
				}

				_, err := ais.ValidateSpec(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
