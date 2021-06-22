#!/bin/bash

namespace="ais"
if [[ -n ${1} ]]; then
    namespace=${1}
fi

nodes=$(kubectl get pods -n ${namespace} -o wide | tail -n +2 | awk '{print $7}' | sort | uniq)

# Delete AIS cluster
kubectl delete aistores.ais.nvidia.com -n "${namespace}" ais

pvs=$(kubectl get pvc -n "${namespace}" | tail -n +2 | awk '{print $3}')
pvs=${pvs//ais-local-storage/}

# Delete all PVCs
kubectl delete pvc -n "${namespace}" --all

# Delete all PVs
kubectl delete pv ${pvs}

# Unlabel all nodes
if [[ -n "${nodes}" ]]; then
    kubectl label nodes ${nodes} nvidia.com/ais-proxy- nvidia.com/ais-target-
fi
