#!/bin/bash

#
# Deploy AIS operator
#

release_version=${RELEASE:-v0.7}

kubectl delete -f https://github.com/NVIDIA/ais-k8s/releases/download/${release_version}/ais-operator.yaml
