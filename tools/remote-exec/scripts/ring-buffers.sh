#!/bin/bash
set -euxo pipefail
# This is just an example config and will not persist through reboot
# Prefer using cloud-init scripts to configure permanent changes
chroot /host /bin/bash -lc "ethtool -G ens300np0 rx 8192 tx 8192"
