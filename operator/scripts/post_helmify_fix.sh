#!/usr/bin/env bash
# Post-helmify script to add CA ConfigMap parameters and replace placeholders
# This script adds Helm chart parameters and converts kustomize placeholders to Helm template syntax
# Run automatically by the build-installer-helm Makefile target

set -e

CHART_DIR="${1:-dist/ais-operator}"
TEMPLATE_FILE="$CHART_DIR/templates/ais-operator.yaml"
VALUES_FILE="$CHART_DIR/values.yaml"

if [[ ! -f "$TEMPLATE_FILE" ]]; then
    echo "Error: Template file not found: $TEMPLATE_FILE"
    exit 1
fi

if [[ ! -f "$VALUES_FILE" ]]; then
    echo "Error: Values file not found: $VALUES_FILE"
    exit 1
fi

echo "Post-helmify: Adding CA ConfigMap parameters to $VALUES_FILE"

# Add CA ConfigMap name parameters to values.yaml after the env section
# We insert them right after the 'env:' block in controllerManager.manager
# Using a more reliable awk approach to maintain proper YAML indentation
awk '
/^    env:/ {
    print
    in_env = 1
    next
}
in_env && /^    [a-zA-Z]/ && !/^      / {
    # Found the next key at the same level, insert our parameters before it
    print "    # ConfigMap names for CA certificates (optional)"
    print "    # Default names are used; ConfigMaps are optional and operator falls back to system CAs if not present"
    print "    # Override these values to use different ConfigMap names (e.g., from trust-manager)"
    print "    authCAConfigmapName: ais-operator-auth-ca  # For operator->auth service TLS verification"
    print "    aisCAConfigmapName: ais-operator-ais-ca   # For operator->AIStore cluster TLS verification"
    in_env = 0
}
{ print }
' "$VALUES_FILE" > "$VALUES_FILE.tmp" && mv "$VALUES_FILE.tmp" "$VALUES_FILE"

echo "Post-helmify: Replacing CA ConfigMap placeholders in $TEMPLATE_FILE"

# Replace AUTH_CA_CONFIGMAP_PLACEHOLDER with Helm template variable
sed -i 's/AUTH_CA_CONFIGMAP_PLACEHOLDER/{{ .Values.controllerManager.manager.authCAConfigmapName }}/g' "$TEMPLATE_FILE"

# Replace AIS_CA_CONFIGMAP_PLACEHOLDER with Helm template variable
sed -i 's/AIS_CA_CONFIGMAP_PLACEHOLDER/{{ .Values.controllerManager.manager.aisCAConfigmapName }}/g' "$TEMPLATE_FILE"

echo "Post-helmify: CA ConfigMap configuration completed successfully"

