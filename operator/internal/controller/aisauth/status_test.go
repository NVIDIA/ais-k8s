/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth

import (
	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AIStoreAuth conditions", Label("short"), func() {
	It("stamps the generation it was evaluated against and updates in place", func() {
		authn := &authv1alpha1.AIStoreAuth{ObjectMeta: metav1.ObjectMeta{Generation: 3}}
		Expect(isReady(authn)).To(BeFalse())

		setReadyCondition(authn, metav1.ConditionTrue,
			authv1alpha1.ReasonAvailable, msgAvailable)
		Expect(isReady(authn)).To(BeTrue())
		Expect(authn.Status.Conditions).To(HaveLen(1))
		Expect(authn.Status.Conditions[0].ObservedGeneration).To(Equal(int64(3)))

		authn.Generation = 4
		setReadyCondition(authn, metav1.ConditionFalse,
			authv1alpha1.ReasonReconcileFailed, "Failed to reconcile ConfigMap")
		Expect(authn.Status.Conditions).To(HaveLen(1))
		Expect(isReady(authn)).To(BeFalse())
		Expect(authn.Status.Conditions[0].ObservedGeneration).To(Equal(int64(4)))
	})

	It("reports whether Ready already carries a reason", func() {
		authn := &authv1alpha1.AIStoreAuth{}
		Expect(hasReadyReason(authn, authv1alpha1.ReasonProgressDeadlineExceeded)).To(BeFalse())

		setReadyCondition(authn, metav1.ConditionFalse,
			authv1alpha1.ReasonProgressDeadlineExceeded, msgNotProgressing)
		Expect(hasReadyReason(authn, authv1alpha1.ReasonProgressDeadlineExceeded)).To(BeTrue())
		Expect(hasReadyReason(authn, authv1alpha1.ReasonDeploymentUnavailable)).To(BeFalse())
	})
})
