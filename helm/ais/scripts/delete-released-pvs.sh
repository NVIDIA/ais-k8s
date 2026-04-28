#!/usr/bin/env bash
set -euo pipefail

DRY_RUN=false
while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [ $# -ne 2 ]; then
    echo "Delete all 'Released' K8s PersistentVolumes with a given cluster label and storage class"
    echo "Usage: $0 [--dry-run] <cluster> <storage-class>"
    exit 1
fi

CLUSTER="$1"
STORAGE_CLASS="$2"

if $DRY_RUN; then
    echo "=== DRY RUN MODE — no resources will be modified ==="
    echo ""
fi

RELEASED_PVS=$(kubectl get pv \
  -l "cluster=$CLUSTER" \
  -o jsonpath="{range .items[?(@.spec.storageClassName==\"$STORAGE_CLASS\")]}{.metadata.name} {.status.phase}{'\n'}{end}" \
  | awk '$2 == "Released" {print $1}')

if [ -z "$RELEASED_PVS" ]; then
    echo "No Released PVs found for cluster '$CLUSTER' with storage class '$STORAGE_CLASS'."
    exit 0
fi

echo "Released PVs to delete for cluster '$CLUSTER':"
# shellcheck disable=SC2086
kubectl get pv $RELEASED_PVS
echo ""
PV_COUNT=$(echo "$RELEASED_PVS" | wc -l | tr -d ' ')

if $DRY_RUN; then
    echo "[dry run] Would delete $PV_COUNT PV(s)."
else
    read -r -p "Delete these $PV_COUNT PV(s)? [y/N]: " CONFIRM
    case "$CONFIRM" in
        [yY][eE][sS]|[yY]) ;;
        *) echo "Aborted. No PVs were deleted."; exit 0 ;;
    esac

    # shellcheck disable=SC2086
    kubectl delete pv $RELEASED_PVS
fi
