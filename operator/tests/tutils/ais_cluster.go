// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TODO: Should be provided from test config.
const (
	aisNodeImage = "aistorage/aisnode:v3.23-RC1"
	aisInitImage = "aistorage/ais-init:v3.22"
)

type (
	ClusterSpecArgs struct {
		Name                 string
		Namespace            string
		StorageClass         string
		Size                 int32
		TargetSize           int32
		ProxySize            int32
		DisableAntiAffinity  bool
		EnableExternalLB     bool
		PreservePVCs         bool
		AllowSharedOrNoDisks bool
	}
)

func NewAISClusterCR(args ClusterSpecArgs) *aisv1.AIStore {
	var storage *string
	if args.StorageClass != "" {
		storage = &args.StorageClass
	}
	mountLabel := "diskless"
	spec := aisv1.AIStoreSpec{
		Size:                   args.Size,
		CleanupData:            apc.Ptr(!args.PreservePVCs),
		NodeImage:              aisNodeImage,
		InitImage:              aisInitImage,
		HostpathPrefix:         "/etc/ais",
		EnableExternalLB:       args.EnableExternalLB,
		DisablePodAntiAffinity: &args.DisableAntiAffinity,
		ProxySpec: aisv1.DaemonSpec{
			ServiceSpec: aisv1.ServiceSpec{
				ServicePort:      intstr.FromInt(51080),
				PublicPort:       intstr.FromInt(51080),
				IntraControlPort: intstr.FromInt(51082),
				IntraDataPort:    intstr.FromInt(51083),
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
					Label:        &mountLabel,
				},
				{
					Path:         "/ais2",
					Size:         resource.MustParse("1Gi"),
					StorageClass: storage,
					Label:        &mountLabel,
				},
			},
		},
	}

	if args.TargetSize != 0 {
		spec.TargetSpec.Size = &args.TargetSize
	}

	if args.ProxySize != 0 {
		spec.ProxySpec.Size = &args.ProxySize
	}

	cluster := &aisv1.AIStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.Name,
			Namespace: args.Namespace,
		},
		Spec: spec,
	}
	return cluster
}
