#!/bin/bash

# Script to run short and long operator tests in CI from an in-cluster pod; 
# requires a running KinD cluster, an external LoadBalancer provider for 
# external IPs (e.g. MetalLB or cloud-provider-kind), and to be run within
# a container running image `aistorage/gitlab-ci`.

set -eo pipefail

TEST_TYPE=${1:-short}

kubectl apply -f "${CI_PROJECT_DIR}/operator/tests/ci/rbac.yaml"
kind load image-archive /operator-test.tar && sleep 2  # Wait for image to register w/ containerd
kubectl run operator-test-pod --image=operator-test --image-pull-policy=Never --privileged
kubectl wait --for=condition=Ready pod/operator-test-pod --timeout=120s
kubectl cp "${CI_PROJECT_DIR}/operator" operator-test-pod:/operator
kubectl exec operator-test-pod -- bash -c "make -C /operator test-e2e-${TEST_TYPE}"
