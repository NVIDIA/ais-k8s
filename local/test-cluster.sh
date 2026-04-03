#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="${SCRIPT_DIR}/../helm"
OPERATOR_ROOT="${SCRIPT_DIR}/../operator"
CLUSTER_NAME="local-test"
DEFAULT_OPERATOR_IMG="ais-operator:local"

source "${SCRIPT_DIR}/cluster-setup.sh"
# Skip helmfile install confirmations
export SKIP_CONFIRM="true"

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Deploy a local AIS test environment on a kind cluster.

Options:
  -b, --build          Build the operator from source
  --reset              Re-run cluster setup (prereqs, certs, namespaces)
                       without recreating the cluster
  --image <image>      Use an operator image from a remote registry
                       (e.g. ghcr.io/org/ais-operator:v1.0)
  --auth               Deploy AuthN service and configure AIS with authentication
  -h, --help           Show this help message

--build and --image are mutually exclusive.

Examples:
  $(basename "$0")                              # Deploy using remote helmfile charts
  $(basename "$0") --build                      # Build operator from source and deploy
  $(basename "$0") --image registry/op:v1.0     # Deploy with a remote operator image
  $(basename "$0") --reset --build              # Re-run setup, then build and deploy
  $(basename "$0") --auth                       # Deploy with AuthN enabled
  $(basename "$0") --build --auth               # Build operator and deploy with auth
EOF
    exit "${1:-0}"
}

parse_image_ref() {
    local image="$1"
    local basename="${image##*/}"
    if [[ "$basename" == *:* ]]; then
        IMAGE_REPO="${image%:*}"
        IMAGE_TAG="${image##*:}"
    else
        IMAGE_REPO="$image"
        IMAGE_TAG="latest"
    fi
}

detect_container_tool() {
    if command -v podman >/dev/null 2>&1; then
        echo "podman"
    else
        echo "docker"
    fi
}

build_operator() {
    local img="$1"
    local container_tool
    container_tool="$(detect_container_tool)"

    echo "Building operator from source: ${img}"
    IMG="$img" make -C "${OPERATOR_ROOT}" "${container_tool}-build"
    VERSION=local IMG="$img" make -C "${OPERATOR_ROOT}" build-installer-helm
}

load_image_to_kind() {
    local image="$1"
    local container_tool
    container_tool="$(detect_container_tool)"

    echo "Loading image '${image}' into kind cluster '${CLUSTER_NAME}'..."

    local tmpdir
    tmpdir="$(mktemp -d -t ais-operator-XXXXXX)"
    trap "rm -rf \"${tmpdir}\"" EXIT

    local tmptar="${tmpdir}/image.tar"
    local save_img="$image"
    if [[ "$container_tool" == "podman" ]]; then
        save_img="docker.io/library/${image}"
        podman tag "$image" "$save_img"
        podman save --format docker-archive -o "$tmptar" "$save_img"
    else
        docker save -o "$tmptar" "$save_img"
    fi

    kind load image-archive "$tmptar" --name "$CLUSTER_NAME"
    rm -rf "$tmpdir"
    trap - EXIT
}

deploy_operator_local() {
    local repo="$1"
    local tag="$2"
    local pull_policy="${3:-IfNotPresent}"

    echo "Deploying operator with image ${repo}:${tag} (pullPolicy=${pull_policy})"

    (cd "${HELM_ROOT}/operator" && helmfile sync -e local -l name=operator-tls-cert)

    helm upgrade --install ais-operator "${OPERATOR_ROOT}/helm/ais-operator" \
        -n ais-operator-system --create-namespace \
        -f "${HELM_ROOT}/operator/config/operator/local.yaml.gotmpl" \
        --set controllerManager.manager.image.repository="$repo" \
        --set controllerManager.manager.image.tag="$tag" \
        --set controllerManager.manager.imagePullPolicy="$pull_policy"
}

deploy_operator_remote() {
    echo "Deploying operator from helmfile..."
    (cd "${HELM_ROOT}/operator" && helmfile sync -e local)
}

wait_for_operator() {
    echo "Waiting for AIS operator rollout to complete..."
    kubectl rollout status deployment/ais-operator-controller-manager \
        -n ais-operator-system --timeout=120s
    echo "AIS operator is ready!"
}

generate_password() {
    LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 24
}

deploy_authn() {
    local admin_password
    admin_password="$(generate_password)"
    echo "Deploying AuthN service..."
    (cd "${HELM_ROOT}/authn" && AUTHN_ADMIN_PASSWORD="$admin_password" helmfile sync -e local)
    echo "Waiting for AuthN deployment..."
    kubectl rollout status deployment/ais-authn -n ais --timeout=120s
    echo "AuthN service is ready!"
}

deploy_ais() {
    local ais_env="local"
    if $DEPLOY_AUTH; then
        ais_env="local-auth"
    fi
    (cd "${HELM_ROOT}/ais" && helmfile sync -e "$ais_env")

    echo "Waiting for AIStore cluster to be ready..."
    kubectl wait --for=jsonpath='{.status.state}'=Ready --timeout=300s aistore/ais -n ais

    cat <<'EOF'

===================================================================
AIStore cluster deployed successfully!
===================================================================

To connect to the admin client and run AIS commands:

  kubectl exec -it -n ais deploy/ais-client -- /bin/bash

===================================================================
EOF
}

# --- Parse arguments ---

BUILD=false
RESET=false
DEPLOY_AUTH=false
OPERATOR_IMG=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -b|--build)
            BUILD=true
            shift
            ;;
        --reset)
            RESET=true
            shift
            ;;
        --image)
            if [[ -z "${2:-}" ]]; then
                echo "Error: --image requires an argument" >&2
                exit 1
            fi
            OPERATOR_IMG="$2"
            shift 2
            ;;
        --auth)
            DEPLOY_AUTH=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Error: unknown option: $1" >&2
            usage 1
            ;;
    esac
done

if $BUILD && [[ -n "$OPERATOR_IMG" ]]; then
    echo "Error: --build and --image are mutually exclusive" >&2
    exit 1
fi

# --- Cluster creation and setup ---

if $RESET; then
    echo "Re-running cluster setup (skipping cluster creation)..."
    install_prereqs
elif kind get clusters 2>/dev/null | grep -qw "$CLUSTER_NAME"; then
    echo "Cluster '${CLUSTER_NAME}' already exists, skipping creation and setup."
else
    setup_cluster "$CLUSTER_NAME"
fi

# --- Build and deploy operator ---

if $BUILD; then
    build_operator "$DEFAULT_OPERATOR_IMG"
    load_image_to_kind "$DEFAULT_OPERATOR_IMG"
    parse_image_ref "$DEFAULT_OPERATOR_IMG"
    deploy_operator_local "$IMAGE_REPO" "$IMAGE_TAG"
elif [[ -n "$OPERATOR_IMG" ]]; then
    parse_image_ref "$OPERATOR_IMG"
    deploy_operator_local "$IMAGE_REPO" "$IMAGE_TAG" "Always"
else
    deploy_operator_remote
fi

wait_for_operator

# --- Deploy AuthN (if enabled) ---

if $DEPLOY_AUTH; then
    deploy_authn
fi

# --- Deploy AIStore ---

deploy_ais
