---
title: Agent Relay for Claude Code
---

> [!NOTE]
> Agent Relay for Claude Code is in [beta](../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).

[Agent Relay](./index.md) connects [Claude Code](https://claude.ai/code) sessions to self-hosted Coder workspaces.
Anthropic calls this self-hosted worker model [self-hosted environments](https://code.claude.com/docs/en/self-hosted-environments).
Developers keep using claude.ai/code, Claude Desktop, the Claude mobile apps, or `claude --cloud`.
The Claude Code runner's tool calls run inside a Coder workspace on infrastructure you control instead of an Anthropic-managed environment.

Anthropic's agent orchestration stays cloud-hosted, and inference is provider-managed.
The runner sends model requests to Anthropic, and Anthropic runs inference on its side.
Coder doesn't proxy or observe model inference.

This page is for Coder administrators who set up Agent Relay for Claude Code.
Complete the shared steps in [Set up Agent Relay](./setup.md) alongside this page.

## How it works

1. A developer starts a Claude Code session and selects a self-hosted environment in the environment picker.
1. Agent Relay claims the session's work order and matches the developer's Anthropic account email to a Coder user.
1. Agent Relay builds a workspace for that user from the environment's mapped template.
1. A Claude Code self-hosted runner in the workspace picks up the session and runs the agent's tool calls.
1. When the runner exits, Agent Relay deletes the workspace.

Each Claude Code environment maps to one Agent Relay pool, one Coder organization, and one template.
Anthropic calls this unit an *environment* in claude.ai and a *pool* in its API, where its ID starts with `ccpool_`.

## Requirements

- An Anthropic Team or Enterprise plan.
  Self-hosted environments are in public beta on these plans and are off by default.
- Cloud sessions turned on for your Anthropic organization.
- An Anthropic organization **without** Zero Data Retention.
  Self-hosted environments are unavailable when it's on.
- Repositories hosted on GitHub.
- A workspace image with Claude Code version 2.1.224 or later, or the module's `install_cli = true`.
- Git version 2.24 or later in the workspace image.

## Configure a Claude Code pool

These steps are [Step 4 of Set up Agent Relay](./setup.md#step-4-configure-provider-pools).
When you finish them, return to [Step 5: Install Agent Relay](./setup.md#step-5-install-agent-relay).

> [!IMPORTANT]
> Steps 1 and 2 require an Owner of your Anthropic organization.

### Step 1: Allow self-hosted environments

1. In claude.ai, open the [Cloud environments](https://claude.ai/admin-settings/cloud-environments) admin page.
1. Turn on **Allow self-hosted environments**.

### Step 2: Create an environment and copy its key

1. Under **Self-hosted environments**, select **New**.
1. Name the environment and select **Create**.
1. On the second step of the wizard, select **Copy environment key**.

claude.ai shows the environment key once.
The key expires 365&nbsp;days after you create it, so plan its rotation.
Repeat these steps for each environment that you want Agent Relay to serve.

Give each environment a name that tells developers which template it uses.
Every self-hosted environment appears in the environment picker for your organization.

### Step 3: Store the environment key

If you install with Helm, store each environment key in a Kubernetes secret:

```shell
kubectl create secret generic agent-relay-pools \
  --namespace coder \
  --from-literal=claude-environment-secret="$CLAUDE_ENVIRONMENT_SECRET"
```

For Docker, set the key in the container's environment.

Agent Relay reads pool secrets only at startup.
When you rotate a key, update the secret, then restart Agent Relay.

The environment key never leaves Agent Relay.
When Agent Relay dispatches a session, it hands the workspace the session's Anthropic-signed work order instead.
The work order:

- Authenticates the runner to Anthropic for that one session.
- Locks the runner to the Anthropic account the session was issued for.
- Reaches the workspace as an ephemeral workspace build secret that later builds don't reuse.
  The workspace agent sets it as `SELF_HOSTED_RUNNER_ENVIRONMENT_SECRET`.
  Coder delivers the secret only to the workspace agent, never to the provisioner, so it doesn't appear in build parameters or Terraform state and can't be read back through the API.

### Step 4: Add the module to your template

Add the [`coder/agent-relay-claude-code`](https://registry.coder.com/modules/coder/agent-relay-claude-code) module and the `agent_relay_status` metadata item:

```tf
module "claude_code_runner" {
  source   = "registry.coder.com/coder/agent-relay-claude-code/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id
  # Set to false and bake the CLI into your image for faster starts.
  install_cli = true
}

resource "coder_agent" "main" {
  # ...
  metadata {
    key          = "agent_relay_status"
    display_name = "Session"
    script       = module.claude_code_runner.status_metadata_script
    interval     = 10
    timeout      = 5
  }
}
```

Use the latest module version from the registry.

> [!NOTE]
> Module version 1.0.0 and later need Coder 2.39 or later and Agent Relay v0.16.0 or later, and receive the credential as a workspace build secret.
> Do not set `SELF_HOSTED_RUNNER_ENVIRONMENT_SECRET` in the template yourself.
> To upgrade from an early access module version, refer to [Upgrade from early access](./setup.md#upgrade-from-early-access).

The template must also meet the [template requirements](./setup.md#step-3-prepare-a-workspace-template) that apply to every provider.
To keep the worker from starting before your startup scripts finish, order the module after them, as described in [Start the worker after workspace setup](./setup.md#start-the-worker-after-workspace-setup).

The module starts `claude self-hosted-runner --capacity 1 --exit-if-unused-min 10 --release-idle-session-min 60` in the background, so the workspace agent reaches `ready` immediately.
A runner that never receives its session exits after 10&nbsp;minutes, and Agent Relay deletes its workspace.
A session with no user message for the pool's `idle_release_timeout`, 60&nbsp;minutes by default, is released: the runner exits, Agent Relay deletes the workspace, and the developer's next message resumes the session on a new runner.
Set `idle_release_timeout` on the pool to change the window, as a duration in whole minutes up to `168h`, such as `30m`, or set it to `0` to turn idle release off.
Refer to [Fields for every pool](./configuration.md#fields-for-every-pool).

> [!WARNING]
> The module runs every session with `--permission-mode bypassPermissions`, because no terminal is attached to answer approval prompts.
> Apply [default-deny egress](https://code.claude.com/docs/en/self-hosted-environments-deploy#default-deny-egress) to relay workspaces, or the agent can reach any host the workspace can.

### Step 5: Add the pool to the Agent Relay configuration

Each pool needs a unique `name`, an `organization`, a `template`, and a `secret_env` that names the environment variable holding its environment key.

<div class="tabs">

### Kubernetes

Add the pool and its secret to `values.yaml`:

```yaml
pools:
  claudeCode:
    - name: example
      secret_env: CLAUDE_ENVIRONMENT_SECRET
      organization: coder
      template: claude-code-relay
      # Optional: fixed values for template parameters. Use the
      # parameters your template defines.
      template_parameters:
        region: us-east-1
      # Optional: the Coder user who owns Claude Tag channel sessions.
      # claude_tag_channel_user: <user-uuid>
      # Optional: release a session after this long without a user message.
      # Default 1h; 0 never releases.
      # idle_release_timeout: 1h
      # Optional: how long the build can take from work order issue.
      # Default 5m, between 10s and 15m.
      # dispatch_deadline: 5m

secretEnv:
  - name: CLAUDE_ENVIRONMENT_SECRET
    secretName: agent-relay-pools
    key: claude-environment-secret
```

### Docker

Add the pool to `config.yaml`:

```yaml
claudeCodeRunnerPools:
  - name: example
    secret_env: CLAUDE_ENVIRONMENT_SECRET
    organization: coder
    template: claude-code-relay
    # Optional: fixed values for template parameters. Use the
    # parameters your template defines.
    template_parameters:
      region: us-east-1
    # Optional: the Coder user who owns Claude Tag channel sessions.
    # claude_tag_channel_user: <user-uuid>
    # Optional: release a session after this long without a user message.
    # Default 1h; 0 never releases.
    # idle_release_timeout: 1h
    # Optional: how long the build can take from work order issue.
    # Default 5m, between 10s and 15m.
    # dispatch_deadline: 5m
```

Pass `CLAUDE_ENVIRONMENT_SECRET` to the container alongside `CODER_AGENT_RELAY_CODERD_SESSION_TOKEN`, as shown in [Install with Docker](./setup.md#docker).
Agent Relay doesn't start without the session token.

</div>

Agent Relay rejects a `template_parameters` entry that uses a name it sets on every build: `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_client_platform`, `agent_relay_claude_code_release_idle_session_min`, `agent_relay_credential`, `agent_relay_claude_code_lock_to_account`, or `agent_relay_attempt`.

To serve Claude Tag channel sessions, refer to [Configure Slack for Agent Relay](./slack.md#claude-tag-channel-sessions).

When the pool is configured, return to [Install Agent Relay](./setup.md#step-5-install-agent-relay).

## Build budget

Each pool's `dispatch_deadline` sets the spawn budget: how long a Claude Code workspace build can take.

- The default is `5m`, the minimum is `10s`, and the maximum is `15m`.
  The value must be whole seconds.
- You can't turn off the budget, because Anthropic requires one.
- Agent Relay sends the budget to Anthropic on every poll.
  If no runner starts within the budget, Anthropic offers the session again.
- The budget starts when Anthropic issues the session's work order, so time in Anthropic's queue and time waiting for a provisioner both count.
- When a build misses the budget, Agent Relay reports the failure to Anthropic, cancels the build if it's still running, and deletes the workspace.
- When a build succeeds but the runner never starts, Agent Relay v0.16.0 or later lets Anthropic retry the session only on its first delivery, and only within `dispatch_deadline` of the build's creation.
  Otherwise, it fails the session, so a broken template doesn't rebuild in a loop.

For templates where the image pull and build regularly take longer than 5&nbsp;minutes, raise `dispatch_deadline`.

To fit inside the budget:

- Pre-pull images and avoid persistent volumes.
- Bake the Claude Code CLI into the image and set `install_cli = false`.
- Pre-clone large repositories into the image.
  The runner fetches into an existing clone instead of cloning again.

## Provisioner capacity

Agent Relay claims at most one new session every 5&nbsp;seconds per environment and builds each workspace in the background.
Deletes run separately.

To size provisioners for each environment, divide your template's typical build time in seconds by 5, then add headroom for deletes.
Multiply the result by the number of environments.
For example, a template that builds in 60&nbsp;seconds needs about 12 provisioners per environment at peak.

> [!WARNING]
> Provisioner shortages feed themselves.
> A build that misses the budget fails, Agent Relay deletes its workspace, and Anthropic sends the session again.
> Each late build adds a delete and a rebuild to the same provisioners.

## Network requirements

Agent Relay needs outbound HTTPS to:

- Your Coder deployment's access URL.
- `api.anthropic.com`.

Workspaces need outbound HTTPS to at least:

- Your Coder deployment's access URL, plus any external DERP or STUN servers your deployment uses.
- `api.anthropic.com`, for the runner control plane, session streaming, and model inference.
- `github.com`, over port 443 or 22.
- `claude.ai` and `downloads.claude.ai`, if `install_cli = true` or sessions install Anthropic marketplace plugins.

Refer to [Anthropic's network requirements](https://code.claude.com/docs/en/self-hosted-environments-deploy#network-requirements) for the authoritative list before you finalize your allowlist.

- Connections are outbound only.
  Anthropic never connects into your network.
- Corporate egress proxies work through `HTTPS_PROXY` and `NO_PROXY`.
  Session streaming uses server-sent events, so a proxy in the path must not buffer responses.
- Workspace clocks must be synchronized, for example with [NTP](https://en.wikipedia.org/wiki/Network_Time_Protocol).
  Anthropic authentication fails when a clock is more than 5&nbsp;minutes off.

## User identity

Agent Relay matches the email that Anthropic attests for the session to a Coder user.
The match is exact, case-insensitive, and limited to members of the pool's organization.
This covers sessions started from claude.ai, the desktop and mobile apps, `claude --cloud`, and a direct message to Claude in Slack.

Agent Relay doesn't serve a session when:

- No member of the organization has the attested email.
- The matching Coder user is suspended or dormant.
- The session attests an agent identity from an Anthropic surface other than Claude Tag.

The reason appears in Agent Relay's logs.
Claude Code doesn't show it to the developer yet.

Confirm that Anthropic and Coder emails match before you onboard users, especially when your identity provider uses a different primary address.

## Source control

Source control for a session has two independent sides, and you must configure both:

1. **Claude Code must see the repository.**
   Claude Code only offers repositories that the Claude GitHub app can reach.
1. **The workspace must be able to clone and push.**
   The runner clones the selected repository with the Git credentials in the workspace.
   Coder can supply each developer's credentials through external authentication, but a template admin must configure the template to provide them.

> [!WARNING]
> If a developer selects a repository that the Claude GitHub app can't access, the session can stall during repository preparation and never start.
> Grant the Claude GitHub app access to every repository developers can select.

Choose how the workspace gets Git access.
Refer to Anthropic's [Configure Git](https://code.claude.com/docs/en/self-hosted-environments-deploy#configure-git) for details.

| Approach                                                   | What it provides                                                                                         | Limits                                                                                            |
|------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| Coder [external authentication](../admin/external-auth.md) | Each session owner's own token for Git over HTTPS, with no credential in the image                       | The owner must link the provider. Claude Tag channel sessions run as a service account.           |
| Credentials in the template or image                       | Your own Git identity and push credentials for any supported host                                        | You own credential scope and rotation                                                             |
| Runner-managed Git config (`--configure-git`)              | Claude's Git identity, Anthropic-signed commits, and a `Co-authored-by:` trailer for the session creator | No push credentials, so combine it with another approach. Requires Git 2.34 or later.             |
| Anthropic Git proxy (`--use-anthropic-git-proxy`)          | No Git credentials in the workspace. Anthropic serves Git with the session creator's GitHub token.       | `github.com` only, session repositories only, and branch pushes only. Requires Git 2.32 or later. |

To give each session its owner's Git credentials, declare the provider in the template with a `data "coder_external_auth"` block.
The Coder agent sets `GIT_ASKPASS` in the workspace, so Git over HTTPS uses the owner's linked token without any credential in the image.
Use HTTPS repository URLs.
Refer to [Git authentication in workspaces](../admin/external-auth.md#oauth-external-auth).
Agent Relay builds the workspace as the developer's matched Coder user, so the token belongs to that developer.
If the provider isn't marked `optional`, Coder refuses to create a workspace for a developer who hasn't linked the provider, and Agent Relay can't serve their session.
Claude Tag channel sessions run as a service account, so give them credentials from the template instead, as described in [Configure Slack for Agent Relay](./slack.md#set-up-claude-tag-channel-sessions).

Pass runner options through the module's `additional_args` input, with one list element per argument:

```tf
module "claude_code_runner" {
  source          = "registry.coder.com/coder/agent-relay-claude-code/coder"
  version         = "1.0.0"
  agent_id        = coder_agent.main.id
  additional_args = ["--configure-git", "--push-outcome-on-release"]
}
```

If you put credentials in the template or image:

- Set a system-wide Git identity.
  Without one, `git commit` fails and the session can't progress.
- Prefer short-lived, least-scoped credentials.
  A credential baked into the image is available to every session.
- Make sure Git works without a prompt.
  The runner sets `GIT_TERMINAL_PROMPT=0` and runs SSH in batch mode, so pre-populate `known_hosts`.

Agent Relay deletes the workspace when the runner exits, so unpushed commits are lost.
Pass `--push-outcome-on-release` to push the session's outcome branches before release.
Before you turn it on, restrict who can push to `claude/*` branches, because a resumed session fetches its branch without checking who pushed it.

Each environment maps to one template, so the template's credentials must cover every repository developers can select in that environment.

## Runner status

The module reports the runner's state through `agent_relay_status`:

| Value             | Meaning                                                                                  |
|-------------------|------------------------------------------------------------------------------------------|
| `pending`         | No state recorded yet.                                                                   |
| `idle`            | No work order: someone created the workspace manually, or started it again after a stop. |
| `working`         | The runner is alive and hasn't picked up a session.                                      |
| `serving`         | The runner is alive and picked up a session (best effort).                               |
| `orphaned`        | The runner process is gone without recording an exit.                                    |
| `done <exitcode>` | The runner exited with that status. `137` means it was stopped by a signal.              |
| `failed <reason>` | The runner couldn't start, such as `runner-agent-missing` when the CLI is absent.        |

Module logs, including the runner's `runner.log`, are in `$HOME/.coder-modules/coder/agent-relay-claude-code/logs/` in the workspace.
They're deleted with the workspace.

## Troubleshoot

Refer to [Troubleshoot Agent Relay](./troubleshooting.md).
Problems that only affect Claude Code are under [Claude Code sessions](./troubleshooting.md#claude-code-sessions).

## Learn more

- [Set up Agent Relay](./setup.md)
- [Configure Slack for Agent Relay](./slack.md)
- [Monitor Agent Relay](./monitoring/index.md)
- [Troubleshoot Agent Relay](./troubleshooting.md)
- [Agent Relay configuration reference](./configuration.md)
- [Anthropic self-hosted environments](https://code.claude.com/docs/en/self-hosted-environments)
