// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021-2026, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"context"
	"path"
	"strconv"
	"strings"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisv1 "github.com/ais-operator/api/v1beta1"
	aisclient "github.com/ais-operator/pkg/client"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DefaultNodeImage     = "aistorage/aisnode:v4.4"
	DefaultInitImage     = "aistorage/ais-init:v4.4"
	DefaultLogsImage     = "aistorage/ais-logs:v1.1"
	DefaultPrevNodeImage = "aistorage/aisnode:v4.3"
	DefaultPrevInitImage = "aistorage/ais-init:v4.3"
	TestNSBase           = "ais-op-test"
	TestNSOtherBase      = "ais-op-test-other"
)

type (
	ClusterSpecArgs struct {
		Name                      string
		Namespace                 string
		StorageClass              string
		Size                      int32
		TargetSize                int32
		ProxySize                 int32
		NodeImage                 string
		InitImage                 string
		LogSidecarImage           string
		DisableTargetAntiAffinity bool
		EnableExternalLB          bool
		EnableAdminClient         bool
		EnableTargetPDB           bool
		TLS                       *TLSArgs
		ShutdownCluster           bool
		CleanupMetadata           bool
		CleanupData               bool
		APIMode                   string
		// Create a cluster with more PVs than targets for future scaling
		MaxTargets int32
		// Where to mount the hostpath storage for actual storage PVs
		StorageHostPath string
	}

	TLSArgs struct {
		SecretName string
		IssuerName string
		IssuerKind string
		Mode       string
	}
)

func clusterName() string {
	return "ais-test-" + strings.ToLower(aiscos.CryptoRandS(6))
}

func NewClusterSpecArgs(testCfg *AISTestCfg, namespace string) *ClusterSpecArgs {
	return &ClusterSpecArgs{
		Name:                      clusterName(),
		Namespace:                 namespace,
		StorageClass:              testCfg.StorageClass,
		StorageHostPath:           testCfg.StorageHostPath,
		Size:                      1,
		NodeImage:                 testCfg.NodeImage,
		InitImage:                 testCfg.InitImage,
		LogSidecarImage:           testCfg.LogsImage,
		APIMode:                   testCfg.APIMode,
		CleanupMetadata:           true,
		CleanupData:               true,
		DisableTargetAntiAffinity: false,
	}
}

func NewAISCluster(ctx context.Context, args *ClusterSpecArgs, client *aisclient.K8sClient) (*aisv1.AIStore, []*corev1.PersistentVolume) {
	mounts := defineMounts(args)
	pvs := createStoragePVs(ctx, args, client, mounts)
	return newAISClusterCR(args, mounts), pvs
}

func NewAISClusterNoPV(args *ClusterSpecArgs) *aisv1.AIStore {
	mounts := defineMounts(args)
	return newAISClusterCR(args, mounts)
}

func createStoragePVs(ctx context.Context, args *ClusterSpecArgs, client *aisclient.K8sClient, mounts []aisv1.Mount) []*corev1.PersistentVolume {
	targetNum := int(args.MaxTargets)
	if targetNum == 0 {
		if args.TargetSize != 0 {
			targetNum = int(args.TargetSize)
		} else {
			targetNum = int(args.Size)
		}
	}

	pvs := make([]*corev1.PersistentVolume, 0, len(mounts)*targetNum)

	selector := map[string]string{"ais-node": "true"}
	nodeList, err := client.ListNodesMatchingSelector(ctx, selector)

	Expect(err).To(BeNil())
	Expect(nodeList.Items).NotTo(BeEmpty())

	for i := range targetNum {
		for _, mount := range mounts {
			var k8sNodeName string
			if args.DisableTargetAntiAffinity {
				k8sNodeName = nodeList.Items[0].Name
			} else {
				k8sNodeName = nodeList.Items[i].Name
			}
			var pvSize resource.Quantity
			if mount.Size != nil {
				pvSize = *mount.Size
			}
			pvData := PVData{
				storageClass: args.StorageClass,
				ns:           args.Namespace,
				cluster:      args.Name,
				mpath:        mount.Path,
				node:         k8sNodeName,
				target:       "target-" + strconv.Itoa(i),
				size:         pvSize,
			}
			// Create required PVs
			if pv, err := CreatePV(ctx, client, &pvData); err == nil {
				pvs = append(pvs, pv)
			}
		}
	}
	return pvs
}

func defineMounts(args *ClusterSpecArgs) []aisv1.Mount {
	var storagePrefix string
	if args.StorageHostPath == "" {
		storagePrefix = "/etc/ais"
	} else {
		storagePrefix = args.StorageHostPath
	}
	size1 := resource.MustParse("2Gi")
	size2 := resource.MustParse("1Gi")
	mounts := []aisv1.Mount{
		{
			Path:         path.Join(storagePrefix, "ais1"),
			Size:         &size1,
			StorageClass: &args.StorageClass,
		},
		{
			Path:         path.Join(storagePrefix, "ais2"),
			Size:         &size2,
			StorageClass: &args.StorageClass,
		},
	}
	for i := range mounts {
		mounts[i].Label = aisapc.Ptr("shared")
	}
	return mounts
}

func newAISClusterCR(args *ClusterSpecArgs, mounts []aisv1.Mount) *aisv1.AIStore {
	spec := aisv1.AIStoreSpec{
		Size:            &args.Size,
		ShutdownCluster: aisapc.Ptr(args.ShutdownCluster),
		CleanupMetadata: aisapc.Ptr(args.CleanupMetadata),
		CleanupData:     aisapc.Ptr(args.CleanupData),
		NodeImage:       args.NodeImage,
		InitImage:       args.InitImage,
		LogSidecar: &aisv1.LogSidecarSpec{
			Image: args.LogSidecarImage,
		},
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
			DisablePodAntiAffinity: &args.DisableTargetAntiAffinity,
		},
	}
	// If not using an LB, use the host port to provide external access
	if !args.EnableExternalLB {
		spec.APIMode = aisapc.Ptr(args.APIMode)
		spec.ProxySpec.HostPort = aisapc.Ptr(int32(51080))
		spec.TargetSpec.HostPort = aisapc.Ptr(int32(51081))
	}

	if args.TargetSize != 0 {
		spec.TargetSpec.Size = &args.TargetSize
	}

	if args.ProxySize != 0 {
		spec.ProxySpec.Size = &args.ProxySize
	}

	if args.EnableAdminClient {
		spec.AdminClient = &aisv1.AdminClientSpec{}
	}

	if args.EnableTargetPDB {
		spec.TargetSpec.PodDisruptionBudget = &aisv1.PDBSpec{Enabled: true}
	}

	if args.TLS != nil {
		spec.TLS = buildTLSSpec(args.TLS)
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

func buildTLSSpec(args *TLSArgs) *aisv1.TLSSpec {
	// Use existing secret
	if args.SecretName != "" {
		return &aisv1.TLSSpec{SecretName: &args.SecretName}
	}
	// Use cert-manager
	kind := args.IssuerKind
	if kind == "" {
		kind = "ClusterIssuer"
	}
	mode := aisv1.TLSCertificateModeSecret
	if args.Mode == "csi" {
		mode = aisv1.TLSCertificateModeCSI
	}
	return &aisv1.TLSSpec{
		Certificate: &aisv1.TLSCertificateConfig{
			IssuerRef: aisv1.CertIssuerRef{
				Name: args.IssuerName,
				Kind: kind,
			},
			Mode: mode,
		},
	}
}
