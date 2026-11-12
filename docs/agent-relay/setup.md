---
title: Set up Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).

This guide is for Coder administrators who deploy Agent Relay against an existing Coder deployment.
It covers the steps that are the same for every provider: the service account, provisioners, template rules, installation, and verification.
Provider-specific steps live on the [Claude Code](./claude-code.md) and [Cursor](./cursor.md) pages.

## Before you begin

> [!IMPORTANT]
> Only a user with the Coder `owner` role can create service accounts and mint their tokens.

You need:

- A Coder deployment with a Premium license, on version 2.39 or later.
- Agent Relay v0.16.0 or later.
- Somewhere to run the Agent Relay container: a Kubernetes cluster that you can reach with `kubectl` and `helm`, or a Docker host.
- The [Coder CLI](https://coder.com/downloads), signed in as an `owner`.
- An account with the provider you want to serve.
  Refer to the requirements on the [Claude Code](./claude-code.md#requirements) or [Cursor](./cursor.md#requirements) page.

## Plan your deployment

Choose one of these layouts:

- **One relay per deployment.**
  One relay serves every organization and both providers with a single service account.
- **One relay per organization.**
  Each relay has its own service account and its own provider credentials, which isolates organizations from each other.
  For example, a deployment with three organizations runs three relays.

In either layout, never give two relays the same provider credentials.
Both relays claim the same sessions and compete over the same workspaces.

Plan provisioner capacity for each pool, because every pool you add can start more builds at once.
Refer to [Provision capacity for Agent Relay](#step-2-provision-capacity-for-agent-relay).

## Step 1: Create a service account

Agent Relay needs no site-wide role.
At startup, it checks that its service account has the permissions that its pools need.

In every organization that a pool names, the service account needs these permissions:

| Resource              | Actions                                       |
|-----------------------|-----------------------------------------------|
| `organization_member` | `read`                                        |
| `workspace`           | `create`, `read`, `start`, `update`, `delete` |
| `workspace_secret`    | `create`                                      |

On every template that a pool names, the service account needs `read` and `use`.
By default, a new template grants `use`, which includes `read`, to its organization's `Everyone` group, so a service account in the organization already has both.
Grant them in the template's permissions only if the template doesn't give the `Everyone` group access.

The `organization-admin` role in each organization covers all of these permissions.
To grant less, create an organization-scoped [custom role](../admin/users/groups-roles.md#custom-roles) with only the permissions above.

1. Create the service account in the organization that Agent Relay serves:

   ```shell
   coder users create --service-account --username agent-relay --org <org>
   ```

1. Add the service account to every other organization that a pool names.

1. Assign the `organization-admin` role, or your custom role, in each of those organizations.

1. Create a long-lived token for the service account:

   ```shell
   RELAY_TOKEN=$(coder tokens create --user agent-relay --name agent-relay-token --lifetime 8760h)
   ```

1. If you install with Helm, store the token in a Kubernetes secret:

   ```shell
   kubectl create secret generic agent-relay-coderd \
     --namespace coder \
     --from-literal=session-token="$RELAY_TOKEN"
   ```

   The Helm chart doesn't accept the token as a plain value.

Agent Relay verifies organization permissions at startup.
A missing organization permission stops the daemon.
For example, a service account without the `workspace_secret` `create` permission stops it with `missing coderd capabilities [set-workspace-secrets[<org id>]]`.
Agent Relay verifies template permissions per pool.
A missing template permission degrades only that pool, and the pool recovers when you fix the template.

Why Agent Relay needs some of these permissions:

- `workspace` `update` lets it extend the autostop deadline of workspaces with an active session.
  It checks for this permission in every organization at startup, even if no template uses keepalive.
- `workspace_secret` `create` lets it deliver each session's provider credential to the workspace as an ephemeral workspace build secret.
  Agent Relay checks the Coder version once, at startup, to decide how to deliver credentials, so restart Agent Relay after you upgrade Coder.

## Step 2: Provision capacity for Agent Relay

Agent Relay builds run on your Coder provisioners, and time spent waiting for a free provisioner counts against each session's build deadline.
We recommend dedicated [external provisioners](../install/operate/provisioners/index.md) for Agent Relay templates, so relay builds don't compete with human workspace builds.

Each provider page explains how to size capacity for its pools:

- [Claude Code provisioner capacity](./claude-code.md#provisioner-capacity)
- [Cursor provisioner capacity](./cursor.md#provisioner-capacity)

To run a dedicated set of provisioners on Kubernetes:

1. Create a provisioner key with a tag for Agent Relay:

   ```shell
   coder provisioner keys create relay-key --org <org> --tag workload=agent-relay
   ```

   Copy the key from the output.
   Coder shows it once.

1. Store the key in a Kubernetes secret:

   ```shell
   kubectl create secret generic agent-relay-provisioner-key \
     --namespace coder \
     --from-literal=key="<provisioner-key>"
   ```

1. Create a `provisioner-values.yaml` file:

   ```yaml
   coder:
     env:
       - name: CODER_URL
         value: "https://coder.example.com"
     # One provisioner runs one build at a time.
     replicaCount: 10
   provisionerDaemon:
     pskSecretName: ""
     keySecretName: agent-relay-provisioner-key
     keySecretKey: key
   ```

   The provisioners pick up the `workload=agent-relay` tag from the key.

1. Install the `coder-provisioner` Helm chart with a version that matches your Coder server version:

   ```shell
   helm install coder-provisioner coder-v2/coder-provisioner \
     --namespace coder \
     --version <coder-version> \
     --values provisioner-values.yaml
   ```

1. When you push an Agent Relay template, use the same tag:

   ```shell
   coder templates push <template> --provisioner-tag workload=agent-relay
   ```

If no provisioner with a matching tag is online, relay builds stay `pending`.

These `coder-provisioner` chart values affect Agent Relay:

| Value                                                        | Effect                                                                                                       |
|--------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| `coder.replicaCount`                                         | The number of provisioner pods, which sets concurrent build capacity. The default is `1`.                    |
| `provisionerDaemon.keySecretName`, `keySecretKey`            | The secret that holds the provisioner key. The key carries the provisioner's tags.                           |
| `coder.resources`, `nodeSelector`, `tolerations`, `affinity` | Where pods run and how large they are. Plan for 1&nbsp;vCPU and 1&nbsp;GB of memory per 2 concurrent builds. |
| `provisionerDaemon.terminationGracePeriodSeconds`            | How long a stopping pod has to finish its build. The default is `600`.                                       |

The chart can install more than once under different release names, so you can run one set of provisioners for people and a tagged set for Agent Relay.

## Step 3: Prepare a workspace template

Each pool builds from one template.
Every Agent Relay template must:

- Include the provider's Agent Relay module from the Coder Registry:
  [`coder/agent-relay-claude-code`](https://registry.coder.com/modules/coder/agent-relay-claude-code) or [`coder/agent-relay-cursor`](https://registry.coder.com/modules/coder/agent-relay-cursor).
- Declare the module's `agent_relay_status` metadata item on its `coder_agent` resource.
  Agent Relay reads this value to decide when to delete the workspace.
  If you rename or omit the key, Agent Relay never deletes the workspace.
- Set `allow_user_cancel_workspace_jobs = true`.
  Agent Relay cancels in-flight builds when it rebuilds or tears down a workspace.
  Without this setting, the pool degrades.
- Provide a default for every template parameter, or set a value in the pool's `template_parameters`.

The provider pages show the module block for each provider.

We also recommend that you:

- **Use a dedicated template.**
  Relay workspaces are ephemeral and machine-driven.
  A separate template keeps quotas, access control, and activity policy for people from interfering.
- **Use quotas to control spend.**
  Relay builds count against the session owner's [quota](../admin/users/quotas.md), the same as any other workspace.
  Coder enforces quota during the build, so a build that exceeds the owner's quota fails and Agent Relay deletes the workspace.
  Cursor shows the build error in the developer's chat.
  Claude Code doesn't show it, so the reason appears only in Agent Relay's logs.
- **Keep builds fast.**
  Pre-pull images, avoid persistent volumes, and bake the provider CLI into the image with `install_cli = false`.
- **Own your workspace image.**
  `ghcr.io/coder/agent-relay-workspace:latest` is a reference image: `codercom/enterprise-base:ubuntu` with both provider CLIs, rebuilt weekly.
  Use it as the base of your own image and add your tooling.
- **Restrict workspace egress.**
  Agent sessions run unattended.
  Allow only the hosts on the provider page and the internal services that sessions need.
- **Configure Git credentials in the template.**
  Coder can supply each session owner's Git credentials through [external authentication](../admin/external-auth.md), but the template must declare the provider.
  Refer to the source control section on each provider page.

### Start the worker after workspace setup

Agent Relay hands a workspace to the provider as soon as its build succeeds, and the provider module starts the worker without waiting for your startup scripts.
Without an ordering rule, the agent can run commands while your startup scripts are still cloning repositories or installing tools.

To make the worker wait, put the setup that sessions need in `coder_script` resources or modules.
Then add a [`coder_script_order`](../templates/extending-templates/script-ordering.md) rule that runs the Agent Relay module after them:

```tf
resource "coder_script" "setup" {
  agent_id     = coder_agent.main.id
  display_name = "Workspace setup"
  run_on_start = true
  timeout      = 120 # 2 minutes
  script       = file("${path.module}/setup.sh")
}

data "coder_script_order" "relay_worker" {
  rule {
    phase = "start"
    # Use module.claude_code_runner for Claude Code.
    run   = ["module.cursor_worker"]
    after = ["coder_script.setup"]
  }
}
```

The module READMEs for [`coder/agent-relay-claude-code`](https://registry.coder.com/modules/coder/agent-relay-claude-code) and [`coder/agent-relay-cursor`](https://registry.coder.com/modules/coder/agent-relay-cursor) show this rule with a full example.

- Set `phase = "start"`, because the module also has a shutdown script.
- Script ordering requires the Coder Terraform provider v2.20.0 or later.
- Script ordering also requires Coder v2.39.0 or later for the control plane, provisioners, and workspace agents.
  On earlier versions, Coder ignores the rule without an error, and the worker starts right away.
- Coder can't order the `startup_script` attribute of `coder_agent`.
  Move setup steps that the worker depends on into `coder_script` resources.
- If a prerequisite fails or times out, Coder skips the module's scripts and the worker never starts.
  Agent Relay v0.16.0 or later fails the session about a minute later.
  A Cursor request fails at that point.
  For Claude Code, Agent Relay hands the work order back to Anthropic once, rebuilds the workspace when Anthropic redelivers it, and fails the session if the second setup fails too.
  Agent Relay skips that retry when the failure comes later than the pool's `dispatch_deadline` after the build was created.
  On a claimed prebuild, the status item keeps reporting the prebuild's `idle`, so Agent Relay neither fails the session nor deletes the workspace.
  To start the worker even when setup fails, set `requires = "completion"` on the rule.
- The session waits while prerequisites run, so set a `timeout` on each prerequisite.
  The `timeout` is in seconds and bounds one script.
  Choose the timeouts so that the build plus the sum of the setup timeouts fits the pool's budget:
  - For a Cursor pool, the budget is the reaper's [startup grace](./configuration.md#--reap-startup-grace), 15&nbsp;minutes from the start of the build by default.
    Past it, Agent Relay fails the request and deletes the workspace.
  - For a Claude Code pool, the budget is the pool's [build budget](./claude-code.md#build-budget), 5&nbsp;minutes by default and at most 15.
    The budget starts when Anthropic issues the work order.
    If the build and setup keep overrunning it, no worker ever serves the session: Anthropic offers the session again, and each new build runs the same setup from the start.

To check whether workers start before setup finishes, compare the [`agent_relay_reaper_claim_to_agent_ready_seconds` and `agent_relay_reaper_claim_to_runner_up_seconds`](./monitoring/index.md#shared-metrics) metrics.
A worker that's up before the agent is ready started while startup scripts were still running.

## Step 4: Configure provider pools

Follow the steps on the provider page to create the provider credential, add the module to your template, and write the pool configuration:

- [Configure a Claude Code pool](./claude-code.md#configure-a-claude-code-pool)
- [Configure a Cursor pool](./cursor.md#configure-a-cursor-pool)

Then return to this page to install Agent Relay.

## Step 5: Install Agent Relay

| Artifact   | Location                                      | Versioning                           |
|------------|-----------------------------------------------|--------------------------------------|
| Image      | `ghcr.io/coder/agent-relay`                   | `X.Y.Z`, `X.Y`, `latest` per release |
| Helm chart | `oci://ghcr.io/coder/chart/coder-agent-relay` | Matches the image it deploys         |

Both are public and need no pull credentials.
For every option and Helm value, refer to the [Agent Relay configuration reference](./configuration.md).

<div class="tabs">

### Kubernetes

1. Create a `values.yaml` file with your Coder URL, the token secret from step 1, and the pools and secrets from the provider page:

   ```yaml
   coderd:
     url: https://coder.example.com
     sessionToken:
       secretName: agent-relay-coderd
       key: session-token

   pools:
     claudeCode: []   # Refer to the Claude Code page.
     cursor: []       # Refer to the Cursor page.

   secretEnv: []      # One entry per pool secret_env.
   ```

1. Install the chart:

   ```shell
   helm install agent-relay oci://ghcr.io/coder/chart/coder-agent-relay \
     --namespace coder \
     --version <version> \
     --values values.yaml
   ```

The chart runs a single replica with `strategy: Recreate` and creates no Kubernetes role-based access control (RBAC) resources.
Agent Relay talks to the Coder control plane over HTTPS and never calls the Kubernetes API.

Pin a chart version in production instead of tracking `latest`.

### Docker

The image is a static binary on a distroless base.
It needs no writable path and no baked-in configuration.

1. Create a `config.yaml` file with your Coder URL and the pools from the provider page:

   ```yaml
   # The Coder deployment to serve. It must be licensed.
   coderdURL: https://coder.example.com

   logLevel: info
   logFormat: json

   claudeCodeRunnerPools: []   # Refer to the Claude Code page.
   cursorWorkerPools: []       # Refer to the Cursor page.
   ```

1. Run the container, passing the service account token and each pool's secret as environment variables:

   ```shell
   docker run --rm \
     -e CODER_AGENT_RELAY_CODERD_SESSION_TOKEN="$RELAY_TOKEN" \
     -e CLAUDE_ENVIRONMENT_SECRET \
     -e CURSOR_SERVICE_ACCOUNT_KEY \
     -e CODER_AGENT_RELAY_PROMETHEUS_ENABLE=true \
     -e CODER_AGENT_RELAY_PROMETHEUS_ADDRESS=0.0.0.0:2112 \
     -e CODER_AGENT_RELAY_CONFIG_PATH=/etc/agent-relay/config.yaml \
     -v "$PWD/config.yaml:/etc/agent-relay/config.yaml:ro" \
     -p 2112:2112 \
     ghcr.io/coder/agent-relay:<version>
   ```

   Pass one `-e` flag for each pool's `secret_env` variable, and leave out the providers you don't serve.

Set the session token with `CODER_AGENT_RELAY_CODERD_SESSION_TOKEN`.
Do not put it in `config.yaml`.
Pass each pool's secret through the environment variable that its `secret_env` names, not inline in `config.yaml`.

In a container, the metrics listener binds to loopback by default.
Bind it to `0.0.0.0` to reach it from outside the container.

</div>

Install Agent Relay with the container image or the Helm chart.

## Step 6: Verify the deployment

Agent Relay fails fast at startup.
An invalid token, an unlicensed deployment, an unsupported Coder version, or a missing organization permission stops the daemon with an explicit message.

1. Read the daemon's logs:

   ```shell
   # Kubernetes
   kubectl logs -n coder deploy/coder-agent-relay

   # Docker
   docker logs <container>
   ```

1. Confirm that the `authenticated against coderd` log line shows `credential_transport=secrets`.

1. Confirm that each pool logs that it's ready.

   A Claude Code pool logs `template verified, polling`, and a Cursor pool logs `pool verified, serving`.

1. Confirm that Agent Relay is ready.
   With Helm, the pod becomes `Ready` only after at least one pool passes verification.
   With Docker, run `curl -sS http://localhost:2112/readyz`, which prints `ok` when a pool is serving and lists any pool that is still verifying or has failed.

1. Start a session from the provider's client and select the pool.

1. In Coder, confirm that a workspace is created for your user.

1. End the session and confirm that Agent Relay deletes the workspace.

For logs, metrics, and troubleshooting, refer to [Monitor Agent Relay](./monitoring/index.md).

## Workspace lifecycle

Agent Relay cleans up the workspaces it creates with a background process called the reaper.
The reaper works in passes called sweeps.
In each sweep, it reads every relay workspace's `agent_relay_status` metadata item, which the provider module publishes, and acts on it.
After a sweep ends, the reaper waits 30&nbsp;seconds before it starts the next one.
A sweep checks each pool in turn, so it takes longer when you have more pools and workspaces.
The reaper can act 30&nbsp;seconds or more after a workspace reaches one of these states:

| Workspace state                                                                                     | What the reaper does                                             |
|-----------------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| The worker is starting, working, or serving a session                                               | Keeps the workspace.                                             |
| The worker exited normally                                                                          | Deletes the workspace.                                           |
| The worker failed or disappeared                                                                    | Reports the failure to the provider, then deletes the workspace. |
| The build succeeded, but the worker hasn't started within the startup grace                         | Reports the failure to the provider, then deletes the workspace. |
| The worker hasn't started, and the agent reports that startup scripts failed or timed out           | Reports the failure to the provider, then deletes the workspace. |
| The build failed or was canceled                                                                    | Deletes the workspace.                                           |
| The build is still running after the longer of the startup grace and the pool's `dispatch_deadline` | Cancels the build, then deletes the workspace.                   |

The startup grace is 15&nbsp;minutes by default and starts when the latest build is created.
For failed or timed-out startup scripts, Agent Relay v0.16.0 or later doesn't wait for the startup grace.
After the agent reports `start_error` or `start_timeout`, the reaper waits for two `pending` reads of the worker status, the second from a newer status sample than the first.
Refer to [Start the worker after workspace setup](#start-the-worker-after-workspace-setup).
If you set a [reap delay](./configuration.md#--reap-delay), the reaper waits for it before it deletes a workspace or reports a failure, except in the cases that the option's reference lists.
If a delete fails three times in a row, the reaper stops and leaves the workspace for you to remove.
Refer to [A workspace is never deleted](./troubleshooting.md#a-workspace-is-never-deleted).

### Choose cleanup settings

Keep the defaults unless you need one of these outcomes:

| Goal                                                              | Setting                                                                                        | Default                        |
|-------------------------------------------------------------------|------------------------------------------------------------------------------------------------|--------------------------------|
| Give a slow template more time to build and start its worker      | [`--reap-startup-grace`](./configuration.md#--reap-startup-grace)                              | `15m`                          |
| Inspect a workspace where the worker failed soon after it started | [`--reap-delay`](./configuration.md#--reap-delay) for every pool, or `reap_delay` for one pool | `0s`, which deletes right away |
| Keep autostop from stopping a workspace during a session          | The [session attachment metadata item](#keep-attached-workspaces-running)                      | Off                            |
| Release idle sessions sooner or later                             | The pool's [`idle_release_timeout`](./configuration.md#fields-for-every-pool)                  | `1h`                           |

The other reaper options, [`--reap-workers`](./configuration.md#--reap-workers), [`--reap-keepalive-refresh-before`](./configuration.md#--reap-keepalive-refresh-before), and [`--reap-keepalive-bump-by`](./configuration.md#--reap-keepalive-bump-by), limit how many workspaces a sweep handles at once and tune when the reaper extends an attached workspace's autostop deadline.
You don't need them for the goals in this table.

Each setting has a cost:

- A longer startup grace also means a broken template takes longer to fail, so developers wait longer before they see an error.
- A reap delay postpones provider retries and the failure messages that developers see, and the kept workspaces keep using resources.
  Set it while you debug, then set it back to `0s`.

Keep these Coder settings in mind for relay templates:

- **Autostop**: Agent Relay extends the autostop deadline of a workspace while a session is attached, if the template declares the module's `agent_relay_session_attached` metadata item.
  Without that item, the template's autostop schedule stops relay workspaces at the deadline, even in the middle of a session.
  Refer to [Keep attached workspaces running](#keep-attached-workspaces-running).
- **Stopped workspaces**: Agent Relay doesn't delete stopped workspaces.
  They remain until someone deletes them or [dormancy auto-deletion](../templates/managing-templates/schedule.md) removes them.
- **Quotas**: Relay builds count against the session owner's [quota](../admin/users/quotas.md).
  Refer to the template recommendations above.
- **Deleted work**: Agent Relay deletes the workspace when the session ends.
  Unpushed commits and files are lost.

### Keep attached workspaces running

To keep autostop from stopping a workspace in the middle of a session, add the module's `agent_relay_session_attached` metadata item next to `agent_relay_status`:

```tf
resource "coder_agent" "main" {
  # ...
  metadata {
    key          = "agent_relay_session_attached"
    display_name = "Session attached"
    # Use module.claude_code_runner for Claude Code.
    script       = module.cursor_worker.session_attached_metadata_script
    interval     = 30
    timeout      = 5
  }
}
```

On each sweep, Agent Relay reads this item.
When the value is `attached` and the sample is less than two minutes old, Agent Relay extends a deadline that is less than 35&nbsp;minutes away to 45&nbsp;minutes from now.
To change these durations, set [`--reap-keepalive-refresh-before` and `--reap-keepalive-bump-by`](./configuration.md#workspace-reaper).

- Keep the metadata `interval` well under two minutes, or Agent Relay can't rely on a fresh sample.
- The image needs `curl`, and the Cursor module also needs `awk`.
- Agent Relay doesn't extend a workspace past its template's maximum deadline, and doesn't extend workspaces that have no autostop deadline.
- Keepalive requires module version 1.0.0 or later.

Workspace names start with a provider prefix: `cc-` for Claude Code and `cur-` for Cursor.

## Upgrade from early access

Agent Relay v0.16.0 or later and module version 1.0.0 deliver provider credentials as workspace build secrets, which need Coder 2.39 or later.
Early access module versions, `agent-relay-claude-code` 0.4.0 or earlier and `agent-relay-cursor` 0.7.0 or earlier, don't support build secrets.
Upgrade Coder, Agent Relay, and every Agent Relay template in the same maintenance window.
If you need help planning the upgrade, contact Coder.

1. Upgrade Coder to 2.39 or later.

1. Grant the service account `workspace` `update` and `workspace_secret` `create` in every organization that a pool names.

   Agent Relay doesn't start without them.
   The `organization-admin` role already includes both.

1. Convert each Cursor pool's `idle_release_timeout` from seconds to a duration, such as `10m` for `600`.

   `0` still turns off idle release.

1. Upgrade Agent Relay to v0.16.0 or later.

1. Update every Agent Relay template to module version 1.0.0.

## Learn more

- [Agent Relay for Claude Code](./claude-code.md)
- [Agent Relay for Cursor](./cursor.md)
- [Configure Slack for Agent Relay](./slack.md)
- [Monitor Agent Relay](./monitoring/index.md)
- [Troubleshoot Agent Relay](./troubleshooting.md)
- [Agent Relay configuration reference](./configuration.md)
- [External provisioners](../install/operate/provisioners/index.md)
