// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"strconv"

	"github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TODO: Should be provided from test config.
const (
	aisNodeImage = "aistorage/aisnode:v3.23-RC2"
	aisInitImage = "aistorage/ais-init:v3.22"
)

type (
	ClusterSpecArgs struct {
		Name                string
		Namespace           string
		StorageClass        string
		Size                int32
		TargetSize          int32
		ProxySize           int32
		DisableAntiAffinity bool
		EnableExternalLB    bool
		PreservePVCs        bool
		// Create a cluster with more PVs than targets for future scaling
		MaxPVs int32
		// For testing deprecated feature
		AllowSharedOrNoDisks bool
	}
)

func NewAISCluster(args ClusterSpecArgs, client *aisclient.K8sClient) (*aisv1.AIStore, []*corev1.PersistentVolume) {
	var (
		storage *string
		pvNum   int
	)
	if args.StorageClass != "" {
		storage = &args.StorageClass
	}

	if args.MaxPVs != 0 {
		pvNum = int(args.MaxPVs)
	} else {
		if args.TargetSize != 0 {
			pvNum = int(args.TargetSize)
		} else {
			pvNum = int(args.Size)
		}
	}

	mounts := defineMounts(storage, !args.AllowSharedOrNoDisks)

	pvs := make([]*corev1.PersistentVolume, 0, len(mounts)*pvNum)

	for i := 0; i < pvNum; i++ {
		for _, mount := range mounts {
			pvData := PVData{
				storageClass: *storage,
				ns:           args.Namespace,
				cluster:      args.Name,
				mpath:        mount.Path,
				target:       "target-" + strconv.Itoa(i),
				size:         mount.Size,
			}
			// Create required PVs
			pv, err := CreatePV(context.Background(), client, &pvData)
			if err == nil {
				pvs = append(pvs, pv)
			}
		}
	}
	return newAISClusterCR(args, mounts), pvs
}

func defineMounts(storage *string, useLabels bool) []aisv1.Mount {
	mpathLabel := "disk1"
	mounts := []aisv1.Mount{
		{
			Path:         "/ais1",
			Size:         resource.MustParse("2Gi"),
			StorageClass: storage,
		},
		{
			Path:         "/ais2",
			Size:         resource.MustParse("1Gi"),
			StorageClass: storage,
		},
	}
	if useLabels {
		for i := range mounts {
			mounts[i].Label = &mpathLabel
		}
	}
	return mounts
}

func newAISClusterCR(args ClusterSpecArgs, mounts []aisv1.Mount) *aisv1.AIStore {
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
				ServicePort:      intstr.FromInt32(51080),
				PublicPort:       intstr.FromInt32(51080),
				IntraControlPort: intstr.FromInt32(51082),
				IntraDataPort:    intstr.FromInt32(51083),
			},
		},

		TargetSpec: aisv1.TargetSpec{
			DaemonSpec: aisv1.DaemonSpec{
				ServiceSpec: aisv1.ServiceSpec{
					ServicePort:      intstr.FromInt32(51081),
					PublicPort:       intstr.FromInt32(51081),
					IntraControlPort: intstr.FromInt32(51082),
					IntraDataPort:    intstr.FromInt32(51083),
				},
			},
			Mounts:               mounts,
			AllowSharedOrNoDisks: &args.AllowSharedOrNoDisks,
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
