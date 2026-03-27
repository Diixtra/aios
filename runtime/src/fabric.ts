import { execFile } from "node:child_process";
import type { CommandResult } from "./types.js";

/**
 * Runs fabric-ai patterns via the fabric CLI.
 */
export class FabricRunner {
  private readonly fabricBin: string;

  constructor(fabricBin: string = "fabric") {
    this.fabricBin = fabricBin;
  }

  /**
   * Run a single fabric pattern with input piped to stdin.
   * @param pattern - The fabric pattern name (e.g., "analyze_code")
   * @param input - Text to pipe into fabric's stdin
   * @returns CommandResult with stdout/stderr and exit code
   */
  async run(pattern: string, input: string): Promise<CommandResult> {
    return new Promise((resolve, reject) => {
      const child = execFile(
        this.fabricBin,
        ["-p", pattern],
        { maxBuffer: 10 * 1024 * 1024 },
        (error, stdout, stderr) => {
          resolve({
            exitCode: error?.code !== undefined ? (error.code as number) : 0,
            stdout: stdout ?? "",
            stderr: stderr ?? "",
          });
        },
      );

      if (child.stdin) {
        child.stdin.write(input);
        child.stdin.end();
      } else {
        reject(new Error("Failed to open stdin for fabric process"));
      }
    });
  }

  /**
   * Chain multiple fabric patterns in a pipeline.
   * The output of each pattern is fed as input to the next.
   * @param patterns - Array of pattern names to run sequentially
   * @param input - Initial input for the first pattern
   * @returns Final CommandResult from the last pattern in the chain
   */
  async pipeline(patterns: string[], input: string): Promise<CommandResult> {
    if (patterns.length === 0) {
      return { exitCode: 0, stdout: input, stderr: "" };
    }

    let currentInput = input;

    for (const pattern of patterns) {
      const result = await this.run(pattern, currentInput);

      if (result.exitCode !== 0) {
        return result;
      }

      currentInput = result.stdout;
    }

    return { exitCode: 0, stdout: currentInput, stderr: "" };
  }
}
