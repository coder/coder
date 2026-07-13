# Advisor

> [!NOTE]
> This feature is experimental. Pin a release before broad rollout and review
> the release notes before upgrading.

## Enable the experiment

```shell
coder server --experiments=chat-advisor
```

Or set the environment variable:

```shell
CODER_EXPERIMENTS=chat-advisor
```

## What it does

Lets a root agent pause its current turn and request strategic guidance from
a separate, single-step model call. The advisor sees recent conversation
context, runs without any tools, and returns concise advice for the root
agent rather than the end user. While active, it is the only tool the root
agent can call for that turn.

Useful for planning ambiguity, architectural tradeoffs, debugging strategy
after repeated failures, or risk reduction before a destructive operation.

## Configuration

Once the experiment is enabled, configure the advisor under **AI Settings** >
**Coder Agents** > **Advisor**.

| Field             | Default              | Notes                                                                                                                   |
|-------------------|----------------------|-------------------------------------------------------------------------------------------------------------------------|
| Max uses per turn | `0` (unlimited)      | Caps how many times the root agent can call the advisor in a single chat turn. Must be a non-negative integer.          |
| Max output tokens | `0` (server default) | Caps the advisor model's response length. `0` uses the server default of 16,384 tokens. Must be a non-negative integer. |
| Advisor model     | Use chat model       | Optional dedicated chat model config for the advisor. When unset, the advisor reuses the root agent's model.            |

The advisor is not available in plan mode or to subagents. Failed advisor
invocations refund the per-turn budget, and advisor calls are not metered
against the root chat's usage limit.

The same configuration is available at:

- `GET /api/experimental/chats/config/advisor`
- `PUT /api/experimental/chats/config/advisor`
