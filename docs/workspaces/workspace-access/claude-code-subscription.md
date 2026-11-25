---
title: Sign in to Claude Code with a Claude subscription
---

Use your Claude subscription to authenticate [Claude Code](https://code.claude.com/docs/en/overview) in a Coder workspace, instead of an Anthropic API key.
This guide is for developers who run the Claude Code CLI in a workspace terminal.

Claude Code in a workspace is the same CLI that you run on your local machine.
When no other credential is set, Claude Code uses your subscription sign-in.
Anthropic's [authentication precedence](https://code.claude.com/docs/en/authentication#authentication-precedence) lists the order in which Claude Code picks a credential.

You can authenticate in one of two ways:

- [Sign in interactively](#sign-in-interactively): Claude Code saves your sign-in in the workspace.
- [Use a long-lived token](#use-a-long-lived-token): Coder injects a token into your workspaces as an environment variable.

## Prerequisites

- A Claude account with a plan that includes Claude Code.
  Refer to Anthropic's [authentication documentation](https://code.claude.com/docs/en/authentication#log-in-to-claude-code) for the account types.
- Claude Code installed in your workspace.
  Your template can install it with the [Claude Code module](https://registry.coder.com/modules/coder/claude-code), or you can [install it yourself](https://code.claude.com/docs/en/setup).
- A terminal in your workspace, such as [`coder ssh`](./index.md#configure-ssh), the [web terminal](./web-terminal.md), or your IDE's terminal.

## Sign in interactively

1. Open a terminal in your workspace:

   ```sh
   coder ssh <workspace-name>
   ```

1. Start the sign-in:

   ```sh
   claude auth login
   ```

   Claude Code prints a sign-in URL and waits for a code:

   ```text
   Opening browser to sign in…
   If the browser didn't open, visit: https://claude.com/cai/oauth/authorize?...
   Paste code here if prompted >
   ```

   You can also run `claude` and then type `/login` at the Claude Code prompt.

1. Open the sign-in URL in the browser on your local machine.
1. Sign in with your Claude account and approve access.
1. Copy the code that the browser shows.
1. Paste the code at the `Paste code here if prompted` prompt in the workspace terminal.
1. Confirm the sign-in:

   ```sh
   claude auth status
   ```

   The output shows `"loggedIn": true` and `"authMethod": "claude.ai"`.

Claude Code saves your sign-in to `~/.claude/.credentials.json` in the workspace.
If your template does not keep your home directory between workspace restarts, sign in again after each restart, or [use a long-lived token](#use-a-long-lived-token).

### Forward the sign-in port

When Claude Code can open a browser on your local machine, for example from your IDE's terminal, the sign-in redirects to a `localhost` address instead of showing a code.
That address points to a port in the workspace, such as `http://localhost:39437/callback`.
Claude Code picks a new port for each sign-in.
Some IDEs forward the port for you.

If the `localhost` page fails to load after you approve access:

1. Note the port number in the `localhost` address.
1. On your local machine, forward the port from the workspace and leave the command running:

   ```sh
   coder port-forward <workspace-name> --tcp <port>
   ```

1. Reload the `localhost` page in your browser.

You can also forward the port [over SSH](./port-forwarding.md#ssh) with `ssh -L`.

## Use a long-lived token

A long-lived token lets Claude Code authenticate without a browser, so every workspace you own can start Claude Code already signed in.

1. Generate a token with Claude Code, on your local machine or in a workspace:

   ```sh
   claude setup-token
   ```

   The command opens the same sign-in flow as `claude auth login` and prints a token when you approve access.
   Claude Code does not save the token.
   Refer to Anthropic's [long-lived token documentation](https://code.claude.com/docs/en/authentication#generate-a-long-lived-token) for its requirements and limits.

1. Copy the token.
1. In the Coder dashboard, select your avatar, then select **Account** > **Secrets**.
1. Select **Add secret**.
1. Enter a **Name**, such as `claude-code-oauth-token`.
1. Paste the token into **Value**.
1. Enter `CLAUDE_CODE_OAUTH_TOKEN` in **Environment variable**.
1. Select **Save**.
1. Restart your workspace.
1. Confirm that Claude Code reads the token:

   ```sh
   claude auth status --text
   ```

   ```text
   Auth token: CLAUDE_CODE_OAUTH_TOKEN
   ```

Coder injects the secret into every workspace you own.
To manage the secret from the CLI instead, refer to [User secrets](../user-secrets.md).

### Set the token in a template

Template administrators can pass a token to the [Claude Code module](https://registry.coder.com/modules/coder/claude-code) with its `claude_code_oauth_token` input.
The module sets `CLAUDE_CODE_OAUTH_TOKEN` in every workspace that the template creates, so every user of the template shares the same subscription.
For a token per user, have each user [store it as a user secret](#use-a-long-lived-token).
A user secret overrides an environment variable with the same name that the template sets.

```tf
variable "claude_code_oauth_token" {
  type      = string
  sensitive = true
}

module "claude-code" {
  source                  = "registry.coder.com/coder/claude-code/coder"
  version                 = "~> 5.2"
  agent_id                = coder_agent.main.id
  claude_code_oauth_token = var.claude_code_oauth_token
}
```

Set only one authentication input on the module.
The module rejects `claude_code_oauth_token` together with `enable_ai_gateway` or `api_key_helper`.

## Troubleshooting

### Claude Code uses an API key instead of your subscription

When `ANTHROPIC_API_KEY` is set, it takes precedence over your subscription sign-in and your token.
In an interactive session, Claude Code asks once whether to use the key.
If the key belongs to a disabled or expired organization, requests fail with authentication errors.

1. Check which credential Claude Code uses:

   ```sh
   claude auth status --text
   ```

   This output shows that the API key overrides the token:

   ```text
   Auth token: CLAUDE_CODE_OAUTH_TOKEN · not in use
   API key: ANTHROPIC_API_KEY
   ```

1. Remove the API key from your shell:

   ```sh
   unset ANTHROPIC_API_KEY
   ```

1. Start Claude Code and type `/status` to confirm that it uses your subscription.

`unset` affects only the current shell.
If `ANTHROPIC_API_KEY` returns in new shells, remove it where it's set, such as a [user secret](../user-secrets.md), your [dotfiles](../workspace-dotfiles.md), or the template.
If your template sets it, ask your template administrator to remove the `anthropic_api_key` input from the Claude Code module.

`ANTHROPIC_AUTH_TOKEN` also overrides your subscription.
When a template enables [AI Gateway](../../ai-governance/ai-gateway/clients/claude-code.md) for Claude Code, Claude Code authenticates with AI Gateway through this variable.

## Learn more

- [Authentication](https://code.claude.com/docs/en/authentication) in the Claude Code documentation
- [Workspace ports](./port-forwarding.md)
- [User secrets](../user-secrets.md)
- [Claude Code module](https://registry.coder.com/modules/coder/claude-code) in the Coder Registry
