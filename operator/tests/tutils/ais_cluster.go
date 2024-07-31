// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"fmt"
	"path"
	"strconv"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TODO: Should be provided from test config.
const (
	aisNodeImage = "aistorage/aisnode:v3.24-rc2"
	aisInitImage = "aistorage/ais-init:v1.2.0"
)

type (
	ClusterSpecArgs struct {
		Name             string
		Namespace        string
		StorageClass     string
		Size             int32
		TargetSize       int32
		ProxySize        int32
		TargetSharedNode bool
		EnableExternalLB bool
		ShutdownCluster  bool
		CleanupMetadata  bool
		CleanupData      bool
		// Create a cluster with more PVs than targets for future scaling
		MaxTargets int32
		// Where to mount the hostpath storage for actual storage PVs
		StorageHostPath string
		// For testing deprecated feature
		AllowSharedOrNoDisks bool
	}
)

func NewAISCluster(args *ClusterSpecArgs, client *aisclient.K8sClient) (*aisv1.AIStore, []*corev1.PersistentVolume) {
	mounts := defineMounts(args)
	pvs := createStoragePVs(args, client, mounts)
	return newAISClusterCR(args, mounts), pvs
}

func createStoragePVs(args *ClusterSpecArgs, client *aisclient.K8sClient, mounts []aisv1.Mount) []*corev1.PersistentVolume {
	targetNum := int(args.MaxTargets)
	if targetNum == 0 {
		if args.TargetSize != 0 {
			targetNum = int(args.TargetSize)
		} else {
			targetNum = int(args.Size)
		}
	}

	pvs := make([]*corev1.PersistentVolume, 0, len(mounts)*targetNum)

	for i := range targetNum {
		for _, mount := range mounts {
			var k8sNodeName string
			if args.TargetSharedNode {
				k8sNodeName = "minikube"
			} else {
				k8sNodeName = determineNode("minikube", "%s-m%02d", i)
			}
			pvData := PVData{
				storageClass: args.StorageClass,
				ns:           args.Namespace,
				cluster:      args.Name,
				mpath:        mount.Path,
				node:         k8sNodeName,
				target:       "target-" + strconv.Itoa(i),
				size:         mount.Size,
			}
			// Create required PVs
			if pv, err := CreatePV(context.Background(), client, &pvData); err == nil {
				pvs = append(pvs, pv)
			}
		}
	}
	return pvs
}

// Determine the hostname of the node for PV affinity.
// Targets will bind to specific PVs so in a multi-node multi-target test we must define the PVs on separate nodes
// By default, a minikube multi-node cluster will create nodes named minikube, minikube-m02, minikube-m03...
func determineNode(base, format string, ordinal int) string {
	if ordinal == 0 {
		return base
	}
	// minikube node names are not zero-indexed, so increment to match target names
	return fmt.Sprintf(format, base, ordinal+1)
}

func defineMounts(args *ClusterSpecArgs) []aisv1.Mount {
	var storagePrefix string
	if args.StorageHostPath == "" {
		storagePrefix = "/etc/ais"
	} else {
		storagePrefix = args.StorageHostPath
	}
	mounts := []aisv1.Mount{
		{
			Path:         path.Join(storagePrefix, "ais1"),
			Size:         resource.MustParse("2Gi"),
			StorageClass: &args.StorageClass,
		},
		{
			Path:         path.Join(storagePrefix, "ais2"),
			Size:         resource.MustParse("1Gi"),
			StorageClass: &args.StorageClass,
		},
	}
	mpathLabel := "disk1"
	if !args.AllowSharedOrNoDisks {
		for i := range mounts {
			mounts[i].Label = &mpathLabel
		}
	}
	return mounts
}

func newAISClusterCR(args *ClusterSpecArgs, mounts []aisv1.Mount) *aisv1.AIStore {
	spec := aisv1.AIStoreSpec{
		Size:              &args.Size,
		ShutdownCluster:   aisapc.Ptr(args.ShutdownCluster),
		CleanupMetadata:   aisapc.Ptr(args.CleanupMetadata),
		CleanupData:       aisapc.Ptr(args.CleanupData),
		NodeImage:         aisNodeImage,
		InitImage:         aisInitImage,
		StateStorageClass: aisapc.Ptr("local-path"),
		EnableExternalLB:  args.EnableExternalLB,
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
			Mounts:                 mounts,
			AllowSharedOrNoDisks:   &args.AllowSharedOrNoDisks,
			DisablePodAntiAffinity: &args.TargetSharedNode,
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
