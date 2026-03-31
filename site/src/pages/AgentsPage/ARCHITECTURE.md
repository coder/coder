# Agents Frontend — Architecture & State of the Code

> **Status**: Early Access (formerly experimental).
> ~31,300 lines of TypeScript/TSX in `site/src/pages/AgentsPage/`,
> ~6,800 lines in shared `ai-elements` and `ChatMessageInput` components,
> ~950 lines in the API query layer.

## Overview

The Agents frontend is a single-page chat application embedded within Coder's
dashboard. It communicates with the backend via REST endpoints under
`/api/experimental/chats/` and real-time one-way WebSocket streams.

**Naming mismatch**: The frontend routes use `/agents` while the backend API
uses `/api/experimental/chats/`. The UI calls them "agents", the backend calls
them "chats".

---

## Route Structure

```
/agents                          → AgentsPage (layout: sidebar + outlet)
  /agents/                       → AgentCreatePage (index — new chat form)
  /agents/settings               → AgentSettingsPage (layout: subnav + outlet)
    /agents/settings/            → AgentSettingsBehaviorPage (index)
    /agents/settings/behavior    → AgentSettingsBehaviorPage
    /agents/settings/providers   → AgentSettingsProvidersPage
    /agents/settings/models      → AgentSettingsModelsPage
    /agents/settings/mcp-servers → AgentSettingsMCPServersPage
    /agents/settings/limits      → AgentSettingsLimitsPage
    /agents/settings/usage       → AgentSettingsUsagePage
    /agents/settings/insights    → AgentSettingsInsightsPage
    /agents/settings/templates   → AgentSettingsTemplatesPage
  /agents/analytics              → AgentAnalyticsPage
  /agents/:agentId               → AgentChatPage (conversation view)

/agents/:agentId/embed           → AgentEmbedPage (iframe wrapper with theme sync)
  /agents/:agentId/embed/        → AgentChatPage (embedded conversation)
```

All pages are lazy-loaded via React Router. `AgentsPage` is the layout wrapper
that provides `AgentsOutletContext` to child routes via React Router's outlet
context. `AgentSettingsPage` is a nested layout that provides a settings subnav
and renders its own `<Outlet>` for the individual settings pages.

---

## Component Hierarchy

```
AgentsPage (layout: sidebar + outlet)
  └─ AgentsPageView (layout shell)
       ├─ AgentsSidebar (Sidebar/AgentsSidebar.tsx)
       │    ├─ Pinned chats section (drag-to-reorder via @dnd-kit)
       │    ├─ Read/unread indicator (bold + blue dot)
       │    └─ SidebarTabView (Sidebar/SidebarTabView.tsx)
       └─ <Outlet>
            ├─ AgentCreatePage (AgentCreateForm.tsx)
            ├─ AgentChatPage (was AgentDetail)
            │    ├─ ChatTopBar.tsx (model selector, interrupt, archive, title regen)
            │    ├─ ChatConversation/ (was ConversationTimeline)
            │    │    ├─ ConversationTimeline.tsx (message rendering, scroll)
            │    │    ├─ LiveStreamTail.tsx (streaming output)
            │    │    ├─ StreamingOutput.tsx
            │    │    └─ ChatStatusCallout.tsx (structured live-status: startup/retry/failure)
            │    ├─ ChatElements/ (message rendering primitives)
            │    │    ├─ Conversation.tsx, Message.tsx, Response.tsx
            │    │    ├─ ModelSelector.tsx, Shimmer.tsx, Thinking.tsx
            │    │    └─ tools/ (per-tool renderers: Tool.tsx dispatches)
            │    ├─ QueuedMessagesList.tsx
            │    ├─ AgentChatInput.tsx (Lexical rich text + ArrowUp edit)
            │    └─ RightPanel/ (desktop VNC, git panel, diff viewer)
            ├─ AgentSettingsBehaviorPage (personal prompt, system prompt, desktop, TTL)
            ├─ AgentSettingsProvidersPage (provider CRUD)
            ├─ AgentSettingsModelsPage (model config CRUD with enabled toggle)
            ├─ AgentSettingsMCPServersPage (MCP server config with model_intent toggle)
            ├─ AgentSettingsLimitsPage (usage limits)
            ├─ AgentSettingsUsagePage (cost summaries)
            ├─ AgentSettingsInsightsPage (PR insights)
            ├─ AgentSettingsTemplatesPage (template allowlist)
            └─ AgentAnalyticsPage

AgentEmbedPage (iframe wrapper with theme sync + navigation blocking)
  └─ <Outlet> → AgentChatPage
```

---

## Data Flow

### State ownership

- **AgentsPage** owns: chat list (infinite query), model list, sidebar state,
  archive mutations, global `watchChats` WebSocket subscription.
- **AgentDetail** owns: individual chat query, paginated messages (infinite
  scroll), workspace query, streaming state (`useChatStore`), model selection,
  send/edit/interrupt mutations.
- State flows **down** via props and `AgentsOutletContext`.
- Mutations flow **up** via callback props.

### Additional state

- **Read/unread tracking**: `has_unread` on `Chat`, `last_read_message_id`
  updated on stream connect/disconnect.
- **Pinned chats**: `pin_order`, optimistic cache reorder,
  `reorderPinnedChat` mutation.
- **`liveStatusModel`**: centralized live-status pipeline
  (`liveStatusModel.ts`) replacing scattered error/stream booleans.
  `ChatStatusCallout` renders structured startup/retry/failure status.
- **Chat labels**: `labels` field on `Chat` (API only, not yet rendered).

### Real-time updates

Primary real-time connections:
1. **`watchChats()`** — global, owned by `AgentsPage`. Receives status, title,
   and diff_status changes for all chats. Updates the sidebar in real time.
2. **`watchChat(chatId)`** — per-chat, owned by `useChatStore` inside
   `AgentDetail`. Receives `message_part`, `message`, `status`, `error`,
   `retry`, and `queue_update` events.
3. **`watchChatGit(chatId)`** — chat-scoped Git WebSocket used by the right
   panel / diff flows.
4. **`watchChatDesktop(chatId)`** — chat-scoped desktop WebSocket for the
   embedded VNC/desktop experience.

There is also an ancillary `watchWorkspace()` subscription for workspace agent
status that isn't part of `/api/experimental/chats` but is part of the Agent
Detail experience.

The chat streams use `createReconnectingWebSocket` for automatic reconnection.

### React Query cache management

The chat list uses `useInfiniteQuery` with careful cache manipulation:
- `updateInfiniteChatsCache` — updates a chat across all pages
- `prependToInfiniteChatsCache` — adds a new chat to page 0 (with cross-page
  dedup)
- `readInfiniteChatsCache` — reads the flat list from cache
- `invalidateChatListQueries` — uses a predicate (`isChatListQuery`) to
  invalidate list queries without touching per-chat detail queries

WebSocket events update the React Query cache directly (optimistic) rather than
triggering refetches. This avoids flicker but means the cache can drift from
the server if events are missed.

---

## Chat Store (`ChatContext.ts`)

The chat store is the most complex piece of frontend state (~1,100 lines). It is
a **framework-agnostic external store** consumed via `useSyncExternalStore`.

### Architecture

```
createChatStore()          — Pure store: Map<id, ChatMessage>, StreamState, retry/reconnect state, etc.
  ↓
useChatStore()             — React hook: wires store to REST queries + WebSocket
  ↓
useChatSelector(selector)  — Thin wrapper: only re-renders on selected slice change
```

### Key design patterns

- **Batching**: `batch()` suppresses listener notifications during a batch,
  emitting once at the end. Collapses N WebSocket events into 1 re-render.
- **Immutable updates with equality guards**: Every setter checks for equality
  before creating a new state object.
- **Bulk upsert**: `upsertDurableMessages` applies all messages in a single
  pass — one Map copy + one sort instead of N copies and N sorts.
- **`setTimeout(0)` coalescing**: `message_part` events are buffered into
  `partsBuf[]` and flushed via a microtask to batch rapid-fire stream updates.

### WebSocket event handling

| Event | Handler | Effect |
|---|---|---|
| `message_part` | Buffered → `applyMessageParts` | Updates `StreamState` via reducer |
| `message` | Collected → `upsertDurableMessages` | Updates `messagesByID` + `orderedMessageIDs` |
| `status` | `setChatStatus` | Clears stream/retry state, updates sidebar |
| `error` | `setStreamError` | Sets terminal error state, updates sidebar |
| `retry` | Clears stream state | Sets retry info |
| socket reconnect schedule | `setReconnectState` | Surfaces reconnect/backoff state when appropriate |
| `queue_update` | `setQueuedMessages` | Updates store + React Query cache |

### Recent changes

- `clearStreamState()` is now called after POST response, gated on
  `!response.queued` so it doesn't clear state for queued messages.
- `model_intent` is extracted from streaming JSON in `streamState.ts` and
  surfaced by `GenericToolRenderer` as a primary label when available.
- Stale per-chat refetches are cancelled before WebSocket cache writes to
  prevent races where a slow REST response overwrites fresh WS data.
- `chat-ready` signal waits for store message count to match fetched count
  before considering the chat fully loaded.

---

## Performance

- **Module-scope `createComponents`** in `Response.tsx` — Streamdown
  component factories are created once at module scope, not per-render.
- **`memo()` on hot components**: `StickyUserMessage`,
  `ConversationTimeline`, `SmoothedResponse`, `ReasoningDisclosure` are all
  wrapped in `memo()` to prevent unnecessary re-renders during streaming.
- **React Compiler**: `buildStreamTools` narrowed, `AgentDetailInput`
  dependency chain reordered, `useSpeechRecognition` moved to compiled
  `hooks/` path.
- **Streaming-mode Streamdown**: live output uses streaming-mode Streamdown
  for incremental rendering; persisted messages use standard rendering.

---

## Scroll Architecture

The conversation uses a top-to-bottom `flex-col` layout with
`overflow-y-auto` and `[overflow-anchor:none]`. Auto-scroll is driven by
`IntersectionObserver` on a sentinel element and `ResizeObserver` on the
content wrapper (to detect growth during streaming). Scroll updates are
RAF-throttled to avoid redundant work on high-refresh-rate displays.

Notable scroll work includes a pin starvation fix for the sticky user
message, a restore-guard lifecycle to prevent scroll jumps on mount, and
WebKit phantom scroll handling where content resizes don't fire scroll
events.

---

## Stream State (`streamState.ts`)

A pure reducer (`applyMessagePartToStreamState`) that transforms `StreamState`
based on incoming `ChatMessagePart` events:

- `text` → appends to last `"response"` block
- `reasoning` → appends to last `"thinking"` block
- `tool-call` → merges into `toolCalls` record (incremental JSON delta
  assembly via `mergeStreamPayload`)
- `tool-result` → merges into `toolResults` record
- `source` → deduplicates by URL, groups consecutive sources

Tool ID resolution has a fallback chain: `part.tool_call_id` → existing entry
matched by name → auto-generated fallback ID.

---

## Message Parsing (`messageParsing.ts`)

Converts persisted `ChatMessage[]` into `ParsedMessageEntry[]` for rendering.

Key function: `parseMessagesWithMergedTools` does **cross-message tool result
joining** — tool results from later messages are matched to tool calls from
earlier messages via a global result map. This handles the common pattern where
a tool call and its result arrive in separate database messages.

---

## Tool Rendering (`components/ChatElements/tools/`)

Each tool has a specialized renderer dispatched by `Tool.tsx`:

| Tool | Renderer | Notes |
|---|---|---|
| `execute` | `ExecuteTool` | Shows command, output, exit code |
| `process_output` | `ProcessOutputTool` | Dedicated process output renderer |
| `read_file` | `ReadFileTool` | File viewer |
| `write_file` | `WriteFileTool` | Diff viewer (before/after) |
| `edit_files` | `EditFilesTool` | Multi-file diff viewer |
| `create_workspace` | `CreateWorkspaceTool` | Workspace name + status |
| `spawn_agent` etc. | `SubagentTool` | Clickable with expand/collapse |
| `chat_summarized` | `ChatSummarizedTool` | Collapsed summary |
| `computer` | `ComputerTool` | Screenshot display |
| `list_templates` | `ListTemplatesTool` | Template listing |
| `read_template` | `ReadTemplateTool` | Template detail |
| `propose_plan` | `ProposePlanTool` | Plan proposal renderer |
| Unknown | `GenericToolRenderer` | Fallback; shows `model_intent` as primary label when available |

Shared primitives: `ToolCollapsible.tsx`, `ToolIcon.tsx`, `ToolLabel.tsx`.

Additional components in the tools directory:
- `InlineDesktopPreview.tsx` — desktop preview for computer use subagent.
- `WebSearchSources.tsx` — web search source display.
- `DesktopPanelContext.tsx` — context provider for desktop panel state.

---

## Chat Input (`ChatMessageInput`)

A **Lexical-based rich text input** with custom plugins:
- `DisableFormattingPlugin` — blocks Cmd+B/I/U
- `PasteSanitizationPlugin` — strips rich text, forwards image pastes
- `EnterKeyPlugin` — Enter submits, Shift+Enter newline
- `FileReferenceNode` — inline file reference chips

Exposes an imperative ref (`ChatMessageInputRef`) with: `insertText`, `clear`,
`focus`, `getValue`, `addFileReference`, `getContentParts`.

---

## Admin Settings

Settings are now split into separate route-level page files under
`AgentSettingsPage` (layout with subnav):

| Page | Content | Access |
|---|---|---|
| `AgentSettingsBehaviorPage` | Personal prompt, system prompt (`include_default_system_prompt` toggle), user compaction thresholds, desktop toggle, autostop TTL | Personal: all users. System/desktop/TTL: admin only |
| `AgentSettingsProvidersPage` | Provider CRUD (API keys, base URLs) | Admin |
| `AgentSettingsModelsPage` | Model config CRUD with `enabled` toggle (per-provider settings, pricing) | Admin |
| `AgentSettingsMCPServersPage` | MCP server config CRUD with `model_intent` toggle | Admin |
| `AgentSettingsLimitsPage` | Usage limit config (global, per-user, per-group overrides) | Admin |
| `AgentSettingsUsagePage` | Cost summaries, per-user drill-down | Admin |
| `AgentSettingsInsightsPage` | PR insights | Admin |
| `AgentSettingsTemplatesPage` | Deployment-wide template allowlist for chat-created workspaces | Admin |

---

## Configuration Sources the UI Must Reconcile

The frontend has to stitch together multiple backend configuration layers that
look similar in the UI but are not the same thing in the API.

### Model/provider data does not come from one source

The create and detail pages need all of the following:

- **`chatProviderConfigs()`** — admin/provider management view over provider
  configs.
- **`chatModelConfigs()`** — admin-managed per-model config rows (default,
  context limit, pricing, compression threshold, provider-specific options).
- **`chatModels()`** — user-facing runtime availability catalog derived from
  enabled providers, enabled model configs, and effective API keys.

That means the frontend cannot treat "models" as one resource. It has to map
between the runtime catalog and the admin config rows.

### Model selection requires ID translation

The UI stores and submits `model_config_id`, but the runtime model selector is
built from provider/model tuples. `utils/modelOptions.ts` bridges that gap by
building lookup maps in both directions (`provider:model` ↔ `model_config_id`).

This is why both `AgentCreatePage` and `AgentDetail` query `chatModels()` and
`chatModelConfigs()` together. The last chosen model config is also cached in
`localStorage` under `agents.last-model-config-id`.

### Behavior settings are split across admin-owned and user-owned state

The `behavior` settings section is really several scopes grouped into one page:

- **User-owned**: personal instructions (`user-prompt`) and per-model user
  compaction thresholds.
- **Admin-owned**: system prompt, desktop enablement, workspace TTL, template
  allowlist, providers/models, MCP server definitions, usage limits.
- **Runtime-derived**: whether computer use is actually possible depends on more
  than the `desktop-enabled` toggle.

The page mostly hides admin controls behind `editDeploymentConfig`, but several
backend read paths are still available to non-admin users. The UI's mental
model is simpler than the backend's actual policy matrix.

### MCP state spans deployment, user, chat, and browser layers

`MCPServerPicker` has to reconcile four layers of state:

1. **Deployment config**: admin-defined MCP server configs.
2. **Per-user auth state**: OAuth connection status (`auth_connected`).
3. **Per-chat selection**: the `mcp_server_ids` sent when creating/sending
   messages.
4. **Browser-local memory**: saved selection in
   `agents.selected-mcp-server-ids`.

`getDefaultMCPSelection()` and `getSavedMCPSelection()` also encode product
policy from `force_on`, `default_on`, and `default_off`. Some of the "what is
selected by default" behavior therefore lives in the client rather than only
in the server.

### The frontend tolerates several backend ownership quirks

- Provider config lists may contain real DB rows, env-preset providers, and
  synthetic supported providers in the same response.
- Runtime model availability is not the same as model-config existence.
- `desktop-enabled` does not guarantee computer use is actually usable.
- Config auth failures are not normalized: some endpoints return `403`, others
  intentionally collapse to `404`.

---

## API Surface

### REST Endpoints (via `ExperimentalApiMethods`)

~50 methods organized into:
- **Core Chat CRUD**: getChats, getChat, createChat, updateChat, getChatMessages,
  createChatMessage, editChatMessage, interruptChat
- **Queue**: deleteChatQueuedMessage, promoteChatQueuedMessage
- **Git/Diff**: getChatGitChanges, getChatDiffContents
- **Models**: getChatModels
- **Config**: system prompt, desktop enabled, workspace TTL, template allowlist,
  user prompt, per-model user compaction thresholds
- **Provider/Model Config**: CRUD for providers and model configs
- **Cost/Insights**: cost summaries, per-user costs, PR insights
- **Usage Limits**: config, status, user overrides, group overrides
- **MCP Servers**: CRUD for MCP server configs
- **Files**: uploadChatFile, getChatFileText
- **Title**: `POST /chats/{chatID}/title/regenerate`
- **Workspace lookup**: `GET /chats/by-workspace`
- **Chat update**: `PATCH /chats/{chat}` with `pin_order` and `labels`
  fields
- **Label filtering**: `?label=key:value` on `GET /chats`

### SSE/WebSocket Streams

| Stream | Route | Purpose |
|---|---|---|
| `watchChat(chatId)` | `/api/experimental/chats/{id}/stream` | Per-chat event stream over one-way WebSocket |
| `watchChats()` | `/api/experimental/chats/watch` | Global chat list watch over one-way WebSocket |
| `watchChatGit(chatId)` | `/api/experimental/chats/{id}/stream/git` | Git change WebSocket |
| `watchChatDesktop(chatId)` | `/api/experimental/chats/{id}/stream/desktop` | Desktop VNC WebSocket |

---

## Realtime Reconciliation Model

The frontend does not treat the websocket stream as the single source of truth.
It uses a **REST + WebSocket hybrid**:

- **REST** owns durable history (`getChat`, paginated `getChatMessages`, chat
  list queries).
- **Per-chat websocket** augments that history with live `message_part`,
  `message`, `status`, `error`, `retry`, and `queue_update` events.
- **Global watch websocket** augments the sidebar/list cache with owner-scoped
  chat events.

### Transport details

`watchChat()` and `watchChats()` use `OneWayWebSocket`, not browser
`EventSource`. The wrapper exists specifically to avoid the browser/HTTP 1.1
connection-limit problems that real SSE would have caused across tabs. The
server still sends event-style payloads, but the transport is one-way
WebSockets.

### Per-chat reconnect strategy

`ChatContext` waits for the initial REST message history before opening the
socket so it can seed `lastMessageIdRef`. Reconnects then call
`watchChat(chatId, after_id)` with the last durable message ID.

On a successful reopen, `resetTransportReplayState()` clears:
- ephemeral stream-part state,
- transport-scoped stream errors,
- reconnect/backoff UI state.

This is an explicit trade-off: durable messages recover; partially streamed
text/thinking/tool-call state may not.

### What is recoverable vs not

| State | Recovery behavior |
|---|---|
| Durable messages | Recovered by `after_id` and upserted by message ID |
| Sidebar/list state | Reconciled by invalidating chat-list queries on reopen |
| Queue state | WebSocket becomes authoritative after `queue_update` events |
| In-flight `message_part` state | Not durably recoverable; rebuilt only from newly streamed parts |
| Edit truncation | Requires explicit REST invalidation because websocket updates cannot remove stale cached messages |

### Per-event ownership

- **`message_part`** -> ephemeral reducer in `streamState.ts`.
- **`message`** -> durable store + React Query cache via `upsertDurableMessages`.
- **`status`** -> chat store + sidebar cache.
- **`error`** -> transport error state, plus persisted error fallback from the
  chat query.
- **`retry`** -> clears current stream UI and surfaces retry/backoff state.
- **`queue_update`** -> replaces queued message state in both the external store
  and React Query cache.

### Non-stream realtime surfaces

`watchChatGit()`, `watchChatDesktop()`, and `watchWorkspace()` sit beside the
main chat stream rather than being part of the durable reconciliation path.
They have their own failure and reconnect semantics.

### Binding-change caveat

Workspace attachment changes are not streamed as a dedicated chat-binding
event. `useWorkspaceCreationWatcher()` works around that by noticing a
`create_workspace` tool result and invalidating the chat query so the updated
`workspace_id` appears in the UI.

---

## Embed Mode (`AgentEmbedPage`)

Supports embedding the agent UI inside VS Code via an iframe. Authentication
flow:
1. Page loads and waits for `coder:vscode-auth-bootstrap` postMessage
2. Exchanges token via `bootstrapChatEmbedSession` mutation
3. On success, wraps child routes in dashboard providers
4. Provides a minimal `AgentsOutletContext` with no-op archive handlers

Additional embed features:
- **Theme sync**: initial theme from `?theme=light|dark` query param, then
  `coder:set-theme` postMessage from parent frame. Applied via
  `applyEmbedTheme()` which sets a class + `data-embed-theme` on `<html>`.
- **Navigation blocking**: `useBlocker` prevents in-app navigation;
  `postMessage` to parent frame for external navigation.
- **Copy/paste support**: for agents desktop within the embedded context.

---

## `hooks/` Directory

The `hooks/` directory contains extracted hooks within the React Compiler
scope:

- `useAgentsPWA.ts` — PWA installation and update prompts.
- `useAgentsPageKeybindings.ts` — keyboard shortcuts for the agents page.
- `useDesktopConnection.ts` — desktop VNC WebSocket connection management.
- `useFileAttachments.ts` — file attachment handling for chat input.
- `useGitWatcher.ts` — Git change WebSocket for the right panel.
- `useOverflowCount.ts` — count of items overflowing a container.
- `useSpeechRecognition.ts` — speech-to-text input.

---

## Open Questions

### Architecture

1. **`AgentChatPage.tsx` (was `AgentDetail.tsx`).** It fetches individual chat,
   paginated messages, parent chat, workspace, models, SSH config, desktop
   enabled, and owns model selection, send/edit/interrupt mutations, and
   streaming state. The rename happened but the decomposition question remains.

2. **`ChatContext.ts` is ~1,100 lines.** The chat store mixes framework-agnostic
   store logic with React-specific wiring (WebSocket lifecycle, REST sync, React
   Query cache updates). Should these be separated?

3. **Naming mismatch: `/agents` routes vs `/api/experimental/chats/` API.**
   The UI calls them "agents", the backend calls them "chats". The generated
   types use `Chat*` prefixes. Is there a plan to align naming, or is the
   divergence intentional (user-facing vs internal)?

4. **WebSocket events update React Query cache directly.** This avoids flicker
   but means the cache can drift if events are missed (e.g. network blip during
   reconnection). The current answer is "reopen + invalidate" for durable
   state, but live `message_part` output is still lossy. Is that acceptable for
   GA?

5. **`setTimeout(0)` coalescing for stream parts.** The `message_part` buffer
   flushes via `setTimeout(0)`. Under heavy tool-call output, how does this
   interact with React's rendering scheduler? Has this been profiled?

### Testing

6. **`ChatContext.test.tsx` is 3,240 lines** — the largest test file. Is this
   sustainable? Are there flakiness issues?

7. **`AgentChatPage.stories.tsx` (was `AgentDetail.stories.tsx`).** How much
   of the agent UX is covered by Storybook vs integration tests?

### Design

8. **The embed authentication flow uses postMessage.** Is there CSRF/origin
   validation on the bootstrap message? Could a malicious page embed the agent
   iframe and inject authentication?

9. **Tool renderers are dispatched by string name.** If the backend adds a new
   tool, the frontend falls back to `GenericToolRenderer`. Is there a process
   to ensure new tools get dedicated renderers, or is the generic fallback
   considered sufficient?

10. **Model/config ownership leaks into the frontend.** Create/detail pages
    need both `chatModels()` and `chatModelConfigs()`, then map
    `provider:model` back to `model_config_id`. Is that layering intentional, or
    should the backend expose a simpler user-facing model selection surface?

11. **Model config schema is generated from Go struct tags** via
    `scripts/modeloptionsgen` → `chatModelOptionsGenerated.json`. How is this
    kept in sync? Is it part of `make gen`?

12. **File uploads go through a separate endpoint** (`uploadChatFile`) that
    returns a file ID. The file is then referenced in messages via
    `FileReferenceNode`. But as noted in the backend doc, `chat_files` has no
    FK to `chats` — so uploaded files are orphaned when chats are archived.
