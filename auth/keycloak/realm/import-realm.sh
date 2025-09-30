#!/bin/bash
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# This script converts a Keycloak realm JSON export into a KeycloakRealmImport YAML resource for Kubernetes.
# It then applies that YAML, monitors the resource for success, and deletes the resource to clean up the job.
# Requires yq

set -e

REALM_JSON="${REALM_JSON:-${1:-$SCRIPT_DIR/ais-realm.json}}"
REALM_NAME="${REALM_NAME:-${2:-aistore-realm}}"
KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-${3:-keycloak}}"
KEYCLOAK_CR="${4:-keycloak-server}"

# Convert JSON to YAML and indent as needed
YAML_DATA=$(yq -p=json -o=yaml "$REALM_JSON" | sed 's/^/    /')

REALM_IMPORT_YAML=$(cat <<EOF
apiVersion: k8s.keycloak.org/v2alpha1
kind: KeycloakRealmImport
metadata:
  name: $REALM_NAME
  namespace: $KEYCLOAK_NAMESPACE
spec:
  keycloakCRName: $KEYCLOAK_CR
  realm:
$YAML_DATA
EOF
)

cat <<EOF | kubectl apply -f -
$REALM_IMPORT_YAML
EOF

while true; do
    # Run kubectl and capture output
    output=$(kubectl get KeycloakRealmImport -n "$KEYCLOAK_NAMESPACE" "$REALM_NAME" -o go-template='{{range .status.conditions}}CONDITION: {{.type}}{{"\n"}}  STATUS: {{.status}}{{"\n"}}  MESSAGE: {{.message}}{{"\n"}}{{end}}')

    # Check if CONDITION Done STATUS is True
    if echo "$output" | grep -A1 "CONDITION: Done" | grep -q "STATUS: True"; then
        echo "Import done, deleting import job resources."  
        kubectl delete KeycloakRealmImport -n "$KEYCLOAK_NAMESPACE" "$REALM_NAME"
        break
    fi

    # Break if CONDITION HasErrors STATUS is True
    if echo "$output" | grep -A1 "CONDITION: HasErrors" | grep -q "STATUS: True"; then
        echo "Import failed"
        echo $output
        break
    fi

    # Echo message from the started condition
    echo "$output" | grep -A2 'CONDITION: Started' | grep 'MESSAGE:' | awk -F'MESSAGE: ' '{print $2}'

    sleep 2
done