package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("compareEnvWithIgnored", func() {
	makeEnv := func(k, v string) corev1.EnvVar {
		return corev1.EnvVar{Name: k, Value: v}
	}

	It("returns false when env slices are identical and nothing ignored", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		ignored := map[string]struct{}{}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeFalse())
	})

	It("returns true when non-ignored env differs", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "DIFF")}
		ignored := map[string]struct{}{"IGNORED": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeTrue())
	})

	It("ignores changes to removed env when mode is IgnoreRemovedEnv", func() {
		des := []corev1.EnvVar{makeEnv("A", "1")} // B removed from desired
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		ignored := map[string]struct{}{"B": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreRemovedEnv)

		Expect(changed).To(BeFalse())
	})

	It("ignores changes to added env when mode is IgnoreAddedEnv", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("B", "2")}
		cur := []corev1.EnvVar{makeEnv("A", "1")} // B only in desired
		ignored := map[string]struct{}{"B": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreAddedEnv)

		Expect(changed).To(BeFalse())
	})

	It("detects value changes for ignored env when present in both", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "y")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreNone)

		Expect(changed).To(BeTrue())
	})

	It("skips ignored env missing from desired in IgnoreRemovedEnv mode", func() {
		des := []corev1.EnvVar{makeEnv("A", "1")}
		cur := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreRemovedEnv)

		Expect(changed).To(BeFalse())
	})

	It("skips ignored env missing from current in IgnoreAddedEnv mode", func() {
		des := []corev1.EnvVar{makeEnv("A", "1"), makeEnv("IGN", "x")}
		cur := []corev1.EnvVar{makeEnv("A", "1")}
		ignored := map[string]struct{}{"IGN": {}}

		changed := compareEnvWithIgnored(des, cur, ignored, SyncModeIgnoreAddedEnv)

		Expect(changed).To(BeFalse())
	})
})
