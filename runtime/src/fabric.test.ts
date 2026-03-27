import { describe, it, expect, vi, beforeEach } from "vitest";
import { FabricRunner } from "./fabric.js";
import * as childProcess from "node:child_process";
import { EventEmitter } from "node:events";
import { Writable } from "node:stream";

vi.mock("node:child_process");

function createMockProcess(stdout: string, stderr: string, exitCode: number) {
  const stdin = new Writable({
    write(_chunk, _encoding, callback) {
      callback();
    },
  });

  const proc = Object.assign(new EventEmitter(), { stdin });

  vi.mocked(childProcess.execFile).mockImplementation(
    (_cmd: any, _args: any, _opts: any, callback: any) => {
      const cb = typeof _opts === "function" ? _opts : callback;
      process.nextTick(() => {
        if (exitCode !== 0) {
          const err = Object.assign(new Error("Process failed"), {
            code: exitCode,
          });
          cb(err, stdout, stderr);
        } else {
          cb(null, stdout, stderr);
        }
      });
      return proc as any;
    },
  );
}

describe("FabricRunner", () => {
  let runner: FabricRunner;

  beforeEach(() => {
    vi.clearAllMocks();
    runner = new FabricRunner("fabric");
  });

  describe("run", () => {
    it("runs a fabric pattern and returns output", async () => {
      createMockProcess("analyzed output", "", 0);

      const result = await runner.run("analyze_code", "function hello() {}");

      expect(result).toEqual({
        exitCode: 0,
        stdout: "analyzed output",
        stderr: "",
      });
      expect(childProcess.execFile).toHaveBeenCalledWith(
        "fabric",
        ["-p", "analyze_code"],
        expect.any(Object),
        expect.any(Function),
      );
    });

    it("returns non-zero exit code on failure", async () => {
      createMockProcess("", "pattern not found", 1);

      const result = await runner.run("nonexistent_pattern", "input");

      expect(result.exitCode).toBe(1);
      expect(result.stderr).toBe("pattern not found");
    });
  });

  describe("pipeline", () => {
    it("chains multiple patterns", async () => {
      let callCount = 0;
      vi.mocked(childProcess.execFile).mockImplementation(
        (_cmd: any, args: any, _opts: any, callback: any) => {
          const cb = typeof _opts === "function" ? _opts : callback;
          callCount++;
          const output =
            callCount === 1 ? "intermediate result" : "final result";
          process.nextTick(() => cb(null, output, ""));

          const stdin = new Writable({
            write(_chunk, _encoding, cb) {
              cb();
            },
          });
          return Object.assign(new EventEmitter(), { stdin }) as any;
        },
      );

      const result = await runner.pipeline(
        ["analyze_code", "summarize"],
        "input code",
      );

      expect(result.exitCode).toBe(0);
      expect(result.stdout).toBe("final result");
      expect(childProcess.execFile).toHaveBeenCalledTimes(2);
    });

    it("returns empty patterns with passthrough", async () => {
      const result = await runner.pipeline([], "passthrough input");

      expect(result).toEqual({
        exitCode: 0,
        stdout: "passthrough input",
        stderr: "",
      });
    });

    it("stops on first failure", async () => {
      createMockProcess("", "error in first", 1);

      const result = await runner.pipeline(
        ["bad_pattern", "good_pattern"],
        "input",
      );

      expect(result.exitCode).toBe(1);
      expect(childProcess.execFile).toHaveBeenCalledTimes(1);
    });
  });
});
