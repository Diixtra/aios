/**
 * code-pr agent skeleton tests — confirms the public re-exports are in place
 * so downstream tasks (C10/C11) can wire runPi + postflight against the
 * stable surface.
 */
import { describe, expect, it } from "vitest";
import * as codePr from "./code-pr.js";

describe("code-pr (skeleton)", () => {
  it("re-exports preflight for the agent entrypoint pipeline", () => {
    expect(typeof codePr.preflight).toBe("function");
  });
});
