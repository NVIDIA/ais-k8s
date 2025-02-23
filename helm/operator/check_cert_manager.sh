#!/bin/bash

READY_COUNT=$(kubectl get pods -A --field-selector=status.phase=Running | grep "cert-manager" | grep -E "[0-9]/[0-9].*Running" | wc -l)

if [ "$READY_COUNT" -ge 3 ]; then
    echo "All cert-manager pods are ready"
    echo "Continuing operator installation"
    exit 0
fi

echo "Not all cert-manager pods are ready. Found $READY_COUNT ready pods"
echo "The AIS K8s operator requires cert-manager."
echo "Run
• \`kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml\`
OR
• Install the cert-manager helm chart https://artifacthub.io/packages/helm/cert-manager/cert-manager
Then re-run the AIS K8s operator helm chart installation."
exit 1