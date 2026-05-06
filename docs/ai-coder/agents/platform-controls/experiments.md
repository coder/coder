# Experiments

The **Experiments** tab under **Agents** > **Settings** > **Manage Agents**
is where administrators opt in to features that are still iterating. The
behavior, configuration surface, and APIs documented here may change between
releases without notice.

> [!NOTE]
> Everything in this page is experimental. Pin a release before broad rollout
> and review the release notes before upgrading.

## Virtual desktop

Lets agents drive a graphical desktop inside the workspace through
`spawn_agent` with `type=computer_use` (screenshots, mouse, keyboard).

To enable, toggle **Virtual Desktop** on, then choose a **Computer use
provider** (Anthropic or OpenAI). It also requires:

- The [portabledesktop](https://registry.coder.com/modules/coder/portabledesktop)
  module installed in the workspace template.
- An API key for the selected provider configured under the **Providers**
  tab.

The Anthropic and OpenAI computer-use models are fixed by Coder per provider
and are not selectable from this UI. Anthropic is the default when no
provider is set.

## Advisor

Lets a root agent pause its current turn and request strategic guidance from
a separate, single-step model call. The advisor sees recent conversation
context, runs without any tools, and returns concise advice for the parent
agent rather than the end user. While active, it is the only tool the parent
can call for that turn.

Useful for planning ambiguity, architectural tradeoffs, debugging strategy
after repeated failures, or risk reduction before a destructive operation.

| Field             | Default              | Notes                                                                                                                   |
|-------------------|----------------------|-------------------------------------------------------------------------------------------------------------------------|
| Advisor (toggle)  | Off                  | Master switch. When off, the advisor tool is not attached to new chats.                                                 |
| Max uses per run  | `0` (unlimited)      | Caps how many times an agent can call the advisor in a single chat run. Must be a non-negative integer.                 |
| Max output tokens | `0` (server default) | Caps the advisor model's response length. `0` uses the server default of 16,384 tokens. Must be a non-negative integer. |
| Reasoning effort  | Use chat model       | One of unset, `low`, `medium`, or `high`. Unset delegates to the underlying model's default.                            |
| Advisor model     | Use chat model       | Optional dedicated chat model config for the advisor. When unset, the advisor reuses the parent chat's model.           |

The advisor is not available in plan mode or to subagents. Failed advisor
invocations refund the per-run budget, and advisor calls are not metered
against the parent chat's usage limit.

The same configuration is available at:

- `GET /api/experimental/chats/config/advisor`
- `PUT /api/experimental/chats/config/advisor`

## Chat debug logging

Records a detailed trace of each chat turn for troubleshooting: the
normalized request sent to the LLM provider, the full response, token usage,
retry attempts, and errors.

Off by default. Three layers control whether it runs for a given chat:

1. **Deployment override.** Setting `CODER_CHAT_DEBUG_LOGGING_ENABLED=true`
   (or `--chat-debug-logging-enabled` at server start) forces debug logging
   on for every chat. The runtime admin and user toggles become read-only.
1. **Runtime admin gate.** With the deployment override unset, the
   *Let users record chat debug logs* toggle decides whether users can opt
   in. Configure it at
   `GET/PUT /api/experimental/chats/config/debug-logging`.
1. **Per-user toggle.** Users with the admin gate enabled can turn debug
   logging on for their own chats from **Agents** > **Settings** > **General**
   under *Record debug logs for my chats*. The endpoint
   `PUT /api/experimental/chats/config/user-debug-logging` returns
   `409 Conflict` if the deployment override is active and `403 Forbidden`
   if the admin has not enabled user opt-in.

> [!IMPORTANT]
> Debug logs may contain sensitive content from prompts, responses, tool
> calls, and errors. Treat them with the same care as conversation history.
> Only the chat owner (or a user with read access to the chat) can fetch a
> chat's debug runs through the API. Administrators do not get blanket
> access to all users' debug data.

When debug logging is active for a chat, a **Debug** tab appears in the
right panel of the Agents page (alongside Git, Terminal, and Desktop) for
that chat's owner. The tab lists recent debug runs and lets you expand a run
into its per-step request, response, token usage, retry attempts, errors,
and policy metadata.

The same data is available through the experimental API:

- `GET /api/experimental/chats/{chat}/runs` lists the most recent runs for a
  chat (up to 100, newest first).
- `GET /api/experimental/chats/{chat}/runs/{debugRun}` returns a single run
  with all of its steps, including normalized request and response bodies.

Debug runs are stored alongside the chat and are removed when the parent
conversation is deleted (manually, by retention, or by chat purge). See
[Data Retention](./chat-retention.md) for the conversation retention
controls.
