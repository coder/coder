# Plan: Implement `completed` Chat Status via Tool-Based Self-Reporting

## Problem

Today, when a chat finishes processing successfully, it always transitions to `waiting`. There is no distinction between:

1. **The agent completed the user's task** (should be `completed`)
2. **The agent finished an intermediate step and is awaiting user input** (should stay `waiting`)

The `completed` status exists in the DB enum and SDK constants but is never set by production code. PR #24264 proposes removing it as dead code.

Instead, we want to **implement it**.

## Approach: Cheap LLM Classification (Post-Processing)

After `runChat` completes successfully, call a cheap model
(haiku / gpt-4o-mini / flash) with the tail of the agent's final
response. The classifier buckets it into one of three categories:
`complete`, `question`, or `update`. Only `complete` changes the
DB status to `ChatStatusCompleted`; everything else stays `waiting`.

The signal for this classification lives almost entirely in the
agent's last few sentences. "I've opened PR #1234 for your review"
is obviously done. "Which approach would you prefer?" is obviously
waiting. You don't need the full 200K-token conversation history
to make this call.

### Approach comparison: tool vs. separate LLM call vs. text marker

Three approaches were evaluated. Each is analyzed for cost, latency,
reliability, and implementation complexity.

#### Approach A — `coder_report_status` tool (CHOSEN)

The agent gets a tool it calls at the end of its work to self-report
its status. A callback captures the value; `processChat` maps it to
a DB status.

**Critical tradeoff: the extra round trip.** The chatloop's
continuation logic is:

```go
result.shouldContinue = hasLocalToolCalls &&
    result.finishReason == fantasy.FinishReasonToolCalls
```

When the agent calls `coder_report_status`, `shouldContinue` is true.
The tool executes, its result is sent back, and the model generates
**one more full response** on the main model with the full
conversation context. This is the dominant cost.

**Cost estimate per invocation (extra round trip):**

| Model | Context size | Extra input cost | Extra output (~50 tok) | Total |
|---|---|---|---|---|
| Claude Sonnet ($3/$15 per MTok) | 50K tokens | $0.15 | $0.001 | ~$0.15 |
| GPT-4o ($2.50/$10 per MTok) | 50K tokens | $0.125 | $0.001 | ~$0.13 |
| Claude Haiku ($1/$5 per MTok) | 50K tokens | $0.05 | <$0.001 | ~$0.05 |
| Any model | 200K tokens | 4x above | ~same | 4x above |

**When the cost is zero:** If the model happens to call
`coder_report_status` alongside other tool calls on a step that
already triggers another round trip (e.g., the agent calls
`write_file` + `coder_report_status` in the same step), the extra
round trip was already happening. But this is the optimistic case —
in the common "final step" scenario, the status tool is the only
tool call and the extra round trip is unavoidable.

**Other downsides:**
- The extra round trip produces an additional assistant message
  that gets persisted, cluttering the conversation history.
- Adds ~50 tokens to the tool list definition on every step of
  every chat, not just the final one.

#### Approach B — Separate cheap LLM call (post-hoc classification)

After `runChat` returns, call a cheap model (haiku/gpt-4o-mini/flash)
with the user's original prompt + the agent's final text to classify
the status.

**Cost estimate per invocation:**

| Model | Input tokens | Cost |
|---|---|---|
| Claude Haiku | ~2.5K | ~$0.003 |
| GPT-4o-mini | ~2.5K | ~$0.001 |
| Gemini Flash | ~2.5K | ~$0.001 |

**This is 50-150x cheaper than the tool approach** when the main
model is Sonnet/GPT-4o, because it uses a cheap model with
truncated input instead of the main model with full context.

**Downsides:**
- Adds 1-3s latency after processing (15s timeout cap).
- Requires model selection logic and retry handling (but this
  pattern already exists in `quickgen.go` for push summaries).
- The classifier has less context than the agent itself.

#### Approach C — Text marker parsing

Instruct the agent to end its response with a structured tag like
`[TASK_COMPLETE]` or `[AWAITING_INPUT]`. Parse `FinalAssistantText`
for these markers after the loop exits.

**Cost:** Zero — no extra calls, no extra round trips.

**Downsides:**
- Fragile. Model compliance varies, especially across providers.
- Pollutes the visible assistant response with machine-readable tags.
- Harder to validate (regex parsing vs. structured output).
- No clean fallback when the model doesn't produce the tag.

#### Verdict

| | Tool (A) | Cheap LLM (B) | Text marker (C) |
|---|---|---|---|
| Extra cost per call | ~$0.05-0.60 (main model) | ~$0.001-0.003 (cheap model) | $0 |
| Extra latency | 2-10s (full model turn) | 1-3s (cheap model) | 0 |
| Classification context | Full (best) | Truncated (good) | Full (best) |
| Reliability | Good (structured tool) | Good (structured output) | Fragile (regex) |
| Conversation clutter | Yes (extra message) | No | Yes (visible tags) |
| Implementation complexity | Low | Medium | Low |
| Failure mode | `waiting` (safe) | `waiting` (safe) | `waiting` (safe) |

**Approach B (cheap LLM call) is recommended.** Here's why:

The classification task is simple text bucketing. The signal lives
almost entirely in the last few sentences of the agent's response:

- "I've opened PR #1234 for your review" → `completed`
- "Which approach would you prefer?" → `waiting_for_input`
- "I've set up the database, now working on the API" → `intermediate`

You don't need 50-200K tokens of conversation history to make this
call. A cheap model reading the tail of the agent's response gets
it right in the vast majority of cases. The marginal accuracy from
full context doesn't justify a 50-150x cost increase.

The tool approach (A) remains a viable alternative if:
- Classification accuracy proves insufficient without full context.
- A "terminal tool" concept is added to the chatloop, eliminating
  the extra round trip cost. At that point the tool approach
  becomes strictly better (zero cost, full context, same
  reliability).

Both approaches share the same status mapping, DB interaction,
frontend behavior, and failure mode (`waiting` as safe default),
so switching between them later is straightforward.

## Implementation Details (Approach B — Cheap LLM Classification)

### 1. New function in `quickgen.go`

Add `classifyCompletionStatus` following the same pattern as
`generatePushSummary`:

```go
type completionClassification struct {
    Status string `json:"status" description:"One of: complete, question, update"`
}

const completionClassificationPrompt = `Classify this agent response into exactly one status:
- "complete": The agent finished the user's requested task.
- "question": The agent is asking the user a question or needs input.
- "update": The agent finished an intermediate step but the task is not done.
Return only the status field.`
```

Function signature:

```go
func classifyCompletionStatus(
    ctx context.Context,
    assistantText string,
    fallbackModel fantasy.LanguageModel,
    keys chatprovider.ProviderAPIKeys,
    chat database.Chat,
    logger slog.Logger,
) database.ChatStatus
```

Key implementation details:
- Uses `preferredTitleModels` candidate list (same cheap models as
  title/push generation: haiku, gpt-4o-mini, flash).
- Falls back to the chat's model if no cheap model is configured.
- Uses `object.Generate[completionClassification]` (structured
  output) with `maxOutputTokens = 32` to force a clean enum.
- Truncates `assistantText` to the **last** 2000 runes (the
  conclusion matters more than the middle).
- 15-second timeout.
- Returns `ChatStatusWaiting` on any failure (safe default).

The input to the classifier is minimal:

```
Agent's response:
{truncated_tail_of_assistant_text}
```

No user prompt needed. The signal is in the agent's last few
sentences, not in what was originally asked.

### 2. Extend `runChatResult`

No changes needed. `FinalAssistantText` is already captured.

### 3. Call classification in `processChat`'s defer

In the deferred cleanup, after `runChat` returns successfully and
status defaults to `ChatStatusWaiting`:

```go
if status == database.ChatStatusWaiting &&
    strings.TrimSpace(runResult.FinalAssistantText) != "" {
    classifiedStatus := classifyCompletionStatus(
        cleanupCtx,
        runResult.FinalAssistantText,
        runResult.PushSummaryModel,
        runResult.ProviderKeys,
        chat,
        logger,
    )
    status = classifiedStatus
}
```

This runs synchronously before the DB transaction that commits the
final status. The 15s timeout caps worst-case latency, and typical
cheap-model responses complete in 1-3s.

The mapping:

| Classifier returns | DB status | Rationale |
|---|---|---|
| `"complete"` | `ChatStatusCompleted` | Agent finished the task |
| `"question"` | `ChatStatusWaiting` | Awaiting user input |
| `"update"` | `ChatStatusWaiting` | Intermediate progress |
| error / unrecognized | `ChatStatusWaiting` | Safe default, same as today |

---

## Alternative: Tool-Based Self-Reporting (Approach A)

Documented here as a viable alternative if classification accuracy
proves insufficient, or if a "terminal tool" concept is later added
to the chatloop (eliminating the extra round trip cost).

### How it works

Add a `coder_report_status` tool to `chattool/`. The agent calls
it at the end of its work to self-report `complete`, `question`,
or `update`. A callback captures the value.

```go
type ReportStatusArgs struct {
    Status  string `json:"status"`
    Summary string `json:"summary"`
}

func ReportStatus(opts ReportStatusOptions) fantasy.AgentTool {
    return fantasy.NewAgentTool(
        "coder_report_status",
        "Report your current status when you finish responding. ...",
        func(ctx context.Context, args ReportStatusArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
            if opts.OnReport != nil {
                opts.OnReport(args.Status, args.Summary)
            }
            return toolResponse(map[string]any{"ok": true}), nil
        },
    )
}
```

Register it in `runChat`, capture via `runChatResult.ReportedStatus`,
map in `processChat`'s defer — same status mapping as approach B.

### Why it's deferred

The chatloop requires a full model round trip after any tool call.
This makes the tool approach 50-150x more expensive per invocation
than a cheap classifier (see cost table above). If a "terminal
tool" concept is added to `chatloop` — tools that execute but
skip the result-back-to-model step — this approach becomes
strictly better: zero cost, full context, same reliability. Until
then, the cost is hard to justify for a text-bucketing task.

### `summary` field bonus

The tool's `summary` arg could replace `generatePushSummary`,
eliminating another LLM call. This is a secondary benefit worth
revisiting if/when the terminal tool path opens up.

### 6. Database / Migration

No schema changes needed. `completed` already exists in the `chat_status` PostgreSQL enum, SDK constants, and DB models.

### 7. Update docs

Update `docs/ai-coder/agents/chats-api.md` to include `completed`:

```
| `completed` | Agent reports it finished the user's requested task.   |
```

### 8. Frontend considerations

The frontend already has `ChatStatusCompleted` in the SDK enum. If the UI currently treats it identically to `waiting` (idle state), that's fine as a starting point. A follow-up can add a visual indicator (checkmark, "Task complete" label, etc.).

## Edge Cases & Robustness

### Classifier returns unexpected value

Anything other than `"complete"` maps to `ChatStatusWaiting`. This
includes empty strings, typos, or structured output parse failures.
No regression from today's behavior.

### `FinalAssistantText` is empty

Skip classification entirely. Status stays `waiting`. This handles
runs that ended with only tool calls and no final text.

### Classification runs alongside push summary

Both `classifyCompletionStatus` and `generatePushSummary` use the
same cheap-model candidate list. They run at different points:
classification is synchronous in the defer (before DB commit),
push summary is async (after). No conflict.

### Push notification summary reuse (future)

If the tool approach (A) is later adopted, its `summary` field
could replace `generatePushSummary` entirely, eliminating another
LLM call. This is noted as a future optimization.

## Testing Strategy

1. **Unit test for `classifyCompletionStatus`**: Mock the LLM model, verify it returns `ChatStatusCompleted` / `ChatStatusWaiting` based on mock structured responses.
2. **Unit test for status mapping**: Verify `"complete"` → `ChatStatusCompleted`, all others → `ChatStatusWaiting`.
3. **Integration test**: End-to-end chat where the mock LLM produces a "task complete" final message → verify DB status is `completed`.
4. **Fallback test**: Classifier model fails / returns garbage → verify DB status is `waiting`.
5. **Truncation test**: Verify that only the tail of `FinalAssistantText` is sent to the classifier.

## Files to Modify

| File | Change |
|---|---|
| `coderd/x/chatd/quickgen.go` | Add `classifyCompletionStatus`, `completionClassification` struct, prompt constant |
| `coderd/x/chatd/chatd.go` | Call classification in `processChat` defer when status is `waiting` |
| `coderd/x/chatd/quickgen_test.go` | Unit tests for classification |
| `coderd/x/chatd/chatd_test.go` or integration test | End-to-end test |
| `docs/ai-coder/agents/chats-api.md` | Add `completed` status to docs table |

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Classifier misreads tone → wrong status | Only `complete` changes behavior; `question`/`update`/unknown all map to `waiting`. False positive (`completed` when not) is the main risk, but the signal in the agent's final text is usually unambiguous. |
| Added latency to status finalization | 15s timeout cap; cheap models respond in 1-3s. Negligible relative to full agent run time. |
| Token cost | ~2-3K input tokens on the cheapest available model (~$0.001-0.003). Comparable to existing push summary cost. |
| Cheap model unavailable | Fallback chain tries multiple providers (same as title gen). Ultimate fallback = `waiting`. |
| `intermediate` / `question` misclassification | Both map to `ChatStatusWaiting`, so confusion between them is harmless. |
