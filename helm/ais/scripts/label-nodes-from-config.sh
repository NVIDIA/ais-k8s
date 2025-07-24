#!/usr/bin/env bash
set -e

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <LABEL_NODES> <VALUES_YAML>"
  exit 1
fi

LABEL_NODES="$1"
VALUES_YAML="$2"

if [[ "$LABEL_NODES" == "true" ]]; then
  echo "Extracting cluster and node information from $VALUES_YAML"
  
  # Check if yq is available, fail if not found
  if ! command -v yq >/dev/null 2>&1; then
    echo "Error: yq is required but not installed."
    echo ""
    echo "Please install yq to parse YAML files properly:"
    echo "  - On macOS: brew install yq"
    echo "  - On Ubuntu/Debian: sudo snap install yq"
    echo "  - On RHEL/CentOS: sudo yum install yq"
    echo "  - Via Go: go install github.com/mikefarah/yq/v4@latest"
    echo "  - Download binary: https://github.com/mikefarah/yq/releases"
    echo ""
    echo "yq is essential for node labeling. Please install it and try again."
    exit 1
  fi

  CLUSTER=$(yq eval '.global.cluster' "$VALUES_YAML")
  NODES=$(yq eval '.global.nodes[]' "$VALUES_YAML")
  
  if [[ -z "$CLUSTER" ]]; then
    echo "Error: Could not extract cluster name from $VALUES_YAML"
    exit 1
  fi
  
  if [[ -z "$NODES" ]]; then
    echo "Error: Could not extract nodes from $VALUES_YAML"
    exit 1
  fi
  
  echo "Found cluster: $CLUSTER"
  echo "Found nodes:"
  echo "$NODES"
  
  # Convert nodes to array and label them directly
  NODE_ARRAY=($NODES)
  echo "Labeling ${#NODE_ARRAY[@]} nodes with cluster '$CLUSTER'"
  
  # Label each node directly
  for NODE in "${NODE_ARRAY[@]}"; do
    echo "Labeling node $NODE"
    kubectl label node "$NODE" \
      nvidia.com/ais-proxy="$CLUSTER" \
      nvidia.com/ais-target="$CLUSTER" \
      --overwrite
  done
  
  echo "Node labeling completed for cluster $CLUSTER."
else
  echo "Skipping node labeling."
fi 