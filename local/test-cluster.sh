#!/usr/bin/env bash
set -e

# This script
# 1. Creates a kind cluster
# 2. Installs necessary prerequisites
# 3. Applies helm charts for the local environment for a local issuer, operator, and AIS cluster

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="${SCRIPT_DIR}/../helm"
CLUSTER_NAME="local-test"
# Used for skipping confirmation of each helm sync
export SKIP_CONFIRM="true"

source "${SCRIPT_DIR}"/start-kind.sh
create_kind_cluster $CLUSTER_NAME

# Install pre-reqs -- certmanager, storage class etc.
helmfile -f prereq-helmfile.yaml sync

# Cluster issuer
cd "${HELM_ROOT}/cluster-issuer"
helmfile sync -e local

# Make sure AIS namespace exists and is labeled for trust manager bundle
kubectl create namespace ais 2>/dev/null || true
kubectl label namespace ais ais-trust=true

# Label nodes for AIS scheduling
"${HELM_ROOT}"/ais/scripts/label-nodes.sh ais --all

# Create trust manager bundle
kubectl apply -f "${SCRIPT_DIR}/manifests/trust-bundle.yaml"

# TODO: Add option to deploy from local/kustomize output
# Operator with cert and AIS cert verification
cd "${HELM_ROOT}/operator"
helmfile sync -e local
echo "Waiting for AIS operator to be ready..."
kubectl wait --for=condition=available --timeout=120s deployment/ais-operator-controller-manager -n ais-operator-system
echo "AIS operator is ready!"

# TODO: Mount certs with certmanager CSI
# AIS local env with admin client and certs
cd "${HELM_ROOT}/ais"
helmfile sync -e local

echo "Waiting for AIStore cluster to be ready..."
kubectl wait --for=jsonpath='{.status.state}'=Ready --timeout=300s aistore/ais -n ais
echo ""
echo "==================================================================="
echo "AIStore cluster deployed successfully!"
echo "==================================================================="
echo ""
echo "To connect to the admin client and run AIS commands:"
echo ""
echo "  kubectl exec -it -n ais deploy/ais-client -- /bin/bash"
echo ""
echo "==================================================================="
