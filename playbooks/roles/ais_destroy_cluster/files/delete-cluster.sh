#!/bin/bash

namespace="ais"
if [[ -n ${1} ]]; then
    namespace=${1}
fi

nodes=$(kubectl get pods -n ${namespace} -o wide | tail -n +2 | awk '{print $7}' | sort | uniq)

# Delete AIS cluster
kubectl delete aistores.ais.nvidia.com -n ${namespace} ais

# Wait for AIS cluster to be deleted
while kubectl get aistores.ais.nvidia.com -n ${namespace} ais &> /dev/null; do 
    echo "Waiting for AIS cluster to be deleted..."
    sleep 5
done

# Get list of PVCs
pvs=$(kubectl get pvc -n "${namespace}" | tail -n +2 | awk '{print $3}')
pvs=${pvs//ais-local-storage/}

# Delete all PVCs
kubectl delete pvc -n "${namespace}" --all

# Wait for PVCs to be deleted
while kubectl get pvc -n "${namespace}" | grep -q "Terminating"; do 
    echo "Waiting for PVCs to be deleted..."
    sleep 5
done

# Delete all PVs
kubectl delete pv ${pvs}

# Wait for PVs to be deleted
for pv in $pvs; do
    while kubectl get pv "$pv" &> /dev/null; do
        echo "Waiting for PV $pv to be deleted..."
        sleep 5
    done
done

# Unlabel all nodes
if [[ -n "${nodes}" ]]; then
    kubectl label nodes ${nodes} nvidia.com/ais-proxy- nvidia.com/ais-target-
fi

echo "AIS cluster and associated resources have been successfully deleted."
