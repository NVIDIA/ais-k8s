#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_BIN="${SCRIPT_DIR}/../bin"
GOMPLATE="${LOCAL_BIN}/gomplate"
YQ="${LOCAL_BIN}/yq"

VALUES_OVERRIDE="${SCRIPT_DIR}/replacements/values.yaml"
VALUES_TARGET="${SCRIPT_DIR}/ais-operator/values.yaml"
TEMPLATES_TARGET_DIR="${SCRIPT_DIR}/ais-operator/templates"
TEMPLATES_SOURCE_DIR="${SCRIPT_DIR}/replacements/templates"
SNIPPETS_SOURCE_DIR="${SCRIPT_DIR}/replacements/snippets"

# Merge values.yaml replacements into the target values.yaml
if [ -f "$VALUES_OVERRIDE" ]; then
    echo "Merging values from $VALUES_OVERRIDE into $VALUES_TARGET"
    ${YQ} eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' "$VALUES_TARGET" "$VALUES_OVERRIDE" > "${VALUES_TARGET}.tmp"
    mv "${VALUES_TARGET}.tmp" "$VALUES_TARGET"
fi

# Replace placeholder with Helm pod annotations via gomplate
POD_ANNOTATIONS_SNIPPET="${SNIPPETS_SOURCE_DIR}/pod_annotations.tpl"
POD_ANNOTATIONS_TMPL="${TEMPLATES_SOURCE_DIR}/replace_pod_annotations.gotmpl"
DEPLOY_TEMPLATE="${TEMPLATES_TARGET_DIR}/deployment.yaml"

if [ -f "$POD_ANNOTATIONS_SNIPPET" ] && [ -f "$POD_ANNOTATIONS_TMPL" ] && [ -f "$DEPLOY_TEMPLATE" ]; then
  AIS_TARGET_TEMPLATE="$DEPLOY_TEMPLATE" \
  AIS_SNIPPET="$POD_ANNOTATIONS_SNIPPET" \
  ${GOMPLATE} -f "$POD_ANNOTATIONS_TMPL" -o "${DEPLOY_TEMPLATE}.tmp"
  mv "${DEPLOY_TEMPLATE}.tmp" "$DEPLOY_TEMPLATE"
fi

# Wrap manager-rbac.yaml with cluster role conditional via gomplate
CLUSTER_ROLE_SNIPPET="${SNIPPETS_SOURCE_DIR}/cluster_role_option.tpl"
CLUSTER_ROLE_TMPL="${TEMPLATES_SOURCE_DIR}/add_cluster_role_option.gotmpl"
RBAC_TEMPLATE="${TEMPLATES_TARGET_DIR}/manager-rbac.yaml"

if [ -f "$CLUSTER_ROLE_SNIPPET" ] && [ -f "$CLUSTER_ROLE_TMPL" ] && [ -f "$RBAC_TEMPLATE" ]; then
  AIS_TARGET_TEMPLATE="$RBAC_TEMPLATE" \
  AIS_SNIPPET="$CLUSTER_ROLE_SNIPPET" \
  ${GOMPLATE} -f "$CLUSTER_ROLE_TMPL" -o "${RBAC_TEMPLATE}.tmp"
  mv "${RBAC_TEMPLATE}.tmp" "$RBAC_TEMPLATE"
fi

echo "Replacements applied successfully"