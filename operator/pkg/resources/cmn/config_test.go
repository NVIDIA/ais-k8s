// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisapc "github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/cmn"
	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", Label("short"), func() {
	Describe("Convert", func() {
		It("should convert without an error", func() {
			toUpdate := &aisv1.ConfigToUpdate{
				Space: &aisv1.SpaceConfToUpdate{
					CleanupWM: aisapc.Ptr[int64](10),
					LowWM:     aisapc.Ptr[int64](20),
					HighWM:    aisapc.Ptr[int64](30),
					OOS:       aisapc.Ptr[int64](40),
				},
				LRU: &aisv1.LRUConfToUpdate{
					Enabled:       aisapc.Ptr(true),
					DontEvictTime: (*aisv1.Duration)(aisapc.Ptr[int64](10)),
				},
				Features: aisapc.Ptr("2568"),
			}

			toSet, err := toUpdate.Convert()
			Expect(err).ToNot(HaveOccurred())
			cfg := &cmn.ClusterConfig{}
			err = cfg.Apply(toSet, aisapc.Cluster)
			Expect(err).ToNot(HaveOccurred())

			Expect(cfg.Space.CleanupWM).To(BeEquivalentTo(10))
			Expect(cfg.Space.LowWM).To(BeEquivalentTo(20))
			Expect(cfg.Space.HighWM).To(BeEquivalentTo(30))
			Expect(cfg.Space.OOS).To(BeEquivalentTo(40))

			Expect(cfg.LRU.Enabled).To(BeEquivalentTo(true))
			Expect(cfg.LRU.DontEvictTime).To(BeEquivalentTo(10))

			Expect(cfg.Features).To(BeEquivalentTo(2568))
		})
	})
})
