---
title: Token budget
---

> [!NOTE]
> This feature is experimental and can change between releases.

Token budget is a compaction strategy for Coder Agents chats that starts a fresh context window instead of summarizing older context.
This page is for chat owners: it shows how to turn on Token budget for your chats and what happens when a context window fills up.

## Prerequisites

An administrator must enable the `chat-token-budget` experiment on the control plane with the `CODER_EXPERIMENTS` environment variable:

```sh
CODER_EXPERIMENTS=chat-token-budget
```

Or pass the flag to `coder server`:

```sh
coder server --experiments=chat-token-budget
```

## Turn on Token budget for your chats

1. Open the **Agents** page.
1. Go to **Settings** > **Compaction**.
1. Set **Compaction strategy** to **Token budget**.

The choice applies to every chat you own, including sub-agent chats, from the next model step on.
**Summarize** is the default and keeps the [summarizing behavior](../architecture.md#context-compaction).
To read your current strategy from a script, send `GET` to `/api/experimental/chats/config/user-compaction-strategy`.
To set it, send `PUT` to the same path with a JSON body such as `{"strategy":"token_budget"}`.
The valid values are `summarize` and `token_budget`.

## When a context window ends

Under Token budget, the agent doesn't summarize automatically.
The per-model threshold on the same settings page becomes the **Handoff target**, and each context window passes through up to 3 stages:

1. **Handoff target**: Coder asks the agent to finish its current small unit of work, save a checkpoint, and request a new context window.
   The transcript shows `Context handoff requested`.
1. **Final prompt**: if the window keeps growing, Coder sends one last prompt near the usable limit.
   It tells the agent to save its checkpoint and request a new context window, and to do nothing else.
   The transcript shows `Context window ending: final handoff`.
1. **Forced rollover**: Coder starts a new context window itself when the window reaches the usable limit, or when the provider rejects a request as too large for the model.

Each stage fires at most once per context window.
The usable limit is the model's context limit minus the smaller of two values: its **Max Output Tokens** [model option](../models.md#general-options) (32,000 when unset) and a quarter of the context limit.
The final prompt fires 16,000&nbsp;tokens below the usable limit, or at half of it if that point is higher.
The handoff target is the threshold percentage of the context limit, at least 10%, and never later than the final prompt.
For example, a model with a 200,000-token context limit, no **Max Output Tokens** value, and a 70% threshold reaches its handoff target at 140,000&nbsp;tokens.
It gets the final prompt at 152,000&nbsp;tokens and rolls over at 168,000&nbsp;tokens.

A threshold of 100% shows as `Automatic rollover disabled`.
No stage fires, and Coder refuses the agent's requests for a new context window.
For a model without a context limit, Coder skips the handoff target, the final prompt, and the rollover at the usable limit.
If its threshold is below 100%, Coder still honors the agent's own requests and still starts a new context window when the provider rejects a request as too large.

The handoff target, the final prompt, and the rollover at the usable limit use the token usage the provider reported for the previous model step.
A single large tool result can therefore pass the usable limit before Coder checks it, and the provider rejection then triggers the forced rollover.

## Agent tools

Under Token budget, the agent gets 3 extra tools.

### `save_checkpoint`

Saves a checkpoint of up to 8&nbsp;KiB with the goal, decisions, progress, and next steps.
Each save replaces the previous checkpoint.

### `new_context`

Asks Coder to start a new context window once the other tool calls in the same step finish.
Coder refuses the request when the threshold is 100%.

### `chat_history`

Reads this chat's transcript across context windows.
It lists windows, lists or searches messages with a literal, case-insensitive match, and reads one message in pages.
It returns message text, tool calls and results, and file names, but not reasoning.
It can't read messages before the latest `/clear`.

## What the new context window contains

A context window that a rollover starts carries no summary of the previous one.
It opens with a short lead-in that explains why the window changed and includes the agent's latest checkpoint, if it saved one.
The agent retrieves earlier detail, such as a completed tool result, with `chat_history` instead of running the tool again.
A message you sent that the agent hasn't answered yet carries over into the new window.
The transcript shows a `Context window rollover` card with the reason and the checkpoint the new window started from, if the agent saved one.
Earlier messages stay visible.

## Differences from Summarize

Some behavior stays the same under Token budget, and some doesn't apply:

- Sending `/compact` still summarizes the conversation, and the summary starts a new context window.
- Sending `/clear` still resets the context.
  It also discards earlier checkpoints, and `chat_history` can't read messages before it.
- Editing a message discards checkpoints the agent saved after it, together with the rest of the later conversation.
- Coder doesn't dispatch the `pre_compact` and `post_compact` [lifecycle hooks](../../../admin/setup/chat-lifecycle-hooks.md) for a rollover, because a rollover doesn't summarize.
- `chat_history` reads only the current chat, not its sub-agent chats.
- If the provider rejects the first request of a new context window as too large, for example because of a large attachment, the chat fails with an error as it does under Summarize.
- An organization's **Compaction** [model override](../models.md#model-overrides) applies only to `/compact`, because automatic summarizing doesn't run.

If an administrator turns off the experiment, every chat uses Summarize again, and Coder keeps your saved choice for when the experiment returns.
The [Prometheus metrics](../../../admin/integrations/prometheus.md) `coderd_chatd_context_handoff_total` and `coderd_chatd_context_rollover_total` count handoff prompts and rollovers.
