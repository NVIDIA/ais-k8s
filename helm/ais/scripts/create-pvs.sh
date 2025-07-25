#!/usr/bin/env bash
set -e

if [ "$#" -ne 4 ]; then
  echo "Usage: $0 <CREATE_PVS> <AIS_VALUES_YAML> <PV_VALUES_YAML> <RELEASE_NAMESPACE>"
  exit 1
fi

CREATE_PVS="$1"
AIS_VALUES_YAML="$2"
PV_VALUES_YAML="$3"
RELEASE_NAMESPACE="$4"

echo "Creating PersistentVolumes with the following parameters:"
echo "CREATE_PVS: $CREATE_PVS"
echo "AIS_VALUES_YAML: $AIS_VALUES_YAML"
echo "PV_VALUES_YAML: $PV_VALUES_YAML"
echo "RELEASE_NAMESPACE: $RELEASE_NAMESPACE"

if [[ "$CREATE_PVS" == "true" ]]; then
  echo "Templating and applying PersistentVolumes with claimRef namespace "$RELEASE_NAMESPACE" and values from $AIS_VALUES_YAML and $PV_VALUES_YAML"
  helm template ais-create-pv ./charts/create-pv -f "$AIS_VALUES_YAML" -f "$PV_VALUES_YAML" --set namespace="$RELEASE_NAMESPACE" | kubectl apply -f -
else
  echo "Skipping PersistentVolume creation."
fi
