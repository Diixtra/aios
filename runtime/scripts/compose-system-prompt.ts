import { promises as fs } from "node:fs";
import path from "node:path";

export interface ComposeOpts {
  /** Absolute path to the base markdown file (e.g. fabric-patterns/_bases/code-pr.md). */
  basePath: string;
  /** Ordered list of fabric pattern names (subdirectory names of patternsDir). */
  patterns: string[];
  /** Directory containing fabric patterns; each pattern lives in <patternsDir>/<name>/system.md. */
  patternsDir: string;
}

/**
 * Compose a SYSTEM.md prompt by concatenating a base markdown file with the
 * `system.md` of each named fabric pattern, in the order provided.
 *
 * Output shape:
 *   <trimmed base>
 *
 *   --- pattern: <name1> ---
 *   <trimmed pattern1 system.md>
 *
 *   --- pattern: <name2> ---
 *   <trimmed pattern2 system.md>
 *
 * Each section is separated by a blank line. The base is included even when
 * `patterns` is empty (in which case the result is just the trimmed base).
 *
 * Throws if the base file or any referenced pattern's `system.md` is missing.
 */
export async function composeSystemPrompt(opts: ComposeOpts): Promise<string> {
  const base = await fs.readFile(opts.basePath, "utf8");
  const parts = [base.trim()];
  for (const name of opts.patterns) {
    const pPath = path.join(opts.patternsDir, name, "system.md");
    const text = await fs.readFile(pPath, "utf8");
    parts.push(`--- pattern: ${name} ---\n${text.trim()}`);
  }
  return parts.join("\n\n");
}
