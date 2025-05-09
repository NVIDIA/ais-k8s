#!/usr/bin/env bash
set -e

if [ "$#" -ne 3 ]; then
  echo "Usage: $0 <CREATE_PVS> <VALUES_YAML> <RELEASE_NAMESPACE>"
  exit 1
fi

CREATE_PVS="$1"
VALUES_YAML="$2"
RELEASE_NAMESPACE="$3"

if [[ "$CREATE_PVS" == "true" ]]; then
  echo "Templating and applying PersistentVolumes with claimRef namespace "$RELEASE_NAMESPACE" and values from $VALUES_YAML "
  helm template ais-create-pv ./charts/create-pv -f "$VALUES_YAML" --set namespace="$RELEASE_NAMESPACE" | kubectl apply -f -
else
  echo "Skipping PersistentVolume creation."
fi
