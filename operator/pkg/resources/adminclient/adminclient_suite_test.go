// Package adminclient contains resources for the AIS admin client deployment
/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */
package adminclient

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdminClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AdminClient Suite")
}
