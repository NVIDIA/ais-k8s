// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func roleName(ais *aisv1.AIStore) string {
	return ais.Name + "-role"
}

func roleBindingName(ais *aisv1.AIStore) string {
	return ais.Name + "-rb"
}

// ClusterRole is cluster scoped, so to ensure uniqueness we
// use both namespace and AIS CR name to generate a unique ClusterRole name.
func ClusterRoleName(ais *aisv1.AIStore) string {
	return ais.Namespace + "-" + ais.Name + "-cr"
}

func ClusterRoleBindingName(ais *aisv1.AIStore) string {
	return ais.Namespace + "-" + ais.Name + "-crb"
}

func ServiceAccountName(ais *aisv1.AIStore) string {
	return ais.Name + "-sa"
}

func NewAISRBACRole(ais *aisv1.AIStore) *rbacv1.Role {
	allRule := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{
			"pods",
			"services",
		},
		Verbs: []string{"get", "list", "watch", "create", "update", "delete"},
	}
	getRule := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{
			"pods/log",
		},
		Verbs: []string{"get"},
	}
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName(ais),
			Namespace: ais.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			allRule, getRule,
		},
	}
}

func NewAISRBACRoleBinding(ais *aisv1.AIStore) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName(ais),
			Namespace: ais.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     roleName(ais),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.ServiceAccountKind,
				Name: ServiceAccountName(ais),
			},
		},
	}
}

func NewAISServiceAccount(ais *aisv1.AIStore) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName(ais),
			Namespace: ais.Namespace,
		},
	}
}
