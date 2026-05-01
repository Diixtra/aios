/**
 * Auth-broker client — thin fetch wrapper around the in-cluster
 * `auth-broker` service that brokers ChatGPT-subscription credential
 * leases for pi-engine agent Jobs.
 *
 * Production: bearer is a projected K8s ServiceAccount token read from the
 *   standard kube-mount path. Broker URL comes from `AUTH_BROKER_URL` env.
 * Tests: replace bearer + URL via the `__set*ForTests` helpers and stub
 *   `globalThis.fetch` with `vi.stubGlobal`.
 *
 * The four functions cover the lifecycle: acquireLease before pi runs;
 * downloadBundle to seed `auth.json`; uploadPostRunBundle to persist
 * the rotated access-token (Spike A4/A6: pi mutates auth.json each call);
 * releaseLease in the entrypoint's finally block.
 */
import { promises as fs } from "node:fs";

const DEFAULT_BROKER_URL = "http://auth-broker.aios.svc:8080";
const SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token";

type BearerReader = () => Promise<string>;

const defaultBearerReader: BearerReader = async () =>
  (await fs.readFile(SA_TOKEN_PATH, "utf8")).trim();

let brokerUrl = process.env.AUTH_BROKER_URL ?? DEFAULT_BROKER_URL;
let bearerReader: BearerReader = defaultBearerReader;

/** Override the broker base URL (test seam). */
export function __setBrokerUrlForTests(url: string): void {
  brokerUrl = url;
}

/** Override the bearer-token reader (test seam). */
export function __setBearerReaderForTests(reader: BearerReader): void {
  bearerReader = reader;
}

/** Restore production defaults (test seam, called from afterEach). */
export function __resetForTests(): void {
  brokerUrl = process.env.AUTH_BROKER_URL ?? DEFAULT_BROKER_URL;
  bearerReader = defaultBearerReader;
}

async function authHeader(): Promise<string> {
  return `Bearer ${await bearerReader()}`;
}

export interface LeaseHandle {
  id: string;
}

/**
 * Acquire a lease before pi runs.
 *
 * The broker enforces a single concurrent holder per credential bundle so two
 * agent Jobs can't race on `auth.json` mutation.
 */
export async function acquireLease(holder: string): Promise<LeaseHandle> {
  const res = await fetch(`${brokerUrl}/v1/leases/acquire`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: await authHeader(),
    },
    body: JSON.stringify({ holder }),
  });
  if (!res.ok) throw new Error(`acquireLease ${res.status}`);
  const body = (await res.json()) as { lease_id: string };
  return { id: body.lease_id };
}

/** Release a previously-acquired lease. */
export async function releaseLease(id: string): Promise<void> {
  const res = await fetch(`${brokerUrl}/v1/leases/release`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: await authHeader(),
    },
    body: JSON.stringify({ lease_id: id }),
  });
  if (!res.ok) throw new Error(`releaseLease ${res.status}`);
}

/** Download the current credential bundle (raw `auth.json` bytes). */
export async function downloadBundle(): Promise<Buffer> {
  const res = await fetch(`${brokerUrl}/v1/auth/bundle`, {
    headers: { Authorization: await authHeader() },
  });
  if (!res.ok) throw new Error(`downloadBundle ${res.status}`);
  const ab = await res.arrayBuffer();
  return Buffer.from(ab);
}

/**
 * Upload the (mutated) bundle back to the broker after pi finishes so the
 * rotated access-token persists for the next run.
 */
export async function uploadPostRunBundle(
  bundle: Buffer,
): Promise<{ accepted: boolean }> {
  const form = new FormData();
  form.append(
    "bundle",
    new Blob([new Uint8Array(bundle)], { type: "application/json" }),
    "auth.json",
  );
  const res = await fetch(`${brokerUrl}/v1/auth/bundle/post-run`, {
    method: "POST",
    headers: { Authorization: await authHeader() },
    body: form,
  });
  if (!res.ok) throw new Error(`uploadPostRunBundle ${res.status}`);
  return (await res.json()) as { accepted: boolean };
}
