import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { discoverFabricSkills } from "./index";

describe("discoverFabricSkills", () => {
  let dir: string;

  beforeEach(async () => {
    dir = await fs.mkdtemp(path.join(os.tmpdir(), "fabric-test-"));
  });

  afterEach(async () => {
    await fs.rm(dir, { recursive: true, force: true });
  });

  it("returns SKILL.md paths for each named pattern (preferred form)", async () => {
    await fs.mkdir(path.join(dir, "extract_requirements"));
    await fs.writeFile(
      path.join(dir, "extract_requirements", "SKILL.md"),
      "---\nname: extract_requirements\n---\nbody",
    );
    const paths = await discoverFabricSkills(dir, ["extract_requirements"]);
    expect(paths).toEqual([
      path.join(dir, "extract_requirements", "SKILL.md"),
    ]);
  });

  it("falls back to system.md when SKILL.md is missing (legacy form)", async () => {
    await fs.mkdir(path.join(dir, "find_hidden_bugs"));
    await fs.writeFile(
      path.join(dir, "find_hidden_bugs", "system.md"),
      "legacy fabric body",
    );
    const paths = await discoverFabricSkills(dir, ["find_hidden_bugs"]);
    expect(paths).toEqual([path.join(dir, "find_hidden_bugs", "system.md")]);
  });

  it("prefers SKILL.md over system.md when both exist", async () => {
    await fs.mkdir(path.join(dir, "write_pull_request"));
    await fs.writeFile(
      path.join(dir, "write_pull_request", "SKILL.md"),
      "preferred",
    );
    await fs.writeFile(
      path.join(dir, "write_pull_request", "system.md"),
      "legacy",
    );
    const paths = await discoverFabricSkills(dir, ["write_pull_request"]);
    expect(paths).toEqual([
      path.join(dir, "write_pull_request", "SKILL.md"),
    ]);
  });

  it("resolves multiple patterns in input order", async () => {
    await fs.mkdir(path.join(dir, "alpha"));
    await fs.writeFile(path.join(dir, "alpha", "SKILL.md"), "a");
    await fs.mkdir(path.join(dir, "beta"));
    await fs.writeFile(path.join(dir, "beta", "system.md"), "b");
    const paths = await discoverFabricSkills(dir, ["alpha", "beta"]);
    expect(paths).toEqual([
      path.join(dir, "alpha", "SKILL.md"),
      path.join(dir, "beta", "system.md"),
    ]);
  });

  it("throws when neither SKILL.md nor system.md exists for a pattern", async () => {
    await fs.mkdir(path.join(dir, "ghost"));
    await expect(discoverFabricSkills(dir, ["ghost"])).rejects.toThrow(
      /ghost/,
    );
  });

  it("throws when the pattern directory itself is missing", async () => {
    await expect(discoverFabricSkills(dir, ["nope"])).rejects.toThrow(/nope/);
  });

  it("returns an empty list when no patterns are requested", async () => {
    const paths = await discoverFabricSkills(dir, []);
    expect(paths).toEqual([]);
  });
});
