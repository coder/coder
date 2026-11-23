---
title: Troubleshoot Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../reference/feature-stages.md#beta) and requires a Premium license.
> To try it, [request a Premium trial](https://coder.com/trial).

This page is for Coder administrators who diagnose Agent Relay problems for Claude Code, Cursor, and Slack.
Entries that apply to one provider say so.
To read logs and metrics, refer to [Monitor Agent Relay](./monitoring/index.md).

## Find where a failure happened

| Symptom                                         | Where to look                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
|-------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A pool isn't serving                            | `/readyz` or `agent_relay_pool_healthy`, then the startup logs for the pool. A ready pool logs `template verified, polling` for Claude Code or `pool verified, serving` for Cursor. A pool that fails a check, such as a template without the module's parameters, logs `pool failed verification, retrying`.                                                                                                                                                                                                                        |
| A session never creates a workspace             | Logs for the `session_id`, or the `session_uuid` for Claude Code. Look for identity resolution failures, a degraded pool, a dispatch error, or `job_error` on a failed build.                                                                                                                                                                                                                                                                                                                                                        |
| The workspace exists but the agent never starts | The workspace's build log, then the module logs in the workspace, then the `agent_relay_status` metadata on the workspace page in Coder.                                                                                                                                                                                                                                                                                                                                                                                             |
| A session ended but its workspace remains       | Reaper decisions in the logs for the `workspace_id`. Agent Relay deletes on the next [sweep](./setup.md#workspace-lifecycle), which can be 30&nbsp;seconds or more later, and doesn't delete stopped workspaces. With a [reap delay](./configuration.md#--reap-delay), the reaper has already decided to delete the workspace and keeps it until the delay passes, counted from when the latest build finished. While it waits, each sweep counts a `delayed` verdict, and debug logs show `workspace awaiting reap` with `reap_at`. |

Module logs are in the workspace, under `$HOME/.coder-modules/coder/agent-relay-claude-code/logs/` or `$HOME/.coder-modules/coder/agent-relay-cursor/logs/`.
They're deleted with the workspace.

## Agent Relay and pools

### Agent Relay exits at startup

An unlicensed deployment, an unsupported Coder version, an invalid token, or missing organization permissions.
A missing permission names the capability, such as `missing coderd capabilities [set-workspace-secrets[<org id>]]`.
Refer to [Create a service account](./setup.md#step-1-create-a-service-account).

Also check the pool configuration:

- Two pools with the same `name`, or a pool missing `organization` or `template`.
- An unset `secret_env` variable, or a reserved `template_parameters` name.
- An `idle_release_timeout` without a unit.
  Claude Code values must be whole minutes, and Cursor values must be whole seconds.
- A `dispatch_deadline` longer than `15m`.
  Claude Code values must also be whole seconds and at least `10s`.
- Cursor only: more than four pools on one API key, or two pools on one key with the same `cursor_pool_name`.

### One pool is degraded and the others are fine

The service account lacks `read` or `use` on the template, the template lacks `allow_user_cancel_workspace_jobs`, or the template doesn't include the module's parameters.
A pool with a pinned `template_version` also degrades when that version is deleted, archived, or renamed.
If the logs show `missing parameter "agent_relay_credential"`, Agent Relay is sending the credential as a build parameter, because Coder is older than 2.39 or Agent Relay hasn't restarted after you upgraded Coder.
Upgrade Coder, then restart Agent Relay.
After you push a fixed template version, the pool checks again within 30&nbsp;seconds and starts serving without a restart.

### A Claude Code pool stops claiming sessions

The environment key expired or was rotated.
`/readyz` reports the pool as `pool stopped until agent-relay restarts`, and `agent_relay_pool_healthy` drops to `0`.
Look for poll errors on that pool, update the secret, then restart Agent Relay with `kubectl rollout restart deploy/coder-agent-relay`, or restart the container.

## Builds and workspaces

### A workspace is created and then deleted

The build failed.
Quota exhaustion is the most common cause.
Find the `workspace build failed` line in the Agent Relay logs and read its `job_error` field.
The workspace's build log in Coder has the full output.

### Builds time out

- Claude Code: the build time plus queue time exceeded the pool's `dispatch_deadline`, `5m` by default.
  Speed up the template, raise the deadline up to `15m`, or add provisioners.
- Cursor: the build exceeded `dispatch_deadline` or waited for a provisioner.
  Speed up the template, raise the deadline up to `15m`, use prebuilds, or add provisioners.

### Builds stay `pending`

No online provisioner matches the template's tags.
Compare `coder provisioner list` with the template's tags.

### The agent fails to start with an unclear CLI error

The provider CLI in the image is out of date.
Rebuild the image, or set `install_cli = true`.

### A worker idles without its credential

A Claude Code runner shows `No work order in this build`, and a Cursor worker shows `No worker token in this build`.
Agent Relay delivers the credential only on the build that it dispatches, so a relay workspace that someone started again after a stop has none.
Delete it instead of starting it.
For Cursor, an unclaimed prebuild also shows this message, which is expected.

If every new relay workspace shows this message, Coder is dropping the credential, which happens when Coder was downgraded below 2.39 after Agent Relay started.
Restart Agent Relay so that it checks the Coder version again.

### A workspace is never deleted

The template renamed or omitted `agent_relay_status`, or the worker reports `orphaned`.

If the logs show `workspace delete keeps failing, leaving it for an operator`, three delete builds failed in a row, usually because of a Terraform destroy error.
Agent Relay stops retrying and counts the workspace as `stuck` in `agent_relay_reaper_verdicts_total`.
Fix the error in Coder, then delete the workspace.

### The agent runs commands before the workspace finishes setting up

Agent Relay hands off the workspace when its build succeeds, and the module starts the worker without waiting for startup scripts.
Order the module after your setup scripts, as described in [Start the worker after workspace setup](./setup.md#start-the-worker-after-workspace-setup).
With the rule in place, Agent Relay v0.16.0 or later notices a failed setup about a minute after the agent reports `start_error`, or `start_timeout` when a setup script reaches its `timeout`.
Cursor fails the request then, and Claude Code retries once on a new workspace before failing the session.
Claude Code skips the retry when the failure comes later than the pool's `dispatch_deadline` after the build was created.
In the relay logs, the reaper's `session did not finish cleanly, reporting and deleting` line carries `agent_lifecycle=start_error` with `reason="the workspace agent's startup scripts failed"`, or `agent_lifecycle=start_timeout` with `reason="the workspace agent's startup scripts timed out"`.
Only Claude Code sessions receive the reason in their error.
Cursor users see a fixed message that the worker stopped before serving the request.
On a claimed prebuild, the status item keeps reporting the prebuild's `idle` after a failed setup, so Agent Relay neither fails the session nor deletes the workspace.

## Claude Code sessions

### Sessions fail and no workspace is created

Identity resolution failed.
Find the attested email in the logs and confirm an active member of the pool's organization has it.

### A session sits queued in Claude Code

Dispatch failed and Agent Relay returned the session for redelivery.
Read the Agent Relay logs for the session's nack reason.

### Anthropic authentication fails

The Agent Relay or workspace clock is more than 5&nbsp;minutes off.
Synchronize clocks.

### A session stalls while preparing the repository

The Claude GitHub app can't access the selected repository.

### Git reports `Please tell me who you are`

The image has no Git identity, and runner-managed Git config is off.

## Cursor sessions

### Cursor shows an `agent-relay:` account error

Identity resolution failed.
The message names the cause: no email on the request, no matching member of the organization, or a suspended or dormant account.

### Cursor shows an `agent-relay:` build or worker error

The build failed, exceeded `dispatch_deadline`, or waited too long for a provisioner, or the worker exited before serving.
Read the Agent Relay logs and `agent_relay_cursor_claim_fails_total`.

### A Cursor chat hangs with no error

The worker stopped after it started serving, or Agent Relay's call to fail the claim errored, or no relay claimed the request.
If autostop stopped the workspace, add the session attachment metadata described in [Keep attached workspaces running](./setup.md#keep-attached-workspaces-running), or check the template's maximum deadline.

### The agent works on the wrong branch

The worker doesn't switch branches.
Add a `sessionStart` hook that checks out the requested ref.

## Slack

| Symptom                                              | Likely cause                                                                                                                                                         |
|------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Claude doesn't reply, or no `cc-` workspace appears  | The channel resolves to another environment, the thread started before the change, a Slack guest is in the channel, or the Tag pool isn't serving.                   |
| Claude Tag channel sessions are rejected             | The pool has no `claude_tag_channel_user`, the relay wasn't restarted after the change, or the value isn't `me` or the UUID of an active member of the organization. |
| A Tag pool workspace is owned by a person            | Someone selected the Tag environment in the environment picker, or a Slack DM landed on it.                                                                          |
| Claude can't reach a repository or service           | Claude Tag connections don't apply in self-hosted sessions, or the template's Tag user UUID doesn't match `claude_tag_channel_user`.                                 |
| Cursor shows an `agent-relay:` account error         | The person's Cursor email doesn't match an active member of the pool's organization, or a service account started the agent.                                         |
| A Cursor mention doesn't run on the Agent Relay pool | **Allow Self-Hosted Machines** is off, the message overrides the pool, no default pool applies, or the person's own default worker takes priority.                   |
| An agent loses its files after a quiet period        | Idle release deleted the workspace, and the next message rebuilt it on a clean disk.                                                                                 |

## Learn more

- [Set up Agent Relay](./setup.md)
- [Agent Relay for Claude Code](./claude-code.md)
- [Agent Relay for Cursor](./cursor.md)
- [Configure Slack for Agent Relay](./slack.md)
- [Monitor Agent Relay](./monitoring/index.md)
