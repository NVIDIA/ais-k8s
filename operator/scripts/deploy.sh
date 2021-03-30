#!/bin/bash

function pre_deploy {
	read -r -p "would you like to deploy cert-manager? [y/n]" response
    if [[ "${response}" == "y" ]]; then  
        kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.2.0/cert-manager.yaml;
    fi
}

pre_deploy