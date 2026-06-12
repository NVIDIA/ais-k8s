#!/bin/bash
# Run Node Doctor health checks (read-only) on the host node.
# Node Doctor ships on OCI worker-node images at /usr/local/bin/node-doctor.sh and
# normally requires SSH (opc + sudo); here we run it via chroot into the mounted host root.
# Docs: https://docs.oracle.com/en-us/iaas/private-cloud-appliance/pca/oke/using-node-doctor-to-troubleshoot-worker-node-issues.htm
set -uo pipefail

echo "Node: $(hostname)"

if ! chroot /host /bin/bash -lc 'test -x /usr/local/bin/node-doctor.sh'; then
  echo "node-doctor.sh not found at /usr/local/bin/node-doctor.sh on the host" >&2
  echo "(this preset expects a worker-node image that ships Node Doctor)" >&2
  echo "node-doctor --check skipped: tool unavailable (keeping container healthy)" >&2
  exit 0
fi

# --check is read-only. It exits non-zero when it raises signals, which is expected,
# so we don't 'set -e' around it and we always exit 0 to keep the pod/initContainer healthy
# and the report readable via 'kubectl logs'.
chroot /host /bin/bash -lc '/usr/local/bin/node-doctor.sh --check'
rc=$?
echo "node-doctor --check exit code: ${rc}"
exit 0
