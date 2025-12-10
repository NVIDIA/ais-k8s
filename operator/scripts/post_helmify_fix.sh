#!/usr/bin/env bash
# Post-helmify script to add CA ConfigMap parameters, pod annotations, and replace placeholders
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

# Detect OS and set sed options accordingly
if [[ "$OSTYPE" == "darwin"* ]]; then
    SED_INPLACE=(-i '')
else
    SED_INPLACE=(-i)
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

echo "Post-helmify: Adding pod annotations parameter to $VALUES_FILE"

# Add pod annotations parameter to values.yaml under controllerManager section
# We insert it after the replicas field
awk '
/^  replicas:/ {
    print
    print "  # Annotations for the operator pods (optional)"
    print "  podAnnotations: {}"
    next
}
{ print }
' "$VALUES_FILE" > "$VALUES_FILE.tmp" && mv "$VALUES_FILE.tmp" "$VALUES_FILE"

echo "Post-helmify: Replacing CA ConfigMap placeholders in $TEMPLATE_FILE"

# Replace AUTH_CA_CONFIGMAP_PLACEHOLDER with Helm template variable
sed "${SED_INPLACE[@]}" 's/AUTH_CA_CONFIGMAP_PLACEHOLDER/{{ .Values.controllerManager.manager.authCAConfigmapName }}/g' "$TEMPLATE_FILE"

# Replace AIS_CA_CONFIGMAP_PLACEHOLDER with Helm template variable
sed "${SED_INPLACE[@]}" 's/AIS_CA_CONFIGMAP_PLACEHOLDER/{{ .Values.controllerManager.manager.aisCAConfigmapName }}/g' "$TEMPLATE_FILE"

echo "Post-helmify: Adding pod annotations support to Deployment in $TEMPLATE_FILE"

# Add pod-level annotations after the labels block in Pod template metadata
# We need to find the specific pattern: template > metadata > labels > selectorLabels
# and add annotations after that block, before 'spec:'
awk '
/^  template:$/ { in_template = 1 }
in_template && /^    metadata:$/ { in_template_metadata = 1 }
in_template_metadata && /{{- include "ais-operator.selectorLabels".*nindent 8/ {
    print
    print "      {{- with .Values.controllerManager.podAnnotations }}"
    print "      annotations:"
    print "        {{- toYaml . | nindent 8 }}"
    print "      {{- end }}"
    in_template_metadata = 0
    in_template = 0
    next
}
{ print }
' "$TEMPLATE_FILE" > "$TEMPLATE_FILE.tmp" && mv "$TEMPLATE_FILE.tmp" "$TEMPLATE_FILE"

echo "Post-helmify: Configuration completed successfully"

