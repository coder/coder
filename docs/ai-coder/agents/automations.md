---
title: Automations
---

> [!NOTE]
> This feature is experimental. Pin a release before broad rollout and review the release notes before upgrading.

An automation sends a saved prompt to a Coder Agents chat when a trigger fires.
This page is for Coder Agents users who create and run automations.
An administrator must turn on the feature first, as described in [Enable automations](#enable-automations).

Each automation has one trigger and one target:

- A webhook trigger sends the prompt when an external system posts an event to the automation's endpoint.
- A schedule trigger sends the prompt on a cron schedule.
- An existing chat target sends the prompt to one chat you own.
- A new chat target starts a new chat for every run.

The automation sends the message as its owner, with the owner's permissions.
An automation only submits messages.
The chat's agent, tools, and workspace do the work, the same way they handle a message you type.

## Enable automations

Automations are behind the `chat-automations` experiment.
To turn it on for every user at startup, run `coder server` with the flag:

```sh
coder server --experiments=chat-automations
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=chat-automations
```

### Turn automations on or off at runtime

`chat-automations` is a user-scoped experiment, so an Owner can change it at runtime with an experiment rule, without a restart.
A rule turns the experiment on for every user, off for every user, or on for the users who match a condition.
Refer to [Target experiments at runtime](../../reference/feature-stages.md#target-experiments-at-runtime) for the rule modes and condition syntax.

To turn automations on for every user:

```sh
coder exp experiment-rules on chat-automations
```

The command prints the new rule, for example `Experiment "chat-automations" rule is now on (revision 1).`

To turn automations off for every user, even if the startup `--experiments` list enables them:

```sh
coder exp experiment-rules off chat-automations
```

The command prints the new rule, for example `Experiment "chat-automations" rule is now off (revision 2).`
Use `off` to stop automations for every user.
`coder exp experiment-rules reset chat-automations` restores the startup default, which can leave automations on for every user.

### What happens when automations are off

When the experiment is off for a user:

- The **Automations** entry is hidden from the Agents sidebar, and the management API returns `404` for that user.
- Webhook deliveries to automations that user owns return `404` when the secret is valid.
  A wrong secret still returns `401`.
- Schedules that user owns don't run.
  Coder checks for due schedules every 30&nbsp;seconds and accepts an occurrence up to 60&nbsp;seconds late.
  An occurrence that came due while the experiment was off can run once, late, if the experiment is back on at a check inside that window.
  This recovery is best effort: lock or hook delays can still push the occurrence past the window.
  Otherwise the occurrence counts as missed and the schedule continues from its next time.
  To avoid depending on this recovery, turn the experiment back on before the occurrence is due.
- Agents in that user's chats aren't offered the `manage_automations` tool, and turning on **Manage automations** for a chat fails.
- The chat list's `automation_id` filter returns `400` for that user.
- Messages that automations already queued in a chat stay queued and still run.
  To remove them, turn off or delete the automation before you turn the experiment off, or select **Remove from queue** on each message in the chat.

## Who can manage automations

The **Coder Agents User** (`agents-access`) organization role lets members create, read, update, and delete their own automations in that organization.
Refer to [Getting started](./getting-started.md) for how members get this role.
Users with the Owner role can read every automation, and organization admins can read every automation in their organization.

Only the automation's owner can edit it, turn it back on, rotate its secret, or run it.
Other users who can update an automation, such as an organization admin, can only turn it off.
Users with delete permission can delete it.
On the **Automations** page, only the owner sees the delete button on the automation's row.
Deleting an automation removes the messages it queued that haven't started yet.

## Create a webhook automation

You can create an automation on the **Automations** page or with the API.

### Create it in the dashboard

1. On the **Agents** page, select **Automations** in the sidebar.
1. If you belong to several organizations, select the organization next to **Create automation**.
1. Select **Create automation**.
1. Enter a **Name**.
1. Enter a **Prompt**.
1. Under **Trigger**, select **Webhook**.
1. Under **Use**, select **Multi-use** or **Single-use**.
1. Under **Runs in**, select **Existing chat** or **New chat each run**.
1. For an existing chat, select a **Chat**.
1. For an existing chat, select a **When busy** option.
1. For a new chat, select a **Model**.
1. For a new chat, optionally select a **Project**.
1. Select **Save**.

The **Copy the webhook secret** dialog shows the **Publish endpoint**, the **Secret**, and an **Example request**.
Copy the secret before you select **Done**.
Coder shows the secret only once.

### Create it with the API

Send a `POST` request to `/api/experimental/organizations/{organization}/chat-automations`, where `{organization}` is the organization name or ID:

```sh
curl -X POST "$CODER_URL/api/experimental/organizations/$ORGANIZATION/chat-automations" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  --data @- <<EOF
{
  "name": "Deploy failures",
  "kind": "webhook",
  "webhook_use": "multi",
  "target_mode": "existing_chat",
  "target_chat_id": "$CHAT_ID",
  "when_busy": "queue",
  "prompt": "Investigate the failed deployment and summarize the cause."
}
EOF
```

The request takes these fields:

| Field                      | Values                                                        | Notes                                                                            |
|----------------------------|---------------------------------------------------------------|----------------------------------------------------------------------------------|
| `name`                     | 1 to 128 characters                                           | Required.                                                                        |
| `prompt`                   | Text                                                          | Required.                                                                        |
| `kind`                     | `webhook` or `schedule`                                       | Required. Can't change after creation.                                           |
| `webhook_use`              | `single` or `multi`                                           | Webhooks only. Defaults to `multi`. Can't change after creation.                 |
| `schedule_cron`            | Five-field cron expression                                    | Schedules only. Required for schedules.                                          |
| `schedule_time_zone`       | IANA time zone name                                           | Schedules only. Required for schedules.                                          |
| `target_mode`              | `existing_chat` or `new_chat`                                 | Required. Can't change after creation.                                           |
| `target_chat_id`           | Chat ID                                                       | Required for `existing_chat`.                                                    |
| `when_busy`                | `queue` or `skip`                                             | `existing_chat` only. Defaults to `queue` for webhooks and `skip` for schedules. |
| `new_chat_model_config_id` | Model configuration ID                                        | Required for `new_chat`.                                                         |
| `reasoning_effort`         | `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max` | `new_chat` only. Optional. Must be one the model offers in `reasoning_efforts`.  |
| `project_id`               | Project ID                                                    | `new_chat` only. Optional. Requires the `chat-projects` experiment.              |
| `enabled`                  | `true` or `false`                                             | Optional. Defaults to `true`. `false` creates the automation turned off.         |

Send `"enabled": false` to save an automation turned off in one request: nothing runs until you turn it on.
A `reasoning_effort` that the model doesn't list in its `reasoning_efforts` returns `400` naming `reasoning_effort`, on create and on any update that changes the model or the effort.
A model without a `reasoning_efforts` list takes any effort.

The response returns `201` with the automation and a `webhook_secret` field.
The secret starts with `coder_automation_`.
Coder returns it only in this response and when you rotate it, and stores only a SHA-256 hash of it.

The response doesn't contain the endpoint URL.
Build it from the automation ID: `/api/experimental/chat-automations/{automation}/events`.

Other management requests use the same base path:

| Request                                                                               | Purpose                                                              |
|---------------------------------------------------------------------------------------|----------------------------------------------------------------------|
| `GET /api/experimental/organizations/{organization}/chat-automations`                 | List the automations you can read.                                   |
| `GET /api/experimental/organizations/{organization}/chat-automations/{automation}`    | Get one automation.                                                  |
| `PATCH /api/experimental/organizations/{organization}/chat-automations/{automation}`  | Change fields. Send `{"enabled": false}` to turn the automation off. |
| `DELETE /api/experimental/organizations/{organization}/chat-automations/{automation}` | Delete the automation.                                               |

Changing the prompt doesn't change messages that the automation already queued: they keep the prompt they were sent with.
To remove those messages, turn the automation off or delete it.

The kind, target mode, and webhook use of an automation can't change.
A `PATCH` that sets one of them, or a field that doesn't exist, returns `400` and names the field in `validations`.
A `PATCH` that changes nothing leaves `updated_at` as it was.

### Read why an automation can't run

List and get responses include `paused_reasons` when a condition stops an automation from running while it's turned on.
Coder refuses its scheduled runs, **Run now**, and webhook deliveries until the condition clears.
The Automations page shows a **Paused** badge with the reason, and **Run now** explains why it can't run.

| Value                 | Meaning                                                                                                                                       |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| `owner_inactive`      | The owner is suspended, dormant, or deleted.                                                                                                  |
| `experiment_disabled` | The `chat-automations` experiment is off for the owner.                                                                                       |
| `target_unavailable`  | The target chat is deleted or archived.                                                                                                       |
| `model_unavailable`   | The model of a new-chat automation is deleted, turned off, or not available to the owner. Coder doesn't check it while the owner is inactive. |
| `owner_not_member`    | The owner is no longer a member of the automation's organization. Coder doesn't report it while the owner is inactive.                        |

The field is a snapshot, not a guarantee.
A response without `paused_reasons` can still be refused, for example when the target chat is busy or its queue is full.

### Read the outcome of the last run

List and get responses include `last_run`, the outcome of the latest schedule occurrence, **Run now**, or webhook delivery.
It's `null` until the first run, and each run replaces it: it's not a run history.

```json
{
  "last_run": {
    "at": "2026-10-12T09:00:01Z",
    "outcome": "skipped",
    "reason": "chat_busy",
    "chat_id": "4d6a3b2e-0f6c-4c39-9a8e-7a1f2d3c4b5a"
  }
}
```

| Field     | Meaning                                                                                                                                                                        |
|-----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `at`      | When Coder decided the run.                                                                                                                                                    |
| `outcome` | `accepted`: the prompt was sent or queued. `skipped`: the target chat was busy or full, the occurrence was missed, or an earlier run is still active. `refused`: see `reason`. |
| `reason`  | Missing for accepted runs. One of the reasons below.                                                                                                                           |
| `chat_id` | The chat the run went to, or was meant for when Coder knew the target chat. Missing when a new-chat run was refused. The chat can be deleted since.                            |

Skipped runs have the reason `chat_busy`, `queue_full`, `missed`, or `previous_run_active`.
For `previous_run_active`, `chat_id` is the earlier run's chat that is still active.
Refused runs have the reason `hook_denied`, `owner_inactive`, `forbidden`, `model_unavailable`, `target_unavailable`, `experiment_disabled`, `disabled`, `single_use_consumed`, `body_too_large`, `invalid_body`, `no_default_model`, or `error`.

Coder records webhook deliveries only after the secret matches, so requests with a wrong secret never change `last_run`.
Coder doesn't start scheduled runs while the automation has `paused_reasons`, so those occurrences don't change `last_run` either.

### Read the referenced chats and owner

Responses also include `target_chat` and `created_by_chat` with the `id` and `title` of the target chat and of the chat that created the automation.
They're included only when you can read that chat, so they're missing for deleted chats and for chats you can't read.
Responses include `owner` with the `id`, `username`, `name`, and `avatar_url` of the automation owner when you can read that user's membership in the organization.

## Send an event to a webhook

Send a `POST` request to the endpoint with the secret as a bearer token and a JSON body:

```sh
curl -X POST "$CODER_URL/api/experimental/chat-automations/$AUTOMATION_ID/events" \
  -H "Authorization: Bearer $WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"event":"deploy","status":"failed"}'
```

The endpoint path carries the `/api/experimental` prefix.
Experimental API paths can change in a later release, so keep the URL configurable in your senders.

The body must be valid JSON of at most 64&nbsp;KiB by default.
An administrator can change the limit with `--chat-automation-webhook-max-body-bytes`, up to 256&nbsp;KiB.
The event data must also fit the limit after escaping.
Coder escapes each `<`, `>`, `&`, U+2028, and U+2029 in a JSON string as a six-byte `\u` sequence, so a body with many of these characters can exceed the limit.
Any JSON value is accepted.
A successful delivery returns `202` with the ID of the saved input and the chat that received it:

```json
{
  "input_id": "8a6f2c3e-5b1d-4f0a-9c47-2e1d6b3a9f10",
  "chat_id": "3c2b1a90-7d6e-4f5a-8b9c-0d1e2f3a4b5c"
}
```

A `202` means Coder saved the message.
It doesn't mean the agent has run the turn yet.

The chat receives the saved prompt, then a separate text part that wraps the body as untrusted event data.
An **Automation run** card above the message shows the automation name and its trigger kind, for example `Deploy hook (webhook)`.
Anyone who can read the chat sees the name, even without access to the automation.
If the automation was deleted, the card reads **deleted automation**. Its ID stays in the tooltip.

### Webhook responses

| Status | Cause                                                                                                                                                                                                    |
|--------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `202`  | Coder saved the message.                                                                                                                                                                                 |
| `400`  | The body isn't valid JSON or couldn't be read, or no chat model is available in the organization.                                                                                                        |
| `401`  | The automation doesn't exist, isn't a webhook, or the secret is missing, wrong, or rotated. Every case returns the same response.                                                                        |
| `403`  | The automation is turned off, its owner isn't active, its owner can't send messages to the target chat or create chats, or a [chat lifecycle hook](../../admin/setup/chat-lifecycle-hooks.md) denied it. |
| `404`  | The experiment is off for the automation owner.                                                                                                                                                          |
| `409`  | A single-use webhook was already used, the target chat is unavailable, the chat is busy and **When busy** is set to **Skip the run**, or the model is unavailable.                                       |
| `413`  | The body, or the event data after escaping, is larger than the webhook body limit (64&nbsp;KiB by default).                                                                                              |
| `429`  | Automations already fill their share of the chat's queue, the chat's queue is full, or the sender hit the rate limit.                                                                                    |
| `502`  | Coder couldn't reach a chat lifecycle hook.                                                                                                                                                              |

The events endpoint has its own rate limit.
It counts requests per client IP address across all automations and uses the deployment's API rate limit, 512 requests per minute by default.

Coder doesn't retry or deduplicate deliveries.
If a sender retries after a timeout, the chat can receive the same event twice.

### Single-use and multi-use webhooks

A multi-use webhook accepts events until you turn the automation off.
A single-use webhook accepts one event.
Accepting that event also turns the automation off, so it counts as used, not as an automation that runs.
Later deliveries return `409`.
In the list, **Next run** and the switch label read **Used**, and turning it on again returns `409`: create a new automation instead.
A used single-use webhook doesn't count toward the per-owner limit.
A delivery that Coder refuses doesn't use up a single-use webhook.
You choose single use or multi use when you create the webhook and can't change it later.
An update request that includes `webhook_use` returns `400`.

### Rotate the webhook secret

Rotate the secret when it leaks or when you hand the endpoint to a new sender.
Rotating the secret invalidates the old one immediately, and requests with the old secret return `401`.

To rotate it in the dashboard, edit the automation and select **Rotate secret**, then confirm.
The **Copy the webhook secret** dialog shows the new secret.

To rotate it with the API, send a `POST` request to `/api/experimental/organizations/{organization}/chat-automations/{automation}/secret/rotate`.
The response contains `webhook_secret` and `webhook_secret_version`.

You can't rotate the secret of a used single-use webhook.
The dashboard hides **Rotate secret** for it, and the rotate request returns `409`.
Create a new automation instead.

## Create a schedule automation

1. On the **Agents** page, select **Automations** in the sidebar.
1. If you belong to several organizations, select the organization next to **Create automation**.
1. Select **Create automation**.
1. Enter a **Name**.
1. Enter a **Prompt**.
1. Under **Trigger**, select **Schedule**.
1. Select a **Repeat** option.
1. If the **Repeat** option runs at a time of day, select a **Time**.
1. For **Hourly**, enter the **Minute** past each hour.
1. To write the schedule yourself, enter a **Cron expression** instead.
1. Select a **Time zone**.
1. Check the times under **Upcoming runs**.
1. Under **Runs in**, select **Existing chat** or **New chat each run**.
1. For an existing chat, select a **Chat**.
1. For an existing chat, select a **When busy** option.
1. For a new chat, select a **Model**.
1. For a new chat, optionally select a **Project**.
1. Select **Save**.

With the API, send `"kind": "schedule"` with `schedule_cron` and `schedule_time_zone` in the create request.

### Write the schedule

The cron expression has five fields: minute, hour, day of month, month, and day of week.
For example, `0 9 * * 1-5` runs at 9:00 AM on weekdays.

Set the time zone in its own field as an IANA name, such as `America/New_York`.
Coder rejects `Local`, descriptors such as `@daily`, and `CRON_TZ=` prefixes.
Coder also rejects a schedule that never runs.

When you restrict both the day of month and the day of week, the schedule runs on days that match either field.
Schedules follow wall-clock time in their time zone, so a `0 9 * * *` schedule runs at 9:00 AM local time before and after a daylight saving time change.
When clocks go back, a time that occurs twice runs once, at its first occurrence.
For example, in `America/New_York` a `30 1 * * *` schedule runs once on the night clocks go back, at the first 1:30 AM.
When clocks go forward, a time that the change skips runs at the first valid time after the change.
For example, in `America/New_York` a `30 2 * * *` schedule runs at 3:00 AM on the night clocks go forward.
Several skipped times in one change run once, together with a run scheduled at the end of the change.

A schedule must leave at least the deployment's minimum interval between two runs, 5&nbsp;minutes by default.
Coder rejects a new or changed schedule that runs more often with a `400` error on `schedule_cron`.
Administrators set the minimum with `--chat-min-automation-schedule-interval`.
Schedules that already run more often keep running, and their responses include `"schedule_interval_below_minimum": true`.
To change the cron expression or time zone of such a schedule, the new schedule must meet the minimum.
Coder measures the interval on the wall clock of the schedule's time zone.
On the night clocks go forward, two runs can be closer together than the minimum, because the clock jumps forward between them.

### Check upcoming runs

The control plane computes the next runs of a schedule:

- The editor lists them under **Upcoming runs** while you type.
- Responses for turned-on schedules include `next_run_times`, up to five times in UTC.
  The list is empty while the schedule has [`paused_reasons`](#read-why-an-automation-cant-run).
- `POST /api/experimental/organizations/{organization}/chat-automations/schedule-preview` evaluates an unsaved schedule.
  Send `schedule_cron` and `schedule_time_zone`, and the response contains `next_run_times`.
  When the time zone changes its clocks within a year, the response also contains `clock_change_note`, which explains how the schedule runs across the change.
  The editor shows the note under **Upcoming runs**.
  When the schedule runs more often than the minimum interval, the response still lists its runs and also contains `interval_warning`.
  The editor shows the warning under **Upcoming runs**, and saving a new or changed schedule like this fails.

### When a schedule runs

Coder checks for due schedules every 30&nbsp;seconds and accepts an occurrence up to 60&nbsp;seconds late.
An occurrence that is older than that is missed and never replayed.
When a check finds a missed occurrence, it still runs the next occurrence that is less than 60&nbsp;seconds late.
After a server outage, schedules continue from their next time and don't catch up on missed runs.
With several Coder replicas, each occurrence's prompt reaches [chat lifecycle hooks](../../admin/setup/chat-lifecycle-hooks.md) once.
If a replica stops while it sends an occurrence's prompt, that occurrence is missed.

A schedule that creates a new chat for each run skips an occurrence while a chat from an earlier run is still active.
A chat is active while it runs a turn, including while it waits to retry a failed model call or waits for a free chat slot.
Without this, a stalled or unreachable model would add one stuck chat per occurrence, and those chats would take the deployment's chat slots from people.
The skip has the reason `previous_run_active`.
Webhook deliveries and **Run now** aren't limited this way.
A hook that fails is called again when Coder retries the occurrence within the 60&nbsp;seconds.
An edit to the automation while the hooks run also calls them again, with the edited prompt, and only the edited prompt is sent.

When you turn a schedule back on, it continues from its next future time.
When you change its cron expression or time zone, Coder recomputes its next run.

### Run a schedule now

To send a schedule's prompt immediately, select **Run now** on its row on the **Automations** page.
With the API, send a `POST` request to `/api/experimental/organizations/{organization}/chat-automations/{automation}/runs`.

Run now works only for schedule automations that are turned on, and only the owner can use it.
It returns `202` with `input_id` and `chat_id`, and doesn't change the schedule's next run.
Unlike a scheduled run, Run now reports refusals: `409` when the automation is off or the chat is busy and **When busy** is set to **Skip the run**, and `429` when the queue share or the queue is full.
Other refusals return the same status codes as [webhook deliveries](#webhook-responses), such as `409` when the target chat or the model is unavailable.

## Choose a target

An existing chat target must be a top-level chat that the automation owner owns, in the same organization as the automation.
Sub-agent chats can't be targets.
Messages to an existing chat use that chat's own model and tools.

A new chat target starts a chat for every run with the model you pick.
You can also set a reasoning effort.
Each new chat is titled with the automation name and the run time, for example `Nightly report · 16 Sep 09:00 CEST`.
Scheduled runs and **Run now** use the schedule's time zone, and webhooks use UTC.
When the time zone has no abbreviation, the title shows its UTC offset, such as `UTC-03`.
New chats get only the MCP servers with the `force_on` [availability policy](./platform-controls/mcp-servers.md#availability-policies) that the owner can access.

A new chat target can also put its chats into a project.
This requires the `chat-projects` experiment.
Select the project in the **Project** field of the editor, or set `project_id` in the API.
The project must be one of yours, in the automation's organization.
To remove the project, select **No project**, or send `project_id` as `00000000-0000-0000-0000-000000000000` in a `PATCH` request.
A `PATCH` request that omits `project_id` or sends `null` leaves the project unchanged.
A change applies to the chats that later runs create.

Coder creates the chat without a project, and the run still happens, in these cases:

- The project was deleted.
  Deleting a project also removes it from the automation.
- The `chat-projects` experiment is off.
  The automation keeps the project and uses it again when the experiment is on.
- The owner can no longer use the project.

To open what an automation wrote, use its row. An existing chat automation shows **Open chat**, which opens its one chat. A new chat automation shows **View chats**, which lists every chat it created. **Edit** and **Delete** are in the row's **More** menu.

If an existing chat target is deleted or archived, the row shows **Missing target**.
Webhook deliveries return `409` and the schedule doesn't run.
Coder doesn't turn the automation off.
Edit the automation to pick another chat, unarchive the chat, or turn the automation off.

## Choose what happens when the chat is busy

The **When busy** setting applies only to existing chat targets.
A chat counts as busy unless it is idle with no queued messages.

| Option               | API value | Behavior                                                                    | Default for |
|----------------------|-----------|-----------------------------------------------------------------------------|-------------|
| **Skip the run**     | `skip`    | Coder refuses the run while the chat is busy.                               | Schedules   |
| **Queue the prompt** | `queue`   | Coder adds the prompt to the chat's queue, behind messages already waiting. | Webhooks    |

Automations never interrupt or steer a turn that is already running.

A chat's queue holds up to the number of messages set by `--chat-max-queued-messages-per-chat`, 20 by default.
Automation messages together can fill at most half of it, rounded down and at least one, which is 10 by default.
When the automation share is full, webhook deliveries and Run now return `429`, and the scheduled occurrence is skipped.

## Let an agent manage automations

An agent can create and manage automations with the `manage_automations` tool.
The **Manage automations** switch controls the tool for each chat:

- New chats have **Manage automations** on by default when the `chat-automations` experiment is on for you.
  To start a chat without it, select the **Automations** chip in the chat input before you send the first message, or turn it off in the plus menu.
  Through the API, send `"manage_automations_enabled": false` when you create the chat.
- In an existing chat, open the plus menu in the chat input and turn **Manage automations** on or off.

Without the experiment, the switch is off.
It's available only on top-level chats, and only the chat owner can change it.
Chats that an automation creates for each run never have the tool.

The agent receives the tool only when all of these are true:

- The chat is a top-level chat and isn't archived.
- **Manage automations** is on for the chat.
- The chat isn't in plan mode.
- The experiment is on for the chat owner.

The tool supports the `list`, `get`, `create`, `update`, `enable`, `disable`, `delete`, and `run_now` actions.
The agent acts as you, so its automations count toward your limit and appear on your **Automations** page with a **Created by agent in** link to the chat.

In a turn that you start, the agent can list, read, turn off, and delete any of your automations in the chat's organization, including automations that target other chats.
Listing automations shows their prompts in the chat, cut to 2,000 characters.
Reading one automation shows its full prompt.

The agent can create, update, enable, or run only automations that stay close to its own chat:

- An existing chat automation must target the calling chat.
- A new chat automation must use the chat's model, or a model without provider tools such as web search.
- A new chat automation must use the chat's project or no project.
  When the agent creates one, it uses the chat's project by default if the `chat-projects` experiment is on.

Coder checks these rules again when it applies the change.

In a turn that an automation started, the agent can't create, update, enable, or run automations.
It sees only automations that target the chat or that created it, and it can delete only the automation that started the turn.
The newest message of the turn decides.
If you send a message after an automation message, even before the agent responds, the turn becomes yours and these limits don't apply.

When the agent creates a webhook in a turn that you start, the result shows the endpoint and the secret once, so the agent can hand them to the sender.
For a multi-use webhook, the agent can store the secret in the sending system, for example as a CI secret.
For a single-use webhook, the agent can pass the secret to a job or another agent as a callback, and the secret stops working after one delivery.
The tool result stays in the chat, so anyone who can read the chat can use the secret: a multi-use secret until you rotate it, a single-use secret until its first delivery.
Don't share the chat while the secret is valid, and rotate a multi-use secret in the dashboard if the chat was shared.
No other action returns a secret.

### What the agent sees in an automation turn

Each message that an automation sends starts with a note that only the model sees.
The note names the automation and its trigger, says that the agent acts for you while you may be away, and tells the agent to reply in the chat with the result.
It says whether a schedule, a webhook, or **Run now** started the run.
For a new chat automation, it says that each run starts a fresh chat and nothing carries over, so lasting changes belong in the automation's prompt.
For a heartbeat on a chat with **Manage automations** on, the note tells the agent how to turn the heartbeat off when it is no longer needed.
For a webhook, the note repeats that the event data is untrusted.
The dashboard doesn't show the note.

### Heartbeats

A heartbeat is a schedule automation that an agent creates for its own chat.
It sends a recurring prompt to the chat, so the agent picks up a task again on a schedule, even while your browser is closed.
To set one up, keep **Manage automations** on and ask the agent to check in on the task on a schedule, for example every hour.

## Limits

| Limit                         | Value                                                                                                                                                                              |
|-------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Automations per owner         | 50 by default, counted across all organizations. Creating one more returns `409`. Set with `--chat-max-automations-per-owner`.                                                     |
| Time between schedule runs    | At least 5&nbsp;minutes by default. Set with `--chat-min-automation-schedule-interval`.                                                                                            |
| Queued messages per chat      | 20 by default. Set with `--chat-max-queued-messages-per-chat`.                                                                                                                     |
| Automation share of the queue | Half of the queue limit, rounded down and at least one.                                                                                                                            |
| Webhook body                  | 64&nbsp;KiB of valid JSON by default, also after escaping `<`, `>`, `&`, U+2028, and U+2029. Set with `--chat-automation-webhook-max-body-bytes`, from 1&nbsp;KiB to 256&nbsp;KiB. |
| Name                          | 1 to 128 characters.                                                                                                                                                               |

Refer to the [configuration reference](../../admin/setup/configuration-reference.md#max-automations-per-owner) for the `coder server` options.

## Known risks and limitations

Automations run without a person watching each turn.
Review these risks before you turn the experiment on for many users.

### Spend

The per-owner limit caps how many automations exist, not how many turns or chats they start.
A schedule that runs as often as the minimum interval allows can start a turn every 5&nbsp;minutes by default.
A leaked multi-use secret for a new chat target lets anyone with the secret start chats as the owner until you act.

To stop the spend, rotate the secret or turn the automation off.
Turning the experiment off stops new runs, but prompts that are already queued still run, and the owner can no longer turn the automation off until the experiment is back on.
To cap each user's AI spend, refer to [Spend management](./platform-controls/spend-management.md).

### Silent failures

Every run of an automation is in the audit log as a **start** entry for the automation, attributed to its owner: each schedule occurrence, including skipped and missed ones, each **Run now**, and each webhook delivery with the right secret.
The entry's additional fields name the `trigger` (`schedule`, `webhook`, or `run_now`), the `outcome`, the `reason`, the `input_id`, and the `chat_id`, and webhook entries carry the sender's IP address and user agent.
Requests with a wrong secret or for an automation that doesn't exist aren't audited.

Skipped and missed schedule runs appear only in the logs of `coderd`, the process that runs the control plane, as `chat automation schedule occurrence skipped` and `chat automation schedule occurrence missed` messages.
A schedule doesn't run or produce logs when its target chat is archived or deleted, or when its owner isn't active.
A refused webhook delivery appears in the HTTP response the sender receives.
Refusals that depend on the target chat, such as a busy chat, a full queue, or a hook denial, also appear in the `coderd` logs as `chat automation input refused` messages.
Coder keeps no run history for automations.

### Prompt injection

Coder labels webhook bodies as untrusted data, but the event text still enters the chat's context, and a model can follow instructions inside it.
A webhook secret works like a credential for whatever the target chat can reach, including the owner's workspace and MCP servers.
People you share the chat with see everything that automation turns produce.

Coder doesn't verify provider signatures.
Senders that sign their payloads with HMAC and can't set an `Authorization` header, such as GitHub webhooks, can't call the endpoint directly.
Send them through a relay that verifies the sender's signature, rejects requests with an invalid signature, and only then forwards the payload with the bearer token.

### Audit logs

If you use [audit logs](../../admin/security/audit-logs.md), Coder records creating, updating, and deleting automations, and rotating their secrets.
The entries mark the prompt and the secret hash as changed without recording their values.

## Learn more

- [Feature stages](../../reference/feature-stages.md)
- [Chat sharing](./chat-sharing.md)
- [Configuration reference](../../admin/setup/configuration-reference.md)
