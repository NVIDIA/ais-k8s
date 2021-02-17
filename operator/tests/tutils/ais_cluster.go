// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package tutils

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	aisv1 "github.com/ais-operator/api/v1alpha1"
)

// TODO: Should be provided from test config.
const (
	aisNodeImage = "aistore/aisnode:3.3.1"
	aisInitImage = "aistore/ais-init:latest"
)

func NewAISClusterCR(name, namespace, storageClass string,
	size int32, disableAntiAffinity, enableExternalLB bool) *aisv1.AIStore {
	var storage *string
	if storageClass != "" {
		storage = &storageClass
	}

	spec := aisv1.AIStoreSpec{
		Size:                   size,
		NodeImage:              aisNodeImage,
		InitImage:              aisInitImage,
		HostpathPrefix:         "/etc/ais",
		EnableExternalLB:       enableExternalLB,
		DisablePodAntiAffinity: &disableAntiAffinity,
		ProxySpec: aisv1.DaemonSpec{
			ServiceSpec: aisv1.ServiceSpec{
				ServicePort:      intstr.FromInt(51080),
				PublicPort:       intstr.FromInt(51080),
				IntraControlPort: intstr.FromInt(51081),
				IntraDataPort:    intstr.FromInt(51082),
			},
		},

		TargetSpec: aisv1.TargetSpec{
			DaemonSpec: aisv1.DaemonSpec{
				ServiceSpec: aisv1.ServiceSpec{
					ServicePort:      intstr.FromInt(51081),
					PublicPort:       intstr.FromInt(51081),
					IntraControlPort: intstr.FromInt(51082),
					IntraDataPort:    intstr.FromInt(51083),
				},
			},
			Mounts: []aisv1.Mount{
				{
					Path:         "/ais1",
					Size:         resource.MustParse("2Gi"),
					StorageClass: storage,
				},
			},

			NoDiskIO: aisv1.NoDiskIO{
				Enabled:    true,
				DryObjSize: resource.MustParse("8M"),
			},
		},
	}

	cluster := &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
	return cluster
}
