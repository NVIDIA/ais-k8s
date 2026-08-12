#!/usr/bin/env bash
#
# Remove AIStore metadata from the mountpath of each given device.
# With -d, also remove the bucket directories.
# With -s, also remove AIStore metadata under the given state directory.
#
set -euo pipefail

usage() {
    echo "usage: $(basename "$0") -p <mpath-prefix> [-d] [-s <state-prefix>] <device>..." >&2
    exit 1
}

prefix=""
state_prefix=""
remove_data=false

while getopts ":p:s:d" opt; do
    case "$opt" in
        p) prefix="$OPTARG" ;;
        s) state_prefix="$OPTARG" ;;
        d) remove_data=true ;;
        *) usage ;;
    esac
done
shift $((OPTIND - 1))

if [ -z "$prefix" ] || [ "$#" -eq 0 ]; then
    usage
fi

for device in "$@"; do
    mpath="${prefix}/${device}"
    rm -rf "${mpath}"/.ais.*
    if [ "$remove_data" = true ]; then
        rm -rf "${mpath}"/@*
    fi
done

if [ -n "$state_prefix" ] && [ -d "$state_prefix" ]; then
    find "$state_prefix" -depth -name '.ais.*' -exec rm -rf {} +
fi
