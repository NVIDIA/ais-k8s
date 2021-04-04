#!/bin/bash

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

cd "${CURRENT_DIR}/.." || exit
if [[ -n "$1" ]]; then
    args="--dataplane=$1"
fi

./deploy.sh k8s --cluster-name="ais-ci-$(cat /dev/urandom | tr -dc 'a-z0-9' | fold -w 5 | head -n 1)" --cloud=gcp --node-cnt=3 --disk-cnt=2 ${args}

kubectl get nodes
