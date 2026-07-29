#!/usr/bin/env bash

# This script fails if a Keycloak realm export contains key material.
# Keycloak generates fresh realm keys whenever a KeyProvider config omits them, so an export
# never needs to carry private keys or shared secrets.
# Requires yq

set -e

REALM_JSON="${1:?usage: check-realm-keys.sh <realm-export.json>}"

KEY_MATERIAL=$(yq --input-format=json --output-format=yaml \
    '.. | select(key == "privateKey" or key == "secret") | path | join(".")' "$REALM_JSON")

if [[ -n "$KEY_MATERIAL" ]]; then
    echo "ERROR: $REALM_JSON contains key material at:" >&2
    echo "$KEY_MATERIAL" | sed 's/^/  /' >&2
    echo "Delete these fields (and any paired 'certificate'/'kid') and re-run; Keycloak will generate new keys." >&2
    exit 1
fi
