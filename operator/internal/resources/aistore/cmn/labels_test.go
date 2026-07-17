/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Labels", Label("short"), func() {
	Describe("LegacyLabels", func() {
		It("returns unprefixed and prefixed app and component labels", func() {
			Expect(LegacyLabels("ais", "proxy")).To(Equal(map[string]string{
				LabelApp:               "ais",
				LabelAppPrefixed:       "ais",
				LabelComponent:         "proxy",
				LabelComponentPrefixed: "proxy",
			}))
		})
	})

	Describe("SelectorLabels", func() {
		It("returns only prefixed selector labels", func() {
			Expect(SelectorLabels("ais", "target")).To(Equal(map[string]string{
				LabelAppPrefixed:       "ais",
				LabelComponentPrefixed: "target",
			}))
		})
	})

	Describe("MergePodLabels", func() {
		It("applies user labels before daemon labels so reserved keys cannot be overridden", func() {
			daemonLabels := LegacyLabels("ais", "proxy")
			podLabels := MergePodLabels(map[string]string{
				"custom":               "value",
				LabelAppPrefixed:       "override",
				LabelComponentPrefixed: "override",
				LabelApp:               "override",
				LabelComponent:         "override",
			}, daemonLabels)

			Expect(podLabels["custom"]).To(Equal("value"))
			Expect(podLabels[LabelAppPrefixed]).To(Equal("ais"))
			Expect(podLabels[LabelComponentPrefixed]).To(Equal("proxy"))
			Expect(podLabels[LabelApp]).To(Equal("ais"))
			Expect(podLabels[LabelComponent]).To(Equal("proxy"))
		})

		It("returns daemon labels when user labels are nil", func() {
			daemonLabels := LegacyLabels("ais", "target")
			Expect(MergePodLabels(nil, daemonLabels)).To(Equal(daemonLabels))
		})
	})
})
