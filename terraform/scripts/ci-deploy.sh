#!/bin/bash

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

cd "${CURRENT_DIR}/.." || exit
./deploy.sh all --cluster-name="ais-ci-$(cat /dev/urandom | tr -dc 'a-z0-9' | fold -w 5 | head -n 1)" --cloud=gcp --node-cnt=3 --disk-cnt=2 --wait=10m --aisnode-image="${AISNODE_IMAGE}:nightly" --admin-image="${ADMIN_IMAGE}:nightly"

kubectl get pods
admin_container=$(kubectl get pods --namespace default -l "component=admin" -o jsonpath="{.items[0].metadata.name}")
kubectl exec $admin_container -- ais show cluster
