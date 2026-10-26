---
title: Monitor Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../../reference/feature-stages.md#beta).
> Metric names, labels, and log fields can change between beta releases.

This page is for operators who run Agent Relay and need to answer how many sessions ran, who ran them, and which failed and why.
Agent Relay has three monitoring surfaces:

- **Logs** answer "what happened, in order?"
- **The status page** answers "what is happening now?"
- **Prometheus metrics** answer "how often, and how long?"

## Logs

Agent Relay writes structured logs to stderr and doesn't write log files.
Your container runtime or log shipper controls retention.

```shell
# Kubernetes
kubectl logs -n coder deploy/coder-agent-relay --since=30m

# Docker
docker logs --since=30m <container>
```

Use JSON logs in production.
The Helm chart defaults to `logFormat: json`.
For Docker, set `logFormat: json` in `config.yaml`.
For a short diagnostic window, set `logLevel: debug`.

A startup failure that occurs before the logger is ready prints as a plain `agent-relay: ...` line, not JSON.

### Correlation fields

These fields mean the same thing for both providers, so one query works across Claude Code and Cursor:

| Field            | Meaning                                    | Available                                        |
|------------------|--------------------------------------------|--------------------------------------------------|
| `provider`       | `claude_code` or `cursor`                  | On pool-scoped logs                              |
| `pool`           | The Agent Relay pool name                  | On pool-scoped logs                              |
| `session_id`     | The provider's session or request ID       | Once Agent Relay handles the session             |
| `delivery_id`    | The ID of this delivery or worker attempt  | Once the provider assigns it                     |
| `owner`          | The Coder username of the workspace owner  | After identity resolution                        |
| `owner_id`       | The Coder user UUID of the workspace owner | After identity resolution                        |
| `workspace_id`   | The Coder workspace UUID                   | After Agent Relay creates or finds the workspace |
| `workspace_name` | The Coder workspace name                   | After Agent Relay creates or finds the workspace |

For Claude Code, `client_platform` records where the session started: `web_claude_ai`, `desktop_app`, `ios`, `android`, `claude_code_cli`, `claude_in_slack`, or `unknown`.
Cursor doesn't report a client platform.

To trace a session, start with `provider`, `pool`, and `session_id`.
After Agent Relay creates the workspace, use `workspace_id` to find the matching build in Coder and the workspace's entry in the [audit log](../../admin/security/audit-logs.md).

For example, to follow one session with `jq`:

```shell
kubectl logs -n coder deploy/coder-agent-relay --since=1h \
  | jq -c 'select(.session_id == "<session-id>")'
```

### What Agent Relay logs don't include

Agent Relay logs its own control plane: queue handling, identity resolution, dispatch, build state, and cleanup.
It doesn't collect conversation transcripts, model output, or the provider CLI's output from inside the workspace.
At the default log level, it doesn't log secrets or work order credentials.

For a failed session, also check:

1. **Coder workspace build logs** for Terraform, provisioner, image, parameter, and startup failures.
1. **Workspace agent and module logs** for provider runner or worker failures after the workspace starts.
   Agent Relay deletes the workspace, and these logs with it, when the session ends.
   To keep them, ship them from the workspace to an external log service.
1. **The provider's admin views** for queue state that the provider doesn't return to Agent Relay.

## Status page

The optional status page lists each pool's health, workspaces as of the last reaper sweep, and recent failures.
For Claude Code it shows the 50 most recent nacks, and for Cursor the 50 most recent claim failures.
This history is in memory, so a restart clears it.
The same data is available as JSON at `/api/status`.

<div class="tabs">

### Kubernetes

Add this to `values.yaml`:

```yaml
statusUI:
  enable: true
  port: 2113
  auth: coder-token

tls:
  secretName: agent-relay-tls
```

Set `tls.secretName` to a `kubernetes.io/tls` secret for anything reachable beyond a port-forward.

### Docker

Add this to `config.yaml`:

```yaml
statusUIEnable: true
statusUIAddress: 0.0.0.0:2113
statusUIAuth: coder-token
tlsCertFile: /etc/agent-relay/tls/tls.crt
tlsKeyFile: /etc/agent-relay/tls/tls.key
```

Agent Relay rereads the certificate every minute, so a renewed certificate needs no restart.

</div>

With `coder-token` authentication, a reader signs in with a Coder [API token](../../admin/users/sessions-tokens.md) minted for this purpose.
Agent Relay rejects the token that the browser or CLI uses for sign-in.
By default, the token is revoked when sign-in succeeds, and the status page session lasts 24&nbsp;hours.
Agent Relay admits a reader who can read workspaces in every organization that the relay serves.

> [!WARNING]
> The status page shows session IDs, workspace names, and owner emails.
> Do not turn off authentication unless another control already restricts access to the port, or anyone who reaches it can read that data.

## Prometheus metrics

The Helm chart turns metrics on by default on port 2112 and adds the `prometheus.io/scrape`, `prometheus.io/port`, and `prometheus.io/path` pod annotations.
The metrics endpoint is plain HTTP and doubles as the readiness probe.
For Docker, set `CODER_AGENT_RELAY_PROMETHEUS_ENABLE=true` and `CODER_AGENT_RELAY_PROMETHEUS_ADDRESS=0.0.0.0:2112`.

Metrics are named `agent_relay_<subsystem>_<name>` and labeled by `pool`.
Shared dispatcher and reaper metrics also carry a `provider` label.

### Shared metrics

| Metric                                                    | What it tells you                                                                              |
|-----------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `agent_relay_dispatcher_dispatches_total{result}`         | Dispatch outcomes: `created`, `rebuilt`, `skipped`, `expired`, or `failed`.                    |
| `agent_relay_dispatcher_claim_to_workspace_built_seconds` | Time from claim to a successful build. Compare it with the pool's build budget or deadline.    |
| `agent_relay_reaper_workspaces{status}`                   | Workspaces by worker status, updated each sweep. Active sessions are `working` plus `serving`. |
| `agent_relay_reaper_verdicts_total{verdict}`              | Reaper decisions, such as `keep`, `delete`, and `failed`.                                      |

### Claude Code metrics

| Metric                                                                        | What it tells you                                                                                                                    |
|-------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `agent_relay_claude_code_work_orders_claimed_total`                           | Work orders Agent Relay claimed from Anthropic. If it stays flat while developers start sessions, sessions aren't reaching the pool. |
| `agent_relay_claude_code_work_orders_rejected_total{reason, client_platform}` | Work orders Agent Relay rejected, by reason and origin.                                                                              |

### Cursor metrics

| Metric                                                     | What it tells you                                                                                                                             |
|------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| `agent_relay_cursor_pending_requests`                      | Requests waiting in the pool's queue. A sustained rise means requests aren't being claimed.                                                   |
| `agent_relay_cursor_claims_total`                          | Requests Agent Relay claimed from Cursor.                                                                                                     |
| `agent_relay_cursor_skips_total{reason}`                   | Requests left unclaimed: `throttle`, or `degraded` when the pool's template failed validation.                                                |
| `agent_relay_cursor_claim_fails_total{retryable, applied}` | Claims that Agent Relay failed in Cursor. `applied="error"` means the call itself failed, so the chat waits until Cursor expires the request. |
| `agent_relay_cursor_requests_rejected_total{reason}`       | Requests that no active Coder user can own: `no_email`, `no_match`, `inactive`, or `dormant`.                                                 |
| `agent_relay_cursor_stream_reconnects_total`               | Reconnects of the event stream to Cursor. A steady climb usually points to a proxy or network problem.                                        |

Agent Relay doesn't expose a metric for successful Cursor polls yet.
Use `pending_requests`, `stream_reconnects_total`, and the status page to confirm that a Cursor pool is live.

### Suggested alerts

- Any increase in `dispatches_total{result="failed"}`.
- Any increase in `cursor_claim_fails_total{applied="error"}`, which leaves chats waiting.
- A sustained rise in rejected requests, which usually means developer emails don't match Coder accounts.
- `cursor_pending_requests` above zero for longer than the pool's `dispatch_deadline`.
- `claim_to_workspace_built_seconds` approaching the pool's build budget or deadline.

## Troubleshoot by symptom

| Symptom                                         | Where to look                                                                                                                                                                 |
|-------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A pool isn't serving                            | Startup logs for the pool. A pool logs that its template is verified when it's ready, and logs a contract check failure when the template is missing the module's parameters. |
| A session never creates a workspace             | Logs for the `session_id`. Look for identity resolution failures, a degraded pool, or a dispatch error.                                                                       |
| The workspace exists but the agent never starts | The workspace's build log, then the module logs in the workspace, then `agent_relay_status` on the status page.                                                               |
| A session ended but its workspace remains       | Reaper decisions in the logs for the `workspace_id`. Agent Relay deletes on a sweep interval, and doesn't delete stopped workspaces.                                          |

For provider-specific symptoms, refer to the troubleshooting tables for [Claude Code](./claude-code.md#troubleshoot), [Cursor](./cursor.md#troubleshoot), and [Slack](./slack.md#troubleshoot).

## Learn more

- [Set up Agent Relay](./setup.md)
- [Audit logs](../../admin/security/audit-logs.md)
- [Coder Prometheus metrics](../../admin/integrations/prometheus.md)
