/*
 * Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
 */

package aisauth_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIStoreAuthResources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIStoreAuth resources suite")
}
