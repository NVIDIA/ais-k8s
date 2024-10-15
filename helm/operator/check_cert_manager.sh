#!/bin/bash

if kubectl get crd issuers.cert-manager.io &> /dev/null; then
  echo "CRD issuers.cert-manager.io exists, proceeding..."
  exit 0
fi
echo "CRD issuers.cert-manager.io does not exist."
echo "Run `helmfile sync --state-values-set certManager.enabled=true` to install cert-manager before the operator."
exit 1