#!/usr/bin/env bash
set -e

kind_provider() {
  if [ -n "${KIND_EXPERIMENTAL_PROVIDER:-}" ]; then
    echo "${KIND_EXPERIMENTAL_PROVIDER}"
  elif command -v docker >/dev/null 2>&1; then
    echo "docker"
  else
    echo "podman"
  fi
}

# `kind get clusters` fails against podman >= 6.0
# so query the container runtime for the cluster label directly when using podman.
kind_cluster_exists() {
  local cluster_name="${1:?cluster name required}"
  local tool
  tool="$(kind_provider)"
  if [ "$tool" = "podman" ]; then
    [ -n "$(podman ps -a --filter "label=io.x-k8s.kind.cluster=${cluster_name}" \
      --format '{{.Names}}' 2>/dev/null)" ]
  else
    kind get clusters 2>/dev/null | grep -qw "$cluster_name"
  fi
}

create_kind_cluster() {
  local CLUSTER_NAME="$1"
  local SCRIPT_DIR="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

  if [ -z "${CLUSTER_NAME}" ]; then
    echo "Usage: create_kind_cluster <cluster-name> [script-dir]"
    return 1
  fi

  if kind_cluster_exists "${CLUSTER_NAME}"; then
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