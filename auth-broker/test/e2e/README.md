# auth-broker e2e smoke

Run locally with a test OpenAI subscription and test Slack workspace. Acceptance:

1. Bootstrap-recipe DM arrives in test Slack channel within 3s of `POST /v1/admin/revalidate` against an empty bundle.
2. After laptop-side `pi /login` + `bootstrap-auth.sh` upload, state transitions to Healthy and a Recovered DM arrives.
3. Restarting the broker container preserves the bundle on the PVC; state remains Healthy without re-bootstrap.
4. Tampering with `auth.json` to corrupt JSON and re-uploading returns HTTP 400 and leaves the previous bundle untouched.

Not a CI gate — human-in-the-loop.

## How to run

```bash
export TEST_SLACK_TOKEN=xoxb-test-...
export TEST_SLACK_USER=U01ABCXYZ
export TEST_ADMIN_TOKEN=$(openssl rand -hex 32)
chmod +x bootstrap-auth.sh smoke.sh
./smoke.sh
```

The script will pause at step 3 to ask you to run `bootstrap-auth.sh` on the laptop (after completing `pi /login` interactively).
