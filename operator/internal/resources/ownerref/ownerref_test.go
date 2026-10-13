/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package ownerref_test

import (
	"testing"

	authv1alpha1 "github.com/ais-operator/api/aisauth/v1alpha1"
	"github.com/ais-operator/internal/resources/ownerref"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	expectedOwnerKind      = "ais.nvidia.com/owner-kind"
	expectedOwnerNamespace = "ais.nvidia.com/owner-namespace"
	expectedOwnerName      = "ais.nvidia.com/owner-name"
)

func TestLabeledOwner(t *testing.T) {
	owner := ownerref.AIStoreAuthOwner(&authv1alpha1.AIStoreAuth{ObjectMeta: metav1.ObjectMeta{Namespace: "ais", Name: "ais-authn"}})
	tests := []struct {
		name   string
		labels map[string]string
		ok     bool
	}{
		{name: "the operator manages the object", labels: map[string]string{}, ok: true},
		{name: "another tool manages the object", labels: map[string]string{ownerref.ManagedByLabel: "helm"}, ok: false},
		{name: "no owner kind", labels: map[string]string{expectedOwnerKind: ""}, ok: false},
		{name: "no owner namespace", labels: map[string]string{expectedOwnerNamespace: ""}, ok: false},
		{name: "no owner name", labels: map[string]string{expectedOwnerName: ""}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			obj := &metav1.ObjectMeta{}
			ownerref.SetLabeledOwner(obj, owner)
			for k, v := range tt.labels {
				obj.Labels[k] = v
			}

			got, ok := ownerref.GetLabeledOwner(obj)
			g.Expect(ok).To(Equal(tt.ok))
			if tt.ok {
				g.Expect(got).To(Equal(owner))
			}
		})
	}
}
