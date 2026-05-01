#!/usr/bin/env bash
set -euo pipefail
podman compose up -d
trap 'podman compose down -v' EXIT

# 1) Confirm broker comes up healthy.
curl -sf http://localhost:18080/healthz

# 2) Confirm initial state is Uninitialised — broker should DM bootstrap recipe.
curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  -X POST http://localhost:18080/v1/admin/revalidate | jq .
echo ">>> Verify bootstrap-recipe DM arrived in test Slack channel <<<"

# 3) Bootstrap from laptop (interactive).
echo ">>> Run on laptop: pi  ->  /login  ->  exit"
echo ">>> Then: AUTH_BROKER_URL=http://localhost:18080 \\"
echo ">>>       AUTH_BROKER_ADMIN_TOKEN=${TEST_ADMIN_TOKEN} \\"
echo ">>>       ./bootstrap-auth.sh"
read -rp "Press Enter once bootstrap-auth.sh has run successfully... "

# 4) Confirm state transitioned to Healthy.
sleep 2
state=$(curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  -X POST http://localhost:18080/v1/admin/revalidate | jq -r .state)
[[ "$state" == "Healthy" ]] || { echo "expected Healthy, got $state"; exit 1; }
echo "State: $state ✓"

# 5) Confirm Recovered DM arrived.
echo ">>> Verify Recovered DM arrived in test Slack channel <<<"

# 6) Bundle persistence across restart.
podman compose restart auth-broker
sleep 3
state=$(curl -sf -H "Authorization: Bearer ${TEST_ADMIN_TOKEN}" \
  -X POST http://localhost:18080/v1/admin/revalidate | jq -r .state)
[[ "$state" == "Healthy" ]] || { echo "expected Healthy after restart, got $state"; exit 1; }
echo "State after restart: $state ✓"
