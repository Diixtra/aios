import { describe, it, expect, vi, beforeEach } from "vitest";
import { SlackNotifier } from "./slack.js";

const mockPostMessage = vi.fn();

vi.mock("@slack/web-api", () => ({
  WebClient: vi.fn().mockImplementation(() => ({
    chat: { postMessage: mockPostMessage },
  })),
}));

describe("SlackNotifier", () => {
  let notifier: SlackNotifier;

  beforeEach(() => {
    vi.clearAllMocks();
    notifier = new SlackNotifier("C12345");
  });

  describe("postTaskStarted", () => {
    it("posts a task started message and returns thread_ts", async () => {
      mockPostMessage.mockResolvedValue({ ts: "1234567890.123456" });

      const threadTs = await notifier.postTaskStarted(
        "implement-feature",
        "owner/repo",
        "Add a new login page",
      );

      expect(threadTs).toBe("1234567890.123456");
      expect(mockPostMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          channel: "C12345",
          text: "Task started: implement-feature",
          blocks: expect.arrayContaining([
            expect.objectContaining({ type: "header" }),
          ]),
        }),
      );
    });

    it("truncates long prompts", async () => {
      mockPostMessage.mockResolvedValue({ ts: "1234567890.123456" });

      const longPrompt = "x".repeat(300);
      await notifier.postTaskStarted("task", "repo", longPrompt);

      const call = mockPostMessage.mock.calls[0][0];
      const sectionBlock = call.blocks.find(
        (b: any) => b.type === "section" && b.text?.text?.includes("Prompt"),
      );
      expect(sectionBlock.text.text).toContain("...");
    });

    it("throws if no thread_ts returned", async () => {
      mockPostMessage.mockResolvedValue({});

      await expect(
        notifier.postTaskStarted("task", "repo", "prompt"),
      ).rejects.toThrow("Failed to get thread_ts");
    });
  });

  describe("postToThread", () => {
    it("posts a message to an existing thread", async () => {
      mockPostMessage.mockResolvedValue({});

      await notifier.postToThread("C12345", "1234.5678", "Progress update");

      expect(mockPostMessage).toHaveBeenCalledWith({
        channel: "C12345",
        thread_ts: "1234.5678",
        text: "Progress update",
      });
    });
  });

  describe("postTaskCompleted", () => {
    it("posts completion with PR URL and voice button", async () => {
      mockPostMessage.mockResolvedValue({});

      await notifier.postTaskCompleted(
        "1234.5678",
        "implement-feature",
        "https://github.com/owner/repo/pull/42",
      );

      const call = mockPostMessage.mock.calls[0][0];
      expect(call.thread_ts).toBe("1234.5678");
      expect(call.blocks).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "actions",
            elements: expect.arrayContaining([
              expect.objectContaining({
                action_id: "aios_voice_session",
                style: "primary",
              }),
            ]),
          }),
        ]),
      );
    });

    it("posts completion without PR URL", async () => {
      mockPostMessage.mockResolvedValue({});

      await notifier.postTaskCompleted("1234.5678", "research-task");

      const call = mockPostMessage.mock.calls[0][0];
      const section = call.blocks.find((b: any) => b.type === "section");
      expect(section.text.text).not.toContain("View Pull Request");
    });
  });

  describe("postEscalation", () => {
    it("posts escalation with danger-styled button", async () => {
      mockPostMessage.mockResolvedValue({});

      await notifier.postEscalation(
        "1234.5678",
        "implement-feature",
        "Tests failing after 3 attempts",
      );

      const call = mockPostMessage.mock.calls[0][0];
      expect(call.thread_ts).toBe("1234.5678");
      expect(call.blocks).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "actions",
            elements: expect.arrayContaining([
              expect.objectContaining({
                action_id: "aios_escalation_discuss",
                style: "danger",
              }),
            ]),
          }),
        ]),
      );
    });
  });
});
