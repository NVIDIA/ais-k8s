#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# AIS Data Filesystem Setup
#
# Creates filesystems on block devices and mounts them for use as AIS data
# paths. Corresponds to the ais_datafs_mkfs / ais_datafs_mount Ansible
# playbooks (playbooks/host-config/).
#
# Can be run standalone or called automatically by ais_host_config.sh.
#
# Requirements:
#   - blkid, mount, mountpoint (util-linux)
#   - mkfs.<fstype>, e.g. mkfs.xfs (xfsprogs) or mkfs.ext4 (e2fsprogs)
#
# Usage:
#   sudo AIS_DEVICES="nvme1n1 nvme2n1" bash ais_datafs.sh
#   sudo AIS_DEVICES="sdb sdc" SKIP_MKFS=true bash ais_datafs.sh
###############################################################################

AIS_DEVICES="${AIS_DEVICES:-}"
MPATH_PREFIX="${MPATH_PREFIX:-/ais}"
FSTYPE="${FSTYPE:-xfs}"
FS_MOUNT_OPTIONS="${FS_MOUNT_OPTIONS:-nofail,noatime,logbufs=8,logbsize=256k,swalloc}"
SKIP_MKFS="${SKIP_MKFS:-false}"

log()  { echo "[ais-datafs] $(date '+%H:%M:%S') $*"; }
warn() { echo "[ais-datafs] WARNING: $*" >&2; }
die()  { echo "[ais-datafs] ERROR: $*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "This script must be run as root"

# =============================================================================
# Dependency check
# =============================================================================
check_dependencies() {
    local missing=()
    for cmd in blkid mount mkfs mountpoint; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ "$FSTYPE" == "xfs" ]] && ! command -v mkfs.xfs &>/dev/null; then
        missing+=("mkfs.xfs (xfsprogs)")
    fi
    if [[ ${#missing[@]} -gt 0 ]]; then
        die "Missing required commands: ${missing[*]}
  Install them first, e.g.:  bash install_deps.sh"
    fi
}

# =============================================================================
# Create filesystems
# =============================================================================
create_filesystems() {
    [[ "$SKIP_MKFS" == "true" ]] && { log "Skipping mkfs (SKIP_MKFS=true)"; return 0; }
    log "Creating ${FSTYPE} filesystems on: ${AIS_DEVICES}..."

    local force_flag=""
    case "$FSTYPE" in
        ext2|ext3|ext4) force_flag="-F" ;;
        xfs)            force_flag="-f" ;;
        *)              warn "No force flag known for FSTYPE=${FSTYPE}" ;;
    esac

    local pids=()
    for d in $AIS_DEVICES; do
        local dev="/dev/${d}"
        [[ -b "$dev" ]] || die "Block device $dev does not exist"
        log "  mkfs -t ${FSTYPE} ${force_flag} ${dev} (background)"
        mkfs -t "$FSTYPE" $force_flag "$dev" &
        pids+=($!)
    done

    local failed=0
    for pid in "${pids[@]}"; do
        wait "$pid" || { warn "mkfs failed (pid $pid)"; failed=1; }
    done
    [[ $failed -eq 0 ]] || die "One or more mkfs operations failed"
    log "All filesystems created"
}

# =============================================================================
# Mount filesystems
# =============================================================================
mount_filesystems() {
    log "Mounting filesystems at ${MPATH_PREFIX}/..."

    for d in $AIS_DEVICES; do
        local dev="/dev/${d}"
        local mnt="${MPATH_PREFIX}/${d}"
        local uuid

        uuid=$(blkid -s UUID -o value "$dev" 2>/dev/null) || true
        if [[ -z "$uuid" ]]; then
            warn "Could not determine UUID for $dev, skipping mount"
            continue
        fi

        mkdir -p "$mnt"

        # Idempotent fstab entry
        if ! grep -q "UUID=${uuid}" /etc/fstab 2>/dev/null; then
            echo "UUID=${uuid} ${mnt} ${FSTYPE} ${FS_MOUNT_OPTIONS} 0 0" >> /etc/fstab
        fi

        if ! mountpoint -q "$mnt" 2>/dev/null; then
            mount "$mnt"
        fi

        chmod 0750 "$mnt"
        chown root:root "$mnt"
        log "  ${dev} -> ${mnt} (UUID=${uuid})"
    done
    log "All filesystems mounted"
}

# =============================================================================
# Main
# =============================================================================
if [[ -z "$AIS_DEVICES" ]]; then
    log "No AIS_DEVICES specified, nothing to do"
    exit 0
fi

check_dependencies
log "=== AIS data filesystem setup starting ==="
log "Devices: ${AIS_DEVICES}"
log "Mount prefix: ${MPATH_PREFIX}"
log "Filesystem: ${FSTYPE}"

create_filesystems
mount_filesystems

log "=== AIS data filesystem setup complete ==="
