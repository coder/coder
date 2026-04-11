# Plan: Implement `completed` Chat Status via Tool-Based Self-Reporting

## Problem

Today, when a chat finishes processing successfully, it always transitions to `waiting`. There is no distinction between:

1. **The agent completed the user's task** (should be `completed`)
2. **The agent finished an intermediate step and is awaiting user input** (should stay `waiting`)

The `completed` status exists in the DB enum and SDK constants but is never set by production code. PR #24264 proposes removing it as dead code.

Instead, we want to **implement it**.

## Approach: `coder_report_status` Tool

Instead of making a separate post-hoc LLM call to classify the agent's output, give the agent a tool it calls as part of its normal work to report its own status. The classification piggybacks on the main agent call — **zero extra LLM calls, zero extra latency, zero extra token cost**.

The agent already has the best context for this decision: it knows what the user asked, what it did, and whether it's done. A separate classifier model would be working with a lossy summary of that same information.

### Why this beats a separate classification call

| | Tool approach | Separate LLM call |
|---|---|---|
| Extra LLM calls | 0 | 1 per chat completion |
| Extra latency | 0 | 1-3s (cheap model) to 15s (timeout) |
| Extra token cost | ~0 (tiny tool call) | ~2-3K input tokens |
| Classification context | Full conversation context | Truncated last message |
| Failure mode | Falls back to `waiting` | Falls back to `waiting` |
| Complexity | 1 tool + system prompt tweak | New function, model selection, retry logic |

## Implementation Details

### 1. New tool: `coder_report_status` in `chattool/`

Create `chattool/reportstatus.go`:

```go
type ReportStatusArgs struct {
    Status string `json:"status"`
    Summary string `json:"summary"`
}

type ReportStatusOptions struct {
    // OnReport is called when the agent reports its status.
    // This callback captures the reported status so the
    // caller can use it after the chat loop exits.
    OnReport func(status string, summary string)
}

func ReportStatus(opts ReportStatusOptions) fantasy.AgentTool {
    return fantasy.NewAgentTool(
        "coder_report_status",
        "Report your current status when you finish responding. "+
            "Call this tool once at the end of your final response.\n\n"+
            "status: one of:\n"+
            "  - \"complete\" — you finished the user's requested task\n"+
            "  - \"question\" — you are asking the user a question or need their input\n"+
            "  - \"update\" — you finished an intermediate step but the overall task is not done\n\n"+
            "summary: a one-sentence summary of what you did or what you need (under 120 chars)",
        func(ctx context.Context, args ReportStatusArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
            status := strings.ToLower(strings.TrimSpace(args.Status))
            summary := strings.TrimSpace(args.Summary)
            if opts.OnReport != nil {
                opts.OnReport(status, summary)
            }
            return toolResponse(map[string]any{
                "ok": true,
            }), nil
        },
    )
}
```

Key design choices:
- **Simple enum with short names**: `complete`, `question`, `update` — easy for any model to produce, no ambiguity.
- **`summary` field**: bonus — we can reuse this for push notifications instead of the separate `generatePushSummary` call, further reducing LLM spend. (Follow-up optimization, not required for v1.)
- **`coder_` prefix**: namespaced to avoid collision with MCP or user-defined tools.
- **Callback-based**: the tool handler captures the status via `OnReport` closure. The chat loop doesn't need to know about status semantics.

### 2. Register the tool in `runChat`

In `chatd.go`, where the tools slice is built (~line 4976):

```go
var reportedStatus string
var reportedSummary string

tools = append(tools, chattool.ReportStatus(chattool.ReportStatusOptions{
    OnReport: func(status string, summary string) {
        reportedStatus = status
        reportedSummary = summary
    },
}))
```

After `chatloop.Run` completes, capture the reported status into `runChatResult`:

```go
result.ReportedStatus = reportedStatus
result.ReportedSummary = reportedSummary
```

### 3. Extend `runChatResult`

```go
type runChatResult struct {
    FinalAssistantText      string
    ReportedStatus          string    // NEW: "complete", "question", "update", or ""
    ReportedSummary         string    // NEW: agent's self-reported summary
    PushSummaryModel        fantasy.LanguageModel
    ProviderKeys            chatprovider.ProviderAPIKeys
    PendingDynamicToolCalls []chatloop.PendingToolCall
}
```

### 4. Map reported status in `processChat`'s defer

In the deferred cleanup, after `runChat` returns successfully and status defaults to `ChatStatusWaiting`:

```go
// Map the agent's self-reported status to a database status.
// Only "complete" upgrades from waiting → completed. All other
// values (including empty/unrecognized) remain waiting, which
// preserves today's behavior as the safe default.
if status == database.ChatStatusWaiting && runResult.ReportedStatus == "complete" {
    status = database.ChatStatusCompleted
}
```

This is intentionally simple. The mapping is:

| Agent reports | DB status | Rationale |
|---|---|---|
| `"complete"` | `ChatStatusCompleted` | Agent says it's done |
| `"question"` | `ChatStatusWaiting` | Awaiting user input |
| `"update"` | `ChatStatusWaiting` | Intermediate progress |
| `""` (not called) | `ChatStatusWaiting` | Safe default, same as today |
| anything else | `ChatStatusWaiting` | Unrecognized = safe default |

### 5. System prompt addition

Add a brief instruction to the system prompt (in the deployment/user system prompt assembly, around line 912 in `chatd.go`) so the agent knows to call the tool:

```
When you finish responding, call the coder_report_status tool to report
your status. Use "complete" when you've finished the task, "question"
when you need user input, or "update" for intermediate progress.
```

This is a soft instruction — the agent may not always comply, but that's fine because the default is `waiting` (today's behavior). Over time, model compliance can be measured and the instruction tuned.

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

### Agent doesn't call the tool

Status stays `waiting` — identical to today's behavior. No regression. This handles:
- Models that ignore tool instructions
- Interrupted chats
- Error paths
- `requires_action` (dynamic tool) paths (we only check when status == `waiting`)

### Agent calls the tool multiple times

The `OnReport` callback overwrites on each call. The **last** call wins, which is correct — the agent's final status report is the most accurate.

### Agent calls it mid-conversation (not at the end)

Same as above — later calls overwrite earlier ones. If the agent reports `complete` mid-way then keeps working and reports `update`, the final status is `update` → `waiting`. Correct.

### Agent reports `complete` but then makes more tool calls

The chatloop continues executing. If the agent calls `coder_report_status` again with a different value, it overwrites. If it doesn't call again, the `complete` sticks. This is acceptable — the agent said it was done, and the subsequent tool calls were part of wrapping up.

### Push notification summary reuse

The `ReportedSummary` field can optionally replace or supplement the existing `generatePushSummary` LLM call. If `ReportedSummary` is non-empty, skip the push summary generation entirely. This is a clean follow-up optimization.

## Testing Strategy

1. **Unit test for `ReportStatus` tool**: Verify the callback fires with correct args, verify the tool response.
2. **Unit test for status mapping**: Verify `"complete"` → `ChatStatusCompleted`, all others → `ChatStatusWaiting`.
3. **Integration test**: End-to-end chat where the mock LLM calls `coder_report_status` with `complete` → verify DB status is `completed`.
4. **Fallback test**: Chat where the LLM never calls the tool → verify DB status is `waiting`.
5. **Overwrite test**: LLM calls the tool twice with different values → verify last value wins.

## Files to Modify

| File | Change |
|---|---|
| `coderd/x/chatd/chattool/reportstatus.go` | New file: `ReportStatus` tool implementation |
| `coderd/x/chatd/chattool/reportstatus_test.go` | New file: unit tests |
| `coderd/x/chatd/chatd.go` | Extend `runChatResult`, register tool in `runChat`, map status in `processChat` defer |
| `coderd/x/chatd/chatd.go` (~line 912) | Add system prompt instruction to call the tool |
| `coderd/x/chatd/chatd_test.go` or integration test | End-to-end test |
| `docs/ai-coder/agents/chats-api.md` | Add `completed` status to docs table |

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Agent doesn't call the tool | Default to `waiting` — no regression from today. |
| Agent misreports status | Only `complete` changes behavior; `question`/`update`/unknown all → `waiting`. False positives (saying complete when not) are the main risk, but models are generally reliable at self-assessment. |
| Tool adds noise to tool list | One extra tool definition in the prompt. Negligible token overhead (~50 tokens for the definition). |
| Older models ignore the tool | Same as "doesn't call" — safe default. |
| System prompt instruction ignored | Graceful degradation to `waiting`. Can tune instruction wording over time. |
