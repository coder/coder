---
title: Configure Slack for Agent Relay
---

> [!NOTE]
> Agent Relay is in [beta](../../reference/feature-stages.md#beta).

This page is for Coder administrators who configure the Claude Code and Cursor Slack integrations to run agent sessions on Agent Relay pools.
Developers don't need to know that Agent Relay serves their Slack sessions.
Claude Code and Cursor handle Slack differently, and the difference decides who owns the workspace.

| Provider    | Slack surface                                       | Workspace owner                                      |
|-------------|-----------------------------------------------------|------------------------------------------------------|
| Claude Code | DM with Claude                                      | The person's matched Coder user (individual)         |
| Claude Code | `@claude` in a channel (Claude Tag channel session) | The pool's `claude_tag_channel_user` (shared)        |
| Cursor      | `@Cursor` in a channel or DM                        | The tagging person's matched Coder user (individual) |

Before you start, deploy Agent Relay with a pool for the provider.
Refer to [Set up Agent Relay](./setup.md).

## Claude Tag channel sessions

A [Claude Tag](https://claude.com/docs/claude-tag/overview) channel session starts when someone tags `@claude` in a Slack channel.
The session runs under your Anthropic organization's agent identity, not a person's account.
Anyone in the channel can steer the thread, so Agent Relay doesn't attribute the session to whoever tagged Claude.
Instead, the workspace belongs to the pool's `claude_tag_channel_user`.

A Slack DM with Claude isn't a channel session.
It runs on the person's own Claude account, and Agent Relay builds the workspace as that person's matched Coder user.

> [!WARNING]
> Anyone who can tag `@claude` in a channel that routes to the Tag environment acts with the credentials and access of the `claude_tag_channel_user`.
> This includes Slack users without a Claude account, unless you restrict who can invoke `@claude`.
> Give the `claude_tag_channel_user` only the access that the channel's work needs.

How channel sessions behave:

- Each channel where Claude works gets a channel session for top-level mentions.
  Larger tasks get their own thread session.
  Each active session has its own workspace, so two threads in one channel can run three workspaces at once.
- Claude Tag connections and Access bundles aren't available in self-hosted sessions yet.
  Credentials must come from the workspace template.
- Without a `claude_tag_channel_user`, Agent Relay rejects channel sessions and doesn't retry them.

### Set up Claude Tag channel sessions

> [!IMPORTANT]
> You need the Coder `owner` role and an Owner of your Anthropic organization.
> Claude Tag must already be connected to your Slack workspace.

1. Create a service account for channel sessions:

   ```shell
   coder users create --service-account --username <tag-user> --org <org>
   ```

1. Record the service account's UUID:

   ```shell
   coder users show <tag-user> -c id
   ```

1. On the claude.ai [Cloud environments](https://claude.ai/admin-settings/cloud-environments) page, create a self-hosted environment for channel sessions and copy its key.
   Give it an obvious name, such as `Slack Tag Channels`.

1. Store the key the same way as your other [environment keys](./claude-code.md#step-3-store-the-environment-key).

1. Create a template for channel sessions based on your Agent Relay template.

1. In the template, inject channel credentials only when the owner is the `claude_tag_channel_user`:

   ```tf
   locals {
     is_tag_session = data.coder_workspace_owner.me.id == "<tag-user-uuid>"
   }

   resource "coder_agent" "main" {
     # ...
     env = local.is_tag_session ? tomap({ GITHUB_TOKEN = var.tag_github_token }) : tomap({})
   }
   ```

   The Tag environment appears in every Claude environment picker, so people can select it for their own sessions.
   This check keeps their workspaces from receiving the channel credentials.

1. Plan [workspace quota](../../admin/users/quotas.md) for the `claude_tag_channel_user`.
   Every channel session's workspace belongs to this one user, so all channel sessions draw on a single quota budget.
   When the budget runs out, builds fail and Agent Relay deletes the workspaces.
   Leave `daily_cost` off the Tag template's resources so its workspaces cost nothing, or add the `claude_tag_channel_user` to a group whose quota allowance covers your expected concurrent sessions.

1. Add a pool for the Tag environment, with the service account's UUID as its `claude_tag_channel_user`:

   ```yaml
   pools:
     claudeCode:
       - name: slack-tag
         secret_env: SLACK_TAG_ENV_SECRET
         organization: <org>
         template: <tag-template>
         claude_tag_channel_user: "<tag-user-uuid>"
   ```

   For Docker, add the same entry under `claudeCodeRunnerPools` in `config.yaml`.

1. Upgrade the Helm release, or restart the container.

1. In claude.ai, open [Claude Tag admin settings](https://claude.ai/admin-settings/claude-tag) and select **Claude's access** > **Channels**.

1. Open the scope to pin: a channel, a Slack workspace, or **Slack** for your whole organization.

1. On the **Advanced** tab, under **Sessions**, select the Tag environment in **Environment**.

1. In a new thread in a pinned channel, tag `@claude` with a task that needs a workspace, such as creating a file.

1. In Coder, confirm that a workspace whose name starts with `cc-` is owned by the `claude_tag_channel_user`.

Claude uses the first environment it finds: the channel's, then the Slack workspace's, then the **Slack** page's, then your organization's default cloud environment.
Threads that are already underway keep their original environment, so test in a new thread.

Do not set the Tag environment as your organization's default cloud environment.
If you do, every channel without a pinned environment lands on it.

### Restrict who can invoke `@claude`

By default, anyone in the connected Slack workspace can tag `@claude`.
An Owner of your Anthropic organization can restrict this in [Claude Tag admin settings](https://claude.ai/admin-settings/claude-tag) under **Claude's access** > **Channels** > **Slack** > **Advanced** > **Access**:

| Plan       | Setting                                      | Who can use `@claude`                                            |
|------------|----------------------------------------------|------------------------------------------------------------------|
| Team       | **Restrict to your organization**            | Slack users with a Claude account in your organization           |
| Enterprise | **Restrict to roles with Claude Tag access** | Members whose role grants the **Claude Tag in Slack** capability |

The setting covers every channel and DM in the Slack workspace.
Restricting who can invoke `@claude` doesn't change workspace ownership.
Refer to [Anthropic's documentation on restricting Claude Tag](https://claude.com/docs/claude-tag/admins/restrict-access) for every related control.

In channels pinned to the Tag environment, keep the guest setting at **Restrict**.
Guests can read everything Claude posts, including output it produced with the Tag user's credentials.

### Channel session lifecycle

- Workspace names start with `cc-`, followed by the session's UUID.
- Idle release is off by default, so each session keeps its workspace until the session ends.
- Claude replaces a channel's session after about an hour without activity, when the channel's configuration changes, or when the session is about a day old.
  Each replacement gets a new workspace.
- Tagging `@claude !restart` replaces the thread's or channel's session with a new one in a new workspace.
- Deleting a thread's first message closes the session, and Agent Relay deletes its workspace.

To release idle sessions, set these runner variables on the template's `coder_agent` resource:

| Variable                                     | Default   | Effect                                                                                                        |
|----------------------------------------------|-----------|---------------------------------------------------------------------------------------------------------------|
| `SELF_HOSTED_RUNNER_SESSION_IDLE_MS`         | `0` (off) | Releases the session after this many milliseconds without user input. Agent Relay then deletes the workspace. |
| `SELF_HOSTED_RUNNER_MAX_LIFETIME_MS`         | `0` (off) | Limits each session's lifetime in milliseconds. Set it whenever you set idle release.                         |
| `SELF_HOSTED_RUNNER_PUSH_OUTCOME_ON_RELEASE` | Off       | Pushes tracked branches before release, so commits survive.                                                   |

After an idle release, the next message rebuilds the workspace with the same name on a fresh disk, so unpushed work is lost.

## Cursor in Slack

Tagging `@Cursor` in a Slack channel or DM starts a cloud agent in a new thread.
Agent Relay builds the workspace as the tagging person's matched Coder user, the same as an agent started from the Cursor client.

- Each agent gets its own workspace.
  Follow-ups in a thread reuse it while the worker is live.
- `@Cursor agent` in a thread starts a new agent with a new workspace.
- If a second person starts an agent in the same thread, that agent's workspace belongs to the second person.
- Workspace names start with `cur-`.
  Slack workspaces look the same in Coder as workspaces started from the Cursor client.

### Set up Cursor in Slack

> [!IMPORTANT]
> The Cursor Slack integration must already be connected to your Slack workspace, and **Allow Self-Hosted Machines** must be on.

1. To route mentions to Agent Relay by default for the whole team, have a Cursor team admin run `@Cursor pool set <cursor_pool_name>` in Slack.

1. To route one public channel instead, run `@Cursor pool set <cursor_pool_name> channel` in that channel.

1. In the Cursor Cloud Agents dashboard's security settings, set **Team follow-ups** to **Disabled**.

1. In a channel that routes to your pool, tag `@Cursor` with a task that needs a workspace.

1. In Coder, confirm that a workspace whose name starts with `cur-` is owned by your matched Coder user.

Cursor picks a pool in this order:

1. Options in the message: `pool=<name>`, `worker=` or `machine=`, or `sh=0` for Cursor's managed infrastructure.
1. The person's own default worker for the repository.
1. The channel's default pool.
1. The team's default pool.

Cursor applies channel defaults only in public channels.
Private channels and DMs need the team default or `pool=` in the message.
Any member of the Cursor team can change a public channel's default pool.

> [!WARNING]
> A follow-up runs in the original person's workspace, with their credentials and source control access.
> With **Team follow-ups** set to **All**, a teammate with less access can direct an agent that holds someone else's credentials.
> Refer to [Cursor's cloud agent settings](https://cursor.com/docs/cloud-agent/settings).

Unless **Require Self-Hosted Machines** is on, people can send a mention to Cursor's managed infrastructure with `sh=0`.

Whether the pool has a `repo_url` also changes how Cursor handles a mention:

| Pool              | Repository resolved                                                            | No repository resolved     |
|-------------------|--------------------------------------------------------------------------------|----------------------------|
| Repository-scoped | Runs if the pool serves that repository. Otherwise, Slack rejects the mention. | Slack rejects the mention. |
| Any repository    | Runs with the repository as context.                                           | Runs without a repository. |

### Cursor in Slack lifecycle

After each turn, the worker waits `idle_release_timeout` seconds, 600 by default, for a follow-up.
Then Agent Relay deletes the workspace.
A later follow-up rebuilds the workspace on a clean disk, so unpushed work is lost.

## Troubleshoot

| Symptom                                                 | Likely cause                                                                                                                                                         |
|---------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Claude doesn't reply, or no `cc-` workspace appears     | The channel resolves to another environment, the thread started before the change, a Slack guest is in the channel, or the Tag pool isn't serving.                   |
| Claude Tag channel sessions are rejected                | The pool has no `claude_tag_channel_user`, the relay wasn't restarted after the change, or the value isn't `me` or the UUID of an active member of the organization. |
| A Tag pool workspace is owned by a person               | Someone selected the Tag environment in the environment picker, or a Slack DM landed on it.                                                                          |
| Claude can't reach a repository or service              | Claude Tag connections don't apply in self-hosted sessions, or the template's Tag user UUID doesn't match `claude_tag_channel_user`.                                 |
| Cursor shows an `agent-relay:` account error            | The person's Cursor email doesn't match an active member of the pool's organization, or a service account started the agent.                                         |
| A Cursor mention doesn't run on the Agent Relay pool    | **Allow Self-Hosted Machines** is off, the message overrides the pool, no default pool applies, or the person's own default worker takes priority.                   |
| Slack privately replies that a Cursor mention can't run | A repository-scoped pool got a mention with no repository, or with a repository the pool doesn't serve.                                                              |
| An agent loses its files after a quiet period           | Idle release deleted the workspace, and the next message rebuilt it on a clean disk.                                                                                 |

## Learn more

- [Agent Relay for Claude Code](./claude-code.md)
- [Agent Relay for Cursor](./cursor.md)
- [Monitor Agent Relay](./monitoring.md)
