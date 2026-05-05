# Advisor

The Advisor is an opt-in tool that lets a root agent pause its current
turn and request strategic guidance from a separate, single-step model
call. The advisor sees recent conversation context, runs without any
tools, and returns concise advice for the parent agent rather than the
end user.

> [!NOTE]
> The advisor is an experimental feature. The configuration surface, API
> shape, and tool behavior may change between releases.

## When to use it

Enable the advisor when you want agents to have an inline second
opinion during long or complex runs. Typical questions agents ask the
advisor include:

- Planning ambiguity: "What is the smallest safe change?"
- Architectural tradeoffs between two approaches.
- Debugging strategy after repeated failures.
- Risk reduction before a destructive operation.

The advisor is intended as an internal consultation. Its output is
delivered to the parent agent, not surfaced to the user as a
conversational message.

## How it works

When an agent invokes the advisor tool, the control plane runs a
nested, isolated model call:

1. The advisor receives recent context from the parent: inherited
   system messages plus up to 20 of the most recent non-system
   messages, capped at roughly 12,000 JSON bytes per slice.
1. The advisor runs a single step with no tools attached.
1. The advisor returns a structured result: either advice text, a
   limit-reached marker, or an error.
1. The result is delivered to the parent agent as a tool response.

While the advisor is active for a turn, it is the only tool the parent
can call. This keeps the advisor's guidance the focus of the turn
rather than mixing it with file edits or other side effects.

## Availability

| Context                   | Advisor available? |
|---------------------------|--------------------|
| Root chat (default mode)  | Yes, when enabled  |
| Root chat in plan mode    | No                 |
| Subagent chats (any type) | No                 |

The advisor is gated to root, non-plan-mode chats. Plan mode and
subagents use a restricted tool set that does not include the advisor.

## Configuration

> [!IMPORTANT]
> Advisor configuration requires the **Owner** role. The PUT endpoint
> requires `update` permission on the deployment configuration resource.

Configure the advisor from **Agents** > **Settings** > **Experiments**.

| Field             | Default              | Notes                                                                                                                   |
|-------------------|----------------------|-------------------------------------------------------------------------------------------------------------------------|
| Advisor (toggle)  | Off                  | Master switch. When off, the advisor tool is not attached to new chats.                                                 |
| Max uses per run  | `0` (unlimited)      | Caps how many times an agent can call the advisor in a single chat run. Must be a non-negative integer.                 |
| Max output tokens | `0` (server default) | Caps the advisor model's response length. `0` uses the server default of 16,384 tokens. Must be a non-negative integer. |
| Reasoning effort  | Use chat model       | One of unset, `low`, `medium`, or `high`. Unset delegates to the underlying model's default.                            |
| Advisor model     | Use chat model       | Optional dedicated chat model config for the advisor. When unset, the advisor reuses the parent chat's model.           |

The same values are exposed over the experimental chat configuration
API:

- `GET /api/experimental/chats/config/advisor`
- `PUT /api/experimental/chats/config/advisor`

The PUT handler validates that `max_uses_per_run` and
`max_output_tokens` are non-negative, that `reasoning_effort` is one of
the allowed enum values, and that `model_config_id` (when set) refers
to an existing enabled chat model config. Invalid values return
`400 Bad Request`.

## Cost and quota

Advisor calls run as nested, isolated model calls and are not metered
against the parent chat's usage limit. Their cost is bounded by the
configured `max_uses_per_run` and `max_output_tokens` values, which
admins set on the deployment.

Failed advisor invocations (provider errors, empty output) refund the
per-run budget so transient failures do not permanently consume an
agent's quota for the run.

## Audit and persistence

Advisor calls are not written to the chat's user-visible message
history. The parent agent sees only the advisor's tool response. The
nested call itself is not stored on the provider side.

## Troubleshooting

### The advisor tool never appears

Verify that **Advisor** is enabled under **Agents** > **Settings** >
**Experiments**. If the chat is in plan mode or is a subagent chat,
the advisor is filtered out by design and is not available.

### Agents hit the advisor limit too quickly

Increase **Max uses per run**, or set it to `0` for unlimited
invocations. The limit applies per chat run.

### The advisor uses the wrong model

If you set **Advisor model** to a model config that is later disabled
or deleted, the advisor falls back to the parent chat's model. Confirm
the model config you selected is still present and enabled under
**Agents** > **Settings** > **Models**.
