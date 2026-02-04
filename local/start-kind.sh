#!/usr/bin/env bash
set -e

create_kind_cluster() {
  local CLUSTER_NAME="$1"
  local SCRIPT_DIR="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

  if [ -z "${CLUSTER_NAME}" ]; then
    echo "Usage: create_kind_cluster <cluster-name> [script-dir]"
    return 1
  fi

  if kind get clusters | grep -qw "${CLUSTER_NAME}"; then
    echo "Cluster ${CLUSTER_NAME} already exists, skipping creation."
  else
    kind create cluster --config="${SCRIPT_DIR}/kind/config.yaml" --name="${CLUSTER_NAME}"
  fi

  # Verify we are running with the right context
  local CURRENT
  CURRENT=$(kubectl config current-context)

  if [ "${CURRENT}" != "kind-${CLUSTER_NAME}" ]; then
    echo "Warning: kubectl context does not match new KinD cluster!"
    return 1
  fi
}