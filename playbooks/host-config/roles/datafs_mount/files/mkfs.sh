#!/usr/bin/env bash
set -euo pipefail

# Expecting:
#   DEVICES: space-separated list (e.g. "sdb1 sdc1 sdd1")
#   FSTYPE: filesystem type (e.g. ext4, xfs)

DEVICES="${DEVICES:-}"
FSTYPE="${FSTYPE:-ext4}"

if [[ -z "$DEVICES" ]]; then
  echo "ERROR: DEVICES is not set" >&2
  exit 1
fi

force_flag=""
case "$FSTYPE" in
  ext2|ext3|ext4)
    force_flag="-F"   # mke2fs force
    ;;
  xfs)
    force_flag="-f"   # mkfs.xfs force
    ;;
  *)
    echo "WARNING: no generic force flag known for FSTYPE=$FSTYPE; proceeding without force" >&2
    force_flag=""
    ;;
esac

for d in $DEVICES; do
  dev="$d"
  if [[ "$dev" != /dev/* ]]; then
    dev="/dev/$dev"
  fi

  {
    echo "Creating filesystem $FSTYPE on $dev (force)..."
    mkfs -t "$FSTYPE" $force_flag "$dev"
    echo "Done creating filesystem on $dev"
  } &
done

echo "All mkfs commands started in parallel, waiting for completion..."
wait
echo "All mkfs operations completed."