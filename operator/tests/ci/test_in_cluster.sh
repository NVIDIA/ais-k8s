#!/bin/bash

# Run E2E operator tests in CI using a cached image and test pod

set -euo pipefail

TEST_POD_NAME="operator-test-pod"
IMAGE_ARCHIVE_TAR="/operator-test.tar"
IMAGE_ARCHIVE_TARGZ="/operator-test.tar.gz"

# Apply RBAC permissions needed for the test pod
kubectl apply -k config/overlays/test

# Normalize to an uncompressed .tar at IMAGE_ARCHIVE_TAR
if [[ -f "${IMAGE_ARCHIVE_TAR}" ]]; then
  : # nothing to do
elif [[ -f "${IMAGE_ARCHIVE_TARGZ}" ]]; then
  echo "Decompressing ${IMAGE_ARCHIVE_TARGZ} to ${IMAGE_ARCHIVE_TAR}"
  # Overwrite any existing tar without prompting
  gzip -dc "${IMAGE_ARCHIVE_TARGZ}" > "${IMAGE_ARCHIVE_TAR}"
else
  echo "ERROR: Neither ${IMAGE_ARCHIVE_TAR} nor ${IMAGE_ARCHIVE_TARGZ} exists" >&2
  exit 1
fi

# Load the cached test image archive into the KinD cluster
kind load image-archive "${IMAGE_ARCHIVE_TAR}" --name "${KIND_CLUSTER_NAME}"

# Apply the test pod manifest with environment variable substitution
envsubst < scripts/test_pod.yaml | kubectl apply -f -

# Wait until the pod is ready
kubectl wait --for=condition=Ready "pod/${TEST_POD_NAME}" --timeout=120s

# Copy the current `operator` source into the pod for testing
kubectl cp . "${TEST_POD_NAME}:/operator"

# Execute tests inside the pod
kubectl exec "${TEST_POD_NAME}" -- bash -c "make -C /operator test-e2e"