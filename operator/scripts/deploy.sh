#!/bin/bash

function pre_deploy {
	read -r -p "would you like to deploy cert-manager? [y/n]" response
    if [[ "${response}" == "y" ]]; then
        kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.2.0/cert-manager.yaml;

        # Wait for cert-manager to be ready.
        kubectl wait --for=condition=ready pods --all -n cert-manager --timeout=5m;
    fi
}

pre_deploy