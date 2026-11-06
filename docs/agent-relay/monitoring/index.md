---
title: Monitor Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).
> Metric names, labels, and log fields can change between beta releases.

This page is for operators who run Agent Relay and need to answer how many sessions ran, who ran them, and which failed and why.
Use Agent Relay's logs and Prometheus metrics, ingested into your own observability stack:

- **Logs** answer "what happened, in order?"
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
When you parse logs, skip lines that aren't JSON instead of failing on them, as the `jq` example below does.

### Correlation fields

Each JSON log line has `ts`, `level`, `msg`, and a `fields` object.
The correlation fields are inside `fields`, so a log query reads them as `fields.session_id`, for example.

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

For Claude Code, some log lines carry the session ID only as `session_uuid`, such as the lines that the reaper writes when it fails a session.
To trace a Claude Code session, match both `session_id` and `session_uuid`.

For Claude Code, `client_platform` records where the session started: `web_claude_ai`, `desktop_app`, `ios`, `android`, `claude_code_cli`, `claude_in_slack`, or `unknown`.
Cursor doesn't report a client platform.

To trace a session, start with `provider`, `pool`, and `session_id`.
After Agent Relay creates the workspace, use `workspace_id` to find the matching build in Coder and the workspace's entry in the [audit log](../../admin/security/audit-logs.md).

For example, to follow one session with `jq`:

```shell
kubectl logs -n coder deploy/coder-agent-relay --since=1h \
  | jq -Rc --arg id '<session-id>' '
      fromjson?
      | select(.fields.session_id == $id or .fields.session_uuid == $id)'
```

The `-R` flag and `fromjson?` skip lines that aren't JSON, such as startup failures.

### What Agent Relay logs don't include

Agent Relay logs its own control plane: queue handling, identity resolution, dispatch, build state, and cleanup.
It doesn't collect conversation transcripts, model output, or the provider CLI's output from inside the workspace.
At the default log level, it doesn't log secrets or work order credentials.

For a failed session, also check:

1. **The `job_error` field** on the log line that starts with `workspace build failed`, for the build's error message.
   Agent Relay deletes a workspace whose build fails, so start here.
1. **Coder workspace build logs** for Terraform, provisioner, image, parameter, and startup failures.
1. **Workspace agent and module logs** for provider runner or worker failures after the workspace starts.
   Agent Relay deletes the workspace, and these logs with it, when the session ends.
   To keep them, ship them from the workspace to an external log service.
1. **The provider's admin views** for queue state that the provider doesn't return to Agent Relay.

### Send logs to a SIEM

To ingest Agent Relay logs into a security information and event management (SIEM) system or another log platform, refer to [Send Agent Relay logs to a SIEM](./siem.md).
It covers the record shape, every field, parsing and scoping rules, and log-based alerts.

## Prometheus metrics

The Helm chart turns metrics on by default on port 2112 and adds the `prometheus.io/scrape`, `prometheus.io/port`, and `prometheus.io/path` pod annotations.
The metrics listener is plain HTTP and also serves `/readyz`, which the Helm chart uses as the readiness probe.
`/readyz` returns `200` once at least one pool has passed verification, and lists any pool that is still verifying or has failed.
For Docker, set `CODER_AGENT_RELAY_PROMETHEUS_ENABLE=true` and `CODER_AGENT_RELAY_PROMETHEUS_ADDRESS=0.0.0.0:2112`.

Metrics are named `agent_relay_<subsystem>_<name>` and labeled by `pool`.
Shared dispatcher, reaper, and pool health metrics also carry a `provider` label.

### Shared metrics

| Metric                                                    | What it tells you                                                                                                                                            |
|-----------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `agent_relay_dispatcher_dispatches_total{result}`         | Dispatch outcomes: `created`, `rebuilt`, `skipped_serving`, `skipped_deleting`, `expired`, or `failed`.                                                      |
| `agent_relay_dispatcher_claim_to_workspace_built_seconds` | Time from claim to a successful build. Compare it with the pool's build budget or deadline.                                                                  |
| `agent_relay_reaper_workspaces{status}`                   | Workspaces by worker status, updated each sweep. Active sessions are `working` plus `serving`.                                                               |
| `agent_relay_reaper_verdicts_total{verdict}`              | Reaper decisions, such as `keep`, `delete`, `failed`, and `stuck`. `stuck` means three delete builds failed in a row and the workspace needs manual cleanup. |
| `agent_relay_pool_healthy`                                | `1` if the pool's last verification passed, `0` if it failed or hasn't run yet.                                                                              |

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
Use `agent_relay_cursor_pending_requests` and `agent_relay_cursor_stream_reconnects_total` to confirm that a Cursor pool is live.

### Suggested alerts

- `agent_relay_pool_healthy` at `0` for more than a few minutes.
  A Claude Code pool that stops because Anthropic rejected its environment key stays down until Agent Relay restarts.
- Any increase in `agent_relay_dispatcher_dispatches_total{result="failed"}`.
- Any increase in `agent_relay_reaper_verdicts_total{verdict="stuck"}`, which leaves a workspace for you to clean up.
- Any increase in `agent_relay_cursor_claim_fails_total{applied="error"}`, which leaves chats waiting.
- A sustained rise in rejected requests, which usually means developer emails don't match Coder accounts.
- `agent_relay_cursor_pending_requests` above zero for longer than the pool's `dispatch_deadline`.
- `agent_relay_dispatcher_claim_to_workspace_built_seconds` approaching the pool's build budget or deadline.

## Troubleshoot by symptom

To find where a failure happened and how to fix it, refer to [Troubleshoot Agent Relay](../troubleshooting.md).

## Learn more

- [Set up Agent Relay](../setup.md)
- [Send Agent Relay logs to a SIEM](./siem.md)
- [Agent Relay configuration reference](../configuration.md)
- [Audit logs](../../admin/security/audit-logs.md)
- [Coder Prometheus metrics](../../admin/integrations/prometheus.md)
