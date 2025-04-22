#!/bin/bash

# Script to create KinD cluster and setup extra components (e.g. LPP, external LB provider) for testing.

set -eo pipefail

LPP_VERSION=v0.0.31

# TODO: Revisit (issue w/ trying to export logs via `kind export logs` in `after_script`)
mkdir -p /ci-kind-logs/{control-plane,worker1,worker2,worker3}
chmod -R 755 /ci-kind-logs

kind create cluster --config operator/tests/ci/kind_cfg.yaml --retain
kubectl cluster-info --context kind-kind

cloud-provider-kind > cloud-provider-kind.log 2>&1 &

kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LPP_VERSION}/deploy/local-path-storage.yaml"