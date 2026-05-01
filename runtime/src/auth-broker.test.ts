/**
 * Auth-broker client tests — verify the runtime fetches a SA token, calls
 * the broker's HTTP surface, and surfaces non-2xx responses as errors.
 *
 * The bearer-token reader is injected so tests don't need a fake
 * /var/run/secrets file. Production uses the default reader which reads
 * the projected SA token from the standard kube-mount path.
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  acquireLease,
  downloadBundle,
  releaseLease,
  uploadPostRunBundle,
  __setBearerReaderForTests,
  __setBrokerUrlForTests,
  __resetForTests,
} from "./auth-broker.js";

const BROKER = "http://broker.test:8080";

afterEach(() => {
  __resetForTests();
  vi.unstubAllGlobals();
});

function setupBroker(): void {
  __setBrokerUrlForTests(BROKER);
  __setBearerReaderForTests(async () => "fake-sa-token");
}

describe("auth-broker client", () => {
  describe("acquireLease", () => {
    it("posts holder + returns lease id on 200", async () => {
      setupBroker();
      const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
        expect(url).toBe(`${BROKER}/v1/leases/acquire`);
        expect(init?.method).toBe("POST");
        const headers = init?.headers as Record<string, string>;
        expect(headers.Authorization).toBe("Bearer fake-sa-token");
        expect(headers["Content-Type"]).toBe("application/json");
        expect(JSON.parse(init?.body as string)).toEqual({ holder: "code-pr" });
        return new Response(
          JSON.stringify({ lease_id: "lease-123", expires_at: "2026" }),
          { status: 200 },
        );
      });
      vi.stubGlobal("fetch", fetchMock);

      const lease = await acquireLease("code-pr");
      expect(lease.id).toBe("lease-123");
      expect(fetchMock).toHaveBeenCalledOnce();
    });

    it("throws on non-2xx", async () => {
      setupBroker();
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response("conflict", { status: 409 })),
      );
      await expect(acquireLease("code-pr")).rejects.toThrow("acquireLease 409");
    });
  });

  describe("releaseLease", () => {
    it("posts the lease id with bearer", async () => {
      setupBroker();
      const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
        expect(url).toBe(`${BROKER}/v1/leases/release`);
        expect(JSON.parse(init?.body as string)).toEqual({
          lease_id: "lease-x",
        });
        return new Response(null, { status: 204 });
      });
      vi.stubGlobal("fetch", fetchMock);
      await releaseLease("lease-x");
      expect(fetchMock).toHaveBeenCalledOnce();
    });
  });

  describe("downloadBundle", () => {
    it("returns the bundle bytes on 200", async () => {
      setupBroker();
      const payload = Buffer.from('{"access_token":"abc"}');
      const fetchMock = vi.fn(async (url: string) => {
        expect(url).toBe(`${BROKER}/v1/auth/bundle`);
        return new Response(payload, { status: 200 });
      });
      vi.stubGlobal("fetch", fetchMock);
      const out = await downloadBundle();
      expect(out.toString()).toBe('{"access_token":"abc"}');
    });

    it("throws on non-2xx", async () => {
      setupBroker();
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response("forbidden", { status: 403 })),
      );
      await expect(downloadBundle()).rejects.toThrow("downloadBundle 403");
    });
  });

  describe("uploadPostRunBundle", () => {
    it("posts a multipart body with bundle field on 200", async () => {
      setupBroker();
      const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
        expect(url).toBe(`${BROKER}/v1/auth/bundle/post-run`);
        expect(init?.method).toBe("POST");
        // Body is FormData; verify we sent something non-empty.
        expect(init?.body).toBeInstanceOf(FormData);
        const fd = init!.body as FormData;
        const file = fd.get("bundle") as File | null;
        expect(file).not.toBeNull();
        expect(file!.size).toBeGreaterThan(0);
        return new Response(JSON.stringify({ accepted: true }), {
          status: 200,
        });
      });
      vi.stubGlobal("fetch", fetchMock);
      const result = await uploadPostRunBundle(Buffer.from("rotated-bundle"));
      expect(result.accepted).toBe(true);
    });

    it("throws on non-2xx", async () => {
      setupBroker();
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response("bad", { status: 500 })),
      );
      await expect(uploadPostRunBundle(Buffer.from("x"))).rejects.toThrow(
        "uploadPostRunBundle 500",
      );
    });
  });
});
