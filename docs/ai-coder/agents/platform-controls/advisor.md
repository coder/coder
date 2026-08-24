# Advisor

> [!NOTE]
> This feature is experimental. Pin a release before broad rollout and review
> the release notes before upgrading.

## Enable the experiment

```sh
coder server --experiments=chat-advisor
```

Or set the environment variable:

```sh
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

Once the experiment is enabled, configure the advisor's runtime limits
under **AI Settings** > **Coder Agents** > **Advisor**. These limits apply
deployment-wide.

| Field             | Default              | Notes                                                                                                                   |
|-------------------|----------------------|-------------------------------------------------------------------------------------------------------------------------|
| Max uses per turn | `0` (unlimited)      | Caps how many times the root agent can call the advisor in a single chat turn. Must be a non-negative integer.          |
| Max output tokens | `0` (server default) | Caps the advisor model's response length. `0` uses the server default of 16,384 tokens. Must be a non-negative integer. |

The advisor model and its reasoning effort are organization-scoped [model overrides](../models.md#model-overrides).
Configure them under **AI Settings** > **Coder Agents** > **Organization settings** for the selected organization.
When no override is set, the advisor reuses the root agent's model.

The advisor is not available in plan mode or to subagents.
Failed advisor invocations refund the per-turn budget.

The same configuration is available through the API: runtime limits at
`GET`/`PUT` `/api/experimental/chats/config/advisor`, and the advisor
model override at
`PUT /api/experimental/organizations/{organization}/chats/model-overrides/advisor`.
