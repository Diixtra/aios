# auth-broker spike

Validates that pi's ChatGPT subscription auth can be exercised non-interactively from
a fresh Linux container with no TTY. Output of this spike is the findings document at
docs/superpowers/spikes/2026-04-30-pi-auth-transport-findings.md.

## Experiments
- A2 — discover PI_CODING_AGENT_DIR layout
- A3 — interactive login from headless container
- A4 — auth bundle portability (login on dev box, transplant to container)
- A5 — concurrent pi processes sharing one auth bundle
- A6 — refresh-token rotation behaviour
- A7 — pi outbound HTTPS transport capture
