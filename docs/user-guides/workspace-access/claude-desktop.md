---
title: Claude Desktop
---

[Claude Desktop](https://claude.com/download) is Anthropic's desktop app for
Claude. Claude Desktop can discover your running Coder workspaces and list them
as SSH targets, so you can connect to them from the app.

> [!NOTE]
> This integration is implemented by Claude Desktop, not by Coder. Anthropic's
> documentation doesn't currently describe it, and the behavior below can change
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

1. Restart Claude Desktop. Your running workspaces appear as SSH targets,
   tagged with the source Coder.

## How discovery works

When Claude Desktop starts, it looks for the Coder CLI and lists your
workspaces:

1. It finds `coder` on your `PATH`, and also checks common install locations
   such as `/usr/local/bin`.
1. It runs `coder list -o json` using your existing CLI login. On macOS, the
   CLI stores this login in `~/Library/Application Support/coderv2`.
   Discovery times out after 5 seconds.
1. For each running workspace agent, it builds the hostname
   `<agent>.<workspace>.<owner>.coder`.
1. It connects through the `Host *.coder` block that `coder config-ssh` adds to
   `~/.ssh/config`. That block runs `coder ssh --stdio` to reach the workspace.

Discovered workspaces are managed by Claude Desktop. Stopped workspaces aren't
listed.

## Troubleshooting

### Workspaces don't appear

Claude Desktop logs `[Coder] discovered N workspace SSH config(s)` when
discovery finishes. On macOS, look for it in the logs under
`~/Library/Logs/Claude/`.

If the count is zero or the line is missing, check the following:

1. `coder` is on your `PATH`. Run `which coder`.
1. You're logged in. Run `coder login <accessURL>` if needed.
1. `coder list -o json` completes successfully and quickly, within the
   5-second discovery timeout.
1. The workspaces you expect are running.
1. You've run `coder config-ssh`, and `~/.ssh/config` contains a `Host *.coder`
   block.

### Connections fail

Confirm that you can connect from a terminal. Replace the placeholders with the
values from `coder list`:

```sh
ssh <agent>.<workspace>.<owner>.coder
```

If this fails, see [Access your workspace](./index.md#configure-ssh) to verify
your SSH setup.
