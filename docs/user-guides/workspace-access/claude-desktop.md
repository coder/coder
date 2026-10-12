---
title: Claude Desktop
---

[Claude Desktop](https://claude.com/download) is Anthropic's desktop app for
Claude. Claude Desktop can list your running Coder workspaces as SSH targets,
so you can connect to them from the app.

> [!NOTE]
> This integration is implemented by Claude Desktop, not by Coder. Anthropic's
> documentation doesn't currently describe it, and the behavior can change
> between Claude Desktop versions.

## Prerequisites

- [Claude Desktop](https://claude.com/download) installed on your local machine.
- The [Coder CLI](../../install/cli.md) installed on your local machine.
- At least one running workspace.

## Set up Claude Desktop

1. Authenticate the Coder CLI with your deployment:

   ```sh
   coder login <accessURL>
   ```

1. Add the Coder SSH configuration:

   ```sh
   coder config-ssh
   ```

   Run `coder config-ssh --dry-run` first if you want to preview the changes.

1. Restart Claude Desktop. Your running workspaces appear as SSH targets.

## Troubleshooting

If your workspaces don't appear in Claude Desktop, check the following:

1. The Coder CLI is installed. Run `coder version`.
1. You're logged in. Run `coder login <accessURL>` if needed.
1. The workspace is running.
1. You've run `coder config-ssh`.

If the workspace still doesn't appear, see
[Access your workspace](./index.md#configure-ssh) to verify your SSH setup.
