#!/usr/bin/env bash
# Uploads ~/.pi/agent/auth.json to the auth-broker.
# Run after `pi /login` completes successfully on the laptop.
set -euo pipefail

BROKER_URL="${AUTH_BROKER_URL:?missing AUTH_BROKER_URL}"
ADMIN_TOKEN="${AUTH_BROKER_ADMIN_TOKEN:?missing AUTH_BROKER_ADMIN_TOKEN}"
BUNDLE="${PI_AUTH_BUNDLE:-$HOME/.pi/agent/auth.json}"

if [[ ! -f "$BUNDLE" ]]; then
  echo "Bundle not found: $BUNDLE" >&2
  echo "Run 'pi' interactively, then '/login' to complete OAuth, then re-run this script." >&2
  exit 1
fi

curl -sf -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "bundle=@${BUNDLE}" \
  "${BROKER_URL%/}/v1/auth/bundle"
echo "Uploaded $BUNDLE to $BROKER_URL"
