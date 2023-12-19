#!/bin/bash
export NAD_NAME="$NAME"
export NAD_NAMESPACE="$NAMESPACE"
export NAD_IFACE="$INTERFACE"
source_dir=$(dirname "${BASH_SOURCE[0]}")

envsubst < "${source_dir}"/nad.template.yaml > /tmp/network-attachment-def.yaml
kubectl apply -f /tmp/network-attachment-def.yaml
rm /tmp/network-attachment-def.yaml