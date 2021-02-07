// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package cmn

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aisv1 "github.com/ais-operator/api/v1alpha1"
)

const (
	resourceTypeStatfulSets = "statefulsets"
	resourceTypeDaemonSets  = "daemonsets"
	resourceTypeNodes       = "nodes"
	resourceTypePodLogs     = "pods/log"

	verbAll = "*"

	roleKind        = "Role"
	clusterRoleKind = "ClusterRole"
)

func rbacRoleName(ais *aisv1.AIStore) string {
	return ais.Name + "-role"
}

func rbacClusterRoleName(ais *aisv1.AIStore) string {
	return ais.Name + "-cr"
}

func ServiceAccountName(ais *aisv1.AIStore) string {
	return ais.Name + "-sa"
}

func NewAISRbacRole(ais *aisv1.AIStore) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacRoleName(ais),
			Namespace: ais.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{
					string(corev1.ResourceSecrets), string(corev1.ResourcePods),
					string(corev1.ResourceConfigMaps), string(corev1.ResourceServices),
					resourceTypeStatfulSets, resourceTypeDaemonSets,
				},
				Verbs: []string{verbAll}, // TODO: set only required permissions
			},
		},
	}
}

func NewAISRbacRoleBinding(ais *aisv1.AIStore) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ais.Name + "-rb",
			Namespace: ais.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     roleKind,
			Name:     rbacRoleName(ais),
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

func NewAISRbacClusterRole(ais *aisv1.AIStore) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: rbacClusterRoleName(ais),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{resourceTypeNodes, resourceTypePodLogs},
				Verbs:     []string{verbAll}, // TODO: set only required permissions
			},
		},
	}
}

func NewAISRbacClusterRoleBinding(ais *aisv1.AIStore) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ais.Name + "-crb",
			Namespace: ais.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     clusterRoleKind,
			Name:     rbacClusterRoleName(ais),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Namespace: ais.Namespace,
				Name:      ServiceAccountName(ais),
			},
		},
	}
}
