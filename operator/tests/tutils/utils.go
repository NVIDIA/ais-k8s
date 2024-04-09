// Package tutils provides utilities for running AIS operator tests
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package tutils

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/NVIDIA/aistore/cmn/k8s"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

const ClusterCreateInterval = time.Second

type SkipArgs struct {
	RequiredProvider string
	RequiresLB       bool
	SkipInternal     bool // test should run inside K8s cluster
}

func GetClusterCreateTimeout() time.Duration {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 4 * time.Minute
	}
	return 2 * time.Minute
}

func GetClusterCreateLongTimeout() time.Duration {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 6 * time.Minute
	}
	return 4 * time.Minute
}

func GetLBExistenceTimeout() (timeout, interval time.Duration) {
	if GetK8sClusterProvider() == K8sProviderGKE {
		return 4 * time.Minute, 5 * time.Second
	}
	return 10 * time.Second, 200 * time.Millisecond
}

func CheckSkip(args *SkipArgs) {
	if args.SkipInternal {
		ginkgo.Skip("Skipping test; requires test to run inside K8s cluster")
	}

	if args.RequiresLB {
		SkipIfLoadBalancerNotSupported()
	}

	if args.RequiredProvider != "" && args.RequiredProvider != GetK8sClusterProvider() {
		ginkgo.Skip(fmt.Sprintf("Skipping test; required provider %q, got %q", args.RequiredProvider, GetK8sClusterProvider()))
	}
}

func isTunnelRunning() bool {
	out, err := exec.Command("ps", "aux").Output()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	vals := strings.Split(string(out), "\n")
	for _, val := range vals {
		if strings.Contains(val, "minikube") && strings.Contains(val, "tunnel") {
			return true
		}
	}
	return false
}

// helpers
func SkipIfLoadBalancerNotSupported() {
	// If the tests are running against non-minikube cluster or inside a pod within K8s cluster
	// we cannot determine if the LoadBalancer service is supported. Proceed to running tests.
	if GetK8sClusterProvider() != K8sProviderMinikube || !k8s.IsK8s() {
		return
	}

	// If test is running against local minikube, check if `minikube tunnel` is running.
	if !isTunnelRunning() {
		ginkgo.Skip("Test requires the cluster to support LoadBalancer service.")
	}
}
