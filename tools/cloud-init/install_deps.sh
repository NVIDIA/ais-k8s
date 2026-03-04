#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# Install dependencies for ais_host_config.sh and ais_datafs.sh
#
# Auto-detects the package manager (apt-get / dnf / yum) and installs any
# packages required by the AIS host config scripts that are not already present.
# Also installs yq (mikefarah/yq) for YAML config file support.
#
# Usage:
#   sudo bash install_deps.sh                    # default: FSTYPE=xfs
#   sudo FSTYPE=ext4 bash install_deps.sh        # skip xfsprogs
###############################################################################

FSTYPE="${FSTYPE:-xfs}"

die() { echo "ERROR: $*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "This script must be run as root"

# ---------------------------------------------------------------------------
# Detect package manager
# ---------------------------------------------------------------------------
PKG_MGR=""
if command -v apt-get &>/dev/null; then
    PKG_MGR="apt-get"
elif command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
elif command -v yum &>/dev/null; then
    PKG_MGR="yum"
else
    die "No supported package manager found (need apt-get, dnf, or yum)"
fi

pkg_install() {
    echo "Installing: $*"
    case "$PKG_MGR" in
        apt-get) apt-get update -qq && apt-get install -y -qq "$@" ;;
        dnf|yum) $PKG_MGR install -y -q "$@" ;;
    esac
}

# ---------------------------------------------------------------------------
# Build list of missing packages
# ---------------------------------------------------------------------------
needed=()

# Always required
command -v curl       &>/dev/null || needed+=(curl)
command -v sysctl     &>/dev/null || needed+=(procps)
command -v blkid      &>/dev/null || needed+=(util-linux)
command -v mount      &>/dev/null || needed+=(util-linux)
command -v udevadm    &>/dev/null || needed+=(udev)

if [[ "$FSTYPE" == "xfs" ]] && ! command -v mkfs.xfs &>/dev/null; then
    needed+=(xfsprogs)
fi

# Deduplicate
if [[ ${#needed[@]} -gt 0 ]]; then
    mapfile -t needed < <(printf '%s\n' "${needed[@]}" | sort -u)
fi

# ---------------------------------------------------------------------------
# Install system packages
# ---------------------------------------------------------------------------
if [[ ${#needed[@]} -eq 0 ]]; then
    echo "All system package dependencies already satisfied."
else
    pkg_install "${needed[@]}"
fi

# ---------------------------------------------------------------------------
# Install yq (not available in standard repos; installed from GitHub binary)
# ---------------------------------------------------------------------------
YQ_VERSION="${YQ_VERSION:-v4.52.2}"

install_yq() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
        *)       die "Unsupported architecture for yq: $arch" ;;
    esac
    local url="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${arch}"
    echo "Installing yq ${YQ_VERSION} (${arch})..."
    curl -fsSL "$url" -o /usr/local/bin/yq
    chmod +x /usr/local/bin/yq
}

if ! command -v yq &>/dev/null; then
    install_yq
else
    echo "yq already installed: $(yq --version)"
fi

echo "Done."
