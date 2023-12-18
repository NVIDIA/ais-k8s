#!/bin/bash

#
# This script is responsible for creating local-persistent volumes on each storage target.
# Run on any node that can run `kubectl` commands and has permissions to create volumes.
#


if [ -z "$MPATHS" ]; then
    echo "Error: Set ais_mpaths in vars/ais_mpaths.yml to define PV mountpaths"
    exit 1
fi
mpaths="$MPATHS"

if [ -z "$MPATH_SIZE" ]; then
    echo "Error: Set ais_mpath_size in vars/ais_mpaths.yml to define PV size"
    exit 1
fi
mpath_size="$MPATH_SIZE"

if [ -z "$NAMESPACE" ]; then
    echo "Error: Set 'cluster' variable to define PV namespace"
    exit 1
fi
namespace="$NAMESPACE"

source_dir=$(dirname "${BASH_SOURCE[0]}")

nodes="${NODES}"

if [[ -z ${nodes} ]]; then
    echo "Error: No nodes provided to create PVs. Ensure the ansible group defined by the 'cluster' variable contains nodes for PV creation."
    exit 1
fi

target_num=0
for n in ${nodes} ; do
    for m in ${mpaths}; do
        name="$n-pv${m//\//\-}"
        export NAME=$name
        export MPATH=$m
        export NODE=$n
        export MPATH_LABEL=pv${m//\//\-}
        export MPATH_SIZE=$mpath_size
        export NAMESPACE=$namespace
        export CLAIM_NAME=ais${m//\//\-}-ais-target-$target_num
        envsubst < "${source_dir}"/pv.template.yaml > /tmp/pv.yaml
        kubectl apply -f /tmp/pv.yaml
        rm /tmp/pv.yaml
    done
    target_num=$((target_num+1))
done