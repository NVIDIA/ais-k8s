// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021-2024, NVIDIA CORPORATION. All rights reserved.
 */
package integration

import (
	"context"
	"time"

	aisapi "github.com/NVIDIA/aistore/api"
	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var (
	proxyURL string
)

// Initialize AIS tutils to use the deployed cluster
func initAISCluster(ctx context.Context, cluster *aisv1.AIStore) {
	proxyURL = tutils.GetProxyURL(ctx, k8sClient, cluster)
	var (
		retries = 2
		err     error
	)
	for retries > 0 {
		err = aistutils.WaitNodeReady(proxyURL, &aistutils.WaitRetryOpts{
			MaxRetries: 12,
			Interval:   10 * time.Second,
		})
		if err == nil {
			break
		}
		retries--
		time.Sleep(5 * time.Second)
	}

	// Wait until the cluster has actually started (targets have registered).
	Expect(err).To(BeNil())
	Expect(aistutils.InitCluster(proxyURL, aistutils.ClusterTypeK8s)).NotTo(HaveOccurred())
}

var _ = Describe("Client tests", Ordered, Label("short"), func() {
	var (
		cc  *clientCluster
		pvs []*corev1.PersistentVolume
	)
	BeforeAll(func() {
		cluArgs := defaultCluArgs()
		cluArgs.EnableExternalLB = testAsExternalClient
		cc, pvs = newClientCluster(cluArgs)
		cc.create()
	})

	It("Should be able to put and get objects", func() {
		var (
			bck       = aiscmn.Bck{Name: "TEST_BUCKET", Provider: aisapc.AIS}
			objPrefix = "test-opr/"
			baseParam = aistutils.BaseAPIParams(proxyURL)
		)
		// Since we are using the same mounts and bucket name, prior test failures may need cleanup
		aisapi.DestroyBucket(baseParam, bck)
		err := aisapi.CreateBucket(baseParam, bck, nil)
		Expect(err).ShouldNot(HaveOccurred())
		names, failCnt, err := aistutils.PutRandObjs(aistutils.PutObjectsArgs{
			ProxyURL:  proxyURL,
			Bck:       bck,
			ObjPath:   objPrefix,
			ObjCnt:    10,
			ObjSize:   10 * cos.KiB,
			FixedSize: true,
			CksumType: cos.ChecksumXXHash,
			IgnoreErr: false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(failCnt).To(Equal(0))
		aistutils.EnsureObjectsExist(testCtx, aistutils.BaseAPIParams(proxyURL), bck, names...)
	})

	AfterAll(func() {
		By("Executing final cleanup")
		cc.cleanup(pvs)
	})
})
