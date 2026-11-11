---
title: Send Agent Relay logs to a SIEM
---

> [!NOTE]
> Agent Relay is in [beta](../../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).
> Log messages and fields can change between beta releases.

This page is for teams that ingest Agent Relay logs into a security information and event management (SIEM) system or another log platform that reads JSON lines.
Agent Relay doesn't emit a separate event ID, so build searches and alerts on `msg`, `level`, `logger_names`, and values inside `fields`.

Set `logFormat: json` and ship the container's stderr to your SIEM with your usual log collector, as you would for [Coder service logs](../../admin/monitoring/connection-logs.md#service-logs).

## Log levels

Each `logLevel` setting includes every more severe level:

| `logLevel` | Records written                  |
|------------|----------------------------------|
| `debug`    | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `info`     | `INFO`, `WARN`, `ERROR`          |
| `warn`     | `WARN`, `ERROR`                  |
| `error`    | `ERROR`                          |

Use `info` for SIEM ingestion.
`debug` adds routine retries, poll backpressure, and license checks without adding alerting value.
For Claude Code, `debug` also writes `work order payload` records that contain the decoded work order, including emails and repository URLs.

## Record shape

Each JSON record has these top-level keys:

| Key            | Meaning                                                                                                                           |
|----------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `ts`           | Event time in UTC, as RFC 3339 with nanosecond precision                                                                          |
| `level`        | `DEBUG`, `INFO`, `WARN`, or `ERROR`                                                                                               |
| `msg`          | The event message                                                                                                                 |
| `caller`       | Source file and line of the log statement. Changes between builds.                                                                |
| `func`         | Go function that wrote the record. Changes between builds.                                                                        |
| `logger_names` | The Agent Relay service that wrote the record, such as `claude-code-runners`, `cursor-workers`, `reaper`, or `coderd` for startup |
| `fields`       | Event data, including the [correlation fields](./index.md#correlation-fields) and the fields below                                |

The reaper deletes finished and failed workspaces for both providers.
Its records carry `workspace_id` and `workspace_name`, but no `provider`, `pool`, or session ID.

## Provider-specific fields

Both providers log the [correlation fields](./index.md#correlation-fields).
Each provider also logs identity fields and aliases that use its own API's names.
An alias always carries the same value as the field it mirrors.

| Provider    | Field              | Meaning                                                                                                       |
|-------------|--------------------|---------------------------------------------------------------------------------------------------------------|
| Claude Code | `session_uuid`     | Alias of `session_id`. Nack records written after a runner failure carry only `session_uuid`.                 |
| Claude Code | `jti`              | Alias of `delivery_id`, the work order's JWT ID                                                               |
| Claude Code | `attempt`          | Anthropic's delivery counter for the session, as a string such as `"0"`. Increases on each redelivery.        |
| Claude Code | `spawn_reason`     | `session_created` for a new session, or `runner_lost` when the runner for an existing session went away       |
| Claude Code | `account_id`       | Anthropic account ID (`user_...`) of the user who started the session. Empty for Claude Tag channel sessions. |
| Claude Code | `account_email`    | Anthropic-attested email used to find the Coder user. Empty for Claude Tag channel sessions.                  |
| Claude Code | `agent_id`         | Claude Tag agent ID (`cagt_...`) that started the session. Empty for most user sessions.                      |
| Claude Code | `org_service_name` | Anthropic service that started an agent session, such as `anthropic:slackbot`. Empty for user sessions.       |
| Claude Code | `username`         | Alias of `owner`                                                                                              |
| Claude Code | `user_id`          | Alias of `owner_id`, the Coder user UUID                                                                      |
| Cursor      | `request_id`       | Alias of `session_id`, Cursor's pending request ID (`bc-...`)                                                 |
| Cursor      | `worker_id`        | Alias of `delivery_id`, the `cw_...` worker ID that Agent Relay sends to Cursor with the claim                |
| Cursor      | `provider_user_id` | Cursor's numeric user ID, logged before Agent Relay finds the Coder user                                      |
| Cursor      | `user_id`          | Alias of `provider_user_id`. Unlike Claude Code's `user_id`, this is a Cursor ID, not a Coder user UUID.      |
| Cursor      | `user_email`       | The requester's email, on records for requests that Agent Relay refuses                                       |

Because `user_id` means different things for each provider, use `owner_id` for the Coder user in reports that cover both.

Startup records also echo pool configuration, such as `organization`, `organization_id`, and `template`.
Claude Code adds `claude_tag_channel_user`, and Cursor adds `cursor_pool`, `relist_interval`, and `relist_jitter`.

## Failure and rebuild fields

| Field            | Meaning                                                                                                                                             |
|------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| `error`          | Error detail on most `WARN` and `ERROR` records, such as a failed claim, identity lookup, build, or delete                                          |
| `retryable`      | `true` when the provider retries or the user can try again. `false` when an administrator must fix configuration or identity.                       |
| `message`        | Cursor: the exact text shown to the user when Agent Relay fails the claim                                                                           |
| `detail`         | Cursor: the local reason for a failed claim. It's never sent to Cursor and is truncated at 512 characters.                                          |
| `stage`          | Cursor: the step where the claim failed, such as `owner` or `building`                                                                              |
| `reason`         | Reaper: why it deleted the workspace, such as `the runner exited with status 1`. Cursor refusals: `no_email`, `no_match`, `inactive`, or `dormant`. |
| `job_status`     | The final provisioner job status of a failed build, such as `failed` or `canceled`                                                                  |
| `job_error`      | Coder's error summary for a failed build                                                                                                            |
| `build_status`   | The latest build's status when Agent Relay rebuilds a workspace or the reaper evaluates it                                                          |
| `deadline`       | Claude Code: when Anthropic stops waiting for a runner. Logged when the deadline passes before dispatch.                                            |
| `status`         | Reaper: the runner status the workspace reported, such as `working`, `done 1`, `failed <reason>`, or `orphaned`                                     |
| `age`            | Reaper: time elapsed after the workspace's latest build was created                                                                                 |
| `failed_deletes` | Reaper: how many delete builds have failed in a row for the workspace                                                                               |

Claude Code nack records don't repeat the failure reason.
Find it in the `error` or `reason` field of the `WARN` record written just before the nack.

## Example records

A Claude Code workspace build starting:

```json
{
  "ts": "2026-10-14T16:02:11.912540118Z",
  "level": "INFO",
  "msg": "workspace build created",
  "caller": "/src/internal/workspacedispatcher/dispatcher.go:344",
  "func": "github.com/coder/agent-relay/internal/workspacedispatcher.(*Dispatcher).createWorkspace",
  "logger_names": ["claude-code-runners"],
  "fields": {
    "provider": "claude_code",
    "pool": "prod",
    "session_id": "00000000-1111-4222-8333-444444444444",
    "session_uuid": "00000000-1111-4222-8333-444444444444",
    "delivery_id": "77777777-6666-4555-8444-333333333333",
    "jti": "77777777-6666-4555-8444-333333333333",
    "attempt": "0",
    "client_platform": "web_claude_ai",
    "spawn_reason": "session_created",
    "account_id": "user_01AAAAAAAAAAAAAAAAAAAAAAAA",
    "account_email": "jdoe@example.com",
    "agent_id": "",
    "org_service_name": "",
    "owner": "jdoe",
    "owner_id": "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
    "username": "jdoe",
    "user_id": "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
    "workspace_id": "99999999-8888-4777-8666-555555555555",
    "workspace_name": "cc-00000000-1111-4222-8333-44444"
  }
}
```

A Cursor claim that failed after a build error. The user sees `message` in Cursor:

```json
{
  "ts": "2026-10-14T16:04:37.391402118Z",
  "level": "WARN",
  "msg": "failed the claim upstream, the user sees the message",
  "caller": "/src/internal/services/servicecursorworkers/fail.go:68",
  "func": "github.com/coder/agent-relay/internal/services/servicecursorworkers.(*Service).failClaim",
  "logger_names": ["cursor-workers"],
  "fields": {
    "provider": "cursor",
    "pool": "prod",
    "session_id": "bc-00000000-1111-4222-8333-444444444444",
    "request_id": "bc-00000000-1111-4222-8333-444444444444",
    "provider_user_id": 4242,
    "user_id": 4242,
    "delivery_id": "cw_77777777-6666-4555-8444-333333333333",
    "worker_id": "cw_77777777-6666-4555-8444-333333333333",
    "owner": "jdoe",
    "owner_id": "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
    "stage": "building",
    "retryable": true,
    "message": "agent-relay: workspace build failed. Try again in a few minutes.",
    "detail": "workspace cur-bc-00000000-1111-4222-8333-4 job failed: terraform apply: exit status 1"
  }
}
```

ID prefixes show where a value comes from:

| Prefix       | Meaning                                                                                        |
|--------------|------------------------------------------------------------------------------------------------|
| `cc-`        | Claude Code workspace name: `cc-` and the session UUID, truncated to 32 characters             |
| `cur-`       | Cursor workspace name: `cur-` and the lowercased Cursor request ID, truncated to 32 characters |
| `bc-`        | Cursor request ID. Share it with Cursor support.                                               |
| `cw_`        | Cursor worker ID that Agent Relay created for a claim                                          |
| `user_`      | Anthropic account ID. Share it with Anthropic support.                                         |
| `cagt_`      | Claude Tag agent ID, on sessions started by Claude Tag in Slack                                |
| `anthropic:` | Anthropic service name. Values that start with `anthropic:slack` mark Claude Tag sessions.     |

## Parse records

- Ingest stderr as one JSON object per line.
  Route lines that aren't JSON to a fallback index instead of dropping them, because a plain `agent-relay: ...` startup failure might be the only sign that the service never started.
- Use `ts` as the event time, not the ingestion time.
- Flatten `fields`, and map IDs and UUIDs as keyword or string types.
- Set field types explicitly:
  - Claude Code `attempt` is a string, not a number.
  - Cursor `provider_user_id` and `user_id` are numbers.
  - `retryable` is a boolean, and `expected_spawn_seconds` is a number.
  - `deadline` is an RFC 3339 timestamp.
  - Durations such as `interval`, `retry_interval`, `relist_interval`, and `age` are Go duration strings, such as `5s` or `5m0s`.
- Treat empty strings as missing.
  On Claude Code records, `account_id`, `account_email`, `agent_id`, and `org_service_name` are present but empty when they don't apply.
- Allow duplicate keys.
  Records written after Agent Relay creates a workspace can contain `owner` twice, and Claude Code nack records can contain `attempt` twice.
  Correlate users on `owner_id`, not `owner`.

## Scope records to a provider

- Select a provider's records by `logger_names`: `claude-code-runners` or `cursor-workers`.
  Some records carry no `provider` field, such as `no claude code runner pools configured, claude provider disabled`, so a filter on `fields.provider` alone misses them.
- Also collect `reaper` records, and attribute them by `workspace_name` prefix: `cc-` for Claude Code and `cur-` for Cursor.
  Join them to a session on `workspace_id`.
- Add `fields.pool` to the filter when one Agent Relay deployment serves more than one pool.

To measure user impact, count distinct session IDs, not records: `session_uuid` for Claude Code and `session_id` for Cursor.
A retried session gets a new `delivery_id` under the same session ID, so one session can fail more than once.

## Match rules

- Match on `msg` and `level`.
  Do not match on `caller` or `func`, because file paths, line numbers, and function names change between builds.
- `msg` text can change between Agent Relay releases.
  Recheck saved searches and alert rules after each upgrade.
- Alert on specific `msg` values, not on every `WARN` record.
  Several `WARN` messages describe automatic recovery, such as `poll failed, retrying next interval`, `identity resolution failed, nacking retryable`, and `claim failed, retrying next pass`.

## Log-based alerts

These alerts complement the [suggested Prometheus alerts](./index.md#suggested-alerts).
Some conditions, such as a stranded Cursor claim, appear only in logs.

| Priority            | `msg`                                                                                                                                      | Provider    | Group by                     | Why                                                                                                                                                          |
|---------------------|--------------------------------------------------------------------------------------------------------------------------------------------|-------------|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Page                | `fatal poll error, stopping this pool until agent-relay restarts with fixed config` (`ERROR`)                                              | Claude Code | `pool`                       | The pool stops taking sessions until Agent Relay restarts with fixed configuration, often after an environment key expires or is rotated.                    |
| Page                | `fatal listing error, degrading the pool until verification passes` (`ERROR`)                                                              | Cursor      | `pool`                       | The pool stops claiming requests until verification passes.                                                                                                  |
| Page                | `fail claim request failed, claim stranded until Cursor-side expiry`                                                                       | Cursor      | `pool`                       | The user's chat waits with no error until Cursor expires the claim. Only the logs record this.                                                               |
| Page                | `nack failed, relying on TTL redelivery` or `nack not applied, session will keep redelivering`                                             | Claude Code | `pool`                       | Anthropic didn't record the failure, so the session waits out its spawn budget or keeps redelivering.                                                        |
| Page when sustained | `pool failed verification, retrying`                                                                                                       | Both        | `pool`, `template`           | A bad API key or environment key, a template without the Agent Relay module, or an organization problem. Alert after several records in a row.               |
| Rate threshold      | `nacked spawn hint`                                                                                                                        | Claude Code | `pool`, `retryable`          | Sessions handed back to Anthropic. `retryable: false` ends the session and needs an administrator.                                                           |
| Rate threshold      | `failed the claim upstream, the user sees the message`                                                                                     | Cursor      | `pool`, `retryable`, `stage` | Users saw a failure. `retryable: false` needs an administrator. `retryable: true` usually means a transient or capacity problem.                             |
| Rate threshold      | `workspace build failed, reporting retryable and deleting`                                                                                 | Both        | `job_error`                  | A template or infrastructure failure, such as exhausted quota. Agent Relay deletes the workspace right away, so `job_error` might be the only record of why. |
| Rate threshold      | `workspace build passed the session deadline, reporting retryable and deleting`                                                            | Both        | `pool`                       | Builds take longer than the pool's `dispatch_deadline`.                                                                                                      |
| Rate threshold      | `session did not finish cleanly, reporting and deleting` (`reaper`)                                                                        | Both        | `reason`                     | Runners that exited non-zero, couldn't start, stopped unexpectedly, or never started.                                                                        |
| Ticket              | `session owner is unresolvable, nacking non-retryable`                                                                                     | Claude Code | `org_service_name`           | A Claude user has no active Coder account in the pool's organization, or a Claude Tag channel session arrived without `claude_tag_channel_user`.             |
| Ticket              | `request owner cannot be served, refusing the request`                                                                                     | Cursor      | `reason`                     | A Cursor request has no email, no matching Coder account in the organization, or a suspended or dormant account.                                             |
| Ticket              | `work order failed verification, nacking non-retryable`                                                                                    | Claude Code | `pool`                       | Agent Relay rejected the Anthropic-signed work order. `error` gives the reason.                                                                              |
| Ticket              | `workspace delete keeps failing, leaving it for an operator` (`reaper`)                                                                    | Both        | `workspace_id`               | The reaper stopped retrying after repeated delete failures. Clean up the workspace by hand.                                                                  |
| Ticket              | `delete workspace failed, leaving for reconciliation` or `delete workspace failed, retrying next sweep`                                    | Both        | `workspace_id`               | A workspace might still be running and using resources.                                                                                                      |
| Dashboard           | `resolved session owner`, `claimed pending request`, `workspace build created`, `workspace built, runner handoff`, or `deleting workspace` | Both        | `pool`, `client_platform`    | The normal session lifecycle, for volume and funnel views.                                                                                                   |

## Retention and privacy

- Keep logs for as long as your audit requirements need.
  Agent Relay doesn't keep session history, so your log platform is the record of who ran which session and why it failed.
- Use Prometheus metrics for rates and latency, and the SIEM for ordered event history, investigations, and the alerts above.
- Records can contain Coder usernames and user IDs, Anthropic account IDs, and Cursor user IDs.
  Claude Code records carry `account_email` on every record after work order verification, and Cursor refusal records carry `user_email`.
- Emails also appear inside `error`, `detail`, and `job_error` strings, such as identity warnings.
  Mask these fields, or restrict them with role-based access, along with the other user-identifying fields.

## Learn more

- [Monitor Agent Relay](./index.md)
- [Troubleshoot Agent Relay](../troubleshooting.md)
- [Agent Relay configuration reference](../configuration.md)
