// Package integration contains AIS operator integration tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package integration

import (
	"context"
	"testing"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	aistutils "github.com/NVIDIA/aistore/tools"
	aisv1 "github.com/ais-operator/api/v1beta1"
	"github.com/ais-operator/tests/tutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var (
	proxyURL string
	tests    = []TableEntry{
		Entry("Should be able to put and get objects", "PutGetObjects", putGetObjects),
	}
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

func putGetObjects(t *testing.T) {
	var (
		bck       = aiscmn.Bck{Name: "TEST_BUCKET", Provider: aisapc.AIS}
		objPrefix = "test-opr/"
	)
	aistutils.CreateBucketWithCleanup(t, proxyURL, bck, nil)
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
	aistutils.EnsureObjectsExist(t, aistutils.BaseAPIParams(proxyURL), bck, names...)
}

func runCustom(name string, method func(t *testing.T)) {
	var success bool
	defer func() {
		GinkgoRecover()
		Expect(success).To(BeTrue())
	}()
	safe := func(t *testing.T) {
		defer GinkgoRecover()
		method(t)
	}
	success = testCtx.Run(name, safe)
}
