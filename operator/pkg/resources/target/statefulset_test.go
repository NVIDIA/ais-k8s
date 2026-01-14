// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"fmt"
	"path"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	sampleMatchExpressions = func(operator metav1.LabelSelectorOperator, values []string) []metav1.LabelSelectorRequirement {
		return []metav1.LabelSelectorRequirement{{
			Key:      "baz",
			Operator: operator,
			Values:   values,
		}}
	}
)

var _ = Describe("Statefulset Target Volumes and Mounts", Label("short"), func() {
	selector := &metav1.LabelSelector{MatchExpressions: sampleMatchExpressions(metav1.LabelSelectorOpIn, []string{"hostname"})}
	size := resource.MustParse("1Gi")
	aisSpec := &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ais",
			Namespace: "test-namespace",
		},
		Spec: aisv1.AIStoreSpec{
			Size:              apc.Ptr(int32(1)),
			StateStorageClass: apc.Ptr("stateStorageClass"),
			TargetSpec:        aisv1.TargetSpec{},
		},
	}
	Describe("New Target with storageClass", func() {
		It("should return with VolumeClaimTemplates for state and data", func() {
			specCopy := aisSpec.DeepCopy()
			specCopy.Spec.TargetSpec.Mounts = []aisv1.Mount{{
				Path:         "/data/test",
				Size:         size,
				StorageClass: apc.Ptr("dataStorageClass"),
				Selector:     selector,
			}}
			result := NewTargetSS(specCopy, *specCopy.Spec.Size)
			Expect(result).To(Not(BeNil()))
			Expect(result.Spec.VolumeClaimTemplates).To(HaveLen(2))
			Expect(result.Spec.VolumeClaimTemplates[0].Name).To(Equal(aisSpec.Name + "-data-test"))
			Expect(*result.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(Equal("dataStorageClass"))
			Expect(result.Spec.VolumeClaimTemplates[0].Spec.Selector).To(Equal(selector))
			Expect(result.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().String()).To(Equal(size.String()))
			Expect(result.Spec.VolumeClaimTemplates[1].Name).To(Equal(fmt.Sprintf("%s-%s-%s", aisSpec.Namespace, aisSpec.Name, "state")))
			Expect(*result.Spec.VolumeClaimTemplates[1].Spec.StorageClassName).To(Equal("stateStorageClass"))
			Expect(result.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests.Storage().String()).To(Equal("1Gi"))

			// config-mount,statsd-config,logs,state,data
			Expect(result.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(5))
		})
		It("should return with VolumeClaimTemplates for state and multiple for data", func() {
			specCopy := aisSpec.DeepCopy()
			moreDataSize := resource.MustParse("1000Gi")
			specCopy.Spec.TargetSpec.Mounts = []aisv1.Mount{
				{
					Path:         "/data/test",
					Size:         size,
					StorageClass: apc.Ptr("dataStorageClass"),
					Selector:     selector,
				},
				{
					Path:         "/mount/largeDisk",
					Size:         moreDataSize,
					StorageClass: apc.Ptr("largeDataStorageClass"),
					Selector:     selector,
				},
			}
			result := NewTargetSS(specCopy, *specCopy.Spec.Size)
			Expect(result).To(Not(BeNil()))
			// data mount 1
			Expect(result.Spec.VolumeClaimTemplates).To(HaveLen(3))
			Expect(result.Spec.VolumeClaimTemplates[0].Name).To(Equal(aisSpec.Name + "-data-test"))
			Expect(*result.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(Equal("dataStorageClass"))
			Expect(result.Spec.VolumeClaimTemplates[0].Spec.Selector).To(Equal(selector))
			Expect(result.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().String()).To(Equal(size.String()))
			// data mount 2
			Expect(result.Spec.VolumeClaimTemplates[1].Name).To(Equal(aisSpec.Name + "-mount-largeDisk"))
			Expect(*result.Spec.VolumeClaimTemplates[1].Spec.StorageClassName).To(Equal("largeDataStorageClass"))
			Expect(result.Spec.VolumeClaimTemplates[1].Spec.Selector).To(Equal(selector))
			// state
			Expect(result.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests.Storage().String()).To(Equal(moreDataSize.String()))
			Expect(result.Spec.VolumeClaimTemplates[2].Name).To(Equal(fmt.Sprintf("%s-%s-%s", aisSpec.Namespace, aisSpec.Name, "state")))
			Expect(*result.Spec.VolumeClaimTemplates[2].Spec.StorageClassName).To(Equal("stateStorageClass"))
			Expect(result.Spec.VolumeClaimTemplates[2].Spec.Resources.Requests.Storage().String()).To(Equal("1Gi"))

			// config-mount,statsd-config,logs,state,data,data
			Expect(result.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(6))
		})
	})
	Describe("New Target with hostMount", func() {
		It("should return no VolumeClaimTemplates but with volume mounts", func() {
			hostPathData := "/node/data"
			testHostMount := []aisv1.Mount{
				{
					Path:        hostPathData,
					Size:        size,
					UseHostPath: apc.Ptr(true),
				},
			}

			specCopy := aisSpec.DeepCopy()

			// Not used for this test, but explicitly set so we don't have a state volume claim to count
			specCopy.Spec.StateStorageClass = nil
			specCopy.Spec.HostpathPrefix = apc.Ptr("/etc/ais")

			specCopy.Spec.TargetSpec.Mounts = testHostMount
			result := NewTargetSS(specCopy, *specCopy.Spec.Size)
			Expect(result).To(Not(BeNil()))
			Expect(result.Spec.VolumeClaimTemplates).To(HaveLen(0))
			Expect(result.Spec.Template.Spec.Volumes).To(HaveLen(7))

			var dataVolume *v1.Volume
			// Expect the host path data mount to be created with a fixed volume name
			expectedName := "host-data-mount-0"
			expectedHostPath := path.Join(hostPathData, specCopy.Namespace, specCopy.Name, apc.Target)
			for _, dv := range result.Spec.Template.Spec.Volumes {
				if dv.Name == expectedName {
					dataVolume = &dv
				}
			}
			// Check that we found the expected data volume in the result with the right properties
			Expect(dataVolume).To(Not(BeNil()))
			Expect(dataVolume.Name).To(Equal(expectedName))
			Expect(dataVolume.HostPath).To(Not(BeNil()))
			Expect(dataVolume.HostPath.Path).To(Equal(expectedHostPath))
			Expect(*dataVolume.HostPath.Type).To(Equal(v1.HostPathDirectoryOrCreate))

			Expect(result.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(5))
		})
	})
})
