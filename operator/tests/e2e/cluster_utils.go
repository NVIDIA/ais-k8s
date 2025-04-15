// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"strings"
	"sync"

	"github.com/NVIDIA/aistore/cmn/cos"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
)

func clusterName() string {
	return "ais-test-" + strings.ToLower(cos.CryptoRandS(6))
}

func defaultCluArgs() *tutils.ClusterSpecArgs {
	return &tutils.ClusterSpecArgs{
		Name:                      clusterName(),
		Namespace:                 testNSName,
		StorageClass:              storageClass,
		StorageHostPath:           storageHostPath,
		Size:                      1,
		NodeImage:                 tutils.DefaultNodeImage,
		InitImage:                 tutils.DefaultInitImage,
		LogSidecarImage:           tutils.DefaultLogsImage,
		CleanupMetadata:           true,
		CleanupData:               true,
		DisableTargetAntiAffinity: false,
	}
}

func createClusters(clusters []*clientCluster, long bool) {
	var wg sync.WaitGroup
	wg.Add(len(clusters))

	for _, cluster := range clusters {
		go func(cc *clientCluster) {
			defer GinkgoRecover()
			defer wg.Done()
			cc.create(long)
		}(cluster)
	}
	wg.Wait()
}
