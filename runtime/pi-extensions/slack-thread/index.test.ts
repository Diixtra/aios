import { describe, it, expect, vi } from "vitest";
import { SlackThread, registerSlackThread } from "./index";

describe("SlackThread", () => {
  it("flushes assistant text deltas to Slack on threshold", async () => {
    const post = vi.fn(async () => undefined);
    const st = new SlackThread({
      post,
      channel: "C1",
      threadTs: "1.0",
      maxChars: 5,
      flushMs: 1000,
    });
    await st.onAssistantText("hello"); // 5 chars — flushes
    await st.onAssistantText(" wor"); // 4 — buffered
    await st.flush();
    expect(post).toHaveBeenCalledTimes(2);
    expect(post).toHaveBeenNthCalledWith(1, {
      channel: "C1",
      thread_ts: "1.0",
      text: "hello",
    });
    expect(post).toHaveBeenNthCalledWith(2, {
      channel: "C1",
      thread_ts: "1.0",
      text: " wor",
    });
  });

  it("flushes after the flushMs timer elapses without further deltas", async () => {
    vi.useFakeTimers();
    try {
      const post = vi.fn(async () => undefined);
      const st = new SlackThread({
        post,
        channel: "C9",
        threadTs: "9.0",
        maxChars: 1000,
        flushMs: 50,
      });
      await st.onAssistantText("hi");
      expect(post).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(50);
      expect(post).toHaveBeenCalledTimes(1);
      expect(post).toHaveBeenCalledWith({
        channel: "C9",
        thread_ts: "9.0",
        text: "hi",
      });
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not post when buffer is empty on flush", async () => {
    const post = vi.fn(async () => undefined);
    const st = new SlackThread({ post, channel: "C1", threadTs: "1.0" });
    await st.flush();
    expect(post).not.toHaveBeenCalled();
  });
});

describe("registerSlackThread", () => {
  it("subscribes to message_update events and forwards text_delta to SlackThread", async () => {
    const post = vi.fn(async () => undefined);
    let listener: ((event: any) => Promise<void> | void) | undefined;
    const session = {
      subscribe: vi.fn((l: (event: any) => Promise<void> | void) => {
        listener = l;
        return () => undefined;
      }),
    } as const;

    registerSlackThread(session, {
      post,
      channel: "C1",
      threadTs: "1.0",
      maxChars: 3,
      flushMs: 5000,
    });

    expect(session.subscribe).toHaveBeenCalledTimes(1);
    expect(listener).toBeDefined();

    // Drive deltas: each delta is 3 chars so each forces a flush.
    await listener!({
      type: "message_update",
      assistantMessageEvent: { type: "text_delta", delta: "abc" },
    });
    await listener!({
      type: "message_update",
      // Non-text events are ignored.
      assistantMessageEvent: { type: "thinking_delta", delta: "ignored" },
    });
    expect(post).toHaveBeenCalledTimes(1);
    expect(post).toHaveBeenCalledWith({
      channel: "C1",
      thread_ts: "1.0",
      text: "abc",
    });
  });

  it("flushes any buffered text on agent_end", async () => {
    const post = vi.fn(async () => undefined);
    let listener: ((event: any) => Promise<void> | void) | undefined;
    const session = {
      subscribe: vi.fn((l: (event: any) => Promise<void> | void) => {
        listener = l;
        return () => undefined;
      }),
    } as const;

    registerSlackThread(session, {
      post,
      channel: "C2",
      threadTs: "2.0",
      maxChars: 1000,
      flushMs: 60_000,
    });

    await listener!({
      type: "message_update",
      assistantMessageEvent: { type: "text_delta", delta: "buffered" },
    });
    expect(post).not.toHaveBeenCalled();

    await listener!({ type: "agent_end", messages: [] });

    expect(post).toHaveBeenCalledTimes(1);
    expect(post).toHaveBeenCalledWith({
      channel: "C2",
      thread_ts: "2.0",
      text: "buffered",
    });
  });
});
