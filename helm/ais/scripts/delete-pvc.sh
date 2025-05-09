#!/usr/bin/env bash

if [ $# -ne 2 ]; then
    echo "Usage: $0 <namespace> <storage-class>"
    exit 1
fi

NAMESPACE="$1"
STORAGE_CLASS="$2"

# Get PVCs in the specified namespace with the target storage class
PVC_LIST=$(kubectl get pvc -n "$NAMESPACE" -o jsonpath="{range .items[?(@.spec.storageClassName == \"$STORAGE_CLASS\")]}{.metadata.name}{'\n'}{end}")

# Delete each PVC
if [ -z "$PVC_LIST" ]; then
    echo "No PVCs found with storage class '$STORAGE_CLASS' in namespace '$NAMESPACE'"
else
    echo "Deleting PVCs in namespace '$NAMESPACE' with storage class '$STORAGE_CLASS':"
    echo "$PVC_LIST" | while read -r PVC; do
        kubectl delete pvc -n "$NAMESPACE" "$PVC"
    done
fi
