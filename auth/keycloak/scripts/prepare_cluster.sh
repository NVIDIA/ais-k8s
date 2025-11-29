#!/usr/bin/env bash
set -e

# This script creates an ais-admin user in an active keycloak
# Note this REQUIRES a port-forward to already be running or a locally accessible cluster

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  echo "Usage: $0 <HOST> <USER> <PASS> [CA_CRT_PATH]" >&2
  exit 1
}

# Require at least 3 args, 4th is optional
if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  usage
fi

KEYCLOAK_HOST="$1"
USER="$2"
PASS="$3"
CA_FILE="${4:-}"

# Set up venv and requirements
if [ -d "$SCRIPT_DIR/venv" ]; then
  echo "using pre-existing venv for keycloak ais-admin creation script"
  source "$SCRIPT_DIR/venv/bin/activate"
else
  echo "venv not found, creating and installing requirements for keycloak ais-admin creation script"
  python3 -m venv venv
  source "$SCRIPT_DIR/venv/bin/activate"
  pip install -r requirements.txt
fi

# Build python arguments, conditionally add --verify-ca
PY_ARGS=(
  "$SCRIPT_DIR/create_ais_admin.py"
  --host "$KEYCLOAK_HOST"
  --realm aistore
  --admin-user "$USER"
  --admin-pass "$PASS"
)

if [ -n "$CA_FILE" ]; then
  PY_ARGS+=( --verify-ca "$CA_FILE" )
fi

python "${PY_ARGS[@]}"