#!/bin/bash

# Run E2E operator tests in CI using a cached image and test pod

set -euo pipefail

TEST_POD_NAME="operator-test-pod"

# Apply RBAC permissions needed for the test pod
kubectl apply -k config/overlays/test

# Load the cached test image archive into the KinD cluster
kind load image-archive /operator-test.tar --name "${KIND_CLUSTER_NAME}"

# Apply the test pod manifest with environment variable substitution
envsubst < scripts/test_pod.yaml | kubectl apply -f -

# Wait until the pod is ready
kubectl wait --for=condition=Ready "pod/${TEST_POD_NAME}" --timeout=120s

# Copy the current `operator` source into the pod for testing
kubectl cp . "${TEST_POD_NAME}:/operator"

# Execute tests inside the pod
kubectl exec "${TEST_POD_NAME}" -- bash -c "make -C /operator test-e2e"