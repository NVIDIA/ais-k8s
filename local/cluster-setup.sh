#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "${SCRIPT_DIR}/start-kind.sh"

install_prereqs() {
    local helm_root="${SCRIPT_DIR}/../helm"
    export SKIP_CONFIRM="true"

    echo "Installing prerequisites (cert-manager, storage class)..."
    helmfile -f "${SCRIPT_DIR}/prereq-helmfile.yaml" sync

    echo "Waiting for cert-manager API service to be available..."
    kubectl wait --for=condition=Available --timeout=120s apiservice/v1.cert-manager.io

    echo "Waiting for trust-manager API service to be available..."
    kubectl wait --for=condition=Available --timeout=120s apiservice/v1alpha1.trust.cert-manager.io

    echo "Setting up cluster issuer..."
    (cd "${helm_root}/cluster-issuer" && helmfile sync -e local)

    echo "Creating AIS namespace..."
    kubectl create namespace ais 2>/dev/null || true
    kubectl label namespace ais ais-trust=true --overwrite

    echo "Creating operator namespace..."
    kubectl create namespace ais-operator-system 2>/dev/null || true
    kubectl label namespace ais-operator-system ais-trust=true --overwrite

    echo "Labeling nodes for AIS scheduling..."
    "${helm_root}/ais/scripts/label-nodes.sh" ais --all

    echo "Creating trust manager bundle..."
    kubectl apply -f "${SCRIPT_DIR}/manifests/trust-bundle.yaml"
}

setup_cluster() {
    local cluster_name="${1:?cluster name required}"
    create_kind_cluster "$cluster_name" "$SCRIPT_DIR"
    install_prereqs
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    setup_cluster "${1:-local-test}"
fi
