---
title: Agent Relay for Cursor
---

> [!NOTE]
> Agent Relay for Cursor is in [beta](../../reference/feature-stages.md#beta).

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

The API key never leaves Agent Relay.
When Agent Relay claims a request, it exchanges the key for a session-scoped Cursor token and passes that token to the workspace as an ephemeral, masked parameter.
The token:

- Authenticates only the claimed worker.
- Stops working when the claim ends or when you delete the API key that minted it.
- Expires after 7&nbsp;days at most.

### Step 4: Add the module to your template

Add the [`coder/agent-relay-cursor`](https://registry.coder.com/modules/coder/agent-relay-cursor) module and the `agent_relay_status` metadata item:

```tf
module "cursor_worker" {
  source   = "registry.coder.com/coder/agent-relay-cursor/coder"
  version  = "0.7.0"
  agent_id = coder_agent.main.id
  # Set to false and bake the CLI into your image for faster starts.
  install_cli = true
}

resource "coder_agent" "main" {
  # ...
  # For repository-scoped pools, start the worker in the repository checkout.
  dir = "/home/coder/example-repo"
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
The template must also meet the [template requirements](./setup.md#step-3-prepare-a-workspace-template) that apply to every provider.

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
      # Optional: scope the pool to one repository.
      repo_url: https://github.com/example-org/example-repo
      # Optional: caps claim-to-ready. Default 10m, maximum 15m.
      dispatch_deadline: 10m
      # Optional: seconds the worker waits after a turn. Keep at or above 300.
      idle_release_timeout: 600
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
    # Optional: scope the pool to one repository.
    repo_url: https://github.com/example-org/example-repo
    # Optional: caps claim-to-ready. Default 10m, maximum 15m.
    dispatch_deadline: 10m
    # Optional: seconds the worker waits after a turn. Keep at or above 300.
    idle_release_timeout: 600
```

Set `CURSOR_SERVICE_ACCOUNT_KEY` in the container's environment.

</div>

Agent Relay refuses to start if:

- More than four pools share one API key.
- Two pools that share an API key use the same `cursor_pool_name`.
- A `repo_url` doesn't include the owner and repository in its path.
- A `dispatch_deadline` is longer than 15&nbsp;minutes.

Agent Relay rejects a `template_parameters` entry that uses a name it sets on every build: `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_cursor_pool_name`, `agent_relay_cursor_repo_url`, `agent_relay_credential`, `agent_relay_cursor_idle_release_timeout`, or `agent_relay_cursor_credential_kind`.

When the pool is configured, return to [Install Agent Relay](./setup.md#step-5-install-agent-relay).

## Build deadline

Each pool's `dispatch_deadline` bounds the time from when Agent Relay claims a request until the workspace build succeeds.
Provisioner queue time counts against it.
Worker startup after the build doesn't.

- The default is `10m` and the maximum is `15m`.
  Set `0` to turn off the deadline.
- When the deadline passes, Agent Relay deletes the workspace and the developer's chat shows `agent-relay: workspace build exceeded the spawn budget`.
- Regardless of the deadline, Agent Relay deletes a workspace that's still building 15&nbsp;minutes after its latest build started.

Slow templates are the most common reason sessions don't start.
Pre-pull images, avoid persistent volumes, and bake the Cursor CLI into the image with `install_cli = false`.
For templates whose cold build approaches the deadline, use prebuilds.

## Prebuilds

[Prebuilt workspaces](../../admin/templates/extending-templates/prebuilt-workspaces.md) are the most reliable way to fit a template inside the build deadline.
A session claims a workspace that's already running instead of building one.
Prebuilds require a Premium license.

The `coder/agent-relay-cursor` module supports prebuilds without changes.
An unclaimed prebuild reports `idle`, which Agent Relay ignores.
When a session claims the prebuild, Coder applies the session's parameters and the module starts the worker.

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
   The template must provide Git credentials.
   Coder and Agent Relay don't supply them.
   The worker token that Agent Relay provides authenticates to Cursor only.

Choose a pool type:

|            | Repository-scoped pool (`repo_url` set)              | Any-repository pool (`repo_url` unset)          |
|------------|------------------------------------------------------|-------------------------------------------------|
| Routing    | Requests for that repository route only to this pool | Requests match on the pool name alone           |
| Who clones | The template, before the worker starts               | Your image, hooks, or the agent after the claim |
| Git egress | Required                                             | Required if sessions clone or push              |

For a repository-scoped pool:

- Agent Relay passes the repository to every build as the `agent_relay_cursor_repo_url` parameter.
- The template must clone that repository and set up Git credentials before the module's start script runs.
- Set the agent's `dir` to the checkout, or the worker registers without a repository.
- The worker doesn't switch branches.
  To check out a requested branch or commit, use a Cursor `sessionStart` hook.

For workspace credentials:

- Set a Git identity in the image.
  Commits fail without one.
- Prefer short-lived, least-scoped credentials.
  A credential baked into the image is available to every session that the template serves.
- Make sure credentials work without a prompt.

Each pool maps to one template.
Use one repository-scoped pool per repository when repositories need different templates or credentials.

## Workspace lifecycle

The module reports the worker's state through `agent_relay_status`, and Agent Relay's reaper acts on it:

| Value             | Meaning                                                                            | Reaper action                         |
|-------------------|------------------------------------------------------------------------------------|---------------------------------------|
| `pending`         | No state recorded yet.                                                             | Keep. Fail after 15&nbsp;minutes.     |
| `idle`            | No credential: created manually, an unclaimed prebuild, or restarted after a stop. | Keep.                                 |
| `working`         | The worker is alive, with or without a session.                                    | Keep.                                 |
| `serving`         | The worker is alive and a session is attached.                                     | Keep.                                 |
| `orphaned`        | The worker process is gone without recording an exit.                              | Fail.                                 |
| `done <exitcode>` | The worker exited with that status. `0` is a normal release.                       | Delete on `0`, fail on anything else. |
| `failed <reason>` | The worker couldn't start, such as `runner-agent-missing` when the CLI is absent.  | Fail.                                 |

Both delete and fail remove the workspace.
A failure is also recorded in logs and metrics, and ends the developer's turn in Cursor with an error if the turn is still waiting.

After each turn, the worker waits `idle_release_timeout` seconds for a follow-up, then exits, and Agent Relay deletes the workspace.

- The default is `600`.
  Keep it at `300` or higher.
- The timer starts when the agent finishes its turn, not when the chat closes.
- A follow-up after release rebuilds the workspace with the same name and owner on a clean disk.
  The agent keeps the conversation, but unpushed work is lost.

Module logs are in `$HOME/.coder-modules/coder/agent-relay-cursor/logs/` in the workspace.
They're deleted with the workspace.

## Troubleshoot

| Symptom                                              | Likely cause                                                                                                                                                                                                                                              |
|------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Agent Relay exits at startup                         | An unlicensed deployment, a Coder version earlier than v2.37, an invalid token, or missing organization permissions. Also check the pool rules in [Add the pool to the Agent Relay configuration](#step-5-add-the-pool-to-the-agent-relay-configuration). |
| One pool is degraded and the others are fine         | The service account lacks `read` or `use` on the template, the template lacks `allow_user_cancel_workspace_jobs`, or the template doesn't include the module's parameters.                                                                                |
| Cursor shows an `agent-relay:` account error         | Identity resolution failed. The message names the cause: no email on the request, no matching member of the organization, or a suspended or dormant account.                                                                                              |
| Cursor shows an `agent-relay:` build or worker error | The build failed, exceeded `dispatch_deadline`, or waited too long for a provisioner, or the worker exited before serving. Read the Agent Relay logs and `agent_relay_cursor_claim_fails_total`.                                                          |
| A workspace is created and then deleted              | The build failed. Quota exhaustion is the most common cause. Read the workspace's build log in Coder.                                                                                                                                                     |
| A Cursor chat hangs with no error                    | The worker stopped after it started serving, for example because of autostop, or Agent Relay's call to fail the claim errored, or no relay claimed the request.                                                                                           |
| Builds time out                                      | The build exceeded `dispatch_deadline` or waited for a provisioner. Speed up the template, raise the deadline up to `15m`, use prebuilds, or add provisioners.                                                                                            |
| Builds stay `pending`                                | No online provisioner matches the template's tags. Compare `coder provisioner list` with the template's tags.                                                                                                                                             |
| The agent fails to start with an unclear CLI error   | The Cursor CLI in the image is out of date. Rebuild the image, or set `install_cli = true`.                                                                                                                                                               |
| A workspace is never deleted                         | The template renamed or omitted `agent_relay_status`, or the worker reports `orphaned`.                                                                                                                                                                   |
| A repository-scoped pool never receives requests     | `repo_url` doesn't match the repository the developer selected, or the checkout's `origin` doesn't match.                                                                                                                                                 |
| The agent works on the wrong branch                  | The worker doesn't switch branches. Add a `sessionStart` hook that checks out the requested ref.                                                                                                                                                          |

For logs and metrics, refer to [Monitor Agent Relay](./monitoring.md).

## Learn more

- [Set up Agent Relay](./setup.md)
- [Use Agent Relay from Slack](./slack.md#cursor-in-slack)
- [Monitor Agent Relay](./monitoring.md)
- [Cursor self-hosted machines](https://cursor.com/docs/cloud-agent/self-hosted)
