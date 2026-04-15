# Fix: Re-inject context files and skills on each turn

## Problem

In `/agents`, context files (AGENTS.md) and skills are only fetched from the workspace on the **first turn** or when the workspace agent changes. On subsequent turns, `instructionFromContextFiles(messages)` reads stale content from persisted messages. Additionally, the `ReloadMessages` callback (used after compaction) captures `instruction` and `skills` from the outer scope and never re-derives them.

This means if AGENTS.md or skills change on the workspace between turns, the agent won't see the updates until the user creates a new chat.

## Solution

### 1. Extract workspace context fetching (`chatd.go`)

Refactor `persistInstructionFiles` to extract the "fetch" logic into a separate `fetchWorkspaceContext` helper that retrieves fresh AGENTS.md and skill metadata from the workspace agent **without** persisting.

### 2. Always re-fetch context on each turn (`chatd.go` — `runChat`)

In the `else if hasContextFiles` branch (subsequent turns), call the new fetch helper to get fresh context from the workspace instead of reading from persisted messages. Fall back to persisted messages if the workspace dial fails.

### 3. Re-derive instruction/skills in `ReloadMessages` (`chatd.go`)

Update the `ReloadMessages` callback to re-derive `instruction` and `skills` from the reloaded database messages instead of using the stale captured closure variables. This ensures compaction recovery uses the latest persisted content.

## Files Changed

- `coderd/x/chatd/chatd.go` — `runChat` and new `fetchWorkspaceContext` helper
- `coderd/x/chatd/chatd_internal_test.go` or `chatd_test.go` — test updates if needed
