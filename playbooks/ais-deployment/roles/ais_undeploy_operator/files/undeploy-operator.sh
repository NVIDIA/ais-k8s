#!/bin/bash

#
# Undeploy AIS operator
#

release_version=${RELEASE:-v2.8.0}

kubectl delete -f https://github.com/NVIDIA/ais-k8s/releases/download/${release_version}/ais-operator.yaml
