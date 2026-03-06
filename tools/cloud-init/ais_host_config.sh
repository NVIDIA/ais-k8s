#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# AIS Host Configuration Script
#
# Replicates the host-config Ansible playbooks (playbooks/host-config/) for
# standalone use on cloud hosts (cloud-init userdata, Packer, etc.).
#
# Configures: sysctl tuning, block device tuning, and (via ais_datafs.sh)
# filesystem creation and mounting. Does NOT deploy AIS itself.
#
# Requirements (see README.md for details):
#   - bash, udev (udevadm), sysctl, yq (mikefarah/yq)
#   - ais_datafs.sh (co-located) for filesystem setup; it requires
#     blkid, mount, mkfs.<fstype> (e.g. xfsprogs for XFS)
#   Use install_deps.sh to install missing packages automatically.
#
# See the CUSTOMIZATION SECTION below or pass values as environment variables.
#
# Usage:
#   sudo AIS_DEVICES="nvme1n1 nvme2n1" bash ais_host_config.sh
#
# As cloud-init userdata:
#   #!/bin/bash
#   export AIS_DEVICES="nvme1n1 nvme2n1 nvme3n1"
#   curl -fsSL <raw-url>/ais_host_config.sh | bash
###############################################################################

# ========================= YAML CONFIG FILE =================================
#
# Optionally provide settings via a YAML config file instead of (or in
# addition to) environment variables.  Requires yq (mikefarah/yq).
# See config.yaml.example for the expected structure.
#
# Precedence: explicit env var > config file value > script default
#
AIS_CONFIG="${AIS_CONFIG:-}"

_cfg() { yq -r "$1" "$AIS_CONFIG"; }
_cfg_set() {
    local var="$1" expr="$2"
    [[ -n "${!var:-}" ]] && return 0
    local val; val="$(_cfg "$expr")"
    if [[ "$val" != "null" && -n "$val" ]]; then
        printf -v "$var" '%s' "$val"
    fi
}

if [[ -n "$AIS_CONFIG" ]]; then
    [[ -f "$AIS_CONFIG" ]] || { echo "ERROR: Config file not found: $AIS_CONFIG" >&2; exit 1; }
    command -v yq &>/dev/null || { echo "ERROR: yq is required to read YAML config (run install_deps.sh)" >&2; exit 1; }

    _cfg_set AIS_DEVICES       '(.devices // []) | join(" ")'
    _cfg_set MPATH_PREFIX      '.mount_prefix'
    _cfg_set FSTYPE            '.fstype'
    _cfg_set FS_MOUNT_OPTIONS  '.mount_options'

    _cfg_set BLKDEVTUNE_PATTERN    '.blkdevtune.pattern'
    _cfg_set BLKDEV_READ_AHEAD_KB  '.blkdevtune.read_ahead_kb'

    _cfg_set SKIP_MKFS        '.skip.mkfs'
    _cfg_set SKIP_BLKDEVTUNE  '.skip.blkdevtune'
    _cfg_set SKIP_SYSCTL      '.skip.sysctl'
fi

# ========================= CUSTOMIZATION SECTION ============================
#
# Default values for all settings. If AIS_CONFIG was loaded above, those
# values are already set; the defaults below apply only to anything still
# unset. Explicit env vars always take precedence over both.

# -- Devices & Mounts ----------------------------------------------------------
#
# Block devices to format and mount (without /dev/ prefix), space-separated.
# Examples: "nvme1n1 nvme2n1 nvme3n1" or "sdb sdc sdd sde"
AIS_DEVICES="${AIS_DEVICES:-}"

# Mount path prefix — each device mounts at <prefix>/<device>.
MPATH_PREFIX="${MPATH_PREFIX:-/ais}"

# Filesystem type
FSTYPE="${FSTYPE:-xfs}"

# Mount options (XFS-optimized defaults from playbooks/host-config)
FS_MOUNT_OPTIONS="${FS_MOUNT_OPTIONS:-nofail,noatime,logbufs=8,logbsize=256k,swalloc}"

# -- Block Device Tuning (udev) ------------------------------------------------
#
# Kernel name pattern for devices to tune via udev rules.
# Supports shell-style globs: "nvme*", "sd*", "nvme[12]n1", etc.
BLKDEVTUNE_PATTERN="${BLKDEVTUNE_PATTERN:-nvme*}"

# read_ahead_kb value — no default (kernel default is typically 128).
# Only set if you want to override the kernel default.
BLKDEV_READ_AHEAD_KB="${BLKDEV_READ_AHEAD_KB:-}"

# -- Sysctl Tuning -------------------------------------------------------------
#
# Sysctl settings are defined entirely in the YAML config file under the
# "sysctl" key.  Each sub-key becomes a separate /etc/sysctl.d/ drop-in file,
# and its entries are arbitrary sysctl key-value pairs passed through verbatim.
# See config.yaml.example for the full structure and recommended defaults.
#
# When no AIS_CONFIG is provided, sysctl configuration is skipped.

# -- Feature Flags --------------------------------------------------------------
SKIP_MKFS="${SKIP_MKFS:-false}"        # true = mount existing FS, don't reformat
SKIP_BLKDEVTUNE="${SKIP_BLKDEVTUNE:-false}"
SKIP_SYSCTL="${SKIP_SYSCTL:-false}"

# ======================== END CUSTOMIZATION ==================================

log()  { echo "[ais-host-config] $(date '+%H:%M:%S') $*"; }
warn() { echo "[ais-host-config] WARNING: $*" >&2; }
die()  { echo "[ais-host-config] ERROR: $*" >&2; exit 1; }

require_root() {
    [[ $EUID -eq 0 ]] || die "This script must be run as root"
}

# =============================================================================
# 0. Dependency check
# =============================================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

check_dependencies() {
    local missing=()

    for cmd in sysctl udevadm; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done

    if [[ -n "$AIS_CONFIG" ]] && ! command -v yq &>/dev/null; then
        missing+=("yq")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        die "Missing required commands: ${missing[*]}
  Install them first, e.g.:  bash install_deps.sh"
    fi
}

# =============================================================================
# 1. Sysctl tuning (driven entirely by YAML config)
# =============================================================================
configure_sysctl() {
    [[ "$SKIP_SYSCTL" == "true" ]] && { log "Skipping sysctl (SKIP_SYSCTL=true)"; return 0; }

    if [[ -z "$AIS_CONFIG" ]]; then
        log "No AIS_CONFIG provided, skipping sysctl (sysctl settings are config-file-only)"
        return 0
    fi

    local has_sysctl
    has_sysctl="$(yq -r '(.sysctl | keys)[0] // ""' "$AIS_CONFIG")"
    if [[ -z "$has_sysctl" ]]; then
        log "No sysctl section in config, skipping"
        return 0
    fi

    log "Configuring sysctl..."

    rm -f /etc/sysctl.d/*-ais-*.conf

    local wrote=0
    for category in $(yq -r '.sysctl | keys | .[]' "$AIS_CONFIG"); do
        local outfile="/etc/sysctl.d/99-ais-${category}.conf"
        yq -r ".sysctl.${category} | to_entries | .[] | \"\(.key) = \(.value)\"" "$AIS_CONFIG" \
            > "$outfile"
        log "  wrote ${outfile}"
        wrote=$((wrote + 1))
    done

    if [[ $wrote -gt 0 ]]; then
        sysctl --system >/dev/null 2>&1
        log "sysctl configured and applied (${wrote} drop-in files)"
    fi
}

# =============================================================================
# 2. Block device tuning (udev rule)
# =============================================================================
configure_blkdevtune() {
    [[ "$SKIP_BLKDEVTUNE" == "true" ]] && { log "Skipping blkdevtune (SKIP_BLKDEVTUNE=true)"; return 0; }
    log "Configuring block device tuning via udev..."

    local rules_file="/etc/udev/rules.d/99-ais-blkdev.rules"

    if [[ -z "$BLKDEVTUNE_PATTERN" || -z "$BLKDEV_READ_AHEAD_KB" ]]; then
        log "No block device tuning values specified, skipping"
        return 0
    fi

    echo "ACTION==\"add|change\", KERNEL==\"${BLKDEVTUNE_PATTERN}\", ATTR{queue/read_ahead_kb}=\"${BLKDEV_READ_AHEAD_KB}\"" \
        > "$rules_file"
    log "  wrote ${rules_file}"

    udevadm control --reload-rules
    udevadm trigger --subsystem-match=block
    log "Block device tuning udev rules installed and triggered"
}

# =============================================================================
# 3. Data filesystems (delegates to ais_datafs.sh)
# =============================================================================
configure_datafs() {
    [[ -z "$AIS_DEVICES" ]] && { log "No AIS_DEVICES specified, skipping filesystem setup"; return 0; }

    local datafs_script="${SCRIPT_DIR}/ais_datafs.sh"
    [[ -f "$datafs_script" ]] || die "Cannot find ais_datafs.sh at ${datafs_script}"

    log "Delegating filesystem setup to ais_datafs.sh..."
    export AIS_DEVICES MPATH_PREFIX FSTYPE FS_MOUNT_OPTIONS SKIP_MKFS
    bash "$datafs_script"
}

# =============================================================================
# Main
# =============================================================================
require_root
log "=== AIS host configuration starting ==="
check_dependencies
log "Devices: ${AIS_DEVICES:-<none>}"
log "Mount prefix: ${MPATH_PREFIX}"

configure_sysctl
configure_blkdevtune
configure_datafs

log "=== AIS host configuration complete ==="
