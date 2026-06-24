/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func Test_toleratesTaints(t *testing.T) {
	type args struct {
		tolerations []corev1.Toleration
		node        *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should_match",
			args: args{tolerations: []corev1.Toleration{
				{
					Key:      "testing.ai/resource-provider",
					Operator: "Exists",
				},
				{
					Key:      "nvidia.com/gpu",
					Operator: "Exists",
				},
				{
					Key:      "testing/dedicated-group-id",
					Operator: "Equal",
					Value:    "something-cool",
				},
			}, node: &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				{
					Key:    "testing.ai/resource-provider",
					Value:  "yes",
					Effect: corev1.TaintEffect("NoExecute"),
				},
				{
					Key:    "testing/dedicated-group-id",
					Value:  "something-cool",
					Effect: corev1.TaintEffect("NoExecute"),
				},
			}}}},
			want: true,
		},
		{
			name: "should_not_match",
			args: args{tolerations: []corev1.Toleration{
				{
					Key:      "notreal",
					Operator: "Exists",
				},
				{
					Key:      "testing/dedicated-group-id",
					Operator: "Equal",
					Value:    "something-else",
				},
			}, node: &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				{
					Key:    "testing.ai/resource-provider",
					Value:  "yes",
					Effect: corev1.TaintEffect("NoExecute"),
				},
				{
					Key:    "testing/dedicated-group-id",
					Value:  "something-cool",
					Effect: corev1.TaintEffect("NoExecute"),
				},
			}}}},
			want: false,
		},
		{
			name: "empty_toleration",
			args: args{
				tolerations: []corev1.Toleration{},
				node: &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
					{
						Key:    "testing.ai/resource-provider",
						Value:  "yes",
						Effect: corev1.TaintEffect("NoExecute"),
					},
					{
						Key:    "testing/dedicated-group-id",
						Value:  "something-cool",
						Effect: corev1.TaintEffect("NoExecute"),
					},
				}}}},
			want: false,
		},
		{
			name: "empty_node",
			args: args{
				tolerations: []corev1.Toleration{
					{
						Key:      "notreal",
						Operator: "Exists",
					},
					{
						Key:      "testing/dedicated-group-id",
						Operator: "Equal",
						Value:    "something-else",
					},
				},
				node: &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{}}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toleratesTaints(t.Context(), tt.args.tolerations, tt.args.node); got != tt.want {
				t.Errorf("toleratesTaints() = %v, want %v", got, tt.want)
			}
		})
	}
}
