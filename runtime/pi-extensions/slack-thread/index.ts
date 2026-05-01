/**
 * Pi slack-thread extension — batches assistant text deltas into Slack posts.
 *
 * Subscribes to the pi session's event stream and forwards
 * `message_update` events whose `assistantMessageEvent.type === "text_delta"`
 * into a buffer. The buffer is flushed when it reaches `maxChars` or after
 * `flushMs` of inactivity, whichever comes first. A final flush is triggered
 * on the `agent_end` event so no trailing text is lost.
 *
 * Note on the session shape: `AgentSession.subscribe` is what the SDK exposes
 * (see @mariozechner/pi-coding-agent dist/core/agent-session.d.ts). The plan's
 * `event.type === "message_update"` / `assistantMessageEvent.type === "text_delta"`
 * shape matches the real `AgentEvent` / `AssistantMessageEvent` unions.
 */

export interface SlackPostMessage {
  channel: string;
  thread_ts: string;
  text: string;
}

export type SlackPost = (msg: SlackPostMessage) => Promise<void>;

export interface SlackThreadOpts {
  post: SlackPost;
  channel: string;
  threadTs: string;
  /** Buffer threshold in characters before an automatic flush. Default 3500. */
  maxChars?: number;
  /** Idle timeout in ms before a buffered chunk is flushed. Default 800ms. */
  flushMs?: number;
}

/** Minimal session-like interface used by registerSlackThread. */
export interface SlackThreadSession {
  subscribe(
    listener: (event: SlackThreadEvent) => Promise<void> | void,
  ): () => void;
}

/**
 * Subset of pi events this extension cares about. Mirrors the union members
 * we consume from `AgentSessionEvent` (and the underlying `AgentEvent`).
 */
export type SlackThreadEvent =
  | {
      type: "message_update";
      assistantMessageEvent?: { type: string; delta?: string };
    }
  | { type: "agent_end" }
  | { type: string; [k: string]: unknown };

const DEFAULT_MAX_CHARS = 3500;
const DEFAULT_FLUSH_MS = 800;

/**
 * Buffers assistant text and flushes to Slack via the supplied `post` function.
 *
 * Two flush triggers:
 * 1. Buffer length >= `maxChars` after appending a delta (synchronous flush).
 * 2. `flushMs` of inactivity since the last delta (timer-driven flush).
 */
export class SlackThread {
  private buffer = "";
  private timer: ReturnType<typeof setTimeout> | undefined;
  private readonly maxChars: number;
  private readonly flushMs: number;

  constructor(private readonly opts: SlackThreadOpts) {
    this.maxChars = opts.maxChars ?? DEFAULT_MAX_CHARS;
    this.flushMs = opts.flushMs ?? DEFAULT_FLUSH_MS;
  }

  /** Append a streaming text delta. Triggers a flush if the threshold is hit. */
  async onAssistantText(delta: string): Promise<void> {
    if (!delta) return;
    this.buffer += delta;
    if (this.buffer.length >= this.maxChars) {
      await this.flush();
      return;
    }
    this.scheduleFlush();
  }

  /** Force-flush any buffered text immediately. No-op if buffer is empty. */
  async flush(): Promise<void> {
    if (this.timer !== undefined) {
      clearTimeout(this.timer);
      this.timer = undefined;
    }
    if (!this.buffer) return;
    const text = this.buffer;
    this.buffer = "";
    await this.opts.post({
      channel: this.opts.channel,
      thread_ts: this.opts.threadTs,
      text,
    });
  }

  private scheduleFlush(): void {
    if (this.timer !== undefined) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      // Fire-and-forget: errors propagate to the post() implementation.
      void this.flush();
    }, this.flushMs);
  }
}

/**
 * Register a SlackThread against a pi session.
 *
 * - Subscribes to the session's event stream.
 * - Forwards `message_update` events with a text_delta into the SlackThread buffer.
 * - On `agent_end`, performs a final flush so trailing text is not lost.
 */
export function registerSlackThread(
  session: SlackThreadSession,
  opts: SlackThreadOpts,
): SlackThread {
  const st = new SlackThread(opts);
  session.subscribe(async (event) => {
    if (event.type === "message_update") {
      const ame = (event as { assistantMessageEvent?: { type: string; delta?: string } })
        .assistantMessageEvent;
      if (ame?.type === "text_delta" && typeof ame.delta === "string") {
        await st.onAssistantText(ame.delta);
      }
      return;
    }
    if (event.type === "agent_end") {
      await st.flush();
    }
  });
  return st;
}
