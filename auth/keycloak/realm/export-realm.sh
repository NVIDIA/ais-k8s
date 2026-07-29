#!/usr/bin/env bash
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# Partial-export the aistore realm from a running Keycloak, strip key material
# (partial-export masks secrets as ********** rather than omitting them), then
# fail if any privateKey/secret fields remain.
# Requires curl, jq, yq

set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-https://localhost:8443}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin}"
REALM="${REALM:-aistore}"
OUT="${1:-$SCRIPT_DIR/aistore-realm.json}"

TOKEN=$(curl -sk \
  -d "client_id=admin-cli" \
  -d "username=${ADMIN_USER}" \
  -d "password=${ADMIN_PASSWORD}" \
  -d "grant_type=password" \
  "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" | jq -r ".access_token")

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "ERROR: failed to obtain admin token from ${KEYCLOAK_URL}" >&2
  exit 1
fi

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

curl -sk -X POST \
  -H "Authorization: Bearer $TOKEN" \
  "${KEYCLOAK_URL}/admin/realms/${REALM}/partial-export?exportGroupsAndRoles=true&exportClients=true" \
  > "$TMP"

# Drop masked or real key material; Keycloak regenerates keys when these are absent.
# Also drop KeyProvider certificate/kid pairs left behind after privateKey/secret removal.
yq --input-format=json --output-format=json '
  del(.. | select(key == "privateKey" or key == "secret")) |
  (.components."org.keycloak.keys.KeyProvider"[]?.config |= del(.certificate, .kid))
' "$TMP" > "$OUT"

"$SCRIPT_DIR/check-realm-keys.sh" "$OUT"
echo "Wrote $OUT"
