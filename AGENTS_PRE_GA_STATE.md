# Agents — Pre-GA State of the System

> **Status:** Early Access (formerly experimental).
> **Snapshot date:** March 31, 2026.
> **Intent:** This document is a factual cross-cutting synthesis of the current
> system. It avoids prioritization and avoids proposing solutions.
>
> **Risk levels:** `low`, `medium`, and `high` are descriptive only. They are
> intended to signal how much uncertainty, ambiguity, or architectural exposure
> a topic currently carries, not to prescribe sequencing.

## Overview and scope

This document sits above the two detailed appendix-style architecture docs:

- [Backend appendix: `coderd/x/chatd/ARCHITECTURE.md`](./coderd/x/chatd/ARCHITECTURE.md)
- [Frontend appendix: `site/src/pages/AgentsPage/ARCHITECTURE.md`](./site/src/pages/AgentsPage/ARCHITECTURE.md)

Those files describe the codebase as it exists in each implementation surface.
This document instead follows the seams that cut across both of them:

- authorization, identity, and visibility,
- chat and workspace lifecycle,
- realtime delivery semantics,
- configuration and ownership boundaries,
- persistence and retention,
- operational behavior, and
- places where the backend and frontend tell slightly different stories.

The goal is to preserve the architecture narrative while making the current
state easier to discuss as one system.

## End-to-end architecture overview

Agents is a chat-driven development system that spans the dashboard frontend,
`coderd` HTTP API handlers, the in-process `chatd` daemon, external model
providers, workspaces, and optional MCP servers.

At a high level, a user starts on the **`/agents`** frontend routes, even though
most of the backend API still uses **`/api/experimental/chats/*`** naming. The
frontend renders chat state from a hybrid of REST queries and realtime streams.
It keeps durable message history, queue state, sidebar state, workspace state,
git state, and desktop state in separate but related caches and stores.

When a user creates a chat or sends a message, the API persists chat state in
PostgreSQL. `chatd` then acquires pending chats, resolves model and prompt
configuration, builds the toolset for that run, and enters the step loop that
streams model output, executes tools, persists results, and repeats until the
turn completes.

Workspace interaction is lazy. A chat can exist with no workspace at all, and a
workspace connection is only established when a tool call actually needs it.
When a workspace is involved, the agent uses the same underlying Coder
workspace connection path that already serves IDEs, terminals, and other
workspace features. LLM credentials and the main agent loop remain in the
control plane rather than in the workspace. Chatd can also discover MCP tools
from a workspace's `.mcp.json` file, and read skill definitions from
`.agents/skills/` directories, extending the tool set beyond built-in and
admin-configured tools.

Results then flow back through a mix of durable database writes, pubsub
notifications, in-memory stream buffers, and websocket streams to the browser.
This is one of the main architectural tensions in the current system: durable
chat state is database-backed, but some of the most important live behavior is
still intentionally ephemeral.

Several cross-cutting topics recur throughout the system:

- identity and authorization are partly request-scoped and partly performed by
  privileged helper actors,
- chat-to-workspace binding is durable only at the workspace level, not the
  agent level,
- realtime delivery mixes durable and non-durable events,
- configuration exists at deployment, user, group, chat, message, and
  browser-local layers, and
- the frontend often presents a simpler mental model than the backend actually
  implements.

## Authorization, identity, and visibility

**Risk level:** `high`

### Architecture context

Agents spans normal user requests, background daemon work, workspace access,
external model providers, and delegated child chats. This makes identity and
visibility one of the main architectural boundaries of the system rather than a
local handler concern.

### Current behavior

There are several distinct principals involved:

- the authenticated request user or API key subject,
- the `chatd` daemon actor (`AsChatd`),
- the system-restricted actor (`AsSystemRestricted`),
- reconstructed owner identity for workspace create/start flows, and
- inherited trust from parent chats to subagents.

Additionally, a new **`agents-access` site-wide role** now gates chat creation.
Members without this role cannot create chats. The role was auto-assigned to all
users who had previously created a chat via migration.

Most ordinary chat reads and writes are authorized through `dbauthz`, often via
`ExtractChatParam` and parent-chat checks rather than explicit handler-level
`Authorize(...)` calls.

Current chat visibility is effectively **owner-scoped**:

- chat records key primarily off `owner_id`,
- the global watch channel is owner-scoped, and
- chat queries do not currently add an organization dimension.

Background processing uses a broad `chatd` actor with site-wide permissions on
chat resources. Some user-facing list and validation paths use
`AsSystemRestricted` to read enabled models, enabled MCP configs, or validate
IDs without requiring the caller to hold deployment-config read permissions.

Workspace creation and start are a separate case. `chatd` decides when those
operations happen, but the actual provisioning call reconstructs the owner's
RBAC subject and performs the workspace operation as that owner.

### Notable properties and awkwardness

The current model contains several asymmetries:

- chats are not organization-scoped, while `chat_files` do carry an
  `organization_id`,
- some deployment-wide config getters are readable by any authenticated actor,
  while others require deployment-config RBAC,
- `watchChatDesktop` checks stronger workspace-connect permissions than
  `watchChatGit`, and
- enabled model and MCP inventory is visible to any authenticated Agents user
  through system-restricted helper paths.

This means the system currently has more than one visibility model at once:

- owner visibility for chats,
- deployment-wide visibility for some config and inventory surfaces,
- owner/org visibility for uploaded files, and
- workspace permission checks that differ by feature.

The `agents-access` role introduces a new access dimension within RBAC: a user
may have workspace permissions but still be unable to create chats without the
role.

`AsSystemRestricted` has been partially replaced with narrower purpose-built
actors (`AsSystemOAuth2`, `AsSystemReadProvisionerDaemons`, `AsProvisionerd`) in
non-chatd paths. The narrowing is not yet complete within chatd itself.

The frontend is not the authoritative enforcement layer. It mostly hides admin
surfaces and assumes the backend is the source of truth.

### Open questions

- What is the intended long-term visibility boundary for chats: deployment,
  organization, user, workspace, or some combination of those?
- Which deployment-wide chat settings are intended to be visible to any
  authenticated Agents user, and which are intended to be admin-only?
- Is the git watch path intentionally weaker than desktop watch, or is that a
  side effect of how workspace access is currently resolved?
- Which uses of `AsSystemRestricted` represent intended policy, and which are
  convenience shortcuts in the current implementation?

### Evidence

- Backend appendix sections:
  - `Organization Scoping — Pre-GA Blocker`
  - `Authorization Model`
- Frontend appendix sections:
  - `Admin Settings`
  - `Configuration Sources the UI Must Reconcile`
- Representative files:
  - `coderd/exp_chats.go`
  - `coderd/database/dbauthz/dbauthz.go`
  - `coderd/httpmw/chatparam.go`
  - `site/src/pages/AgentsPage/components/ChatConversation/ChatAccessDeniedAlert.tsx`
    (shown when a user lacks the `agents-access` role)

## Chat/workspace binding and lifecycle

**Risk level:** `high`

### Architecture context

A chat can be a pure control-plane conversation, or it can grow into a
workspace-backed execution context. That transition sits at the center of the
feature: it determines when infrastructure is created, what tools become
available, and how state survives across turns and subagents.

### Current behavior

The durable binding includes **`chats.workspace_id`**, **`chats.build_id`**, and
**`chats.agent_id`**. The `build_id` and `agent_id` columns were re-added after
an earlier removal; they serve as an optimistic cache of the agent a chat should
use.

On tool/prompt setup calls (`getWorkspaceAgent`), the bound agent is loaded by
ID directly. Connection establishment (`workspaceAgentIDForConn`) always
re-verifies against the latest build. Initial binding and the repair/re-resolve
path use best-match agent selection via `FindChatAgent` (suffix-matching for
`*-coderd-chat` agents, deterministic sort fallback).

A chat can begin in three states:

1. with no workspace,
2. already attached to an existing workspace, or
3. unbound at first and later bound by `create_workspace`.

`start_workspace` operates only on the currently bound workspace. It does not
create a new binding.

Subagents inherit the parent's workspace ID at spawn time, but that inheritance
is a snapshot rather than a live shared binding.

Within a running turn, `turnWorkspaceContext` keeps turn-local workspace and
agent state. It can reload the chat row when a previously unbound chat acquires
its first workspace, which is what makes mid-turn `nil -> workspace` acquisition
possible.

### Notable properties and awkwardness

Several behaviors are more fluid than the UI suggests:

- the binding is to a workspace row, not to a stable workspace agent,
- agent selection uses best-match logic via `FindChatAgent` (suffix-matching
  for `*-coderd-chat` agents, deterministic sort fallback), while connection
  establishment always re-verifies against the latest build,
- `create_workspace` may reuse, wait, suggest `start_workspace`, or create a
  replacement workspace depending on current state,
- pre-attached workspaces are validated for access but not for liveness, and
- workspace-binding changes are not streamed as their own dedicated realtime
  event.

This produces a few subtle consequences:

- multi-agent workspaces have no especially explicit selection contract,
- rebinding from one workspace to another inside a single turn is weaker than
  the `nil -> workspace` case,
- workspace-derived instructions are resolved from the initial chat snapshot and
  do not automatically refresh after late binding, and
- soft-deleted workspaces can leave chats pointing at logically dead rows.

Workspace MCP tool discovery adds a new dimension to the binding: not just
"which workspace" but also "what tools does that workspace provide via
`.mcp.json`".

Instruction files (AGENTS.md) are now persisted as `context-file` message parts
on first workspace attachment via `persistInstructionFiles()`. The
`last_injected_context` column stores the most recent context to avoid redundant
writes.

`dialvalidation.go` implements fast-fail for stopped workspaces via
`errChatHasNoWorkspaceAgent`, preventing the previous indefinite hang.

The frontend currently compensates for the missing binding-change event by
watching for a `create_workspace` tool result and invalidating the chat query.

### Open questions

- Is a chat fundamentally bound to a workspace, to an agent, or to whatever the
  latest live execution context for that workspace happens to be?
- What is the intended behavior when a previously bound workspace becomes
  deleted, stopped, disconnected, or agentless?
- Should parent and child chats continue to snapshot workspace binding, or is a
  live shared binding the intended model?
- How much of workspace context is expected to refresh within a running turn?

### Evidence

- Backend appendix section: `Workspace Binding Lifecycle`
- Frontend appendix sections:
  - `Realtime Reconciliation Model`
  - `Configuration Sources the UI Must Reconcile`
- Representative files:
  - `coderd/x/chatd/chatd.go`
  - `coderd/x/chatd/chattool/createworkspace.go`
  - `coderd/x/chatd/chattool/startworkspace.go`
  - `site/src/pages/AgentsPage/components/ChatConversation/useWorkspaceCreationWatcher.ts`

## Realtime model and delivery semantics

**Risk level:** `high`

### Architecture context

Agents is presented as an interactive chat UI, so the perceived behavior of the
system depends heavily on what is streamed live versus what is only eventually
reflected through durable state. This theme spans both backend stream assembly
and frontend reconciliation behavior.

### Current behavior

There are two main realtime surfaces:

- the per-chat stream, and
- the global chat-list watch.

The frontend transports both through **one-way WebSockets**, not browser
`EventSource`, even though the payloads are event-shaped.

The per-chat stream carries a mix of:

- ephemeral `message_part` events,
- durable `message` events,
- live control events such as `status`, `retry`, `error`, and `queue_update`.

The backend assembles this stream from multiple sources:

- local in-memory buffering,
- pubsub notifications,
- database catch-up for durable messages, and
- enterprise relay for cross-replica live partials.

The per-chat stream now also delivers `status` events from the local in-process
channel (alongside `message_part`), not only via pubsub. This fixes a race
where `message_part` could outrun the `status=running` that logically precedes
it.

The frontend does not treat the stream as the single source of truth. It uses a
REST + websocket hybrid where durable history comes from REST, and the stream
augments that history with live changes.

### Notable properties and awkwardness

The stream is not a single durable event log.

Durable messages can be recovered after reconnect by `after_id`, but
`message_part` is intentionally weaker:

- it is buffered in memory,
- it is bounded,
- it can be dropped,
- it is not replayed from the database, and
- in OSS multi-replica setups it does not cross replicas.

This means the current system makes a clear distinction between:

- **durable chat history**, and
- **live partial presentation**.

The frontend leans into that distinction. On reconnect, it resets transport-only
stream state and rebuilds from whatever new partials arrive after the socket
reopens. It relies on REST and durable message upserts to converge on the final
state.

Edits are another good example of the split: websocket events alone are not
sufficient to remove stale post-edit history from the cache, so the frontend
also invalidates REST queries.

Provider `Retry-After` headers are now respected as a floor for backoff delays.
`chaterror.ClassifiedError.RetryAfter` feeds into
`chatretry.effectiveDelay()` which computes `max(baseDelay, RetryAfter)`.

The frontend now uses a centralized `liveStatusModel` pipeline that replaces
scattered error/stream booleans. `ChatStatusCallout` renders startup, retry, and
terminal failure states from one source of truth.

### Open questions

- What delivery guarantees are intended to exist at GA for `message_part`
  versus durable messages?
- Is the current distinction between durable history and lossy live partials a
  deliberate product contract or an early-access implementation detail?
- What level of cross-replica consistency is expected for live streaming in OSS
  and enterprise deployments?
- How visible should reconnect, replay, and partial-loss behavior be to users?

### Evidence

- Backend appendix section: `Streaming Architecture`
- Frontend appendix sections:
  - `Chat Store (`chatStore.ts`)`
  - `Realtime Reconciliation Model`
- Representative files:
  - `coderd/x/chatd/chatd.go`
  - `coderd/x/chatd/chatloop/chatloop.go`
  - `coderd/exp_chats.go`
  - `site/src/pages/AgentsPage/components/ChatConversation/chatStore.ts`
  - `site/src/utils/OneWayWebSocket.ts`

## Configuration surfaces and ownership

**Risk level:** `high`

### Architecture context

Agents behavior is controlled by multiple layers of configuration. Some of that
configuration is deployment-wide, some of it is user-specific, some of it is
attached to chats or messages, and some of it exists only in the browser. This
is one of the main places where backend and frontend complexity meet.

### Current behavior

The current system spans all of the following scopes:

- deployment-global provider configs,
- deployment-global model configs,
- deployment-global system prompt, desktop toggle, workspace TTL, template
  allowlist, MCP server configs, and global usage-limit default,
- per-user prompt and per-user compaction thresholds,
- per-user and per-group usage-limit overrides,
- per-user MCP OAuth tokens,
- per-chat and per-message model and MCP selections,
- browser-local remembered model and MCP selections,
- `model_intent` boolean per MCP server config — wraps tool schemas to request
  human-readable intent strings from the LLM,
- `include_default_system_prompt` toggle — controls whether the built-in default
  system prompt is prepended to admin custom instructions,
- `enabled` toggle on model configs — disabled models stay visible in admin but
  don't appear in user selectors, and
- `agents-access` role — gates chat creation access.

There are also three different model/provider surfaces that sound similar but
are not the same thing:

1. provider config CRUD,
2. model config CRUD, and
3. the runtime model catalog shown to ordinary users.

The runtime catalog is derived from enabled providers, enabled model configs,
and effective API-key availability rather than being a direct reflection of one
database table.

### Notable properties and awkwardness

The frontend currently has to understand more of this layering than the UI might
imply.

For example:

- the UI stores `model_config_id`, but model selection is presented through
  provider/model tuples and mapped back through helper code,
- the provider list may contain database rows, env-preset providers, and
  synthetic supported providers in one response,
- MCP state spans deployment config, per-user auth state, per-chat selection,
  and browser-local defaults,
- `desktop_enabled` is not a full capability flag on its own, and
- several runtime paths deliberately fail open when configuration cannot be
  loaded or parsed.

Template allowlist, model enabled toggle, MCP `model_intent`, and the system
prompt toggle all add more configuration surface with different admin/user
visibility rules.

Read and write semantics are also uneven across config surfaces. Some endpoints
present admin-owned settings that are still readable by normal authenticated
users, while other deployment-wide settings remain admin-only even for reads.

### Open questions

- Which configuration layers are intended to be deployment-wide, user-specific,
  organization-specific, or purely runtime-derived?
- Which user-visible selections are meant to reflect runtime availability, and
  which are meant to reflect admin configuration state?
- How much of the current provider/model layering is intended product shape, and
  how much is a byproduct of the current implementation?
- Which fail-open behaviors are part of the intended contract, and which are
  temporary early-access safety valves?

### Evidence

- Backend appendix section: `Configuration Layers and Ownership`
- Frontend appendix sections:
  - `Configuration Sources the UI Must Reconcile`
  - `Admin Settings (`AgentSettingsPageView`)`
- Representative files:
  - `coderd/exp_chats.go`
  - `coderd/mcp.go`
  - `coderd/x/chatd/chatprovider/chatprovider.go`
  - `site/src/api/queries/chats.ts`
  - `site/src/pages/AgentsPage/utils/modelOptions.ts`
  - `site/src/pages/AgentsPage/components/MCPServerPicker.tsx`

## Chat data lifecycle and retention

**Risk level:** `high`

### Architecture context

Chat state is one of the feature's main long-lived assets. It includes user and
assistant messages, tool calls, tool results, files, queue state, diff state,
and hidden compaction artifacts. This makes persistence policy part of the
product architecture, not just storage implementation.

### Current behavior

Chat data is retained in the control-plane database.

Current persistence behavior includes:

- chats are archived, not deleted,
- edited messages are soft-deleted, not pruned,
- chat files have no direct foreign key to chats,
- compaction inserts additional summary records rather than reducing stored
  history, and
- subagent chats create additional stored conversations that are not surfaced in
  the main UI.

Recent schema additions include `last_read_message_id`, `pin_order`,
`build_id`, `agent_id`, `labels`, and `last_injected_context` columns on the
chats table. These serve read-tracking, ordering, optimistic agent caching,
automation tagging, and context-injection deduplication respectively.

The current system does not define a retention or cleanup policy for this data.

### Notable properties and awkwardness

The most notable property is that compaction is a **context-window** mechanism,
not a **storage-reduction** mechanism. It reduces what the model sees, but it
adds stored records rather than removing them.

There is also a mismatch between visible and invisible persistence:

- archived chats disappear from the normal UI but remain stored,
- subagent storage growth is mostly invisible to end users,
- soft-deleted messages are hidden from normal history but remain in the table,
- and uploaded files can outlive the chats that referenced them.

`context-file` message parts add to the unbounded-growth concern — instruction
files are stored as messages rather than being re-resolved per turn.

This makes the stored footprint of a long-lived or automation-heavy deployment
larger than the UI alone would suggest.

### Open questions

- What is the intended lifecycle for archived chats, soft-deleted messages,
  subagent chats, and uploaded files?
- Is compaction intended to remain purely contextual, or is there an expectation
  that storage behavior and context behavior eventually align more closely?
- Which kinds of stored chat artifacts are meant to remain user-visible,
  admin-visible, or effectively internal?

### Evidence

- Backend appendix section: `Chat Data Lifecycle`
- Frontend appendix open question: file uploads and orphaned `chat_files`
- Representative files:
  - `coderd/database/migrations/000422_chats.up.sql`
  - `coderd/database/migrations/000429_chat_files.up.sql`
  - `coderd/database/queries/chats.sql`

## Operational characteristics and testing reality

**Risk level:** `medium`

### Architecture context

The feature runs as part of the control plane, depends on multiple external
providers, and coordinates live chat state, workspace state, and background
processing. Operational clarity matters even before any discussion of GA, but
many of the current concerns are more about observability and confidence than
about a broken product model.

### Current behavior

`chatd` currently uses a polling acquisition loop rather than a block-until-work
pattern. It also manages heartbeats, stale-chat recovery, queue promotion,
retries, compaction, and push notifications inside the same package.

Test coverage follows the broader codebase style of heavy local and CI testing.
At the same time:

- provider integration coverage is strongest for OpenAI and Anthropic,
- other provider paths appear less exercised,
- there are no Prometheus metrics or dashboards dedicated to `chatd`, and
- stream backpressure remains unresolved even though stream-buffer limits have
  already been hit locally.

### Notable properties and awkwardness

The main theme here is observability lagging behind system complexity.

The code already contains explicit handling for:

- stale worker recovery,
- retry behavior,
- queueing and interruption,
- compaction,
- fallback push summaries,
- and stream drop warnings.

That means many of the relevant failure modes are known to the implementation,
but they are not yet described through a matching operational surface of
metrics, dashboards, or coverage depth.

### Open questions

- Which parts of `chatd` are intended to become operationally visible as their
  own monitored subsystem, versus remaining part of the general `coderd`
  surface?
- How much confidence is expected across providers before the provider matrix is
  considered representative rather than partial?
- Which failure modes are currently understood only by reading code and tests,
  rather than by using observable production signals?

### Evidence

- Backend appendix sections:
  - `Known Technical Debt`
  - `Open Questions -> Testing`
  - `Open Questions -> Operational`
- Representative files:
  - `coderd/x/chatd/chatd.go`
  - `coderd/x/chatd/chattest/*`
  - `coderd/x/chatd/chatd_test.go`

## Frontend/backend contract mismatches

**Risk level:** `medium`

### Architecture context

The system is presented as one product, but the frontend and backend currently
use different names, different abstractions, and in some cases slightly
different implied contracts. These are not all correctness problems, but they do
shape how the feature is understood.

### Current behavior

Some of the more visible mismatches include:

- `/agents` routes in the frontend versus `/chats` naming in the API and SDK,
- a user-facing model picker that actually bridges multiple backend config
  surfaces,
- frontend workarounds for missing backend binding events,
- admin UI grouping that is cleaner than the backend's actual permission model,
- desktop enablement in the UI versus additional backend/runtime conditions for
  actual computer-use availability,
- and embed-mode trust depending on `postMessage` flows that currently use `*`
  for the ready signal and do not visibly validate origin on the incoming
  bootstrap message.

The main chat page component is now `AgentChatPage`. The settings monolith
(`AgentSettingsPageView`) has been decomposed into separate page-level
components per section.

Workspace MCP tools create a new asymmetry: admin-configured tools are managed
centrally, workspace tools are discovered per-workspace and may differ between
chats.

Chat labels exist in the API but have no frontend rendering yet.

### Notable properties and awkwardness

These mismatches matter because the frontend is where most people will infer the
product model.

In several places, the UI presents a simpler and more coherent story than the
backend actually guarantees today. That is often a good product instinct, but it
also means some important current-state behavior only becomes visible when
reading the backend or the reconciliation logic in the chat store.

The naming split between "agents" and "chats" is the broadest example. More
specific cases include model selection, workspace binding visibility, and config
read semantics.

### Open questions

- Which product terms are intended to remain user-facing labels only, and which
  are intended to become stable concepts across API, SDK, and UI?
- How closely is the frontend expected to mirror backend policy and event
  semantics, versus abstracting them into a simpler product model?
- Which current frontend workarounds represent intended long-term behavior, and
  which simply fill gaps in the current backend contract?

### Evidence

- Frontend appendix sections:
  - `Route Structure`
  - `Configuration Sources the UI Must Reconcile`
  - `Realtime Reconciliation Model`
  - `Open Questions`
- Backend appendix sections:
  - `Authorization Model`
  - `Workspace Binding Lifecycle`
  - `Configuration Layers and Ownership`
- Representative files:
  - `site/src/pages/AgentsPage/AgentEmbedPage.tsx`
  - `site/src/pages/AgentsPage/AgentChatPage.tsx`
  - `site/src/pages/AgentsPage/components/ChatConversation/chatStore.ts`
  - `coderd/exp_chats.go`

## Open questions for discussion

The current state of the system raises a recurring set of cross-cutting
questions. The list below is intentionally more concrete than the earlier
sections: these are the decision-shaped questions the current implementation
keeps surfacing.

### Authorization, identity, and visibility

- Should chats remain effectively **owner-scoped**, or is the intended model
  **organization-scoped** once the feature matures?
- If chats gain organization scoping, which related surfaces should follow that
  same boundary and which should remain deployment-global?
  - model configs,
  - provider configs,
  - MCP server definitions,
  - usage limits,
  - sidebar/watch channels.
- Which deployment-wide chat settings are supposed to be readable by any
  authenticated Agents user, and which are supposed to be admin-only even for
  reads?
- Is the current `watchChatGit` versus `watchChatDesktop` permission asymmetry
  intended policy, or just where the code currently landed?
- Which current uses of `AsSystemRestricted` represent the intended product
  contract, and which are simply implementation shortcuts for now?
- How should the `agents-access` role interact with organization-level access
  control when org scoping lands?

### Chat/workspace binding and lifecycle

- When a chat starts using a workspace, what is the intended durable binding?
  - chat -> workspace,
  - chat -> workspace build,
  - chat -> workspace agent,
  - or chat -> whatever the latest healthy execution target happens to be?
- When a bound workspace becomes stopped, deleted, disconnected, or agentless,
  what state is the chat conceptually in?
  - still bound but degraded,
  - waiting for restart,
  - waiting for replacement,
  - or effectively unbound?
- Are subagents intended to inherit a **snapshot** of the parent's workspace
  state, or should they track the parent's later rebinding changes?
- How much workspace context is supposed to refresh during an in-flight turn?
  - just connection state,
  - newly attached workspace IDs,
  - workspace-derived instructions such as `AGENTS.md`, OS, and working
    directory,
  - or none of the above until the next turn?
- Is multi-agent workspace behavior supposed to be stable enough that a chat can
  be thought of as talking to a specific agent, or is the current
  best-match agent selection (via `FindChatAgent`) behavior the intended
  abstraction?
- How should workspace MCP tool discovery interact with the admin-configured MCP
  server list? What happens when they conflict?

### Realtime model and delivery semantics

- Which parts of the stream are meant to be **durable product guarantees** and
  which are meant to be **best-effort presentation**?
  - final messages,
  - queue state,
  - status transitions,
  - partial text,
  - partial reasoning,
  - partial tool-call state.
- Is `message_part` loss on reconnect, overflow, or replica movement an intended
  contract, or an early-access limitation?
- Should users be expected to think of reconnect as:
  - resuming the same live stream,
  - catching up durable history only,
  - or starting a fresh transport view over the same durable chat?
- Is the lack of replay on the global chat watch intentional, or should sidebar
  state eventually have stronger catch-up semantics?
- Are OSS and enterprise supposed to differ materially in cross-replica live
  partial streaming behavior, or is that difference just a current deployment
  artifact?

### Configuration surfaces and ownership

- Which settings are intended to be:
  - deployment-global,
  - organization-scoped,
  - user-specific,
  - per-chat,
  - per-message,
  - or browser-local convenience state?
- Are the three current model/provider surfaces supposed to remain distinct?
  - provider config admin view,
  - model config admin view,
  - runtime model catalog for ordinary users.
- Which parts of current config behavior are intended policy versus current
  implementation detail?
  - fail-open allowlist behavior,
  - fail-open usage-limit checks,
  - fail-open prompt fallback,
  - enabled model/MCP inventory visibility to non-admin users.
- Is `desktop_enabled` intended to mean:
  - "show computer-use-related UI",
  - "computer use is possible in principle",
  - or "computer use is fully available end to end"?
- Which pieces of configuration are expected to be understandable from the UI
  alone, and which are acceptable as backend-only distinctions?

### Chat data lifecycle and retention

- Are archived chats expected to be retained indefinitely, or is archiving only a
  UI visibility state rather than a lifecycle decision?
- Should these artifacts all share the same lifecycle, or are they expected to
  diverge?
  - visible chats,
  - subagent chats,
  - soft-deleted messages,
  - uploaded files,
  - compaction summaries,
  - queue artifacts,
  - diff status records.
- Is compaction expected to remain a context-only mechanism even if that means
  storage only grows?
- Which stored artifacts are intended to be:
  - user-visible,
  - admin/audit-visible,
  - or effectively internal implementation details?
- Chat labels are general-purpose `map[string]string` designed for Automations.
  No label-based retention or cleanup policy exists yet.
- `last_injected_context` stores the most recently persisted instruction files.
  Should this be periodically refreshed to capture workspace changes?

### Frontend/backend contract and product model

- Is the `/agents` versus `/chats` naming split intended to remain a user-facing
  label difference, or should the underlying concepts eventually line up across
  UI, API, and SDK?
- How much simplification is the frontend expected to perform over backend
  reality?
  - hide complexity but stay semantically accurate,
  - present a cleaner product model even if the backend is messier,
  - or mirror backend distinctions more directly?
- Which current frontend workarounds are expected to remain part of the product
  contract?
  - invalidating chat state after `create_workspace`,
  - REST refetch after edit truncation,
  - mapping runtime model choices back to `model_config_id`,
  - postMessage bootstrap assumptions in embed mode.
- Which mismatches are acceptable internal implementation details, and which are
  already shaping user expectations in a way that deserves explicit product
  language?

### Suggested ways to structure the discussion

Without prescribing solutions, a few framing devices would make these
conversations more concrete:

- For each topic, separate **intended product contract** from **current
  implementation artifact**.
- For each boundary, explicitly name the intended scope:
  - deployment,
  - organization,
  - user,
  - chat,
  - message,
  - workspace,
  - browser-local.
- For each realtime behavior, explicitly classify it as:
  - durable,
  - best-effort,
  - replayable,
  - or transport-local only.
- For each user-visible setting or UI concept, decide whether it is meant to be:
  - a full capability signal,
  - a partial hint,
  - or simply one input into a more complex backend decision.
- For each mismatch between frontend and backend, decide whether it is:
  - a deliberate abstraction,
  - a temporary workaround,
  - or an unresolved ambiguity.

## Evidence map

| Topic                                           | Primary appendix                                                                                   | Secondary appendix                                                                                             | Representative files                                                                                                                         |
|-------------------------------------------------|----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| Authorization, identity, and visibility         | `coderd/x/chatd/ARCHITECTURE.md` -> `Organization Scoping — Pre-GA Blocker`, `Authorization Model` | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Admin Settings`, `Configuration Sources the UI Must Reconcile` | `coderd/exp_chats.go`, `coderd/database/dbauthz/dbauthz.go`, `coderd/httpmw/chatparam.go`, `coderd/rbac/roles.go`                                                    |
| Chat/workspace binding and lifecycle            | `coderd/x/chatd/ARCHITECTURE.md` -> `Workspace Binding Lifecycle`                                  | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Realtime Reconciliation Model`                                 | `coderd/x/chatd/chatd.go`, `coderd/x/chatd/chattool/createworkspace.go`, `coderd/x/chatd/chattool/startworkspace.go`, `coderd/x/chatd/dialvalidation.go`, `coderd/x/chatd/chattool/mcpworkspace.go`, `coderd/x/chatd/chattool/skill.go`                         |
| Realtime model and delivery semantics           | `coderd/x/chatd/ARCHITECTURE.md` -> `Streaming Architecture`                                       | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Chat Store`, `Realtime Reconciliation Model`                   | `coderd/x/chatd/chatd.go`, `coderd/exp_chats.go`, `site/src/pages/AgentsPage/components/ChatConversation/chatStore.ts`, `coderd/x/chatd/chaterror/`                          |
| Configuration surfaces and ownership            | `coderd/x/chatd/ARCHITECTURE.md` -> `Configuration Layers and Ownership`                           | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Configuration Sources the UI Must Reconcile`                   | `coderd/exp_chats.go`, `coderd/mcp.go`, `site/src/api/queries/chats.ts`, `site/src/pages/AgentsPage/utils/modelOptions.ts`, `coderd/x/chatd/configcache.go`                   |
| Chat data lifecycle and retention               | `coderd/x/chatd/ARCHITECTURE.md` -> `Chat Data Lifecycle`                                          | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Open Questions`                                                | `coderd/database/migrations/000422_chats.up.sql`, `coderd/database/migrations/000429_chat_files.up.sql`, `coderd/database/queries/chats.sql` |
| Operational characteristics and testing reality | `coderd/x/chatd/ARCHITECTURE.md` -> `Known Technical Debt`, `Open Questions`                       | `site/src/pages/AgentsPage/ARCHITECTURE.md` -> `Open Questions`                                                | `coderd/x/chatd/chatd.go`, `coderd/x/chatd/chatd_test.go`, `coderd/x/chatd/chattest/*`                                                       |
| Frontend/backend contract mismatches            | both appendices                                                                                    | both appendices                                                                                                | `site/src/pages/AgentsPage/AgentEmbedPage.tsx`, `site/src/pages/AgentsPage/AgentChatPage.tsx`, `coderd/exp_chats.go`                           |

## Adjacent public documentation

The repository already contains public-facing Agents docs that describe the
control-plane architecture and the early-access status of the feature:

- [`docs/ai-coder/agents/index.md`](./docs/ai-coder/agents/index.md)
- [`docs/ai-coder/agents/architecture.md`](./docs/ai-coder/agents/architecture.md)
- [`docs/ai-coder/agents/early-access.md`](./docs/ai-coder/agents/early-access.md)

Those pages describe the product and deployment model. This document is a
separate internal-style snapshot of how the current implementation behaves
across the system.
