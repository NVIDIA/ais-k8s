// Package client contains wrapper for k8s client
/*
 * Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package client

import (
	"testing"

	aisv1 "github.com/ais-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Client Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	err := scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = aisv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme
})

func newFakeClient(objs []runtime.Object) client.Client {
	return fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		WithScheme(scheme.Scheme).
		Build()
}
