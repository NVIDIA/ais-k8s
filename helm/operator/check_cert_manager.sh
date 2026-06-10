#!/bin/bash

CERT_MANAGER_SELECTOR="app.kubernetes.io/instance=cert-manager"

cert_manager_missing_msg() {
    echo "The AIS K8s operator requires cert-manager."
    echo "Install according to the cert-manager docs https://cert-manager.io/docs/installation/ and re-run the AIS K8s operator helm chart installation."
}

OUTPUT=$(kubectl get deployments -A -l "$CERT_MANAGER_SELECTOR" \
    -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Available")].status}{"\n"}{end}' \
    2>&1)
RC=$?

if [ "$RC" -ne 0 ]; then
    if echo "$OUTPUT" | grep -qiE "forbidden|does not have .* permission|cannot (list|get|watch)"; then
        echo "WARNING: Unable to verify cert-manager (insufficient permissions):"
        echo "$OUTPUT" | sed 's/^/  /'
        echo "Skipping cert-manager precheck. Ensure cert-manager is installed before proceeding."
        exit 0
    fi
    echo "ERROR: Unable to verify cert-manager (kubectl error):"
    echo "$OUTPUT" | sed 's/^/  /'
    exit 1
fi

TOTAL=$(kubectl get deployments -A -l "$CERT_MANAGER_SELECTOR" \
    -o jsonpath='{.items[*].metadata.name}' | wc -w | tr -d ' ')
READY=$(echo "$OUTPUT" | grep -c '^True$')

if [ "$TOTAL" -gt 0 ] && [ "$READY" -eq "$TOTAL" ]; then
    echo "All cert-manager deployments are Available ($READY/$TOTAL)"
    echo "Continuing operator installation"
    exit 0
fi

echo "cert-manager is not ready ($READY/$TOTAL deployments Available)"
cert_manager_missing_msg
exit 1
