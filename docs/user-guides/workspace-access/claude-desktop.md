---
title: Claude Desktop
---

[Claude Desktop](https://claude.com/download) is Anthropic's desktop app for
Claude. Claude Desktop can list your running Coder workspaces,
so you can connect to them from the app.

> [!NOTE]
> This integration is implemented by Claude Desktop, not by Coder. Anthropic's
> documentation doesn't currently describe it, and the behavior can change
> between Claude Desktop versions.

## Prerequisites

- [Claude Desktop](https://claude.com/download) installed on your local machine.
- The [Coder CLI](../../install/cli.md) installed and on your `PATH`.
- The Coder CLI authenticated with your deployment:

  ```sh
  coder login <deployment-url>
  ```

- At least one running workspace.

## Set up Claude Desktop

1. Install the Coder CLI and confirm `coder version` works in your terminal.

1. Authenticate the CLI with your deployment:

   ```sh
   coder login <deployment-url>
   ```

1. Restart Claude Desktop. Your running workspaces appear in Claude Desktop.

## Troubleshooting

If your workspaces don't appear in Claude Desktop, check the following:

1. The Coder CLI is installed and on your `PATH`. Run `coder version`.
1. You're logged in. Run `coder login <deployment-url>` if needed.
1. The workspace is running.

## Next steps

To explore other ways to use Claude with Coder:

- [Agent Relay for Claude Code](../../ai-coder/agent-relay/claude-code.md):
  run Claude Code's cloud sessions inside self-hosted Coder workspaces. See the
  [Agent Relay overview](../../ai-coder/agent-relay/index.md) for how it works.
- [Claude Code with AI Gateway](../../ai-coder/ai-gateway/clients/claude-code.md):
  route Claude Code running in a workspace through AI Gateway.
