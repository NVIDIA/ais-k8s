#!/usr/bin/env bash

if [ $# -ne 1 ]; then
  echo "Usage: $0 <storage-class>"
  exit 1
fi

STORAGE_CLASS="$1"

# Get all PVs with given storage class and status Released
PVS=$(kubectl get pv \
  --no-headers \
  -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,SC:.spec.storageClassName \
  | awk -v sc="$STORAGE_CLASS" '$2 == "Released" && $3 == sc {print $1}')

if [ -z "$PVS" ]; then
  echo "No Released PVs found for storage class '$STORAGE_CLASS'."
  exit 0
fi

echo "Deleting the following Released PVs with storage class '$STORAGE_CLASS':"
echo "$PVS"

read -p "Are you sure you want to delete ALL of these PVs? [y/N]: " CONFIRM
case "$CONFIRM" in
  [yY][eE][sS]|[yY]) ;;
  *) echo "Aborted. No PVs were deleted."; exit 0 ;;
esac

for pv in $PVS; do
  kubectl delete pv "$pv"
done