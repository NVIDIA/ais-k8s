// Package services contains services for the operator to use when reconciling AIS
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package services

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestServices(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Services Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})
