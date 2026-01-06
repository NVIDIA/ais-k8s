// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func pdbName(ais *aisv1.AIStore) string {
	return statefulSetName(ais)
}

func PDBNSName(ais *aisv1.AIStore) types.NamespacedName {
	return types.NamespacedName{
		Name:      pdbName(ais),
		Namespace: ais.Namespace,
	}
}

func NewTargetPDB(ais *aisv1.AIStore) *policyv1.PodDisruptionBudget {
	maxUnavailable := ais.GetTargetPDBMaxUnavailable()
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName(ais),
			Namespace: ais.Namespace,
			Labels:    BasicLabels(ais),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: BasicLabels(ais),
			},
		},
	}
}
