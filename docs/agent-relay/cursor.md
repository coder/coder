---
title: Agent Relay for Cursor
---

> [!NOTE]
> Agent Relay for Cursor is in [beta](../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).

[Agent Relay](./index.md) connects [Cursor Cloud Agents](https://cursor.com/cloud) to self-hosted Coder workspaces.
Cursor calls this self-hosted worker model [self-hosted machines](https://cursor.com/docs/cloud-agent/self-hosted).
Developers keep using the Cursor client and cloud agent workflow they already know.
The agent's tool calls run inside a Coder workspace on infrastructure you control instead of a Cursor-managed environment.

Cursor's agent orchestration and AI inference remain cloud-hosted.
Coder doesn't proxy or observe model inference.

This page is for Coder administrators who set up Agent Relay for Cursor.
Complete the shared steps in [Set up Agent Relay](./setup.md) alongside this page.

## How it works

1. A developer selects a Cursor pool that Agent Relay serves, then starts a cloud agent.
1. Agent Relay claims the pending request and matches the developer's Cursor account email to a Coder user.
1. Agent Relay builds a workspace for that user from the pool's mapped template.
1. A Cursor worker in the workspace connects to the request and runs the agent's tool calls.
1. When the worker releases the session, Agent Relay deletes the workspace.

Each Agent Relay pool maps to one Cursor pool, one Coder organization, and one template.
Cursor allows up to 200 workers per user and 1,000 per team.
Refer to [Cursor's team pools documentation](https://cursor.com/docs/cloud-agent/self-hosted/pool).

## Requirements

- A Cursor Enterprise plan with [self-hosted machines](https://cursor.com/docs/cloud-agent/self-hosted).
- A Cursor team admin who can turn on **Allow Self-Hosted Machines** in the Cloud Agents dashboard.
- A Cursor service account API key.
  One key can serve at most four pools.

## Configure a Cursor pool

These steps are [Step 4 of Set up Agent Relay](./setup.md#step-4-configure-provider-pools).
When you finish them, return to [Step 5: Install Agent Relay](./setup.md#step-5-install-agent-relay).

### Step 1: Allow self-hosted machines

A Cursor team admin must turn on **Allow Self-Hosted Machines** in the Cursor Cloud Agents dashboard.
With this setting turned on, developers can send cloud agents to self-hosted workers.

To send every cloud agent to self-hosted workers, also turn on **Require Self-Hosted Machines**.

### Step 2: Create a Cursor service account API key

1. Open the [Cursor API key dashboard](https://cursor.com/dashboard/api).
1. Go to **API & SSH Keys**.
1. Select **New Service Account**.
1. Enter a name and description.
1. Select **Create** and copy the key.

### Step 3: Store the API key

If you install with Helm, store the key in a Kubernetes secret:

```shell
kubectl create secret generic agent-relay-pools \
  --namespace coder \
  --from-literal=cursor-service-account-api-key="$CURSOR_SERVICE_ACCOUNT_KEY"
```

For Docker, set the key in the container's environment.

Agent Relay reads pool secrets only at startup.
When you rotate a key, update the secret, then restart Agent Relay.

The API key never leaves Agent Relay.
When Agent Relay claims a request, it exchanges the key for a session-scoped Cursor token and delivers that token to the workspace as an ephemeral workspace build secret, which the workspace agent sets as `AGENT_RELAY_CURSOR_TOKEN`.
Coder delivers the secret only to the workspace agent, never to the provisioner, so it doesn't appear in build parameters or Terraform state and can't be read back through the API.
The token:

- Authenticates only the claimed worker.
- Stops working when the claim ends or when you delete the API key that minted it.
- Expires after 7&nbsp;days at most.

### Step 4: Add the module to your template

Add the [`coder/agent-relay-cursor`](https://registry.coder.com/modules/coder/agent-relay-cursor) module and the `agent_relay_status` metadata item:

```tf
module "cursor_worker" {
  source   = "registry.coder.com/coder/agent-relay-cursor/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id
  # Set to false and bake the CLI into your image for faster starts.
  install_cli = true
}

resource "coder_agent" "main" {
  # ...
  metadata {
    key          = "agent_relay_status"
    display_name = "Worker"
    script       = module.cursor_worker.status_metadata_script
    interval     = 10
    timeout      = 5
  }
}
```

Use the latest module version from the registry.

> [!NOTE]
> Module version 1.0.0 and later need Coder 2.39 or later and Agent Relay v0.16.0 or later, and receive the credential as a workspace build secret.
> Do not set `AGENT_RELAY_CURSOR_TOKEN` in the template yourself.
> To upgrade from an early access module version, refer to [Upgrade from early access](./setup.md#upgrade-from-early-access).

The template must also meet the [template requirements](./setup.md#step-3-prepare-a-workspace-template) that apply to every provider.
To keep the worker from starting before your startup scripts finish, order the module after them, as described in [Start the worker after workspace setup](./setup.md#start-the-worker-after-workspace-setup).

When Agent Relay creates the workspace, the module adds **Open in Cursor Web** and **Open in Cursor Desktop** apps that open the cloud agent.

### Step 5: Add the pool to the Agent Relay configuration

Each pool needs a unique `name`, a `cursor_pool_name`, an `organization`, a `template`, and a `secret_env` that names the environment variable holding the API key.

<div class="tabs">

### Kubernetes

Add the pool and its secret to `values.yaml`:

```yaml
pools:
  cursor:
    - name: example
      secret_env: CURSOR_SERVICE_ACCOUNT_KEY
      # The pool name developers select in Cursor.
      cursor_pool_name: example-pool
      organization: coder
      template: cursor-relay
      # Optional: caps claim-to-ready. Default 10m, maximum 15m.
      dispatch_deadline: 10m
      # Optional: how long the worker waits for a follow-up after a turn,
      # a duration in whole seconds, such as 30m. Default 1h; 0 never releases.
      idle_release_timeout: 1h
      # Optional: fixed values for template parameters. Use the
      # parameters your template defines.
      template_parameters:
        region: eu-west-1

secretEnv:
  - name: CURSOR_SERVICE_ACCOUNT_KEY
    secretName: agent-relay-pools
    key: cursor-service-account-api-key
```

### Docker

Add the pool to `config.yaml`:

```yaml
cursorWorkerPools:
  - name: example
    secret_env: CURSOR_SERVICE_ACCOUNT_KEY
    # The pool name developers select in Cursor.
    cursor_pool_name: example-pool
    organization: coder
    template: cursor-relay
    # Optional: caps claim-to-ready. Default 10m, maximum 15m.
    dispatch_deadline: 10m
    # Optional: how long the worker waits for a follow-up after a turn,
    # a duration in whole seconds, such as 30m. Default 1h; 0 never releases.
    idle_release_timeout: 1h
```

Pass `CURSOR_SERVICE_ACCOUNT_KEY` to the container alongside `CODER_AGENT_RELAY_CODERD_SESSION_TOKEN`, as shown in [Install with Docker](./setup.md#docker).
Agent Relay doesn't start without the session token.

</div>

Agent Relay refuses to start if:

- More than four pools share one API key.
- Two pools that share an API key use the same `cursor_pool_name`.
- A `dispatch_deadline` is longer than 15&nbsp;minutes.
- An `idle_release_timeout` has no unit, such as `600`, or isn't whole seconds.
  If you're upgrading from early access, refer to [Upgrade from early access](./setup.md#upgrade-from-early-access).

Agent Relay rejects a `template_parameters` entry that uses a name it sets on every build: `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_cursor_pool_name`, `agent_relay_cursor_repo_url`, `agent_relay_credential`, `agent_relay_cursor_idle_release_timeout`, or `agent_relay_cursor_credential_kind`.

When the pool is configured, return to [Install Agent Relay](./setup.md#step-5-install-agent-relay).

## Build deadline

Each pool's `dispatch_deadline` bounds the time from when Agent Relay claims a request until the workspace build succeeds.
Provisioner queue time counts against it.
Worker startup after the build doesn't.

- The default is `10m` and the maximum is `15m`.
  Set `0` to turn off the deadline.
- When the deadline passes, Agent Relay deletes the workspace and the developer's chat shows `agent-relay: workspace build exceeded the spawn budget`.
- Regardless of the deadline, Agent Relay deletes a workspace that's still building after the longer of `dispatch_deadline` and the reaper's [startup grace](./configuration.md#--reap-startup-grace).
  The grace is 15&nbsp;minutes by default and starts when the latest build is created.

Slow templates are the most common reason sessions don't start.
Pre-pull images, avoid persistent volumes, and bake the Cursor CLI into the image with `install_cli = false`.
For templates whose cold build approaches the deadline, use prebuilds.

## Prebuilds

[Prebuilt workspaces](../templates/extending-templates/prebuilt-workspaces.md) are the most reliable way to fit a template inside the build deadline.
A session claims a workspace that's already running instead of building one.
Prebuilds require a Premium license.

The `coder/agent-relay-cursor` module supports prebuilds without changes.
An unclaimed prebuild reports `idle`, which Agent Relay ignores.
When a session claims the prebuild, Coder applies the session's parameters and the module starts the worker.
When the template orders the module after setup scripts, setup runs again on the claim, and the status item keeps reporting `idle` until the worker starts.
If that setup fails, the status stays `idle` and Agent Relay neither fails the session nor deletes the workspace.

Add a preset with a `prebuilds` block to the template:

```tf
data "coder_workspace_preset" "relay" {
  name = "Agent Relay"
  # Pin only parameters that the pool sets in template_parameters, with the
  # same values. Omit parameters entirely if the pool sets none.
  parameters = {
    region = "eu-west-1"
  }
  prebuilds {
    # Each instance is a workspace that runs continuously.
    instances = 2
    expiration_policy {
      # Rebuild unclaimed prebuilds hourly so sessions don't start on a
      # stale image or CLI.
      ttl = 3600
    }
  }
}
```

- **Pin only the pool's `template_parameters`.**
  A build claims a prebuild only when every parameter the preset pins matches the build request.
  Agent Relay's request carries the pool's `template_parameters` but never template defaults, so a preset that pins anything else never matches.
- **Size `instances` to concurrent session starts.**
  When every prebuild is claimed, new sessions fall back to a cold build.
- **Expire unclaimed prebuilds.**
  Cursor rejects an outdated agent CLI, which matters most with `install_cli = false`.
- **Keep the claim from replacing infrastructure.**
  Claiming changes the workspace owner and name.
  Keep owner- and name-derived values out of fields that force resource replacement, such as immutable Kubernetes pod fields.
- **Make external authentication optional.**
  A system user creates prebuilds and can't link external accounts.
  Declare any `coder_external_auth` data source with `optional = true`.
- **Plan provisioner capacity for prebuilds.**
  Prebuild jobs compete with session builds on shared provisioners.

## Provisioner capacity

Agent Relay runs at most 10 concurrent workspace builds per pool.
Deletes run separately.
Plan for 10 provisioners per pool, plus headroom for deletes, multiplied by the number of pools.
With fewer provisioners, builds queue, and queue time counts against `dispatch_deadline`.

## Network requirements

Agent Relay needs outbound HTTPS to:

- Your Coder deployment's access URL.
- `api.cursor.com`.

Agent Relay keeps a long-lived event stream open to Cursor's API.
A proxy in front of Agent Relay must not buffer the stream.

Workspaces need outbound HTTPS to at least:

- Your Coder deployment's access URL, plus any external DERP or STUN servers your deployment uses.
- `api2.cursor.sh` and `api2direct.cursor.sh`.
- `downloads.cursor.com`.
- `cursor.com`, if `install_cli = true`.
- `cloud-agent-artifacts.s3.us-east-1.amazonaws.com`, the Cursor artifact bucket.
  Refer to [Cursor's artifacts documentation](https://cursor.com/docs/cloud-agent/self-hosted/pool#artifacts).
- Your Git host, over port 443 or 22, whenever sessions clone or push.

Refer to [Cursor's team pools documentation](https://cursor.com/docs/cloud-agent/self-hosted/pool) for the authoritative list before you finalize your allowlist.

## User identity

Agent Relay matches the email that Cursor reports for the request to a Coder user.
The match is exact, case-insensitive, and limited to members of the pool's organization.

When Agent Relay can't serve a request, it claims the request and fails it immediately, so the developer's chat explains what to fix.
This happens when:

- The request has no email, for example because a Cursor service account started it.
- No member of the organization has the email.
- The matching Coder user is suspended or dormant.

Confirm that Cursor and Coder emails match before you onboard users, especially when your identity provider uses a different primary address.

## Source control

Source control for a session has two independent sides, and you must configure both:

1. **Cursor must see the repository.**
   Connect every repository developers will select to your Cursor team's source control integration.
1. **The workspace must be able to clone and push.**
   Coder can supply each developer's Git credentials through external authentication, but the template must declare the provider.
   The worker token that Agent Relay provides authenticates to Cursor only.

The worker doesn't clone repositories.
Clone them in your image, in a Cursor hook, or let the agent clone them after the claim.
The worker doesn't switch branches.
To check out a requested branch or commit, use a Cursor `sessionStart` hook.

For workspace credentials:

- Set a Git identity in the image.
  Commits fail without one.
- Prefer short-lived, least-scoped credentials.
  A credential baked into the image is available to every session that the template serves.
- Make sure credentials work without a prompt.
- To give each session its owner's Git credentials, declare the provider in the template with a `data "coder_external_auth"` block.
  The Coder agent sets `GIT_ASKPASS` in the workspace, so Git over HTTPS uses the owner's linked token, as described in [external authentication](../admin/external-auth.md#oauth-external-auth).
  Use HTTPS repository URLs.
  Agent Relay builds the workspace as the developer's matched Coder user, so the token belongs to that developer.
  If the provider isn't marked `optional`, Coder refuses to create a workspace for a developer who hasn't linked the provider, and Agent Relay can't serve their session.

Each pool maps to one template.
Use separate pools when repositories need different templates or credentials.

To give one repository a template of its own, such as an image that has the repository cloned and its toolchain installed, set [`repo_url`](./configuration.md#cursor-pool-fields) on a pool for that repository.
Cursor calls this a repo-backed pool.
A pool without `repo_url` is an any-repo pool, which serves sessions for any repository, so its workspaces get the repository in one of the ways described earlier in this section.
For how Cursor routes sessions to each kind of pool, refer to [Cursor's any-repo pools documentation](https://cursor.com/docs/cloud-agent/self-hosted/pool#any-repo-pools).

In a repo-backed pool, start the worker in the clone.
The worker runs sessions in the directory it starts in, and the module doesn't pass `--worker-dir`, so the worker starts in the agent's directory, which is usually the home directory.
Set `dir` on `coder_agent` to the clone's path:

```tf
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  # The path the template or image clones the repository into.
  dir  = "/home/coder/repo"
}
```

The directory must exist before the worker starts, so clone into it in the image or in a [script that runs before the module](./setup.md#start-the-worker-after-workspace-setup).
To start the worker in a different directory than the agent, pass `additional_args = ["--worker-dir", "/home/coder/repo"]` to the module instead.

For example, these two pools share one API key:

```yaml
# Under cursorWorkerPools in config.yaml, or pools.cursor in values.yaml.
- name: coder-repo
  secret_env: CURSOR_SERVICE_ACCOUNT_KEY
  cursor_pool_name: coder-repo
  organization: coder
  # The image has coder/coder cloned and its toolchain installed.
  template: cursor-coder
  repo_url: https://github.com/coder/coder
- name: any-repo
  secret_env: CURSOR_SERVICE_ACCOUNT_KEY
  cursor_pool_name: any-repo
  organization: coder
  template: cursor-relay
```

In the Cursor dashboard, `coder-repo` appears under the `coder/coder` repository, and `any-repo` appears under **Any repo**.

## Workspace lifecycle

The module reports the worker's state through `agent_relay_status`, and Agent Relay's reaper acts on it:

| Value             | Meaning                                                                            | Reaper action                                                   |
|-------------------|------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| `pending`         | No state recorded yet.                                                             | Keep. Fail after the startup grace, 15&nbsp;minutes by default. |
| `idle`            | No credential: created manually, an unclaimed prebuild, or restarted after a stop. | Keep.                                                           |
| `working`         | The worker is alive, with or without a session.                                    | Keep.                                                           |
| `serving`         | The worker is alive and a session is attached.                                     | Keep.                                                           |
| `orphaned`        | The worker process is gone without recording an exit.                              | Fail.                                                           |
| `done <exitcode>` | The worker exited with that status. `0` is a normal release.                       | Delete on `0`, fail on anything else.                           |
| `failed <reason>` | The worker couldn't start, such as `runner-agent-missing` when the CLI is absent.  | Fail.                                                           |

Both delete and fail remove the workspace, after the [reap delay](./configuration.md#--reap-delay) if you set one.
A failure is also recorded in logs and metrics, and ends the developer's turn in Cursor with an error if the turn is still waiting.

After each turn, the worker waits for a follow-up for the pool's `idle_release_timeout`, then exits, and Agent Relay deletes the workspace.

- The default is `1h`.
  `0` turns off idle release, so the worker and its workspace stay up until something else stops them.
- Agent Relay logs a warning at startup for values below `5m`.
- The timer starts when the agent finishes its turn, not when the chat closes.
- A follow-up after release rebuilds the workspace with the same name and owner on a clean disk.
  The agent keeps the conversation, but unpushed work is lost.

Module logs are in `$HOME/.coder-modules/coder/agent-relay-cursor/logs/` in the workspace.
They're deleted with the workspace.

## Troubleshoot

Refer to [Troubleshoot Agent Relay](./troubleshooting.md).
Problems that only affect Cursor are under [Cursor sessions](./troubleshooting.md#cursor-sessions).

## Learn more

- [Set up Agent Relay](./setup.md)
- [Configure Slack for Agent Relay](./slack.md#cursor-in-slack)
- [Monitor Agent Relay](./monitoring/index.md)
- [Troubleshoot Agent Relay](./troubleshooting.md)
- [Agent Relay configuration reference](./configuration.md)
- [Cursor self-hosted machines](https://cursor.com/docs/cloud-agent/self-hosted)
