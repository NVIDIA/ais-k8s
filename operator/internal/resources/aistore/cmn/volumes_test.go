/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/aistore/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("NodeJoin secret volume", func() {
	var ais *aisv1.AIStore

	BeforeEach(func() {
		ais = newTestAIS()
		ais.Spec.NodeJoin = &aisv1.NodeJoinSpec{SecretName: aisapc.Ptr("node-join-creds")}
	})

	It("mounts the secret at 0400 with no subPath", func() {
		volumes := NewAISVolumes(ais, aisapc.Target)
		var vol *corev1.Volume
		for i := range volumes {
			if volumes[i].Name == nodeJoinSecretVolume {
				vol = &volumes[i]
			}
		}
		Expect(vol).NotTo(BeNil())
		Expect(vol.Secret).NotTo(BeNil())
		Expect(vol.Secret.SecretName).To(Equal("node-join-creds"))
		Expect(*vol.Secret.DefaultMode).To(BeEquivalentTo(0o400))

		mounts := NewAISVolumeMounts(ais, aisapc.Target)
		var mount *corev1.VolumeMount
		for i := range mounts {
			if mounts[i].Name == nodeJoinSecretVolume {
				mount = &mounts[i]
			}
		}
		Expect(mount).NotTo(BeNil())
		Expect(mount.ReadOnly).To(BeTrue())
		Expect(mount.SubPath).To(BeEmpty())
		Expect(mount.SubPathExpr).To(BeEmpty())
	})

	It("omits the volume and mount when unset", func() {
		ais.Spec.NodeJoin = nil

		for _, vol := range NewAISVolumes(ais, aisapc.Target) {
			Expect(vol.Name).NotTo(Equal(nodeJoinSecretVolume))
		}
		for _, mount := range NewAISVolumeMounts(ais, aisapc.Target) {
			Expect(mount.Name).NotTo(Equal(nodeJoinSecretVolume))
		}
	})
})
