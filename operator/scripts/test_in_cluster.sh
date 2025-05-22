#!/bin/bash

# Run E2E operator tests locally from an in-cluster test pod

set -euo pipefail

IMAGE_NAME="operator-test"
TEST_POD_NAME="operator-test-pod"

cleanup() {
  kubectl delete pod "${TEST_POD_NAME}" --ignore-not-found
  kubectl delete -f scripts/rbac.yaml --ignore-not-found
}
trap cleanup EXIT

# Apply RBAC permissions needed for the test pod
kubectl apply -f scripts/rbac.yaml

# Build test image and load it into the local KinD cluster
docker build -t "${IMAGE_NAME}" -f tests/test.dockerfile .
kind load docker-image "${IMAGE_NAME}" --name "${KIND_CLUSTER_NAME}"

# Apply the test pod manifest with environment variable substitution
envsubst < scripts/test_pod.yaml | kubectl apply -f -

# Wait until the pod is ready
kubectl wait --for=condition=Ready "pod/${TEST_POD_NAME}" --timeout=120s

# Execute tests inside the pod
kubectl exec "${TEST_POD_NAME}" -- bash -c "make -C /operator test-e2e"