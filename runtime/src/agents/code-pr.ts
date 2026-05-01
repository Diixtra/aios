/**
 * code-pr agent entrypoint (skeleton).
 *
 * This is the per-AgentTask container entrypoint that the operator's Job
 * launches when an AgentConfig with `engine: pi` is reconciled. The full
 * preflight -> runPi -> postflight pipeline is wired in Task C10 — for now
 * this file documents the shape of the entrypoint and exposes `preflight`
 * for direct unit testing.
 */
export { preflight } from "./preflight.js";
export type { GhClient, PreflightResult } from "./preflight.js";
