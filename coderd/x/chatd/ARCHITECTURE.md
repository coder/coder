# chatd — Architecture & State of the Code

> **Status**: Early Access (formerly experimental).
> Moved from `coderd/chatd/` to `coderd/x/chatd/` in March 2025.
> ~120 commits in the first month of development, ~26,700 lines of Go (including ~13,300 lines of tests).

## What is chatd?

`chatd` is the **server-side chat daemon** that powers Coder's "Agents" feature. It
processes user chat messages by orchestrating LLM calls, tool execution inside
workspaces, and real-time streaming of results to the frontend.

At a high level: a user sends a message → `chatd` acquires the pending chat →
calls the configured LLM → the LLM optionally invokes tools (shell commands,
file reads/writes, workspace creation, etc.) → results are persisted and
streamed back to the user.

---

## Package Layout

```
coderd/x/chatd/
├── chatd.go              # Core Server: polling, acquire, processChat, runChat (~3,900 lines)
├── prompt.go             # DefaultSystemPrompt constant
├── instruction.go        # AGENTS.md file reading from workspaces
├── subagent.go           # Delegated child agent (subagent) tools
├── usagelimit.go         # Spend-limit enforcement (per-user, per-period)
├── quickgen.go           # Lightweight LLM tasks: title generation, push summaries
│
├── chatloop/             # The step-stream loop (LLM call → tool exec → persist → repeat)
│   ├── chatloop.go       # Run(), processStepStream, tool execution
│   └── compaction.go     # Context window compaction (summarization)
│
├── chatprompt/           # Message serialization/deserialization (DB ↔ fantasy)
│   └── chatprompt.go     # ConvertMessages, ParseContent, content versioning (V0/V1)
│
├── chatprovider/         # LLM provider abstraction (model catalog, API key management)
│   ├── chatprovider.go   # ModelCatalog, ProviderAPIKeys, supported providers
│   └── useragent.go      # User-agent header construction for LLM API calls
│
├── chattool/             # Tool implementations
│   ├── chattool.go       # Shared helpers (toolResponse, truncateRunes)
│   ├── execute.go        # Shell command execution (foreground, background, process mgmt)
│   ├── readfile.go       # Line-numbered file reading
│   ├── writefile.go      # File writing
│   ├── editfiles.go      # Search-and-replace file editing
│   ├── computeruse.go    # Anthropic computer use (desktop screenshots, clicks, typing)
│   ├── listtemplates.go  # List workspace templates (with popularity sorting)
│   ├── readtemplate.go   # Get template details + parameters
│   ├── createworkspace.go # Create workspace from template (idempotent, waits for build)
│   └── startworkspace.go # Start a stopped workspace
│
├── chatcost/             # Token cost calculation (microdollar precision)
│   └── chatcost.go       # CalculateTotalCostMicros
│
├── chatretry/            # Retry logic for transient LLM errors
│   └── chatretry.go      # IsRetryable, Retry with exponential backoff
│
├── chattest/             # Test helpers (mock OpenAI/Anthropic servers)
│   ├── openai.go
│   ├── anthropic.go
│   └── errors.go
│
└── mcpclient/            # MCP (Model Context Protocol) server integration
    └── mcpclient.go      # ConnectAll, tool discovery, allow/deny lists
```

---

## Core Concepts

### 1. The Server (chatd.go)

`Server` is the central orchestrator. It runs a **polling loop** that:
1. Acquires pending chats from the database (up to `DefaultMaxChatsPerAcquire = 10` at a time)
2. Processes each chat in a separate goroutine
3. Heartbeats to prevent stale chat detection
4. Recovers stale chats (running for > 5 minutes without heartbeat)

Key configuration:
- `DefaultPendingChatAcquireInterval = 1s` — how often it polls for work
- `DefaultInFlightChatStaleAfter = 5min` — when a chat is considered orphaned
- `DefaultChatHeartbeatInterval = 30s` — heartbeat frequency
- `maxChatSteps = 1200` — absolute upper bound on LLM round-trips per chat turn

### 2. The Chat Loop (chatloop/)

`chatloop.Run()` is the inner execution engine. For each user message it:
1. Calls the LLM via `fantasy.LanguageModel.Stream()`
2. Processes the streaming response (text, reasoning, tool calls)
3. Executes any tool calls in parallel
4. Persists the step (assistant response + tool results)
5. Checks for context compaction needs
6. Repeats if the model wants to continue (tool calls present)

The loop has retry logic (`chatretry.Retry`) wrapping each LLM call with
exponential backoff for transient errors (rate limits, 5xx, etc.).

**Compaction** happens when context token usage exceeds a configurable threshold
(default 70%). It asks the LLM to summarize the conversation so far, persists
the summary, and reloads the message history. There is a
`maxCompactionRetries = 3` safety valve to prevent infinite loops.

### 3. Message Serialization (chatprompt/)

This package handles the gnarly problem of converting between:
- **Database format** (`database.ChatMessage` with `pqtype.NullRawMessage` content)
- **SDK format** (`codersdk.ChatMessagePart`)
- **LLM format** (`fantasy.Message` / `fantasy.MessagePart`)

There are **two content versions**:
- **V0 (legacy)**: Role-aware heuristic parsing. Assistant messages use structural
  heuristics to distinguish "fantasy envelope" format from SDK parts. This was the
  format during the initial experimental phase.
- **V1 (current)**: Clean `[]codersdk.ChatMessagePart` JSON for all roles.

The `ParseContent` function dispatches on `ContentVersion`. Legacy V0 parsing
includes fallback logic for tool results, assistant blocks, and system messages
that each had different serialization formats.

### 4. Provider Abstraction (chatprovider/)

Supported providers: Anthropic, Azure OpenAI, AWS Bedrock, Google, OpenAI,
OpenAI-Compatible, OpenRouter, Vercel AI Gateway.

The `ModelCatalog` resolves model names to `fantasy.LanguageModel` instances.
API keys come from a layered system: env presets → database config → static keys.

### 5. Tools (chattool/)

Tools are implemented as `fantasy.AgentTool` values. Each tool gets a
`GetWorkspaceConn` function that lazily connects to the workspace agent.

The execute tool has notable complexity:
- Detects shell-style backgrounding (trailing `&`) and promotes to background mode
- Sets non-interactive env vars (`GIT_TERMINAL_PROMPT=0`, `TERM=dumb`, etc.)
- Detects file-dump commands (`cat`, `grep -l`) and suggests `read_file` instead
- Handles foreground/background execution, process output retrieval, and signals
- Has a `snapshotTimeout = 30s` fallback for when blocking waits fail

### 6. MCP Client (mcpclient/)

Connects to external MCP servers configured in the database. Tools discovered
from MCP servers are prefixed with `serverSlug__toolName` using double-underscore
separation.

Auth types: `oauth2`, `api_key`, `custom_headers`, `none`.
Transport types: `sse`, `streamable_http` (default).

### 7. Subagents (subagent.go)

The parent chat can spawn child "subagent" chats via tools:
- `spawn_agent` — creates a child chat that runs independently
- `spawn_computer_use_agent` — child chat with Anthropic computer use (desktop)
- `wait_agent` — polls/pubsub-waits for child completion (default 5min timeout)
- `message_agent` — sends follow-up messages to a running child
- `close_agent` — interrupts and stops a child

Ancestry is verified via `isSubagentDescendant` which walks the parent chain
(O(depth) DB queries).

### 8. Usage Limits (usagelimit.go)

Spend limits are enforced per-user with configurable periods (day/week/month).
The limit resolution order is: individual override > group limit > global default.

The system **fails open** on errors — if the limit check fails, the chat proceeds.
There is an acknowledged race condition where concurrent messages can both pass the
limit check, bounded by `message_cost × concurrency`.

### 9. Cost Tracking (chatcost/)

Costs are calculated in **microdollars** (millionths of a dollar) and rounded up.
The system distinguishes "zero cost" from "unpriced" (nil return) to handle models
without pricing configuration.

Output tokens include reasoning tokens per provider semantics — adding
`ReasoningTokens` separately would double-count.

---

## Key Dependencies

- **`charm.land/fantasy`** — LLM abstraction layer (streaming, tool execution, multi-provider).
  Currently a **replace directive** pointing to `github.com/kylecarbs/fantasy`, a fork.
- **`github.com/mark3labs/mcp-go`** — MCP protocol client library.
- **`github.com/coder/quartz`** — Deterministic clock for testing.

---

## Streaming Architecture

Chat events flow through multiple channels:
1. **Local buffer** (`maxStreamBufferSize = 10000`): In-memory ring buffer of message_part events
2. **Pubsub**: Cross-replica notifications for multi-replica deployments
3. **Enterprise relay**: Optional remote relay for HA setups
4. **Durable message cache** (`maxDurableMessageCacheSize = 256`): Recent messages cached for same-replica catch-up

The `Subscribe` method merges all three sources into a single event channel.

---

## Message Queue

When a chat is busy (running/pending), new user messages are **queued** (up to 20).
Two modes:
- `SendMessageBusyBehaviorQueue` — appends to queue
- `SendMessageBusyBehaviorInterrupt` — interrupts the running chat

After a chat completes, it automatically promotes the next queued message.

---

## Known Technical Debt

1. **`chatd.go` needs decomposition.** At ~3,900 lines, this single file contains
   the Server struct, all HTTP-facing methods (Create/Send/Edit/Archive/Delete/
   Promote/Interrupt/RefreshStatus/Subscribe), the background processing loop,
   the full `runChat` orchestration, stream management, push notifications,
   model resolution, instruction resolution, and stale recovery. This is a
   direct result of fast iterative shipping during the experimental phase. It
   must be broken up before the codebase can scale to more contributors.
   Likely candidates for extraction: stream management, model/instruction
   resolution, push notifications, message queue handling.

2. **`charm.land/fantasy` fork must be reconciled with upstream.** The `go.mod`
   replaces the canonical `charm.land/fantasy` module with
   `github.com/kylecarbs/fantasy`. The primary blocker is a Go version mismatch:
   Coder is on Go 1.25, upstream fantasy requires Go 1.26. There are also
   fork-only features currently being upstreamed. Once the Go version is bumped
   and patches are merged upstream, the replace directive should be removed.

3. **Decide whether to drop content version V0 support.** The `chatprompt`
   package supports two content serialization versions. V0 is the legacy format
   from the experimental phase — it uses role-aware heuristics and has a
   comment explicitly calling out a "brittle invariant tied to Go's json decoder
   behavior." V1 is the clean current format. Since Agents was never officially
   released during the V0 era, the only users with V0 data are those running
   `main`. Decision needed: drop V0 (simpler code, breaks `main` runners) or
   keep it (more code to maintain, but no data loss for anyone).

---

## Open Questions

> These are things that looked unusual, surprising, or potentially problematic
> during the initial code review. They need human answers.

### Architecture

2. ~~**Why the `coderd/x/` prefix?**~~ **Resolved.** The `x/` prefix denotes an
   experimental package. Once the feature graduates from early access, `chatd`
   will be promoted out of `x/` back to `coderd/chatd/`.

3. **`charm.land/fantasy` is a fork.** ~~The `go.mod` replaces the canonical
   module with `github.com/kylecarbs/fantasy`. What is the relationship between
   Coder and this library? Is there a plan to upstream changes, or will this
   remain a fork? What happens when fantasy has breaking changes?~~
   **Resolved.** See Known Technical Debt item #2.

4. **Polling-based chat acquisition needs rethinking.** The server polls every
   1 second for pending chats (`DefaultPendingChatAcquireInterval`). With N
   idle replicas this means N queries/second of database load with zero pending
   work. Two architectural decisions are needed:
   - **Short-term**: Move to long-poll/block-until-acquire, consistent with how
     `provisionerdserver` acquires jobs.
   - **Long-term**: Decide whether `chatd` should be runnable as a standalone
     daemon (like `provisionerd`). If so, a DRPC interface and the associated
     plumbing will be needed. This is a significant architectural investment.

### Code Quality

5. ~~**Content version V0 parsing is complex.**~~ **Resolved.** See Known
   Technical Debt item #3.

6. **`maxChatSteps = 1200` is arbitrarily high and should be tuned.** This was
   set as a generous upper bound during experimental development. There should
   be sufficient internal usage data now to determine a realistic maximum and
   bring this down. TODO: query production chat step counts (p99/max) and set a
   defensible limit.

7. **The tool name separator `__` has a known ambiguity.** The TODO in
   `mcpclient.go` acknowledges that tool names containing `__` produce ambiguous
   prefixed names. No real-world issues reported yet, but worth exploring a
   more robust separator or escaping scheme before MCP adoption widens.

8. **Usage limits fail open — and may be in the wrong place.** The current
   fail-open behavior is a safety net while the feature is being ironed out;
   the intent is to eventually enforce limits strictly. Beyond that, there are
   larger architectural questions:
   - Should usage limit enforcement live inside `coderd/x/chatd`, or should it
     be a shared concern at a higher level?
   - How will this integrate with aibridge?
   - Will chatd continue to be the enforcement point, or will quotas move to a
     centralized gateway?

9. **Subagent nesting depth needs a policy decision.** The ancestry check in
   `isSubagentDescendant` is O(depth), but the more pressing question is: how
   deep should the family tree be allowed to go? Currently, line 360 of
   `subagent.go` enforces a hard limit of **one level** — a child chat with a
   `ParentChatID` cannot spawn further children. The underlying architecture
   (parent/root chain tracking) supports arbitrary depth, but the guard
   prevents it. TODO: decide on a nesting policy. If depth > 1 is ever
   allowed, the O(depth) ancestry walk should be revisited (e.g. materialized
   path or depth column).

### Testing

10. ~~**No documentation or README existed before this file.**~~ **Resolved.**
    Test philosophy follows the rest of the codebase: tests all the time, in CI,
    on PRs, and locally. The `chatd` package tests take ~38s on beefy dev boxes.
    Common codebase-wide pattern: lots of tests, long runtimes, occasional
    flakiness. No chatd-specific test strategy beyond that.

11. **Most providers lack integration test coverage.** `chattest/` has mock
    servers for OpenAI and Anthropic only. The remaining 6 providers (Azure,
    Bedrock, Google, OpenRouter, OpenAI-Compatible, Vercel) are largely
    untested and not all are configured in the dogfood environment. TODO: set
    up real-world integration tests for each provider using cheap/fast models
    to catch provider-specific serialization or behavioral differences.

### Operational

12. **No Prometheus metrics or alerting for chatd.** Stale chat recovery (5-min
    threshold, 30s heartbeat = ~10 missed beats) has no monitoring. More
    broadly, the package lacks Prometheus instrumentation entirely. TODO: add
    metrics (chat processing latency, step counts, stale recoveries, queue
    depth, compaction events, retry counts) and contribute a sample dashboard
    to `coder/observability`.

13. ~~**Push notifications.**~~ **Resolved.** If the LLM summary generation
    fails, the push notification still goes out with the fallback message
    "Agent has finished running."

14. **Stream buffer eviction needs backpressure.** The 10,000-event buffer has
    been hit in practice (locally), causing warn-level log spam (mitigated by
    `streamDropWarnInterval`). The UX is degraded streaming for that one chat,
    typically caused by the client not consuming events fast enough. The buffer
    evicts oldest events silently. TODO: explore a backpressure mechanism
    (e.g. slow down LLM streaming, pause tool execution, or signal the client
    to reconnect) rather than silently dropping events.

### Design Decisions That Need Rationale

15. **Tool organization needs rethinking.** Workspace tools live in `chattool/`
    but subagent tools live in `chatd/subagent.go` because they couple directly
    to `Server` methods. Beyond the extraction question, there is a larger
    design conversation: should agents have configurable toolsets? e.g. an
    "explore/review" toolset (read-only: read_file, execute, list_templates)
    vs an "implementation" toolset (read/write: edit_files, write_file,
    create_workspace). TODO: define a toolset abstraction and decide where
    tool registration/selection lives.

16. **Computer use is Anthropic-only and will need a provider support matrix.**
    `ComputerUseModelProvider = "anthropic"` and `ComputerUseModelName =
    "claude-opus-4-6"` are hardcoded because computer use is a relatively new
    capability. As other providers ship their own computer use APIs, this will
    need per-provider implementations. TODO: before GA, build a provider
    capability/support matrix that tracks which providers support which
    features (computer use, web search, reasoning, etc.).

17. **Prompt construction is scattered and should be consolidated.** The
    `chatprompt` package handles serialization/deserialization and provides
    message manipulation helpers (PrependSystem, InsertSystem, AppendUser), but
    actual prompt assembly (system prompt prepending, instruction injection,
    user prompt wrapping) lives in `chatd.go`'s `runChat`. This is likely just
    how it grew. Given the `chatd.go` decomposition goal (debt item #1),
    moving prompt assembly into `chatprompt` is a natural extraction candidate.

18. **Preferred title/summary models should be admin-configurable.** The
    `preferredTitleModels` list in `quickgen.go` is hardcoded per provider
    (Claude Haiku 4.5, GPT-4o-mini, Gemini 2.5 Flash, etc.). When a new
    cheap model is released, the code must be updated. TODO: make this a
    deployment configuration option so admins can set their preferred
    lightweight model for background tasks.

19. **Instruction caching (5 minutes) is undocumented but intentional.** If a
    user edits `AGENTS.md`, the change won't take effect for up to 5 minutes.
    The cache exists for good reason: without it, every LLM turn would re-read
    the file from the workspace agent, and uncached system prompt changes would
    invalidate the provider's prompt cache, significantly increasing cost. In
    practice, most chats are expected to target new workspaces where `AGENTS.md`
    comes from cloning a repo or running a startup script (stable after initial
    setup). TODO: document the caching behavior somewhere user-facing so people
    aren't surprised.

20. ~~**Cost calculation uses `github.com/shopspring/decimal`.**~~ **Resolved.**
    Deliberate choice to avoid float64 precision issues in financial
    calculations. An int64 microdollar representation could also work but
    `shopspring/decimal` handles the intermediate fractional arithmetic cleanly
    before the final ceil-to-int64 step. Not a concern.
