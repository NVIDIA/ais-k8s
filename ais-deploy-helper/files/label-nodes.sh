#!/bin/bash

CLUSTER=${CLUSTER:-ais}
NODES=${NODES:-"--all"}
NODES=${NODES%\"}
NODES=${NODES#\"}

# Label all nodes to be proxy/target
kubectl label nodes ${NODES} nvidia.com/ais-proxy="${CLUSTER}" nvidia.com/ais-target="${CLUSTER}" || true