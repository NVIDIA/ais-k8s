#!/bin/bash
set -euo pipefail
echo "Running oci-growfs on host $(hostname)..."
if ! chroot /host /usr/libexec/oci-growfs -y; then
  echo "oci-growfs failed" >&2
  exit 1
fi
