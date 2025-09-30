#!/usr/bin/env bash
set -e

if [ "$#" -ne 3 ]; then
  echo "Usage: $0 <LABEL_NODES> <CLUSTER_NAME> <NODE1,NODE2,...>"
  exit 1
fi

LABEL_NODES="$1"
CLUSTER="$2"
NODE_LIST="$3"

if [[ "$LABEL_NODES" != "true" ]]; then
  echo "Skipping node labeling"
  exit 0
fi

if [[ -z "$CLUSTER" ]]; then
  echo "Error: Cluster name is required"
  exit 1
fi

if [[ -z "$NODE_LIST" ]]; then
  echo "Error: At least one node is required to label nodes. Check your config file in config/cluster-setup/<env>.yaml"
  exit 1
fi

# Convert comma-separated list to bash array
IFS=',' read -ra NODES <<< "$NODE_LIST"

echo "Found cluster: $CLUSTER"
echo "Labeling ${#NODES[@]} nodes with cluster '$CLUSTER'"

PROXY_LABEL=nvidia.com/ais-proxy="$CLUSTER"
TARGET_LABEL=nvidia.com/ais-target="$CLUSTER"

for NODE in "${NODES[@]}"; do
  echo "Labeling node $NODE with "$PROXY_LABEL" and "$TARGET_LABEL""
  kubectl label node $NODE "$PROXY_LABEL" "$TARGET_LABEL" --overwrite
done

echo "Node labeling completed for cluster $CLUSTER."
