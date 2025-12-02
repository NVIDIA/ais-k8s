#!/usr/bin/env bash
set -e

if [ "$#" -ne 4 ]; then
  echo "Usage: $0 <CREATE_PVS> <AIS_VALUES_YAML> <CLUSTER_NAME> <RELEASE_NAMESPACE>"
  exit 1
fi

CREATE_PVS="$1"
AIS_VALUES_YAML="$2"
CLUSTER_NAME="$3"
RELEASE_NAMESPACE="$4"

echo "Creating PersistentVolumes with the following parameters:"
echo "CREATE_PVS: $CREATE_PVS"
echo "AIS_VALUES_YAML: $AIS_VALUES_YAML"
echo "CLUSTER_NAME: $CLUSTER_NAME"
echo "RELEASE_NAMESPACE: $RELEASE_NAMESPACE"

if [[ ! "$CREATE_PVS" == "true" ]]; then
  echo "Skipping PersistentVolume creation."
  exit 0
fi

if [ ! -f $AIS_VALUES_YAML ]; then
  echo "AIS values file '$AIS_VALUES_YAML' does not exist."
  exit 1
fi

if [[ -z "$CLUSTER_NAME" ]]; then
  echo "Error: Cluster name is required"
  exit 1
fi

# Query K8s for nodes with nvidia.com/ais-target=<cluster>
NODES=$(kubectl get nodes -l "nvidia.com/ais-target=$CLUSTER_NAME" -o jsonpath='{.items[*].metadata.name}' | tr ' ' ',')

if [[ -z "$NODES" ]]; then
  echo "Error: No nodes found with label nvidia.com/ais-target=$CLUSTER_NAME. Please ensure your nodes are labeled beforehand."
  exit 1
fi

echo "Templating and applying PersistentVolumes for nodes: $NODES"
helm template ais-create-pv ./charts/create-pv -f "$AIS_VALUES_YAML" --set namespace="$RELEASE_NAMESPACE" --set-string nodes="{$NODES}" | kubectl apply -f -
