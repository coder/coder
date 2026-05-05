# Migrating from the Tasks API to the Chats API

The [Tasks API](../../reference/api/tasks.md) (`/api/v2/tasks`) and the
[Chats API](../../reference/api/chats.md) (`/api/experimental/chats`) serve similar
goals (programmatic access to AI-powered coding agents) but they differ
significantly in architecture, capabilities, and usage patterns.

This guide walks you through updating your integrations from the Tasks API
to the Chats API.

> [!NOTE]
> The Chats API is experimental in current Coder releases. Endpoints live under `/api/experimental/chats` and may change without notice until the feature graduates to GA.

## When to migrate

Coder Tasks is being deprecated. Support continues on the ESR release and
through Coder v2.36. See the deprecation notice on the [Coder Tasks](../tasks.md) page for the full timeline.

If you currently run workflows on the Tasks API, you should plan to
migrate to the Chats API and [Coder Agents](./index.md). Coder Agents
runs the agent loop in the Coder control plane rather than inside the
workspace, and is the supported path going forward.

The two systems are not interchangeable. Tasks and Chats are separate
resources with separate APIs, so plan to update your integrations rather
than expecting a drop-in replacement.

## Key architectural differences

Before mapping individual endpoints, understand the structural changes:

| Aspect                 | Tasks API                                                                        | Chats API                                                  |
|------------------------|----------------------------------------------------------------------------------|------------------------------------------------------------|
| Agent execution        | Agent runs **inside the workspace** (via AgentAPI)                               | Agent loop runs **in the control plane**                   |
| LLM credentials        | Injected into workspace environment                                              | Stored in control plane only; never enters the workspace   |
| Workspace provisioning | You specify a `template_version_id` at creation                                  | The agent auto-selects a template and provisions on demand |
| Template requirements  | Requires `coder_ai_task` resource, `coder_task` data source, and an agent module | Any template with a clear description works                |
| Chat state             | Stored in the workspace (AgentAPI state file)                                    | Persisted in the Coder database                            |
| Conversation model     | Single prompt with optional follow-up input                                      | Multi-turn chat with message history, queuing, and editing |
| Real-time updates      | HTTP polling (`GET .../logs`)                                                    | WebSocket streaming (`GET .../stream`)                     |
| Sub-agents             | Not supported                                                                    | Built-in sub-agent delegation                              |

## Endpoint mapping

The table below maps each Tasks API endpoint to its Chats API equivalent.

| Operation         | Tasks API                                 | Chats API                                                           |
|-------------------|-------------------------------------------|---------------------------------------------------------------------|
| List              | `GET /api/v2/tasks`                       | `GET /api/experimental/chats`                                       |
| Create            | `POST /api/v2/tasks/{user}`               | `POST /api/experimental/chats`                                      |
| Get by ID         | `GET /api/v2/tasks/{user}/{task}`         | `GET /api/experimental/chats/{chat}`                                |
| Delete            | `DELETE /api/v2/tasks/{user}/{task}`      | `PATCH /api/experimental/chats/{chat}` with `{"archived": true}`    |
| Send follow-up    | `POST /api/v2/tasks/{user}/{task}/send`   | `POST /api/experimental/chats/{chat}/messages`                      |
| Update input      | `PATCH /api/v2/tasks/{user}/{task}/input` | `PATCH /api/experimental/chats/{chat}/messages/{message}`           |
| Get logs / stream | `GET /api/v2/tasks/{user}/{task}/logs`    | `GET /api/experimental/chats/{chat}/stream` (WebSocket)             |
| Pause             | `POST /api/v2/tasks/{user}/{task}/pause`  | `POST /api/experimental/chats/{chat}/interrupt`                     |
| Resume            | `POST /api/v2/tasks/{user}/{task}/resume` | `POST /api/experimental/chats/{chat}/messages` (send a new message) |
| Watch all         | n/a                                       | `GET /api/experimental/chats/watch` (WebSocket)                     |
| Get messages      | n/a                                       | `GET /api/experimental/chats/{chat}/messages`                       |
| List models       | n/a                                       | `GET /api/experimental/chats/models`                                |
| Upload file       | n/a                                       | `POST /api/experimental/chats/files`                                |

## Migration steps

### 1. Configure an LLM provider

With Tasks, LLM credentials are injected into the workspace as environment
variables (e.g. `ANTHROPIC_API_KEY`). With Coder Agents, credentials are
configured once in the control plane:

1. Navigate to the **Agents** page in the Coder dashboard.
1. Click **Admin** > **Providers**, select a provider, enter your API key,
   and save.
1. Under **Models**, add at least one model and set it as the default.

You no longer pass API keys in template variables or workspace environment. See https://coder.com/docs/ai-coder/agents/getting-started for more information.

### 2. Update task creation calls

**Tasks API**. You specify the user, template version, and a prompt
string:

```sh
# Tasks API: create a task
curl -X POST https://coder.example.com/api/v2/tasks/me \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "template_version_id": "<template-version-uuid>",
    "input": "Fix the failing tests in the auth service"
  }'
```

**Chats API**. You send structured content parts. No template or user
path segment is required:

```sh
# Chats API: create a chat
curl -X POST https://coder.example.com/api/experimental/chats \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "organization_id": "<your-org-id>",
    "content": [
      {"type": "text", "text": "Fix the failing tests in the auth service"}
    ]
  }'
```

Key differences:

- The `{user}` path parameter is removed. The authenticated user is
  inferred from the session token.
- `organization_id` is required in the request body. The caller must be a
  member of that organization.
- The prompt is now an array of `ChatInputPart` objects (supporting `text`,
  `file`, and `file-reference` types) instead of a plain string.
- `template_version_id` and `template_version_preset_id` are removed. The
  agent selects a template automatically based on the prompt and available
  template descriptions. To pin to a specific workspace, pass
  `workspace_id` instead.
- Optionally pass `model_config_id` to override the default model, or
  `mcp_server_ids` to attach MCP servers.

### 3. Update follow-up message calls

**Tasks API**. Follow-ups use the send endpoint with a plain string:

```sh
# Tasks API: send input
curl -X POST https://coder.example.com/api/v2/tasks/me/my-task/send \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input": "Now also update the integration tests"}'
```

**Chats API**. Follow-ups use the messages endpoint with content parts:

```sh
# Chats API: send a message
curl -X POST \
  https://coder.example.com/api/experimental/chats/$CHAT_ID/messages \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {"type": "text", "text": "Now also update the integration tests"}
    ]
  }'
```

The Chats API supports message queuing. If the agent is busy, the message
is queued automatically and delivered when the agent finishes its current
step. The response includes a `queued` field indicating whether the message
was delivered immediately or queued.

### 4. Switch from log polling to WebSocket streaming

**Tasks API**. You poll for logs:

```sh
# Tasks API: get logs
curl https://coder.example.com/api/v2/tasks/me/my-task/logs \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"
```

**Chats API**. You open a one-way WebSocket connection:

```text
GET wss://coder.example.com/api/experimental/chats/{chat}/stream
```

The WebSocket sends JSON envelopes with a `type` field (`"ping"`,
`"data"`, or `"error"`). Data envelopes contain batches of events:

| Event type     | Description                                             |
|----------------|---------------------------------------------------------|
| `message_part` | A chunk of the agent's response (text, tool call, etc.) |
| `message`      | A complete message has been persisted                   |
| `status`       | The chat status changed (e.g. `running` → `waiting`)    |
| `error`        | An error occurred during processing                     |
| `retry`        | The server is retrying a failed LLM call                |
| `queue_update` | The queued message list changed                         |

Use `after_id` as a query parameter when reconnecting to skip messages the
client already has.

### 5. Update status handling

Task and chat statuses use different values. The Chats API status set is
defined in `codersdk.ChatStatus`:

| Tasks API status | Chats API status  | Notes                                                                                                                                                                                                                         |
|------------------|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `pending`        | `pending`         | Queued for processing.                                                                                                                                                                                                        |
| `running`        | `running`         | Agent is actively working.                                                                                                                                                                                                    |
| `complete`       | `waiting`         | Idle. Newly created, finished successfully, or interrupted. This is the default idle state.                                                                                                                                   |
| `paused`         | n/a               | The Tasks API pause stops the workspace; the Chats API equivalent is `interrupt` plus separate workspace lifecycle. The `paused` enum value exists in code but no production path on `main` transitions a chat into it today. |
| `failed`         | `error`           | Agent encountered an error.                                                                                                                                                                                                   |
| n/a              | `requires_action` | Agent invoked a client-provided tool and is waiting for the result before continuing.                                                                                                                                         |

The Chats API uses `waiting` as the default idle state (not `complete`).
A chat enters `waiting` when it is first created (before any message is
queued) and again whenever a run finishes or is interrupted, so treat
`waiting` as "the agent is not currently working" rather than only "the
agent just finished." The `completed` enum value is also defined but is
not currently set by any production code path on `main`.

### 6. Replace delete with archive

The Tasks API uses `DELETE` to remove a task. The Chats API uses archiving:

```diff
- curl -X DELETE https://coder.example.com/api/v2/tasks/me/my-task \
-   -H "Coder-Session-Token: $CODER_SESSION_TOKEN"

+ curl -X PATCH https://coder.example.com/api/experimental/chats/$CHAT_ID \
+   -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
+   -H "Content-Type: application/json" \
+   -d '{"archived": true}'
```

Archived chats can be restored by setting `archived` to `false`.

### 7. Replace pause/resume with interrupt and messaging

**Tasks API**. Pause and resume stop and start the workspace:

```sh
# Tasks API
curl -X POST \
  https://coder.example.com/api/v2/tasks/me/my-task/pause \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"

curl -X POST \
  https://coder.example.com/api/v2/tasks/me/my-task/resume \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"
```

**Chats API**. Interrupt stops the current agent loop. Sending a new
message resumes processing:

```sh
# Chats API: interrupt
curl -X POST \
  https://coder.example.com/api/experimental/chats/$CHAT_ID/interrupt \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"

# Chats API: resume by sending a new message
curl -X POST \
  https://coder.example.com/api/experimental/chats/$CHAT_ID/messages \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {"type": "text", "text": "Continue where you left off"}
    ]
  }'
```

In the Tasks API, pausing stops the workspace and frees compute. In the
Chats API, interrupt stops the agent loop in the control plane; the
workspace may remain running. The workspace lifecycle is managed
independently.

### 8. Update GitHub Actions integrations

If you use the
[Create Task Action](https://github.com/coder/create-task-action) GitHub
Action, replace it with the dedicated
[`coder/create-agent-chat-action`](https://github.com/coder/create-agent-chat-action).
It handles the API call, the GitHub user lookup, and the optional issue
comment, so most existing workflows can swap one `uses:` line and rename
a few inputs.

We are actively shipping new features for `create-agent-chat-action`, so
pin to a major version (for example `@v0`) and watch the
[releases](https://github.com/coder/create-agent-chat-action/releases) for
updates.

```diff
# .github/workflows/triage-bug.yaml
jobs:
  coder-create-task:
    runs-on: ubuntu-latest
    if: github.event.label.name == 'coder'
    steps:
-     - name: Coder Create Task
-       uses: coder/create-task-action@v0
-       with:
-         coder-url: ${{ secrets.CODER_URL }}
-         coder-token: ${{ secrets.CODER_TOKEN }}
-         coder-organization: "default"
-         coder-template-name: "my-template"
-         coder-task-name-prefix: "gh-task"
-         coder-task-prompt: >-
-           Use the gh CLI to read
-           ${{ github.event.issue.html_url }},
-           fix the issue, and create a PR.
-         github-user-id: ${{ github.event.sender.id }}
-         github-issue-url: ${{ github.event.issue.html_url }}
-         github-token: ${{ github.token }}
-         comment-on-issue: true
+     - name: Coder Create Agent Chat
+       uses: coder/create-agent-chat-action@v0
+       with:
+         coder-url: ${{ secrets.CODER_URL }}
+         coder-token: ${{ secrets.CODER_TOKEN }}
+         chat-prompt: >-
+           Use the gh CLI to read
+           ${{ github.event.issue.html_url }},
+           fix the issue, and create a PR.
+         github-user-id: ${{ github.event.sender.id }}
+         github-issue-url: ${{ github.event.issue.html_url }}
+         github-token: ${{ github.token }}
+         comment-on-issue: true
```

Key differences from the Tasks GHA:

- No `coder-template-name` or `coder-task-name-prefix`. The agent
  auto-provisions a workspace; pass `workspace-id` if you want to pin to
  an existing workspace instead.
- The prompt input is renamed from `coder-task-prompt` to `chat-prompt`.
- LLM credentials are no longer passed through the template. They are
  configured in the Coder control plane.
- Identify the user with `github-user-id` (the action resolves it to a
  Coder user via the GitHub OAuth link) or with `coder-username`
  directly.

See the
[action README](https://github.com/coder/create-agent-chat-action#inputs)
for the full input and output reference, including the `existing-chat-id`
input for sending follow-up messages on a previous chat.

## Template recommendations

<!-- NEEDS REVIEW: This section contains initial recommendations based on
     the current Coder Agents architecture. Platform teams should validate
     these against their own deployment patterns before adopting them. -->

> [!NOTE]
> This section contains recommendations that may evolve as Coder Agents
> matures. Review these against your deployment requirements.

With Coder Tasks, every task-capable template requires specific Terraform
resources (`coder_ai_task`, `coder_task`, agent modules, and LLM API
keys). With Coder Agents, templates no longer need any of these. The
agent runs in the control plane and treats the workspace as plain compute.

However, **we still recommend creating dedicated templates for agent
workloads** rather than reusing your standard developer templates
unchanged. The reasons are different from Tasks, but the principle holds:

### Why dedicated agent templates still matter

- **Network boundaries.** Agent workspaces inherit whatever network access
  the template allows. Because the agent does not need outbound access to
  LLM providers (that happens in the control plane), you can lock down
  agent templates to only reach the Coder control plane and your git
  provider. Standard developer templates typically allow broader access.
- **No IDE tooling overhead.** The agent connects via the workspace
  daemon's HTTP API, not through VS Code or JetBrains. Removing IDE
  extensions, desktop environments, and similar tooling from agent
  templates reduces image size and startup time.
- **Scoped credentials.** Agent workloads may warrant more restrictive
  credentials than interactive developer sessions. A dedicated template
  lets you provide a separate, narrower-scoped git token or service
  account without affecting your developers' workflow.
- **Cost control.** Agent workspaces can often use smaller compute
  resources than developer workspaces since they don't need to run IDEs,
  language servers, or other interactive tooling. A dedicated template lets
  you right-size the infrastructure.

### What to include in agent templates

<!-- TODO: Expand this section with concrete Terraform examples once
     template patterns stabilize. -->

- **Clear descriptions.** The agent selects templates by reading names and
  descriptions. Include the target language, framework, repository, and
  type of work. For example: *"Python backend services for the payments
  repo. Includes Poetry, Python 3.12, and PostgreSQL."*
- **Pre-installed dependencies.** Language runtimes, build tools, `git`,
  and project-specific dependencies should be baked into the image. Time
  the agent spends installing tools is time not spent on the task.
- **Git configuration.** Ensure `git` is configured with credentials and
  author information so the agent can commit and push without additional
  setup.
- **Minimal parameters.** Use sensible defaults so the agent can provision
  workspaces without guessing. Avoid required parameters with opaque
  identifiers.

### What to remove from migrated task templates

If you are converting an existing task template for use with Coder Agents,
you can safely remove the Tasks-specific Terraform resources. They are
unused when the chat is driven by the Chats API:

```diff
  terraform {
    required_providers {
      coder = {
        source  = "coder/coder"
-       version = ">= 2.13"
+       version = ">= 2.13"
      }
    }
  }

- data "coder_task" "me" {}
-
- resource "coder_ai_task" "task" {
-   app_id = module.claude-code.task_app_id
- }
-
- module "claude-code" {
-   source         = "registry.coder.com/coder/claude-code/coder"
-   version        = "4.0.0"
-   agent_id       = coder_agent.main.id
-   ai_prompt      = data.coder_task.me.prompt
-   claude_api_key = var.anthropic_api_key
- }
-
- variable "anthropic_api_key" {
-   type        = string
-   description = "Anthropic API key"
-   sensitive   = true
- }

  resource "coder_agent" "main" {
    os   = "linux"
    arch = "amd64"
+   # No agent modules, no AgentAPI, no LLM keys needed.
+   # The Coder Agents control plane handles the agent loop.
  }
```

> [!TIP]
> You do not have to remove these resources immediately. Templates can
> serve both Tasks and Chats simultaneously during a transition period.
> The Tasks-specific resources are simply unused when work comes through
> the Chats API.

See
[Template Optimization](./platform-controls/template-optimization.md)
for the full guide on writing discoverable descriptions, configuring
network boundaries, scoping credentials, and pre-installing dependencies.

### Pre-creating a workspace for deterministic results

Letting the agent pick a template and provision a workspace works well
for exploratory chats. If your workflow requires deterministic results like:

- Automations
- Recurring processes
- Generally any case that needs a known reproducible environment

pre-create the workspace yourself and attach it when you create the chat.

The pattern is two API calls:

1. Create a workspace from a specific template via
   [`POST /api/v2/users/{user}/workspaces`](../../reference/api/workspaces.md#create-user-workspace).
   You control the template, the version, and any rich parameters.
2. Create the chat with `workspace_id` set to the workspace you just
   created. The agent runs against that workspace instead of selecting
   one heuristically.

```sh
# 1. Provision the workspace from the exact template you want.
WORKSPACE_ID=$(curl -s -X POST \
  https://coder.example.com/api/v2/users/me/workspaces \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "template_id": "<your-agent-template-uuid>",
    "name": "agent-run-${GITHUB_RUN_ID}"
  }' | jq -r '.id')

# 2. Create the chat bound to that workspace.
curl -s -X POST https://coder.example.com/api/experimental/chats \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"organization_id\": \"<your-org-id>\",
    \"workspace_id\": \"$WORKSPACE_ID\",
    \"content\": [
      {\"type\": \"text\", \"text\": \"Fix the failing tests in the auth service\"}
    ]
  }"
```

This pattern is the closest analogue to the Tasks API behavior of
`template_version_id` plus `coder-template-name`: you decide which
template runs, the agent decides what to do inside it. The same approach
works from the
[`coder/create-agent-chat-action`](https://github.com/coder/create-agent-chat-action)
GHA, which exposes the same pin via its `workspace-id` input.

## How to test your migration

After completing the migration steps above, walk through these checks to
confirm the Chats API integration is working end-to-end.

### 1. Confirm LLM provider connectivity

List available models to verify at least one provider is configured and
reachable:

```sh
curl -s https://coder.example.com/api/experimental/chats/models \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" | jq '.[].display_name'
```

If this returns an empty list or an error, revisit
[Step 1: Configure an LLM provider](#1-configure-an-llm-provider).

### 2. Create a chat and confirm the response

Create a simple chat that does not require a workspace:

```sh
curl -s -X POST https://coder.example.com/api/experimental/chats \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [{"type": "text", "text": "What is 2 + 2?"}]
  }' | jq '{id, status, title}'
```

You should receive a `Chat` object with `status` set to `"waiting"` or
`"pending"`. Save the `id` for subsequent steps.

### 3. Stream the response

Open a WebSocket connection to verify the agent processes the prompt and
returns a response. Using [websocat](https://github.com/vi/websocat):

```sh
websocat -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "wss://coder.example.com/api/experimental/chats/$CHAT_ID/stream"
```

You should see JSON envelopes with `"type": "data"` containing
`message_part` and `status` events. The chat should eventually reach
`"waiting"` status, indicating the agent completed its response.

### 4. Send a follow-up message

Verify multi-turn conversation works:

```sh
curl -s -X POST \
  "https://coder.example.com/api/experimental/chats/$CHAT_ID/messages" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [{"type": "text", "text": "Now multiply that by 10"}]
  }' | jq '{queued}'
```

The response should include `"queued": false` (delivered immediately) or
`"queued": true` (agent was busy. The message is queued and will be
processed next).

### 5. Test workspace provisioning

Create a workspace from your converted agent template through the
standard Coder UI, then attach it to a new chat from the chat composer:

1. In the Coder dashboard, create a workspace from the agent template
   you migrated.
2. Open **Agents** and start a new chat.
3. In the composer, use the workspace picker to attach the workspace you
   just created.
4. Send a prompt that exercises the workspace, for example: *"List the
   files in the root directory of this workspace."*

The response stream should show the agent invoking workspace tools (such
as `execute`) against the attached workspace. After the chat finishes,
verify the chat is bound to the workspace via the API:

```sh
curl -s "https://coder.example.com/api/experimental/chats/$CHAT_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" | jq '{workspace_id, status}'
```

A `workspace_id` matching the workspace you attached confirms the chat
is driving that workspace end-to-end. Auto-provisioning from the chat
flow is also supported but is easier to verify once the manual-attach
path is working.

### 6. Verify interrupt works

Start a long-running chat and interrupt it:

```sh
curl -s -X POST \
  "https://coder.example.com/api/experimental/chats/$CHAT_ID/interrupt" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"
```

Then confirm the chat status returns to `"waiting"`:

```sh
curl -s "https://coder.example.com/api/experimental/chats/$CHAT_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" | jq '.status'
```

### 7. Validate archive and restore

```sh
# Archive
curl -s -X PATCH \
  "https://coder.example.com/api/experimental/chats/$CHAT_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"archived": true}'

# Confirm it no longer appears in the default list
curl -s "https://coder.example.com/api/experimental/chats" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  | jq --arg id "$CHAT_ID" '[.[] | select(.id == $id)] | length'
# Should return 0

# Restore
curl -s -X PATCH \
  "https://coder.example.com/api/experimental/chats/$CHAT_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"archived": false}'
```

### Quick checklist

Use this checklist to confirm each part of your integration:

- [ ] At least one LLM model is configured and returned by `/chats/models`
- [ ] `POST /chats` creates a chat and returns a valid `Chat` object
- [ ] WebSocket stream at `/chats/{chat}/stream` delivers events
- [ ] Follow-up messages via `/chats/{chat}/messages` are accepted
- [ ] Chat attached to a workspace from the converted template runs
      tools against that workspace
- [ ] `POST /chats/{chat}/interrupt` stops the agent and returns to `waiting`
- [ ] Archive and restore via `PATCH /chats/{chat}` works
- [ ] (If applicable) GitHub Actions workflow creates chats successfully

## Features available only in the Chats API

The Chats API includes capabilities that have no equivalent in the Tasks
API:

| Feature                              | Description                                                                    |
|--------------------------------------|--------------------------------------------------------------------------------|
| **WebSocket streaming**              | Real-time event stream via `GET /chats/{chat}/stream` instead of HTTP polling  |
| **Watch all chats**                  | `GET /chats/watch` pushes events for all chats owned by the user               |
| **Message editing**                  | `PATCH /chats/{chat}/messages/{message}` to edit a sent message and re-process |
| **Message queuing**                  | Follow-up messages are automatically queued when the agent is busy             |
| **File uploads**                     | Attach images via `POST /chats/files` and reference them in messages           |
| **Model selection**                  | `GET /chats/models` to discover models; override per-chat or per-message       |
| **MCP server attachment**            | Attach MCP servers to a chat for tool augmentation                             |
| **Labels**                           | Key-value metadata on chats for filtering (`label` query parameter)            |
| **Sub-agents**                       | Agent can spawn child agents for parallel work                                 |
| **Diff/PR tracking**                 | `GET /chats/{chat}/diff` returns change tracking and PR metadata               |
| **Title regeneration**               | `POST /chats/{chat}/title/regenerate`                                          |
| **Pinning**                          | Pin and reorder chats via the `pin_order` field                                |
| **Automatic workspace provisioning** | No workspace needed for Q&A. Provisioned only when the agent needs to act      |

## Response schema changes

The Tasks API returns a `Task` object with workspace-centric fields. The
Chats API returns a `Chat` object with conversation-centric fields:

| Tasks API field    | Chats API equivalent                          | Notes                                                            |
|--------------------|-----------------------------------------------|------------------------------------------------------------------|
| `id`               | `id`                                          | Both are UUIDs                                                   |
| `initial_prompt`   | First message in `GET /chats/{chat}/messages` | Prompt is a message, not a top-level field                       |
| `display_name`     | `title`                                       | Auto-generated or set via `PATCH`                                |
| `status`           | `status`                                      | Different enum values (see status table above)                   |
| `current_state`    | Latest `status` event from the stream         | No equivalent top-level field                                    |
| `workspace_id`     | `workspace_id`                                | Nullable in Chats. May be `null` if no workspace was provisioned |
| `workspace_status` | n/a                                           | Manage workspace lifecycle separately                            |
| `template_id`      | n/a                                           | Not exposed; the agent selects templates internally              |
| `owner_id`         | `owner_id`                                    | Same concept                                                     |
| `name`             | n/a                                           | Chats use `id` for identification, not human-readable names      |

## CLI changes

The Tasks CLI (`coder task`) and the Coder Agents CLI are separate. Coder
ships an experimental TUI for Coder Agents at `coder exp agents` (planned
to graduate to `coder agents` in the May Beta release per
[#24432](https://github.com/coder/coder/pull/24432)). The TUI talks to the
same `/api/experimental/chats` endpoints documented in this guide; for
automation, prefer direct API calls.

| Tasks CLI           | Chats equivalent                        |
|---------------------|-----------------------------------------|
| `coder task create` | `coder exp agents` TUI or `POST /chats` |
| `coder task list`   | `coder exp agents` TUI or `GET /chats`  |
| `coder task logs`   | `GET /chats/{chat}/stream` (WebSocket)  |
| `coder task pause`  | `POST /chats/{chat}/interrupt`          |
| `coder task resume` | Send a follow-up message to the chat    |

> [!NOTE]
> The Coder Agents CLI today is an interactive TUI rather than a set of
> per-action subcommands like `coder task`. Use `curl`, the SDK, or your
> HTTP client of choice for non-interactive automation. Dedicated
> non-interactive subcommands may be added in a future release.
