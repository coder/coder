# chatd — Architecture & State of the Code

> **Status**: Early Access (formerly experimental).
> Moved from `coderd/chatd/` to `coderd/x/chatd/` in March 2025.
> ~120 commits in the first month of development, ~31,900 lines of Go (including ~16,800 lines of tests).

## What is chatd?

`chatd` is the **server-side chat daemon** that powers Coder's "Agents" feature. It
processes user chat messages by orchestrating LLM calls, tool execution inside
workspaces, and real-time streaming of results to the frontend.

At a high level: a user sends a message → `chatd` acquires the pending chat →
calls the configured LLM → the LLM optionally invokes tools (shell commands,
file reads/writes, workspace creation, etc.) → results are persisted and
streamed back to the user.

---

## Architecture Diagram

```mermaid
flowchart TB
    %% ── External actors ──────────────────────────────────
    User(["User / Frontend"])
    LLM["LLM Provider<br/><small>Anthropic · OpenAI · Google<br/>Azure · Bedrock · OpenRouter<br/>Vercel · OpenAI-Compat</small>"]
    MCP["External MCP Servers"]
    WA["Workspace Agent<br/><small>shell · files · desktop</small>"]

    %% ── Persistence ──────────────────────────────────────
    DB[("PostgreSQL<br/><small>chats · chat_messages<br/>chat_files · chat_model_configs<br/>chat_providers · chat_queued_messages</small>")]
    PS(["Pubsub<br/><small>cross-replica events</small>"])

    %% ── API layer ────────────────────────────────────────
    subgraph API ["coderd HTTP API"]
        REST["REST Handlers<br/><small>/api/experimental/chats/*</small>"]
        SSE["SSE Streams<br/><small>watchChat · watchChats</small>"]
    end

    %% ── chatd Server ─────────────────────────────────────
    subgraph chatd ["chatd.Server"]
        direction TB

        ACQ["Acquire Loop<br/><small>poll DB every 1s<br/>batch up to 10 chats</small>"]
        HB["Heartbeat<br/><small>every 30s</small>"]
        STALE["Stale Recovery<br/><small>reclaim after 5min</small>"]
        QUEUE["Message Queue<br/><small>up to 20 queued msgs<br/>auto-promote on complete</small>"]

        subgraph processChat ["processChat goroutine"]
            direction TB
            RC["runChat<br/><small>resolve model · load messages<br/>build prompt · connect tools</small>"]

            subgraph chatloop ["chatloop.Run  ×  up to 1200 steps"]
                direction TB
                STREAM["LLM Stream<br/><small>fantasy.Stream()</small>"]
                TOOLS["Tool Execution<br/><small>parallel tool calls</small>"]
                PERSIST["Persist Step<br/><small>assistant + tool results → DB</small>"]
                COMPACT["Compaction Check<br/><small>if context > 70% limit<br/>summarize & reload</small>"]
                RETRY["Retry Logic<br/><small>exponential backoff<br/>up to 25 attempts</small>"]

                STREAM -->|"text · reasoning · tool_calls"| TOOLS
                TOOLS -->|"tool results"| PERSIST
                PERSIST --> COMPACT
                COMPACT -->|"continue?"| STREAM
                STREAM -.->|"retryable error"| RETRY
                RETRY -.->|"backoff"| STREAM
            end

            RC --> chatloop
        end

        ACQ -->|"acquire pending"| processChat
        HB -.->|"update heartbeat_at"| DB
        STALE -.->|"reset orphaned chats"| DB
    end

    %% ── Tool subsystem ───────────────────────────────────
    subgraph tools ["Tools"]
        direction TB
        FT["File Tools<br/><small>read · write · edit</small>"]
        ET["Execute Tool<br/><small>shell commands<br/>fg/bg · process mgmt</small>"]
        WT["Workspace / Planning Tools<br/><small>list/read templates<br/>create/start workspace<br/>propose plan</small>"]
        CU["Computer Use<br/><small>Anthropic-only<br/>screenshots · clicks</small>"]
        SA["Subagent Tools<br/><small>spawn · wait · message · close</small>"]
        MT["MCP Tools<br/><small>slug__toolName prefix<br/>allow/deny filtering</small>"]
        PT["Provider-native Tools<br/><small>e.g. web_search</small>"]
    end

    %% ── Supporting packages ──────────────────────────────
    subgraph support ["Supporting Packages"]
        direction LR
        CP["chatprompt<br/><small>DB ↔ SDK ↔ LLM<br/>message conversion</small>"]
        CV["chatprovider<br/><small>ModelCatalog<br/>API key mgmt</small>"]
        CC["chatcost<br/><small>microdollar<br/>cost calc</small>"]
        UL["usagelimit<br/><small>spend limits<br/>fail-open</small>"]
        QG["quickgen<br/><small>title generation<br/>push summaries</small>"]
    end

    %% ── Connections ──────────────────────────────────────
    User -->|"send message<br/>create chat"| REST
    User <-.->|"stream events"| SSE
    REST -->|"insert message<br/>set status=pending"| DB
    REST -->|"notify"| PS
    SSE <-.->|"subscribe"| PS

    ACQ -->|"SELECT pending"| DB
    PERSIST -->|"INSERT messages"| DB
    PERSIST -->|"publish message_part"| PS
    RC -->|"load messages"| DB
    RC -->|"check limits"| UL
    RC -->|"resolve model"| CV
    RC -->|"convert messages"| CP
    STREAM <-->|"generate / stream"| LLM
    COMPACT -->|"summarize"| LLM
    QG -->|"title / summary"| LLM

    TOOLS --> FT & ET & CU
    FT & ET & CU <-->|"agent HTTP API"| WA
    TOOLS --> WT
    WT -->|"create/start"| DB
    TOOLS --> SA
    SA -->|"create child chat"| DB
    TOOLS --> MT
    MT <-->|"SSE / streamable HTTP"| MCP
    TOOLS --> PT
    PT -->|"provider executes"| LLM

    CC -.->|"calculate after persist"| PERSIST

    %% ── Styling ──────────────────────────────────────────
    classDef external fill:#e8f4f8,stroke:#2196F3,color:#000
    classDef storage fill:#fff3e0,stroke:#FF9800,color:#000
    classDef api fill:#e8f5e9,stroke:#4CAF50,color:#000
    classDef core fill:#f3e5f5,stroke:#9C27B0,color:#000
    classDef tool fill:#fce4ec,stroke:#E91E63,color:#000
    classDef support fill:#e0f2f1,stroke:#009688,color:#000
    classDef loop fill:#ede7f6,stroke:#673AB7,color:#000

    class User,LLM,MCP,WA external
    class DB,PS storage
    class REST,SSE api
    class ACQ,HB,STALE,QUEUE,RC core
    class FT,ET,WT,CU,SA,MT,PT tool
    class CP,CV,CC,UL,QG support
    class STREAM,TOOLS,PERSIST,COMPACT,RETRY loop
```

### Reading the diagram

- **Solid arrows** show the primary request/data flow.
- **Dashed arrows** show background or secondary flows (heartbeats, pubsub, cost calculation).
- The **chatloop** box is the inner step-stream loop that repeats up to `maxChatSteps` times per chat turn.
- **Tools** fan out to the workspace agent (file/exec/desktop), the database (workspace management), child chats (subagents), and external MCP servers.
- The **supporting packages** are stateless helpers used by `runChat` and the persist layer.

---

## Package Layout

```
coderd/x/chatd/
├── chatd.go              # Core Server: polling, acquire, processChat, runChat (~4,200 lines)
├── prompt.go             # DefaultSystemPrompt constant
├── instruction.go        # AGENTS.md file reading from workspaces
├── sanitize.go           # Prompt/input sanitization for invisible Unicode + blank-line collapse
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
│   └── useragent.go      # User-agent + Coder identity headers for LLM API calls
│
├── chattool/             # Tool implementations
│   ├── chattool.go       # Shared helpers (toolResponse, truncateRunes)
│   ├── execute.go        # Shell command execution (foreground, background, process mgmt)
│   ├── readfile.go       # Line-numbered file reading
│   ├── writefile.go      # File writing
│   ├── editfiles.go      # Search-and-replace file editing
│   ├── computeruse.go    # Anthropic computer use (desktop screenshots, clicks, typing)
│   ├── listtemplates.go  # List workspace templates (with popularity sorting + allowlist)
│   ├── readtemplate.go   # Get template details + parameters
│   ├── createworkspace.go # Create workspace from template (idempotent, waits for build)
│   ├── startworkspace.go # Start a stopped workspace
│   └── proposeplan.go    # Present a markdown plan file for user review
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
Outgoing LLM requests also attach Coder identity headers (`X-Coder-Owner-Id`,
`X-Coder-Chat-Id`, optional subchat/workspace IDs) so intermediaries such as
`aibridged` can correlate traffic back to Coder entities.

### 5. Tools (chattool/)

Tools are implemented as `fantasy.AgentTool` values. Each tool gets a
`GetWorkspaceConn` function that lazily connects to the workspace agent.

Notable tool groupings:
- **Process tools**: `execute`, `process_output`, `process_list`, `process_signal`
- **Workspace tools**: `list_templates`, `read_template`, `create_workspace`, `start_workspace`
- **Planning tool**: `propose_plan` for presenting Markdown plans to the user
- **Provider-native tools**: e.g. `web_search`, defined separately from `chattool/`

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

Result conversion now handles text, images, audio, structured content,
`EmbeddedResource`, and `ResourceLink` payloads from MCP servers.

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

## Chat Data Lifecycle

There is currently **no retention or cleanup policy** for chat data. This is a
significant gap that needs addressing before scale becomes a concern.

### Current state

| Data | Soft-delete? | Hard-delete? | Retention policy? |
|---|---|---|---|
| `chats` | Yes (`archived = true`) | No | None |
| `chat_messages` | Yes (`deleted = true`, for edits) | No | None |
| `chat_files` | No | No | None |
| `chat_queued_messages` | N/A (deleted on consume) | Yes (on consume) | N/A |
| `chat_diff_statuses` | No | `ON DELETE CASCADE` from chats | None |

### Concerns at scale

- **Unbounded growth.** Every chat and every message is retained forever.
  Archived chats are hidden from the UI but remain in the database. Soft-deleted
  messages (from edits) are never pruned. With automation-driven usage, this
  could reach hundreds of thousands of chats with millions of messages.
- **`chat_messages.content` is JSONB.** Tool results (especially `execute`
  output, up to 32KB per tool call) are stored inline. A single long chat
  session can produce tens of megabytes of message content.
- **`chat_files` has no FK to `chats`.** Files are owned by user/org but not
  linked to a specific chat. When a chat is archived, its referenced files
  become orphans with no cleanup path. File data is stored as `BYTEA` directly
  in the table.
- **No partitioning.** `chat_messages` uses a `BIGSERIAL` primary key and is
  indexed by `(chat_id)` and `(chat_id, created_at)`. At high row counts,
  index maintenance and vacuum costs will grow.
- **Compaction summaries don't reduce storage.** Compaction summarizes context
  for the LLM but does not delete the original messages. The summary is stored
  as an additional hidden model-only message plus visible tool call/result
  messages, so compaction *increases* total storage.
- **Subagent chats are invisible to users** but still consume storage. A parent
  chat spawning multiple subagents creates additional chats and messages that
  are never surfaced in the UI and have no independent cleanup path.

### Questions to resolve

- Should there be a configurable retention period after which archived chats
  (and their messages) are hard-deleted?
- Should soft-deleted messages (from edits) be pruned after some period?
- Should compaction have an option to tombstone the original messages it
  summarized, reducing storage while preserving the summary?
- Should `chat_files` have a FK to `chats` or at least a `last_referenced_at`
  column to enable orphan cleanup?
- Should subagent chats have a shorter retention than user-facing chats?
- Do we need `chat_messages` table partitioning (e.g. by `created_at` range)
  for large deployments?
- Is there a role for an async background job (similar to workspace
  auto-deletion) that prunes old chat data?

---

## Organization Scoping — Pre-GA Blocker

**Chats are not scoped to organizations.** This is a significant gap that must
be resolved before GA.

### Current state

- The `chats` table has **no `organization_id` column**. Chats are owned by a
  user (`owner_id`) but have no organizational boundary.
- `chat_files` *does* have an `organization_id` FK, but this is the only
  chat-related table with org awareness.
- The `chatd` RBAC subject (`subjectChatd`) has **site-wide** permissions on
  `ResourceChat` (create/read/update/delete) with no org-scoped `ByOrgID`
  rules. It operates as a single global actor.
- `chatd.go` uses `dbauthz.AsChatd(ctx)` for background processing, bypassing
  any per-user or per-org authorization for the daemon's own DB operations.
- Chat queries (`GetChatsByOwnerID`, `GetChatByID`, etc.) filter by `owner_id`
  only, with no organization predicate.
- The `chat_model_configs` and `chat_providers` tables are global deployment
  config with no org dimension.

### What needs to happen

- Add `organization_id` to the `chats` table (migration + backfill for existing
  rows).
- Add org-scoped RBAC rules so users can only access chats within their
  organization.
- Decide whether `chat_model_configs` and `chat_providers` should be
  per-organization or remain global deployment settings.
- Decide whether usage limits (`chat_spend_limit_micros`) should be
  per-organization or remain per-user/global.
- Update the `chatd` RBAC subject to operate within org boundaries, or
  document why site-wide access is necessary for the background worker.
- Ensure subagent chats inherit the parent's organization.

---

## Known Technical Debt

1. **`chatd.go` needs decomposition.** At ~4,200 lines, this single file contains
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
   deep should the family tree be allowed to go? Currently,
   `createChildSubagentChat` enforces a hard limit of **one level** — a child
   chat with a `ParentChatID` cannot spawn further children. The underlying architecture
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
