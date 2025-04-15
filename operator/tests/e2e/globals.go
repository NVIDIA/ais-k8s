// Package e2e contains AIS operator integration tests
/*
 * Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
 */
package e2e

import (
	"testing"
	"time"

	aisclient "github.com/ais-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	k8sClient *aisclient.K8sClient
	testEnv   *envtest.Environment
	testCtx   *testing.T

	storageClass    string // storage-class to use in tests
	storageHostPath string // where to mount hostpath test storage
	testNS          *corev1.Namespace
	nsExists        bool
)

const (
	testNSName        = "ais-op-test"
	testNSAnotherName = "ais-op-test-other"

	EnvTestEnforceExternal = "TEST_EXTERNAL_CLIENT" // if set, will force the test suite to run as external client to deployed k8s cluster.
	EnvTestStorageClass    = "TEST_STORAGECLASS"
	EnvTestStorageHostPath = "TEST_STORAGE_HOSTPATH"
	BeforeSuiteTimeout     = 60
	AfterSuiteTimeout      = 60

	clusterCreateInterval     = time.Second
	clusterReadyRetryInterval = 5 * time.Second
	clusterReadyTimeout       = 3 * time.Minute
	clusterDestroyInterval    = 2 * time.Second
	clusterDestroyTimeout     = 2 * time.Minute
	clusterUpdateTimeout      = 30 * time.Second
	clusterUpdateInterval     = 2 * time.Second
)
