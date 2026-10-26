---
title: Set up Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../../reference/feature-stages.md#beta) and requires a Premium license.

This guide is for Coder administrators who deploy Agent Relay against an existing Coder deployment.
It covers the steps that are the same for every provider: the service account, provisioners, template rules, installation, and verification.
Provider-specific steps live on the [Claude Code](./claude-code.md) and [Cursor](./cursor.md) pages.

## Before you begin

> [!IMPORTANT]
> Only a user with the Coder `owner` role can create service accounts and mint their tokens.

You need:

- A Coder deployment with a Premium license, on version 2.37 or later.
  Agent Relay refuses to start against versions earlier than v2.37.
- `kubectl` and `helm` access to a Kubernetes cluster, or another runtime that can run a container.
- The [Coder CLI](../../install/cli.md), signed in as an `owner`.
- An account with the provider you want to serve.
  Refer to the requirements on the [Claude Code](./claude-code.md#requirements) or [Cursor](./cursor.md#requirements) page.

## Plan your deployment

- **Run one relay per set of pools.**
  Two relays that serve the same pools both claim the same sessions and contend over the same workspaces.
- **Run one relay per organization to isolate organizations.**
  Give each relay its own service account and its own provider pools.
  For example, a deployment with three organizations runs three relays.
- **Plan provisioner capacity per pool.**
  Every pool you add can start more builds at once.
  Refer to [Provision capacity for Agent Relay](#step-2-provision-capacity-for-agent-relay).

## Step 1: Create a service account

Agent Relay needs no site-wide role.
At startup, it checks that its service account has the permissions that its pools need.

In every organization that a pool names, the service account needs these permissions:

| Resource              | Actions                             |
|-----------------------|-------------------------------------|
| `organization_member` | `read`                              |
| `workspace`           | `create`, `read`, `start`, `delete` |

On every template that a pool names, the service account needs `read` and `use`.
These can come from the template's access control list instead of a role.

The `organization-admin` role in each organization covers all of these permissions.
To grant less, create an organization-scoped [custom role](../../admin/users/groups-roles.md#custom-roles) with only the permissions above.

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
Agent Relay verifies template permissions per pool.
A missing template permission degrades only that pool, and the pool recovers when you fix the template.

## Step 2: Provision capacity for Agent Relay

Agent Relay builds run on your Coder provisioners, and time spent waiting for a free provisioner counts against each session's build deadline.
We recommend dedicated [external provisioners](../../install/operate/provisioners/index.md) for Agent Relay templates, so relay builds don't compete with human workspace builds.

Each provider page explains how to size capacity for its pools:

- [Claude Code provisioner capacity](./claude-code.md#provisioner-capacity)
- [Cursor provisioner capacity](./cursor.md#provisioner-capacity)

To run a dedicated set of provisioners on Kubernetes:

1. Create a provisioner key with a tag for Agent Relay:

   ```shell
   coder provisioner keys create relay-key --org <org> --tag workload=agent-relay
   ```

1. Store the key in a Kubernetes secret.

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
- **Do not put relay templates under a quota.**
  Coder enforces quota during the build.
  A relay build that exceeds the session owner's quota fails, and Agent Relay deletes the workspace.
  Failed builds also consume the owner's quota, which can block their other workspaces.
- **Keep builds fast.**
  Pre-pull images, avoid persistent volumes, and bake the provider CLI into the image with `install_cli = false`.
- **Own your workspace image.**
  `ghcr.io/coder/agent-relay-workspace:latest` is a reference image: `codercom/enterprise-base:ubuntu` with both provider CLIs, rebuilt weekly.
  Fork it and add your own tooling.
- **Restrict workspace egress.**
  Agent sessions run unattended.
  Allow only the hosts on the provider page and the internal services that sessions need.
- **Provide Git credentials in the template.**
  Coder and Agent Relay don't supply Git credentials to relay workspaces.
  Refer to the source control section on each provider page.

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

The chart runs a single replica with `strategy: Recreate` and creates no Kubernetes RBAC.
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
     -e CODER_AGENT_RELAY_PROMETHEUS_ENABLE=true \
     -e CODER_AGENT_RELAY_PROMETHEUS_ADDRESS=0.0.0.0:2112 \
     -e CODER_AGENT_RELAY_CONFIG_PATH=/etc/agent-relay/config.yaml \
     -v "$PWD/config.yaml:/etc/agent-relay/config.yaml:ro" \
     -p 2112:2112 \
     ghcr.io/coder/agent-relay:<version>
   ```

The session token has no config file key.
Set it with `CODER_AGENT_RELAY_CODERD_SESSION_TOKEN`.
Pass each pool's secret through the environment variable that its `secret_env` names, not inline in `config.yaml`.

In a container, the metrics listener binds to loopback by default.
Bind it to `0.0.0.0` to reach it from outside the container.

</div>

Agent Relay release binaries for running as a `systemd` service aren't published yet.

## Step 6: Verify the deployment

Agent Relay fails fast at startup.
An invalid token, an unlicensed deployment, a Coder version earlier than v2.37, or a missing organization permission stops the daemon with an explicit message.

1. Read the daemon's logs:

   ```shell
   kubectl logs -n coder deploy/coder-agent-relay
   ```

1. Confirm that each pool logs that its template is verified and the pool is serving.

1. Start a session from the provider's client and select the pool.

1. In Coder, confirm that a workspace is created for your user.

1. End the session and confirm that Agent Relay deletes the workspace.

For logs, metrics, and troubleshooting, refer to [Monitor Agent Relay](./monitoring.md).

## Workspace lifecycle

The provider module publishes the worker's state through the `agent_relay_status` metadata item.
Agent Relay's reaper reads it on each sweep:

- When the worker exits normally, Agent Relay deletes the workspace.
- When the worker fails or disappears, Agent Relay deletes the workspace and records the failure.
- When a build fails or is canceled, Agent Relay deletes the workspace on the next sweep.
- Agent Relay deletes a workspace that is still building or not started after 15&nbsp;minutes.

Keep these Coder settings in mind for relay templates:

- **Autostop**: Relay sessions don't extend a workspace's autostop deadline.
  A template's autostop schedule stops relay workspaces at the deadline, even in the middle of a session.
  Set the template's default TTL longer than your longest expected session.
- **Stopped workspaces**: Agent Relay doesn't delete stopped workspaces.
  They remain until someone deletes them or [dormancy auto-deletion](../../admin/templates/managing-templates/schedule.md) removes them.
- **Quotas**: Relay builds count against the session owner's [quota](../../admin/users/quotas.md).
  Refer to the template recommendations above.
- **Deleted work**: Agent Relay deletes the workspace when the session ends.
  Unpushed commits and files are lost.

Workspace names start with a provider prefix: `cc-` for Claude Code and `cur-` for Cursor.

## Learn more

- [Agent Relay for Claude Code](./claude-code.md)
- [Agent Relay for Cursor](./cursor.md)
- [Use Agent Relay from Slack](./slack.md)
- [Monitor Agent Relay](./monitoring.md)
- [External provisioners](../../install/operate/provisioners/index.md)
