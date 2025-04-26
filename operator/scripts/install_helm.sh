#!/usr/bin/env bash

set -e

if [ $# -ne 2 ]; then
    echo "Usage: $0 <DESIRED_PATH> <HELM_VERSION>"
    echo "Example: $0 /home/user/.local/bin v3.14.4"
    exit 1
fi

DESIRED_PATH="$1"
HELM_VERSION="$2"

# Check if helm is already installed in the desired path
if [ -f "$DESIRED_PATH/helm" ]; then
  echo "Helm already installed at $DESIRED_PATH/helm"
  exit 0
fi

TMP_DIR="$(mktemp -d)"
mkdir -p "$DESIRED_PATH"
cd "$TMP_DIR"
echo "Downloading 'get-helm' script"
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
HELM_INSTALL_DIR="$DESIRED_PATH" ./get_helm.sh --version "$HELM_VERSION" --no-sudo
rm -rf "$TMP_DIR"