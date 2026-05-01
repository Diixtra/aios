/**
 * Pi fabric-skill loader — resolves fabric pattern directories into skill paths
 * suitable for `createAgentSession({ skills: [...] })`.
 *
 * Spec rule (revised fabric-as-pi-skills contract):
 *   - Preferred form: <rootDir>/<name>/SKILL.md
 *   - Legacy fallback: <rootDir>/<name>/system.md
 * If neither file is present, this function throws — fabric patterns must be
 * fully on disk before the session starts (no silent skips).
 *
 * This is a *loader helper*, not a `register*(session, ...)` extension. The
 * SDK accepts skill paths directly via `createAgentSession({ skills })`, so
 * the agent entrypoint calls `discoverFabricSkills` BEFORE creating the
 * session and passes the resolved paths into the config.
 */

import { promises as fs } from "node:fs";
import path from "node:path";

/**
 * Resolve a list of fabric pattern names to absolute skill file paths.
 *
 * @param rootDir - Directory containing per-pattern subfolders (e.g. /fabric-patterns).
 * @param patternNames - Pattern subfolder names (e.g. ["extract_requirements"]).
 * @returns Absolute paths to each pattern's resolved skill file, in input order.
 * @throws Error when a pattern has neither SKILL.md nor system.md (or no folder at all).
 */
export async function discoverFabricSkills(
  rootDir: string,
  patternNames: string[],
): Promise<string[]> {
  const resolved: string[] = [];
  for (const name of patternNames) {
    const skillPath = path.join(rootDir, name, "SKILL.md");
    const systemPath = path.join(rootDir, name, "system.md");

    if (await fileExists(skillPath)) {
      resolved.push(skillPath);
      continue;
    }
    if (await fileExists(systemPath)) {
      resolved.push(systemPath);
      continue;
    }

    throw new Error(
      `fabric-skill: no SKILL.md or system.md found for pattern "${name}" under ${rootDir}`,
    );
  }
  return resolved;
}

async function fileExists(p: string): Promise<boolean> {
  try {
    const stat = await fs.stat(p);
    return stat.isFile();
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return false;
    throw err;
  }
}
