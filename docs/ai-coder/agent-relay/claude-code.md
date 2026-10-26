---
title: Agent Relay for Claude Code
---

> [!NOTE]
> Agent Relay for Claude Code is in [beta](../../reference/feature-stages.md#beta).

[Agent Relay](./index.md) connects [Claude Code](https://claude.ai/code) sessions to self-hosted Coder workspaces.
Anthropic calls this self-hosted worker model [self-hosted environments](https://code.claude.com/docs/en/self-hosted-environments).
Developers keep using claude.ai/code, the Claude apps, or `claude --cloud`.
The Claude Code runner's tool calls run inside a Coder workspace on infrastructure you control instead of an Anthropic-managed environment.

Anthropic's agent orchestration and AI inference remain cloud-hosted.
Coder doesn't proxy or observe model inference.
Inference can't be routed through Amazon Bedrock, Google Cloud, Microsoft Foundry, or an LLM gateway.

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
- An Anthropic organization without Zero Data Retention.
  Self-hosted environments are unavailable when it's on.
- Repositories hosted on GitHub or GitHub Enterprise Server.
- A workspace image with Claude Code version 2.1.224 or later, or the module's `install_cli = true`.
- Git version 2.24 or later in the workspace image.

## Configure a Claude Code pool

> [!IMPORTANT]
> Steps 1 and 2 require an Owner of your Anthropic organization.

### 1. Allow self-hosted environments

1. In claude.ai, open the [Cloud environments](https://claude.ai/admin-settings/cloud-environments) admin page.
1. Turn on **Allow self-hosted environments**.

### 2. Create an environment and copy its key

1. Under **Self-hosted environments**, select **New**.
1. Name the environment and select **Create**.
1. On the second step of the wizard, select **Copy environment key**.

claude.ai shows the environment key once.
The key expires 365&nbsp;days after you create it, so plan its rotation.
Repeat these steps for each environment that you want Agent Relay to serve.

Give each environment a name that tells developers which template it uses.
Every self-hosted environment appears in the environment picker for your organization.

### 3. Store the environment key

If you install with Helm, store each environment key in a Kubernetes secret:

```shell
kubectl create secret generic agent-relay-pools \
  --namespace coder \
  --from-literal=claude-environment-secret="$CLAUDE_ENVIRONMENT_SECRET"
```

For Docker, set the key in the container's environment.

The environment key never leaves Agent Relay.
When Agent Relay dispatches a session, it hands the workspace the session's Anthropic-signed work order instead.
The work order:

- Authenticates the runner to Anthropic for that one session.
- Locks the runner to the Anthropic account the session was issued for.
- Reaches the workspace as an ephemeral, masked parameter that later builds don't reuse.

### 4. Add the module to your template

Add the [`coder/agent-relay-claude-code`](https://registry.coder.com/modules/coder/agent-relay-claude-code) module and the `agent_relay_status` metadata item:

```tf
module "claude_code_runner" {
  source   = "registry.coder.com/coder/agent-relay-claude-code/coder"
  version  = "0.4.0"
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
The template must also meet the [template requirements](./setup.md#3-prepare-a-workspace-template) that apply to every provider.

The module starts `claude self-hosted-runner --capacity 1 --exit-if-unused-min 10` in the background, so the workspace agent reaches `ready` immediately.
A runner that never receives its session exits after 10&nbsp;minutes, and Agent Relay deletes its workspace.

> [!WARNING]
> The module runs every session with `--permission-mode bypassPermissions`, because no terminal is attached to answer approval prompts.
> Apply [default-deny egress](https://code.claude.com/docs/en/self-hosted-environments-deploy#default-deny-egress) to relay workspaces, or the agent can reach any host the workspace can.

### 5. Add the pool to the Agent Relay configuration

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
```

Set `CLAUDE_ENVIRONMENT_SECRET` in the container's environment.

</div>

Agent Relay rejects a `template_parameters` entry that uses a name it sets on every build: `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_client_platform`, `work_order`, `lock_to_account`, or `agent_relay_attempt`.

To serve Claude Tag channel sessions, refer to [Use Agent Relay from Slack](./slack.md#claude-tag-channel-sessions).

When the pool is configured, return to [Install Agent Relay](./setup.md#5-install-agent-relay).

## Build budget

Agent Relay gives each Claude Code workspace a fixed 300-second spawn budget.
The budget starts when Anthropic issues the session's work order, so time in Anthropic's queue and time waiting for a provisioner both count.
When a build misses the budget, Agent Relay returns the session to Anthropic as retryable and deletes the workspace.
The budget isn't configurable.

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
> A build that misses the budget is returned to Anthropic and its workspace is deleted, and Anthropic then sends the session again.
> Each late build adds a delete and a rebuild to the same provisioners.

## Network requirements

Agent Relay needs outbound HTTPS to:

- Your Coder deployment's access URL.
- `api.anthropic.com`.

Workspaces need outbound HTTPS to at least:

- Your Coder deployment's access URL, plus any external DERP or STUN servers your deployment uses.
- `api.anthropic.com`, for the runner control plane, session streaming, and model inference.
- Your Git host, such as `github.com` or your GitHub Enterprise Server host, over port 443 or 22.
- `claude.ai` and `downloads.claude.ai`, if `install_cli = true` or sessions install Anthropic marketplace plugins.

Refer to [Anthropic's network requirements](https://code.claude.com/docs/en/self-hosted-environments-deploy#network-requirements) for the authoritative list before you finalize your allowlist.

- Connections are outbound only.
  Anthropic never connects into your network.
- Corporate egress proxies work through `HTTPS_PROXY` and `NO_PROXY`.
  Session streaming uses server-sent events, so a proxy in the path must not buffer responses.
- Workspace clocks must be synchronized, for example with NTP.
  Anthropic authentication fails when a clock is more than 5&nbsp;minutes off.

## User identity

Agent Relay matches the email that Anthropic attests for the session to a Coder user.
The match is exact, case-insensitive, and limited to members of the pool's organization.
This covers sessions started from claude.ai, the desktop and mobile apps, `claude --cloud`, and a Slack DM with Claude.

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
   Claude Code only offers repositories that the Claude GitHub app or your GitHub Enterprise Server integration can reach.
1. **The workspace must be able to clone and push.**
   The runner clones the selected repository with the Git credentials in the workspace.
   Coder and Agent Relay don't supply them.

> [!WARNING]
> If a developer selects a repository that the Claude GitHub app can't access, the session can stall during repository preparation and never start.
> Grant the Claude GitHub app access to every repository developers can select.

Choose how the workspace gets Git access.
Refer to Anthropic's [Configure Git](https://code.claude.com/docs/en/self-hosted-environments-deploy#configure-git) for details.

| Approach                                                         | What it provides                                                                                         | Limits                                                                                            |
|------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| Credentials in the template or image                             | Your own Git identity and push credentials for any supported host                                        | You own credential scope and rotation                                                             |
| Runner-managed Git config (`SELF_HOSTED_RUNNER_CONFIGURE_GIT=1`) | Claude's Git identity, Anthropic-signed commits, and a `Co-authored-by:` trailer for the session creator | No push credentials, so combine it with another approach. Requires Git 2.34 or later.             |
| Anthropic Git proxy (`CLAUDE_RUNNER_USE_GIT_PROXY=1`)            | No Git credentials in the workspace. Anthropic serves Git with the session creator's GitHub token.       | `github.com` only, session repositories only, and branch pushes only. Requires Git 2.32 or later. |

Set runner options as environment variables on the `coder_agent` resource:

```tf
resource "coder_agent" "main" {
  # ...
  env = {
    SELF_HOSTED_RUNNER_CONFIGURE_GIT           = "1"
    SELF_HOSTED_RUNNER_PUSH_OUTCOME_ON_RELEASE = "1"
  }
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
Set `SELF_HOSTED_RUNNER_PUSH_OUTCOME_ON_RELEASE=1` to push tracked branches before release.

Each environment maps to one template, so the template's credentials must cover every repository developers can select in that environment.

## Runner status

The module reports the runner's state through `agent_relay_status`:

| Value             | Meaning                                                                           |
|-------------------|-----------------------------------------------------------------------------------|
| `pending`         | No state recorded yet.                                                            |
| `idle`            | No work order, because someone created the workspace manually.                    |
| `working`         | The runner is alive and hasn't picked up a session.                               |
| `serving`         | The runner is alive and picked up a session (best effort).                        |
| `orphaned`        | The runner process is gone without recording an exit.                             |
| `done <exitcode>` | The runner exited with that status. `137` means it was stopped by a signal.       |
| `failed <reason>` | The runner couldn't start, such as `runner-agent-missing` when the CLI is absent. |

Module logs, including the runner's `runner.log`, are in `$HOME/.coder-modules/coder/agent-relay-claude-code/logs/` in the workspace.
They're deleted with the workspace.

## Troubleshoot

| Symptom                                            | Likely cause                                                                                                                                                                                                                                                                            |
|----------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Agent Relay exits at startup                       | An unlicensed deployment, a Coder version older than 2.37, an invalid token, or missing organization permissions. Also check for two pools with the same `name`, a pool missing `organization` or `template`, an unset `secret_env` variable, or a reserved `template_parameters` name. |
| One pool is degraded and the others are fine       | The service account lacks `read` or `use` on the template, the template lacks `allow_user_cancel_workspace_jobs`, or the template doesn't include the module's parameters.                                                                                                              |
| Sessions fail and no workspace is created          | Identity resolution failed. Find the attested email in the logs and confirm an active member of the pool's organization has it.                                                                                                                                                         |
| A session sits queued in Claude Code               | Dispatch failed and Agent Relay returned the session for redelivery. Read the logs and the status page's recent nacks.                                                                                                                                                                  |
| A workspace is created and then deleted            | The build failed. Quota exhaustion is the most common cause. Read the workspace's build log in Coder.                                                                                                                                                                                   |
| Builds time out                                    | The build plus provisioner queue time exceeded the 300-second budget. Speed up the template or add provisioners.                                                                                                                                                                        |
| Builds stay `pending`                              | No online provisioner matches the template's tags. Compare `coder provisioner list` with the template's tags.                                                                                                                                                                           |
| The agent fails to start with an unclear CLI error | The Claude Code CLI in the image is out of date. Rebuild the image, or set `install_cli = true`.                                                                                                                                                                                        |
| A workspace is never deleted                       | The template renamed or omitted `agent_relay_status`, or the runner reports `orphaned`.                                                                                                                                                                                                 |
| A pool stops claiming sessions                     | The environment key expired or was rotated. Look for poll errors on that pool, then update the secret.                                                                                                                                                                                  |
| Anthropic authentication fails                     | The Agent Relay or workspace clock is more than 5&nbsp;minutes off. Synchronize clocks.                                                                                                                                                                                                 |
| A session stalls while preparing the repository    | The Claude GitHub app can't access the selected repository.                                                                                                                                                                                                                             |
| `Please tell me who you are`                       | The image has no Git identity, and runner-managed Git config is off.                                                                                                                                                                                                                    |

For logs, metrics, and the status page, refer to [Monitor Agent Relay](./monitoring.md).

## Learn more

- [Set up Agent Relay](./setup.md)
- [Use Agent Relay from Slack](./slack.md)
- [Monitor Agent Relay](./monitoring.md)
- [Anthropic self-hosted environments](https://code.claude.com/docs/en/self-hosted-environments)
