#!/usr/bin/env bash
set -e

LPP_VERSION="${LPP_VERSION:-v0.0.32}"

kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LPP_VERSION}/deploy/local-path-storage.yaml"

