# Plan: Implement `completed` Chat Status via LLM Classification

## Problem

Today, when a chat finishes processing successfully, it always transitions to `waiting`. There is no distinction between:

1. **The agent completed the user's task** (should be `completed`)
2. **The agent finished an intermediate step and is awaiting user input** (should stay `waiting`)

The `completed` status exists in the DB enum and SDK constants but is never set by production code. PR #24264 proposes removing it as dead code.

Instead, we want to **implement it** by using a lightweight LLM call to classify the agent's final message.

## Approach: Post-Processing LLM Classification

After `runChat` returns successfully (status would be `waiting`), make an async LLM call to classify the agent's final output into one of:

| Classification | Maps to DB Status | Meaning |
|---|---|---|
| `completed` | `ChatStatusCompleted` | Agent finished the user's requested task |
| `waiting_for_input` | `ChatStatusWaiting` | Agent is asking a question or needs user direction |
| `intermediate` | `ChatStatusWaiting` | Agent finished a step but the overall task isn't done |

### Two Modes

**Mode A — Response-only classification (default, cheaper)**
Only send the agent's final text. The LLM decides based solely on the tone/content of the response (e.g., "I've completed the changes and opened PR #1234" → `completed`; "Which approach would you prefer?" → `waiting_for_input`).

**Mode B — Prompt-aware classification (opt-in, more accurate)**
Send both the original user prompt and the agent's final text. The LLM compares the request against the response to judge task completion. More accurate for ambiguous responses like "Done." where the user's original ask provides necessary context.

The implementation should support both modes. Mode B is strictly better in accuracy but costs ~2x the input tokens. Let the caller decide (or always use Mode B given the prompts are short — see token analysis below).

### Token Budget Analysis

We're capping `FinalAssistantText` at `maxConversationContextRunes` (6000 runes ≈ ~1500-2000 tokens). The system prompt is ~50 tokens. The initial user message is typically short (< 200 tokens). So even in Mode B, total input is well under 3K tokens. With a 16-token structured output, this is extremely cheap on any lightweight model. **Recommendation: always use Mode B — the marginal cost is negligible and the accuracy gain is significant.**

## Implementation Details

### 1. New function in `quickgen.go`

Add a `classifyCompletionStatus` function following the exact same pattern as `generatePushSummary`:

```go
type completionClassification struct {
    Status string `json:"status" description:"One of: completed, waiting_for_input, intermediate"`
}

const completionClassificationPrompt = `Classify the agent's response status. ` +
    `Given the user's original request and the agent's final response, determine if:` +
    `- "completed": The agent finished the user's requested task.` +
    `- "waiting_for_input": The agent is asking the user a question or needs direction.` +
    `- "intermediate": The agent finished a processing step but the overall task is not done.` +
    `Return only the status field.`
```

Use `object.Generate[completionClassification]` (structured output) with `maxOutputTokens = 32` to force a clean enum response. This is the same pattern used for title generation.

The function signature:

```go
func classifyCompletionStatus(
    ctx context.Context,
    userPrompt string,      // first user message text (Mode B context)
    assistantText string,    // FinalAssistantText from runChatResult
    fallbackModel fantasy.LanguageModel,
    keys chatprovider.ProviderAPIKeys,
    chat database.Chat,
    logger slog.Logger,
) database.ChatStatus
```

- Uses `preferredTitleModels` candidate list (same cheap models as title/push).
- Falls back to the chat's model.
- Returns `ChatStatusWaiting` on any failure (safe default — same behavior as today).
- 15-second timeout (faster than title gen since input is smaller).

### 2. Extract initial user prompt

Add a helper to pull the first user message text from the loaded messages:

```go
func firstUserMessageText(messages []database.ChatMessage) string
```

This iterates `messages`, skips `VisibilityModel` entries, and returns the text content of the first `RoleUser` message. This data is already loaded in `runChat` — we just need to capture it into `runChatResult`.

### 3. Extend `runChatResult`

```go
type runChatResult struct {
    FinalAssistantText      string
    InitialUserPrompt       string  // NEW: first user message text
    PushSummaryModel        fantasy.LanguageModel
    ProviderKeys            chatprovider.ProviderAPIKeys
    PendingDynamicToolCalls []chatloop.PendingToolCall
}
```

Populate `InitialUserPrompt` in `runChat` right after messages are loaded:

```go
result.InitialUserPrompt = firstUserMessageText(messages)
```

### 4. Call classification in `processChat`'s defer

In the deferred cleanup of `processChat`, after the current logic that defaults `status = ChatStatusWaiting`, add the classification step. This runs **before** the DB transaction that commits the final status:

```go
// After runResult is available and status == ChatStatusWaiting:
if status == database.ChatStatusWaiting && runResult.FinalAssistantText != "" {
    classifiedStatus := classifyCompletionStatus(
        cleanupCtx,
        runResult.InitialUserPrompt,
        runResult.FinalAssistantText,
        runResult.PushSummaryModel,
        runResult.ProviderKeys,
        chat,
        logger,
    )
    status = classifiedStatus
}
```

**Important**: This must happen synchronously (not in a goroutine) because the status is committed in the DB transaction that follows in the same defer. The 15s timeout caps worst-case latency.

### 5. Sync vs Async Tradeoff

**Option A — Synchronous (recommended):**
Run classification before the DB transaction in the defer. Adds up to 15s latency to status finalization in the worst case, but the classification call typically completes in 1-3s on cheap models. The user sees `running` → `completed`/`waiting` in one transition.

**Option B — Async with two-phase status:**
Set `waiting` immediately, fire classification async, then update to `completed` if warranted. This is faster but introduces a visible `waiting` → `completed` transition flicker. Also requires a second DB write and pubsub publish.

**Recommendation: Option A (synchronous).** The latency is acceptable since:
- The user already waited for the full agent run (seconds to minutes).
- An extra 1-3s for classification is imperceptible.
- It avoids the complexity of two-phase status updates.
- The push notification path already blocks on a similar LLM call.

### 6. Database / Migration

No schema changes needed. `completed` already exists in the `chat_status` PostgreSQL enum. The SDK constants (`ChatStatusCompleted`) and DB model (`ChatStatusCompleted`) are already defined. The only change PR #24264 proposed was removing it from docs — instead we'll be populating the docs with the newly-working status.

### 7. Update docs

Update `docs/ai-coder/agents/chats-api.md` to include `completed` with accurate semantics:

```
| `completed` | Agent finished the user's requested task.              |
```

### 8. Frontend considerations

The frontend likely already handles `completed` since the enum existed in the SDK. If the UI treats `completed` the same as `waiting` (idle state), that's fine initially. A follow-up can add a visual indicator (e.g., checkmark badge).

## System Prompt Design

The classification prompt should be minimal to reduce token spend and latency:

```
Classify this agent response. Return one status:
- "completed": The agent finished the requested task.
- "waiting_for_input": The agent needs user input or is asking a question.
- "intermediate": The agent finished a step but not the full task.

User's request:
<request>{truncated_user_prompt}</request>

Agent's response:
<response>{truncated_assistant_text}</response>
```

Key design choices:
- No chain-of-thought / reasoning requested — just bucketing.
- XML tags for clear boundary delineation.
- Structured output (`object.Generate`) forces valid enum response.
- `maxOutputTokens = 32` — we only need one short string.
- Truncate inputs: user prompt to 1000 runes, assistant text to 2000 runes (last 2000, not first — the conclusion matters more than the middle).

## Testing Strategy

1. **Unit tests for `classifyCompletionStatus`**: Mock the LLM model, verify it returns `ChatStatusCompleted` / `ChatStatusWaiting` based on mock responses.
2. **Unit tests for `firstUserMessageText`**: Verify extraction from various message arrays.
3. **Integration test**: Verify end-to-end that a chat processing a simple "create a file" task ends in `completed` status.
4. **Fallback test**: Verify that LLM failures gracefully fall back to `ChatStatusWaiting`.
5. **Test the structured output parsing**: Ensure invalid LLM outputs (e.g., random text) map to `ChatStatusWaiting`.

## Files to Modify

| File | Change |
|---|---|
| `coderd/x/chatd/quickgen.go` | Add `classifyCompletionStatus`, `completionClassification` struct, prompt constant, `firstUserMessageText` helper |
| `coderd/x/chatd/chatd.go` | Extend `runChatResult` with `InitialUserPrompt`, populate it in `runChat`, call classification in `processChat` defer |
| `coderd/x/chatd/quickgen_test.go` | Unit tests for classification and prompt extraction |
| `coderd/x/chatd/chatd_test.go` or integration test | End-to-end test |
| `docs/ai-coder/agents/chats-api.md` | Add `completed` status back to docs table |

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| LLM misclassifies → wrong status | Safe default to `waiting` on any error or ambiguity. `waiting` is strictly correct (just less informative). |
| Added latency to status finalization | 15s timeout cap; cheap models typically respond in 1-3s. Negligible relative to full agent run time. |
| Token cost | ~2-3K input tokens per classification on the cheapest available model. Comparable to existing push summary cost. |
| LLM provider unavailable | Fallback chain tries multiple providers (same as title gen). Ultimate fallback = `waiting`. |
| `intermediate` misuse | Both `intermediate` and `waiting_for_input` map to `ChatStatusWaiting`, so misclassification between these two is harmless. |
