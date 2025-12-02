#!/usr/bin/env bash
set -e

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <CLUSTER_NAME> <NODE1,NODE2,...|--all>"
  exit 1
fi

CLUSTER="$1"
NODE_ARG="$2"

if [[ "$NODE_ARG" == "--all" ]]; then
  IFS=' ' read -ra NODES <<< "$(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[*].metadata.name}')"
else
  IFS=',' read -ra NODES <<< "$NODE_ARG"
fi

if [[ ${#NODES[@]} -eq 0 ]]; then
  echo "Error: No nodes found"
  exit 1
fi

echo "Labeling ${#NODES[@]} nodes for cluster '$CLUSTER'"

for NODE in "${NODES[@]}"; do
  kubectl label node "$NODE" "nvidia.com/ais-proxy=$CLUSTER" "nvidia.com/ais-target=$CLUSTER" --overwrite
done
