import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { promises as fs } from "node:fs";
import os from "node:os";
import path from "node:path";

import { composeSystemPrompt } from "./compose-system-prompt";

describe("composeSystemPrompt", () => {
  let tmpDir: string;
  let basePath: string;
  let patternsDir: string;

  beforeAll(async () => {
    tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), "compose-system-prompt-"));

    basePath = path.join(tmpDir, "base.md");
    await fs.writeFile(basePath, "BASE\n", "utf8");

    patternsDir = path.join(tmpDir, "patterns");
    await fs.mkdir(path.join(patternsDir, "extract_requirements"), { recursive: true });
    await fs.writeFile(
      path.join(patternsDir, "extract_requirements", "system.md"),
      "ER\n",
      "utf8",
    );
    await fs.mkdir(path.join(patternsDir, "write_pull_request"), { recursive: true });
    await fs.writeFile(
      path.join(patternsDir, "write_pull_request", "system.md"),
      "WPR\n",
      "utf8",
    );
  });

  afterAll(async () => {
    await fs.rm(tmpDir, { recursive: true, force: true });
  });

  it("concatenates base + named patterns with separators", async () => {
    const out = await composeSystemPrompt({
      basePath,
      patterns: ["extract_requirements", "write_pull_request"],
      patternsDir,
    });
    expect(out).toContain(
      "BASE\n\n--- pattern: extract_requirements ---\nER\n\n--- pattern: write_pull_request ---\nWPR",
    );
  });

  it("returns just the trimmed base when no patterns are requested", async () => {
    const out = await composeSystemPrompt({
      basePath,
      patterns: [],
      patternsDir,
    });
    expect(out).toBe("BASE");
  });

  it("preserves the order patterns are passed in", async () => {
    const out = await composeSystemPrompt({
      basePath,
      patterns: ["write_pull_request", "extract_requirements"],
      patternsDir,
    });
    const idxWPR = out.indexOf("--- pattern: write_pull_request ---");
    const idxER = out.indexOf("--- pattern: extract_requirements ---");
    expect(idxWPR).toBeGreaterThan(-1);
    expect(idxER).toBeGreaterThan(idxWPR);
  });

  it("throws a useful error when a pattern's system.md is missing", async () => {
    await expect(
      composeSystemPrompt({
        basePath,
        patterns: ["nonexistent_pattern"],
        patternsDir,
      }),
    ).rejects.toThrow();
  });
});
