#!/usr/bin/env bash
set -e

REMOVE=false
while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --remove) REMOVE=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 [--remove] <CLUSTER_NAME> <NODE1,NODE2,...|--all>"
  exit 1
fi

CLUSTER="$1"
NODE_ARG="$2"

if [[ "$NODE_ARG" == "--all" ]]; then
  if $REMOVE; then
    # Nodes carrying either AIS label for this cluster
    LABELED=""
    for KEY in nvidia.com/ais-proxy nvidia.com/ais-target; do
      LABELED+=" $(kubectl get nodes -l "$KEY=$CLUSTER" -o jsonpath='{.items[*].metadata.name}')"
    done
    IFS=' ' read -ra NODES <<< "$(tr ' ' '\n' <<< "$LABELED" | sort -u | tr '\n' ' ')"
  else
    IFS=' ' read -ra NODES <<< "$(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[*].metadata.name}')"
  fi
else
  IFS=',' read -ra NODES <<< "$NODE_ARG"
fi

if [[ ${#NODES[@]} -eq 0 ]]; then
  echo "Error: No nodes found"
  exit 1
fi

if $REMOVE; then
  LABEL_ARGS=(nvidia.com/ais-proxy- nvidia.com/ais-target-)
  echo "Removing AIS labels from ${#NODES[@]} nodes for cluster '$CLUSTER'"
else
  LABEL_ARGS=("nvidia.com/ais-proxy=$CLUSTER" "nvidia.com/ais-target=$CLUSTER" --overwrite)
  echo "Labeling ${#NODES[@]} nodes for cluster '$CLUSTER'"
fi

for NODE in "${NODES[@]}"; do
  kubectl label node "$NODE" "${LABEL_ARGS[@]}"
done
