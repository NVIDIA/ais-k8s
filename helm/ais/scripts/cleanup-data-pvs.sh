#!/usr/bin/env bash
set -euo pipefail

DRY_RUN=false
while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [ $# -ne 3 ]; then
    echo "Warning! This script deletes AIS data K8s PersistentVolumes and associated PVCs"
    echo "Usage: $0 [--dry-run] <namespace> <cluster> <storage-class>"
    exit 1
fi

NAMESPACE="$1"
CLUSTER="$2"
STORAGE_CLASS="$3"

if $DRY_RUN; then
    echo "=== DRY RUN MODE — no resources will be modified ==="
    echo ""
fi

# --- Phase 1: Delete PVCs ---

LABEL_SELECTOR="app.kubernetes.io/component=target,app.kubernetes.io/name=${CLUSTER}"

PVC_LIST=$(kubectl get pvc -n "$NAMESPACE" \
  -l "$LABEL_SELECTOR" \
  -o jsonpath="{range .items[?(@.spec.storageClassName==\"$STORAGE_CLASS\")]}{.metadata.name}{'\n'}{end}")

if [ -z "$PVC_LIST" ]; then
    echo "No PVCs found with storage class '$STORAGE_CLASS' and labels '$LABEL_SELECTOR' in namespace '$NAMESPACE'."
else
    echo "PVCs to delete in namespace '$NAMESPACE':"
    # shellcheck disable=SC2086
    kubectl get pvc -n "$NAMESPACE" $PVC_LIST
    echo ""
    PVC_COUNT=$(echo "$PVC_LIST" | wc -l | tr -d ' ')

    if $DRY_RUN; then
        echo "[dry run] Would delete $PVC_COUNT PVC(s)."
        # shellcheck disable=SC2086
        BOUND_PVS=$(kubectl get pvc -n "$NAMESPACE" $PVC_LIST \
          -o jsonpath='{range .items[*]}{.spec.volumeName}{"\n"}{end}')
    else
        read -r -p "Delete these $PVC_COUNT PVC(s)? [y/N]: " CONFIRM
        case "$CONFIRM" in
            [yY][eE][sS]|[yY]) ;;
            *) echo "Aborted. No PVCs were deleted."; exit 0 ;;
        esac
        # Get the backing PV names before deleting the PVCs
        # shellcheck disable=SC2086
        BOUND_PVS=$(kubectl get pvc -n "$NAMESPACE" $PVC_LIST \
          -o jsonpath='{range .items[*]}{.spec.volumeName}{"\n"}{end}')

        # shellcheck disable=SC2086
        kubectl delete pvc -n "$NAMESPACE" $PVC_LIST

        if [ -n "$BOUND_PVS" ]; then
            echo "Waiting for PVs to be released..."
            # shellcheck disable=SC2086
            if ! kubectl wait pv $BOUND_PVS \
              --for=jsonpath='{.status.phase}'=Released --timeout=60s; then
                echo "Warning: not all PVs reached Released state; continuing anyway."
            fi
        fi
    fi
fi

# --- Phase 2: Delete Released PVs ---

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if $DRY_RUN; then
    # In dry-run mode, PVCs weren't deleted so PVs are still Bound; include them
    # along with any already-Released PVs.
    RELEASED_PVS=$(kubectl get pv \
      -l "cluster=$CLUSTER" \
      -o jsonpath="{range .items[?(@.spec.storageClassName==\"$STORAGE_CLASS\")]}{.metadata.name} {.status.phase}{'\n'}{end}" \
      | awk '$2 == "Released" {print $1}')

    if [ -n "${BOUND_PVS:-}" ]; then
        PV_LIST=$(printf '%s\n%s' "${RELEASED_PVS:-}" "$BOUND_PVS" | sort -u | grep -v '^$' || true)
    else
        PV_LIST="$RELEASED_PVS"
    fi
    if [ -z "$PV_LIST" ]; then
        echo "No PVs found for cluster '$CLUSTER'."
        exit 0
    fi
    echo ""
    echo "PVs that would be deleted for cluster '$CLUSTER':"
    # shellcheck disable=SC2086
    kubectl get pv $PV_LIST
    echo ""
    PV_COUNT=$(echo "$PV_LIST" | wc -l | tr -d ' ')
    echo "[dry run] Would delete $PV_COUNT PV(s)."
else
    "$SCRIPT_DIR/delete-released-pvs.sh" "$CLUSTER" "$STORAGE_CLASS"
fi
