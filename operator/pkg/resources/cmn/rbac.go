// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

import (
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/pkg/resources/ownerref"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
)

func roleName(ais *aisv1.AIStore) string {
	return ais.Name + "-role"
}

func roleBindingName(ais *aisv1.AIStore) string {
	return ais.Name + "-rb"
}

func ServiceAccountName(ais *aisv1.AIStore) string {
	return ais.Name + "-sa"
}

func ServiceAccount(ais *aisv1.AIStore) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: ServiceAccountName(ais), Namespace: ais.Namespace},
	}
}

func Role(ais *aisv1.AIStore) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName(ais), Namespace: ais.Namespace},
	}
}

func RoleBinding(ais *aisv1.AIStore) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: roleBindingName(ais), Namespace: ais.Namespace},
	}
}

func NewAISRBACRole(ais *aisv1.AIStore) *rbacv1ac.RoleApplyConfiguration {
	allRule := rbacv1ac.PolicyRule().
		WithAPIGroups("").
		WithResources("pods", "services").
		WithVerbs("get", "list", "watch", "create", "update", "delete")
	getRule := rbacv1ac.PolicyRule().
		WithAPIGroups("").
		WithResources("pods/log").
		WithVerbs("get")
	return rbacv1ac.Role(roleName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithRules(allRule, getRule)
}

func NewAISRBACRoleBinding(ais *aisv1.AIStore) *rbacv1ac.RoleBindingApplyConfiguration {
	roleRef := rbacv1ac.RoleRef().
		WithAPIGroup(rbacv1.SchemeGroupVersion.Group).
		WithKind("Role").
		WithName(roleName(ais))
	subject := rbacv1ac.Subject().
		WithKind(rbacv1.ServiceAccountKind).
		WithNamespace(ais.Namespace).
		WithName(ServiceAccountName(ais))
	return rbacv1ac.RoleBinding(roleBindingName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais)).
		WithRoleRef(roleRef).
		WithSubjects(subject)
}

func NewAISServiceAccount(ais *aisv1.AIStore) *corev1ac.ServiceAccountApplyConfiguration {
	sa := corev1ac.ServiceAccount(ServiceAccountName(ais), ais.Namespace).
		WithOwnerReferences(ownerref.NewControllerRef(ais))
	for _, ref := range ais.Spec.ImagePullSecrets {
		sa = sa.WithImagePullSecrets(corev1ac.LocalObjectReference().WithName(ref.Name))
	}
	return sa
}
