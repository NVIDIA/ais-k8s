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

# Helper to apply gomplate to a template with a snippet
apply_snippet() {
  local snippet="$1"
  local tmpl="$2"
  local target="$3"

  if [ -f "$snippet" ] && [ -f "$tmpl" ] && [ -f "$target" ]; then
    AIS_TARGET_TEMPLATE="$target" \
    AIS_SNIPPET="$snippet" \
    ${GOMPLATE} -f "$tmpl" -o "${target}.tmp"
    mv "${target}.tmp" "$target"
  fi
}

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
apply_snippet "$POD_ANNOTATIONS_SNIPPET" "$POD_ANNOTATIONS_TMPL" "$DEPLOY_TEMPLATE"

# Replace manager arg template with additional optional ones via gomplate
MANAGER_ARGS_SNIPPET="${SNIPPETS_SOURCE_DIR}/manager_args.tpl"
MANAGER_ARGS_TMPL="${TEMPLATES_SOURCE_DIR}/replace_manager_args.gotmpl"
apply_snippet "$MANAGER_ARGS_SNIPPET" "$MANAGER_ARGS_TMPL" "$DEPLOY_TEMPLATE"

# Wrap the ClusterRoleBinding in manager-rbac.yaml with a conditional via gomplate
CLUSTER_ROLE_SNIPPET="${SNIPPETS_SOURCE_DIR}/cluster_rolebinding_option.tpl"
CLUSTER_ROLE_TMPL="${TEMPLATES_SOURCE_DIR}/add_cluster_rolebinding_option.gotmpl"
RBAC_TEMPLATE="${TEMPLATES_TARGET_DIR}/manager-rbac.yaml"
apply_snippet "$CLUSTER_ROLE_SNIPPET" "$CLUSTER_ROLE_TMPL" "$RBAC_TEMPLATE"

echo "Replacements applied successfully"