#!/bin/bash

release_version=${RELEASE:-v0.5}

function pre_deploy {
	read -r -p "would you like to deploy cert-manager? [y/n]" response
    if [[ "${response}" == "y" ]]; then
        kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.9.1/cert-manager.yaml

        # Wait for cert-manager to be ready.
        kubectl wait --for=condition=ready pods --all -n cert-manager --timeout=5m;
    fi
}

should_build=0
for arg in "$@"
do
    case $arg in
        -b|--build)
        should_build=1
    esac
done

pre_deploy
if [[ $should_build == 0 ]]; then
    kubectl apply -f https://github.com/NVIDIA/ais-k8s/releases/download/${release_version}/ais-operator.yaml
else
    bin/kustomize build config/default | kubectl apply -f -
fi
