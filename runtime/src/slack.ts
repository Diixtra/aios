import { WebClient } from "@slack/web-api";

/**
 * Slack notifier for task lifecycle events.
 * Posts messages with voice session buttons for human-in-the-loop escalation.
 */
export class SlackNotifier {
  private readonly client: WebClient;
  private readonly channel: string;

  constructor(channel: string, client?: WebClient) {
    this.channel = channel;
    this.client = client ?? new WebClient(process.env.SLACK_BOT_TOKEN);
  }

  /**
   * Post a "task started" notification. Returns the thread_ts for follow-up messages.
   */
  async postTaskStarted(
    taskName: string,
    repo: string,
    prompt: string,
  ): Promise<string> {
    const result = await this.client.chat.postMessage({
      channel: this.channel,
      text: `Task started: ${taskName}`,
      blocks: [
        {
          type: "header",
          text: {
            type: "plain_text",
            text: `Task Started: ${taskName}`,
          },
        },
        {
          type: "section",
          fields: [
            { type: "mrkdwn", text: `*Repo:*\n${repo}` },
            { type: "mrkdwn", text: `*Status:*\n:hourglass: In Progress` },
          ],
        },
        {
          type: "section",
          text: {
            type: "mrkdwn",
            text: `*Prompt:*\n>${prompt.slice(0, 200)}${prompt.length > 200 ? "..." : ""}`,
          },
        },
      ],
    });

    const threadTs = result.ts;
    if (!threadTs) {
      throw new Error("Failed to get thread_ts from Slack response");
    }
    return threadTs;
  }

  /**
   * Post a message to an existing thread.
   */
  async postToThread(
    channel: string,
    threadTs: string,
    message: string,
  ): Promise<void> {
    await this.client.chat.postMessage({
      channel,
      thread_ts: threadTs,
      text: message,
    });
  }

  /**
   * Post a "task completed" notification with a PR link and voice session button.
   */
  async postTaskCompleted(
    threadTs: string,
    taskName: string,
    prUrl?: string,
  ): Promise<void> {
    const blocks: any[] = [
      {
        type: "section",
        text: {
          type: "mrkdwn",
          text: `:white_check_mark: *Task Completed:* ${taskName}${prUrl ? `\n<${prUrl}|View Pull Request>` : ""}`,
        },
      },
      {
        type: "actions",
        elements: [
          {
            type: "button",
            text: { type: "plain_text", text: "Discuss (Voice)" },
            action_id: "aios_voice_session",
            value: JSON.stringify({ taskName, prUrl }),
            style: "primary",
          },
        ],
      },
    ];

    await this.client.chat.postMessage({
      channel: this.channel,
      thread_ts: threadTs,
      text: `Task completed: ${taskName}`,
      blocks,
    });
  }

  /**
   * Post an escalation notification with a danger-styled discuss button.
   */
  async postEscalation(
    threadTs: string,
    taskName: string,
    reason: string,
  ): Promise<void> {
    await this.client.chat.postMessage({
      channel: this.channel,
      thread_ts: threadTs,
      text: `Escalation: ${taskName} - ${reason}`,
      blocks: [
        {
          type: "section",
          text: {
            type: "mrkdwn",
            text: `:warning: *Escalation:* ${taskName}\n*Reason:* ${reason}`,
          },
        },
        {
          type: "actions",
          elements: [
            {
              type: "button",
              text: { type: "plain_text", text: "Discuss" },
              action_id: "aios_escalation_discuss",
              value: JSON.stringify({ taskName, reason }),
              style: "danger",
            },
          ],
        },
      ],
    });
  }
}
