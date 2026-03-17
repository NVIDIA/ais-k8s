// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package target

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCommon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Target Suite")
}
