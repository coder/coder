---
title: Agent Relay configuration reference
---

This page lists the Agent Relay options, pool fields, and Helm chart values that the beta supports.
For a guided installation, refer to [Set up Agent Relay](./setup.md).

## Configuration sources

Agent Relay reads each option from three sources, in this order of precedence:

1. A command-line flag, such as `--log-level`.
1. An environment variable, such as `CODER_AGENT_RELAY_LOG_LEVEL`.
1. A key in the YAML file that `--config` points to, such as `logLevel`.

The Helm chart sets options through environment variables and doesn't use a configuration file.
Each option below lists its Helm value.
Refer to [Helm values](#helm-values) for chart settings that aren't Agent Relay options.

Keep credentials out of the configuration file:

- Set the session token with the `CODER_AGENT_RELAY_CODERD_SESSION_TOKEN` environment variable.
  Do not put it in the configuration file.
- Set each pool's `secret_env` to the name of an environment variable that holds the pool's secret.

Agent Relay validates its options and pool fields at startup.
An invalid value stops the daemon with an error that names the option or pool.

## Coder deployment

### `--coderd-url`

| Property    | Value                                      |
|-------------|--------------------------------------------|
| Type        | <code>url</code>                           |
| Environment | <code>$CODER_AGENT_RELAY_CODERD_URL</code> |
| YAML        | <code>coderdURL</code>                     |
| Helm        | <code>coderd.url</code>                    |

The access URL of the Coder deployment that Agent Relay serves.
This flag is **required**.

### `--coderd-session-token`

| Property    | Value                                                                                |
|-------------|--------------------------------------------------------------------------------------|
| Type        | <code>string</code>                                                                  |
| Environment | <code>$CODER_AGENT_RELAY_CODERD_SESSION_TOKEN</code>                                 |
| YAML        | <code>coderSessionToken</code>                                                       |
| Helm        | <code>coderd.sessionToken.secretName</code> and <code>coderd.sessionToken.key</code> |

The session token of the Agent Relay service account.
This flag is **required**.
Prefer the environment variable to the flag or the `coderSessionToken` configuration file key.
The Helm chart reads it from a Kubernetes secret and doesn't accept it as a plain value.
Refer to [Step 1: Create a service account](./setup.md#step-1-create-a-service-account).

### `--config`

| Property    | Value                                       |
|-------------|---------------------------------------------|
| Type        | <code>path</code>                           |
| Environment | <code>$CODER_AGENT_RELAY_CONFIG_PATH</code> |

The path to a YAML configuration file.

## Pools

A pool maps a provider credential to a Coder organization and template.
Configure at least one pool across both providers.
A provider without pools is turned off.

Set pools in the YAML file, in Helm values, or as a JSON array in the pool option's environment variable:

```shell
export CODER_AGENT_RELAY_CLAUDE_CODE_RUNNER_POOLS='[{"name":"example","secret_env":"CLAUDE_ENVIRONMENT_SECRET","organization":"coder","template":"claude-code-relay"}]'
export CODER_AGENT_RELAY_CURSOR_WORKER_POOLS='[{"name":"example","secret_env":"CURSOR_SERVICE_ACCOUNT_KEY","cursor_pool_name":"example-pool","organization":"coder","template":"cursor-relay"}]'
```

### `--claude-code-runner-pools`

| Property    | Value                                                    |
|-------------|----------------------------------------------------------|
| Type        | <code>list of pools</code>                               |
| Environment | <code>$CODER_AGENT_RELAY_CLAUDE_CODE_RUNNER_POOLS</code> |
| YAML        | <code>claudeCodeRunnerPools</code>                       |
| Helm        | <code>pools.claudeCode</code>                            |

The Claude Code self-hosted environments to serve.
Each pool maps an environment key to a Coder organization and template.
Refer to [Pool fields](#pool-fields) and [Agent Relay for Claude Code](./claude-code.md).

### `--cursor-worker-pools`

| Property    | Value                                               |
|-------------|-----------------------------------------------------|
| Type        | <code>list of pools</code>                          |
| Environment | <code>$CODER_AGENT_RELAY_CURSOR_WORKER_POOLS</code> |
| YAML        | <code>cursorWorkerPools</code>                      |
| Helm        | <code>pools.cursor</code>                           |

The Cursor self-hosted pools to serve.
Each pool maps a Cursor service account API key to a Coder organization and template.
Refer to [Pool fields](#pool-fields) and [Agent Relay for Cursor](./cursor.md).

## Pool fields

### Fields for every pool

| Field                  | Required | Description                                                                                                                                                                                                                                                                                                                                                                                           |
|------------------------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`                 | Yes      | A unique label for the pool in Agent Relay logs and metrics. It has no meaning to the provider.                                                                                                                                                                                                                                                                                                       |
| `secret_env`           | Yes      | The name of the environment variable that holds the pool's secret: the environment key for Claude Code, or the service account API key for Cursor. Agent Relay doesn't start if the variable is unset or empty.                                                                                                                                                                                       |
| `organization`         | Yes      | The name or UUID of the Coder organization whose members run sessions from this pool. The Agent Relay service account must be a member of it.                                                                                                                                                                                                                                                         |
| `template`             | Yes      | The name or UUID of the template that the pool's workspaces build from. Each pool uses one template.                                                                                                                                                                                                                                                                                                  |
| `template_parameters`  | No       | Fixed parameter values that Agent Relay sets on every build from this pool, as a map of parameter names to values. Use it for required or environment-specific parameters. Agent Relay doesn't log the values.                                                                                                                                                                                        |
| `idle_release_timeout` | No       | How long a session can go without a new message after the agent finishes a turn before it's released and its workspace is deleted, as a duration such as `30m`. Defaults to `1h`. `0` turns off idle release. Claude Code needs whole minutes, up to `168h`. Cursor needs whole seconds and logs a warning at startup for values below `5m`. Agent Relay refuses to start on a number without a unit. |
| `template_version`     | No       | A version name or UUID of `template` that every build from this pool uses, including rebuilds. Leave it unset to use the active version. Refer to [Pin a template version](#pin-a-template-version).                                                                                                                                                                                                  |
| `reap_delay`           | No       | Overrides [`--reap-delay`](#--reap-delay) for this pool, as a duration that isn't negative, such as `10m`. Set `0s` to turn off the delay for this pool.                                                                                                                                                                                                                                              |

Agent Relay also accepts an inline `secret` field in place of `secret_env`.
Set exactly one of the two.
Use `secret_env` to keep the secret out of configuration files and Helm values.

### Claude Code pool fields

| Field                     | Required | Description                                                                                                                                                                                                                                                                                                                                               |
|---------------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `claude_tag_channel_user` | No       | The Coder user who owns workspaces for Claude Tag channel sessions. Set `me` for the Agent Relay service account, or the UUID of a user in the pool's organization. When it's unset, Agent Relay rejects channel sessions. Refer to [Configure Slack for Agent Relay](./slack.md).                                                                        |
| `dispatch_deadline`       | No       | The spawn budget, from when Anthropic issues the work order to a successful build, as a duration in whole seconds such as `5m`. Defaults to `5m`, with a minimum of `10s` and a maximum of `15m`. You can't turn it off. Agent Relay also sends it to Anthropic as the time to wait for a runner. Refer to [Build budget](./claude-code.md#build-budget). |

### Cursor pool fields

| Field               | Required | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
|---------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `cursor_pool_name`  | Yes      | The pool name that workers register under in Cursor and that developers select. It's separate from `name`, which only labels the pool in Agent Relay.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `repo_url`          | No       | Ties the pool to one repository, such as `https://github.com/coder/coder`. The URL path must end in `<owner>/<repo>`. Agent Relay registers the pool in Cursor with this repository, and sets `agent_relay_cursor_repo_url` to it on builds for requests that don't name a repository. The template must clone the repository and provide Git credentials before the worker starts, for example in the image or in a [script that runs before the module](./setup.md#start-the-worker-after-workspace-setup), and start the worker in the clone. Leave it unset for a pool that isn't tied to a repository. For when to set it, refer to [Source control](./cursor.md#source-control). |
| `dispatch_deadline` | No       | The longest time from claim to a ready workspace, as a duration such as `10m`. Defaults to `10m`, with a maximum of `15m`. `0` turns off the deadline, but Agent Relay still deletes a workspace that is building after the [startup grace](#--reap-startup-grace).                                                                                                                                                                                                                                                                                                                                                                                                                    |

Agent Relay refuses to start if:

- More than four pools share one API key.
- Two pools that share an API key use the same `cursor_pool_name`.
- A `dispatch_deadline` is longer than 15&nbsp;minutes.

### Pin a template version

Leave `template_version` unset so that builds use the template's active version, and roll out changes by promoting a version in Coder.
That includes rebuilds: when Agent Relay rebuilds an existing workspace for a session that the provider sends again, it moves the workspace to the active version.
A stopped workspace then picks up the new version, and still starts on a template that requires the active version.
If Coder refuses a rebuild with `400` or `403`, such as for a version that the service account can't use, Agent Relay reports the failure as non-retryable instead of retrying in a loop.

When a pool sets `template_version`, all of its builds use that version, including rebuilds of existing workspaces:

- If the template requires the active version, Coder refuses builds from any other version unless the Agent Relay service account has `update` on the template.
  Agent Relay marks the pool degraded before it starts serving if the account lacks that permission, but doesn't check again.
  If someone promotes a newer version later, every session fails at build time.
- If the version is deleted, archived, or renamed, the pool degrades and its sessions fail without retry until you fix the configuration.
- The pool doesn't pick up newly promoted versions, so module and contract changes don't reach it.

### Reserved template parameter names

Agent Relay sets these parameters on every build to carry the session's identity.
It refuses to start if a pool's `template_parameters` uses one of them.

| Provider    | Reserved names                                                                                                                                                                                                                                         |
|-------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Claude Code | `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_client_platform`, `agent_relay_claude_code_release_idle_session_min`, `agent_relay_credential`, `agent_relay_claude_code_lock_to_account`, `agent_relay_attempt` |
| Cursor      | `agent_relay_session_id`, `agent_relay_delivery_id`, `agent_relay_pool`, `agent_relay_cursor_pool_name`, `agent_relay_cursor_repo_url`, `agent_relay_credential`, `agent_relay_cursor_idle_release_timeout`, `agent_relay_cursor_credential_kind`      |

## Cursor request queue

These options apply to every Cursor pool.
The Helm chart has no dedicated values for them.
Set them with [`extraEnv`](#helm-values).

### `--cursor-relist-interval`

| Property    | Value                                                  |
|-------------|--------------------------------------------------------|
| Type        | <code>duration</code>                                  |
| Environment | <code>$CODER_AGENT_RELAY_CURSOR_RELIST_INTERVAL</code> |
| YAML        | <code>cursorRelistInterval</code>                      |
| Default     | <code>5m0s</code>                                      |

How often each Cursor pool lists pending requests.
Agent Relay also follows Cursor's event stream, but the list is the source of truth.
A shorter interval makes more Cursor API calls in exchange for a more current view of the queue.
Cursor expires a stream position 5&nbsp;minutes after the list that issued it, so a longer interval still lists again when the position expires.

### `--cursor-relist-jitter`

| Property    | Value                                                |
|-------------|------------------------------------------------------|
| Type        | <code>duration</code>                                |
| Environment | <code>$CODER_AGENT_RELAY_CURSOR_RELIST_JITTER</code> |
| YAML        | <code>cursorRelistJitter</code>                      |
| Default     | <code>30s</code>                                     |

The longest random delay that Agent Relay adds to each pool's list interval, so that pools don't list in lockstep.
Set `0` to list exactly on the interval.

### `--cursor-api-url`

| Property    | Value                                          |
|-------------|------------------------------------------------|
| Type        | <code>url</code>                               |
| Environment | <code>$CODER_AGENT_RELAY_CURSOR_API_URL</code> |
| YAML        | <code>cursorAPIURL</code>                      |
| Default     | <code>https://api.cursor.com/</code>           |

The base URL of Cursor's fleet management API.

## Logging

For log fields and examples, refer to [Monitor Agent Relay](./monitoring/index.md#logs).

### `--log-level`

| Property    | Value                                     |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_AGENT_RELAY_LOG_LEVEL</code> |
| YAML        | <code>logLevel</code>                     |
| Helm        | <code>logLevel</code>                     |
| Default     | <code>info</code>                         |

The minimum log level: `debug`, `info`, `warn`, or `error`.

### `--log-format`

| Property    | Value                                                      |
|-------------|------------------------------------------------------------|
| Type        | <code>string</code>                                        |
| Environment | <code>$CODER_AGENT_RELAY_LOG_FORMAT</code>                 |
| YAML        | <code>logFormat</code>                                     |
| Helm        | <code>logFormat</code>                                     |
| Default     | <code>human</code>, or <code>json</code> in the Helm chart |

The log output format: `human` or `json`.
Use `json` for log aggregation.

## Prometheus metrics

For the metrics that Agent Relay exports, refer to [Monitor Agent Relay](./monitoring/index.md#prometheus-metrics).

### `--prometheus-enable`

| Property    | Value                                                      |
|-------------|------------------------------------------------------------|
| Type        | <code>bool</code>                                          |
| Environment | <code>$CODER_AGENT_RELAY_PROMETHEUS_ENABLE</code>          |
| YAML        | <code>prometheusEnable</code>                              |
| Helm        | <code>prometheus.enable</code>                             |
| Default     | <code>false</code>, or <code>true</code> in the Helm chart |

Serve Prometheus metrics, and the `/readyz` readiness endpoint, on the address in `--prometheus-address`.
Metrics are served over plain HTTP.
`/readyz` returns `200` once at least one pool has passed verification and `503` before that, and lists each pool that is still verifying or has failed.
The Helm chart uses `/readyz` as the pod's readiness probe.

### `--prometheus-address`

| Property    | Value                                              |
|-------------|----------------------------------------------------|
| Type        | <code>host:port</code>                             |
| Environment | <code>$CODER_AGENT_RELAY_PROMETHEUS_ADDRESS</code> |
| YAML        | <code>prometheusAddress</code>                     |
| Helm        | <code>prometheus.port</code>                       |
| Default     | <code>127.0.0.1:2112</code>                        |

The address that the metrics listener binds to.
The default binds to loopback, which isn't reachable from outside a container.
In Docker, set `0.0.0.0:2112`.
The Helm chart always binds to `0.0.0.0` on `prometheus.port`.

## Workspace reaper

The reaper is the process that deletes relay workspaces when their sessions end or fail.
Two options control when it deletes a workspace:

- `--reap-startup-grace` applies while a workspace starts.
  It sets how long a new workspace has to build and start its worker before the reaper deletes it.
- `--reap-delay` applies after the reaper decides to delete a workspace.
  It sets how long the reaper keeps the workspace first, so you can inspect it.

The other options in this section limit how many workspaces the reaper handles at once and tune when it extends an attached workspace's autostop deadline.
For how the reaper works and which setting to change, refer to [Workspace lifecycle](./setup.md#workspace-lifecycle).

### `--reap-startup-grace`

| Property    | Value                                              |
|-------------|----------------------------------------------------|
| Type        | <code>duration</code>                              |
| Environment | <code>$CODER_AGENT_RELAY_REAP_STARTUP_GRACE</code> |
| YAML        | <code>reapStartupGrace</code>                      |
| Helm        | <code>reapStartupGrace</code>                      |
| Default     | <code>15m0s</code>                                 |

How long a new workspace has to build and start its worker before the reaper deletes it.
The grace starts when the latest build is created, so it includes the time spent building.
Must be positive.

- If a build is still pending or starting after the longer of this grace and the pool's `dispatch_deadline`, the reaper deletes the workspace.
- A workspace where the build succeeded but the worker still reports no status, or `pending`, fails after this grace.
  In Agent Relay v0.16.0 or later, a workspace whose agent reports that startup scripts failed or timed out fails sooner, about a minute after the agent reports it.
  Refer to [Start the worker after workspace setup](./setup.md#start-the-worker-after-workspace-setup).
- A short grace can fail workspaces where the agent connects slowly or the worker starts slowly.
  Unless you run an isolated test deployment, keep the default.
- When the template orders the worker after setup scripts, this is the budget those scripts share with the build.
  Refer to [Start the worker after workspace setup](./setup.md#start-the-worker-after-workspace-setup).

The reaper acts on the first [sweep](./setup.md#workspace-lifecycle) after the grace passes, which can be 30&nbsp;seconds or more later.
Raise the grace when a working template takes longer than 15&nbsp;minutes to build and start its worker.
Refer to [Choose cleanup settings](./setup.md#choose-cleanup-settings).

### `--reap-delay`

| Property    | Value                                      |
|-------------|--------------------------------------------|
| Type        | <code>duration</code>                      |
| Environment | <code>$CODER_AGENT_RELAY_REAP_DELAY</code> |
| YAML        | <code>reapDelay</code>                     |
| Default     | <code>0s</code>                            |

How long the reaper keeps a workspace after it decides to delete it, so you can inspect the workspace first.
The delay counts from when the latest build finished, whether that build succeeded, failed, or was canceled.
It doesn't count from when the worker exits, so a session that runs longer than the delay gets no extra time.
The delay applies when a worker exits cleanly, when a worker fails, and when a build fails or is canceled.
For a failed worker, the reaper also waits before it reports the failure to the provider, which postpones provider retries and the failure messages that developers see.
Must not be negative.
A pool's `reap_delay` overrides this option.

- If a build fails or is still running when the pool's `dispatch_deadline` passes, Agent Relay deletes the workspace immediately.
- If autostop could stop the workspace before the delay ends, the reaper deletes it right away.
- A worker that never starts fails after the startup grace, so its delay might already have passed.
- If the provider sends the session again and Agent Relay rebuilds the workspace, the delay starts over from the new build.
- While the reaper keeps a workspace, each sweep counts a `delayed` verdict instead of `delete` or `failed`.
  When the delay passes, the next sweep counts `delete` or `failed` and acts.
  With `--log-level` set to `debug`, each kept workspace logs `workspace awaiting reap` with `reap_at`, the time when the delay ends.

Set a delay while you debug a worker that fails soon after it starts, so you can open the workspace and read its logs before Agent Relay deletes it.
To keep a workspace where the worker fails later in a session, set a delay longer than the time between the build finishing and the worker failing.
Refer to [Choose cleanup settings](./setup.md#choose-cleanup-settings).
The Helm chart has no dedicated value for this option.
Set it with [`extraEnv`](#helm-values).

### `--reap-workers`

| Property    | Value                                        |
|-------------|----------------------------------------------|
| Type        | <code>int</code>                             |
| Environment | <code>$CODER_AGENT_RELAY_REAP_WORKERS</code> |
| YAML        | <code>reapWorkers</code>                     |
| Default     | <code>5</code>                               |

How many workspaces the reaper handles at the same time during each sweep.
Workspace statuses and session attachment arrive with the workspace list, so this option limits only the delete, cancel, nack, and deadline extension calls that a sweep makes.
The Helm chart has no dedicated value for this option.
Set it with [`extraEnv`](#helm-values).

### `--reap-keepalive-refresh-before`

| Property    | Value                                                         |
|-------------|---------------------------------------------------------------|
| Type        | <code>duration</code>                                         |
| Environment | <code>$CODER_AGENT_RELAY_REAP_KEEPALIVE_REFRESH_BEFORE</code> |
| YAML        | <code>reapKeepaliveRefreshBefore</code>                       |
| Default     | <code>35m0s</code>                                            |

Extends an attached workspace when its autostop deadline is less than this duration away and its attachment sample is fresh.
Must be at least `5m`.
The sum of this option and `--reap-keepalive-bump-by` must be at least `31m`.
This option is shared by every pool and is independent of `idle_release_timeout`.
The Helm chart has no dedicated value for this option.
Set it with [`extraEnv`](#helm-values).
Refer to [Keep attached workspaces running](./setup.md#keep-attached-workspaces-running).

### `--reap-keepalive-bump-by`

| Property    | Value                                                  |
|-------------|--------------------------------------------------------|
| Type        | <code>duration</code>                                  |
| Environment | <code>$CODER_AGENT_RELAY_REAP_KEEPALIVE_BUMP_BY</code> |
| YAML        | <code>reapKeepaliveBumpBy</code>                       |
| Default     | <code>10m0s</code>                                     |

Extra time beyond `--reap-keepalive-refresh-before` for the new deadline.
Agent Relay measures the new deadline from now, not from the old deadline.
Must be positive.
The workspace TTL takes precedence when it's longer, and the build's maximum deadline still caps the new deadline.
The Helm chart has no dedicated value for this option.
Set it with [`extraEnv`](#helm-values).

## Helm values

The chart's Agent Relay options are listed with each option above.
The chart also accepts these values.
The example shows the values you're most likely to change, with their defaults:

```yaml
image:
  repository: ghcr.io/coder/agent-relay
  # Defaults to the chart's appVersion. Set it to pin an image
  # independently of the chart.
  tag: ""
  pullPolicy: IfNotPresent
  # The published image is public, so this is normally empty.
  pullSecrets: []

# Pool secrets, one entry per secret_env. Each entry becomes an
# environment variable read from a Kubernetes secret.
secretEnv: []
#  - name: BYOC_PROD_SECRET
#    secretName: agent-relay-pools
#    key: byoc-prod

prometheus:
  # Add the prometheus.io/* pod annotations that annotation-based
  # service discovery uses.
  annotations: true

# More environment variables, added last. Use it for options without a
# dedicated value.
extraEnv: []

service:
  # Create a Service for the metrics port. Annotation-based scraping
  # targets pods directly and doesn't need it.
  enable: false
  type: ClusterIP
  annotations: {}

serviceAccount:
  # Agent Relay doesn't call the Kubernetes API, so the chart binds the
  # service account to no roles.
  create: true
  name: coder-agent-relay
  annotations: {}

# Merged over the prometheus.io/* annotations.
podAnnotations: {}
podLabels: {}
resources: {}

# Agent Relay stops claiming sessions as soon as it receives a shutdown
# signal.
terminationGracePeriodSeconds: 60
```

The chart also accepts the standard pod settings `nodeSelector`, `tolerations`, `affinity`, and `priorityClassName`, which are empty by default.
By default, `podSecurityContext` runs as the non-root user `65532` with the `RuntimeDefault` seccomp profile, and `securityContext` sets a read-only root filesystem, disables privilege escalation, and drops all capabilities.

For example, to set options that have no dedicated value:

```yaml
extraEnv:
  - name: CODER_AGENT_RELAY_REAP_WORKERS
    value: "10"
  - name: CODER_AGENT_RELAY_CURSOR_RELIST_INTERVAL
    value: 2m
```

The chart always runs one replica with the `Recreate` strategy.
Two relays that serve the same pools both claim sessions and compete over the same workspaces.

## Learn more

- [Set up Agent Relay](./setup.md)
- [Agent Relay for Claude Code](./claude-code.md)
- [Agent Relay for Cursor](./cursor.md)
- [Monitor Agent Relay](./monitoring/index.md)
