#!/usr/bin/env bash
set -euo pipefail

# Install Go tools with versioned binaries
# Usage: ./go-install-tool.sh <target-binary> <package-url> <version>

# Verify arguments
if [ $# -ne 3 ]; then
    echo "Error: Invalid arguments"
    echo "Usage: $0 <target-binary> <package-url> <version>"
    exit 1
fi

target_binary="$1"
package_url="$2"
version="$3"

# Extract components
install_dir=$(dirname "$target_binary")
binary_name=$(basename "$target_binary")
base_name=$(echo "$binary_name" | sed "s/-${version}$//")
src_binary="${install_dir}/${base_name}"

# Create installation directory if missing
mkdir -p "$install_dir"

# Install if target doesn't exist
if [ ! -f "$target_binary" ]; then
    echo "Installing ${package_url}@${version}"

    # Install to temporary name
    GOBIN="$install_dir" go install "${package_url}@${version}"

    # Rename if versioned name requested
    if [ "$src_binary" != "$target_binary" ]; then
        echo "Renaming ${base_name} => ${binary_name}"
        mv -f "$src_binary" "$target_binary"
    fi
fi

echo "Verified: ${target_binary}"