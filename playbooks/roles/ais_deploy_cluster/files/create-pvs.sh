#!/bin/bash

#
# This script is responsible for creating local-persistent volumes on each storage target.
# Run on any node that can run `kubectl` commands and has permissions to create volumes.
#

mpaths=${MPATHS:-"/ais/sda /ais/sdb /ais/sdc /ais/sdd /ais/sde /ais/sdf /ais/sdg /ais/sdh /ais/sdi /ais/sdj"}

source_dir=$(dirname "${BASH_SOURCE[0]}")

nodes="${NODES}"

if [[ -z ${nodes} ]]; then
    nodes=$(kubectl get nodes | tail -n +2 | awk '{print $1}')
fi

for n in ${nodes} ; do
    for m in ${mpaths}; do
        name="$n-pv${m//\//\-}"
        export NAME=$name
        export MPATH=$m
        export NODE=$n
        export MPATH_LABEL=pv${m//\//\-}
        envsubst < "${source_dir}"/pv.template.yaml > /tmp/pv.yaml
        kubectl apply -f /tmp/pv.yaml
        rm /tmp/pv.yaml
    done
done
