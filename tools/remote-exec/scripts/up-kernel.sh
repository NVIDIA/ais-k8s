#!/bin/bash
set -euxo pipefail

echo "Node: $(hostname)"
echo "Before:"
chroot /host /bin/bash -lc 'uname -r; yum repolist enabled || true; rpm -q kernel-uek || true'

echo "Updating kernel-uek from ol8_UEKR7 only..."
chroot /host /bin/bash -lc "yum --disablerepo='*' --enablerepo='ol8_UEKR7' -y update kernel-uek"

echo "After install:"
chroot /host /bin/bash -lc 'rpm -q kernel-uek'

echo "Done. Node still needs reboot to boot into new kernel."