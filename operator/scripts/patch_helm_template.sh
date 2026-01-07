#!/usr/bin/env bash
set -e

HELM_TEMPLATE="${1:-dist/ais-operator/templates/ais-operator.yaml}"

if [[ ! -f "$HELM_TEMPLATE" ]]; then
    echo "Error: Helm template not found: $HELM_TEMPLATE"
    exit 1
fi

echo "Injecting Helm pod annotations template into $HELM_TEMPLATE"

# Replace the placeholder annotation with proper Helm template
awk '
/__HELM_POD_ANNOTATIONS__:/ {
    # Replace the placeholder line with proper Helm template
    print "            {{- with .Values.controllerManager.podAnnotations }}"
    print "            {{- toYaml . | nindent 12 }}"
    print "            {{- end }}"
    next
}
{ print }
' "$HELM_TEMPLATE" > "$HELM_TEMPLATE.tmp" && mv "$HELM_TEMPLATE.tmp" "$HELM_TEMPLATE"

echo "Successfully injected pod annotations template"