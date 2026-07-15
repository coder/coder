# Coder Agents frontend data flow

## Status

This document defines the current backend contracts and the intended frontend
state ownership for Coder Agents. It is an implementation architecture document,
not a description of the public product API.

The frontend source-role migration is complete. React Query owns durable REST and
snapshot projections. `ChatStreamStore` owns only transient preview, retry, and
transport presentation. New data-flow work must follow the ownership rules below
and must not add another durable source of truth.

## Backend authority

PostgreSQL is the durable authority for chat execution state. Mutations run
through `chatstate.ChatMachine`, which serializes transitions and publishes
post-commit hints. Worker ownership, runner fencing, heartbeats, and internal
pubsub versions are chatd implementation details. The browser must not reproduce
those mechanisms.

The frontend receives durable state through REST and two public WebSockets. The
WebSockets have different contracts and must not be treated as equivalent event
sources.

## Transport contracts

### REST

REST is the correctness source for:

- Chat list membership, ordering, filtering, pins, sharing, unread state, and
  embedded child summaries.
- Chat detail metadata that is not projected by the per-chat stream.
- Initial paginated committed message history and the initial queue snapshot.
- Prompts, ACL, diff contents, model configuration, and related resources.

React Query owns REST data and all durable state reconciled from server events.
Queries remain independent resource projections. React Query is not used as a
normalized entity cache.

### Per-chat stream

`/api/experimental/chats/{chat}/stream` is a database-backed snapshot
synchronizer for one chat plus a transient preview relay.

For every connection, chatd subscribes to internal updates before reading a
consistent database snapshot. Internal version checks and a periodic sync poller
repair duplicate, reordered, or dropped pubsub hints before events reach the
browser.

The public stream projects:

- Committed messages.
- Complete queue snapshots.
- Execution status and persisted errors.
- Pending action-required state.
- Retry state for the current generation attempt.
- Complete history replacement after a history reset.
- Transient preview parts and preview controls.

`after_id` filters already-known committed messages from the initial snapshot.
It is an optimization, not a general event cursor. Status, queue, errors, retry,
and action-required state are snapshot-derived on every connection.

A `history_reset` is followed by the complete current visible history. The
frontend must apply the replacement atomically, even when the event sequence is
split across WebSocket frames.

Preview parts are best effort and are not durable. Their identity is
`(history_version, generation_attempt, seq)`. Losing preview text is acceptable;
losing a committed message is not.

The per-chat stream is scoped to exactly one chat. Parent streams do not carry
child or subagent events. Opening a child conversation uses that child's detail
and stream. Owner trees use embedded `Chat.children` summaries patched by global
hints and repaired by REST. Embed and shared trees use root-detail snapshots with
15-second foreground polling, not one stream per visible child.

### Global chat watch

`/api/experimental/chats/watch` is an owner-scoped, best-effort projection hint
channel. It has no initial snapshot, replay cursor, periodic repair, or public
ordering version. Its `Chat` payload is sparse and is not a complete list or
detail representation.

The global watch is suitable for low-latency discovery, deletion, and
field-specific projection hints. REST refetch remains the correctness path for
list membership, ordering, filters, unread state, pins, sharing, children, and
metadata.

A global watch payload must never be merged wholesale into exact chat detail.
On connection or reconnection, active projections must be revalidated through
REST. Events racing a baseline refetch must leave the affected projection dirty
so it is revalidated again.

Shared viewers do not receive the owner's global watch stream. Active detail,
root-child snapshots, and metadata collections poll every 15 seconds while the
document is visible. A shared-reader stream closes when the document becomes
hidden. Visibility restoration or window focus must complete an exact-detail
refetch before the stream reopens. Exact-detail 403 or 404 responses evict the
complete chat family and every loaded collection occurrence, then navigate the
full page away or render not found in an embed.

### Workspace stream

The workspace WebSocket updates the workspace React Query resource only. On
reconnection, the workspace detail query is invalidated to recover missed
updates.

## Frontend state ownership

| State | Canonical frontend owner | Authoritative inputs |
|---|---|---|
| Chat detail metadata | React Query | REST and field-specific metadata reconciliation |
| Committed messages | React Query infinite query | REST bootstrap and per-chat snapshot stream |
| Queue | React Query | REST bootstrap and complete per-chat queue snapshots |
| Execution status and persisted error | React Query exact chat detail | REST bootstrap and active per-chat snapshot stream |
| Pending action-required state | React Query exact chat detail | Active per-chat snapshot stream |
| List, search, by-workspace, child, unread projections | React Query | REST, with global watch as a dirty hint |
| Preview parts and episode identity | `ChatStreamStore` | Per-chat transient preview events |
| Retry countdown presentation | `ChatStreamStore` | Per-chat snapshot, scoped to the active episode |
| Connection and transport errors | `ChatStreamStore` | Browser transport lifecycle |
| Drafts, attachments, panels, and selections | React state or browser storage | User interaction |

Committed message history has one frontend owner: the canonical React Query
infinite entry. Rendering uses a stable projection that flattens all pages,
deduplicates by message ID with the newest page taking precedence, and orders
messages by creation time with the ID as a deterministic tie-breaker. The cache
itself remains `InfiniteData`; normal stream upserts preserve `pages` and
`pageParams`.

`ChatStreamStore` does not retain committed messages, queue rows, execution status,
persisted errors, or pending action state. It retains only transient preview,
retry, reconnect, and request or parse error presentation. A committed assistant
message is written to React Query before its preview is cleared, and both
operations occur in the same stream batch so the transcript never renders
duplicate durable and preview output.

A `history_reset` buffers its complete replacement across WebSocket frames, then
replaces the infinite entry with one complete page and matching page parameters.
Disconnecting during that run discards the partial replacement immediately. The
next connection starts from a fresh stream snapshot and triggers broad repair
invalidation.

The queue uses the same canonical messages infinite entry as committed history.
Page 0 owns the complete current `queued_messages` snapshot, and rendering reads
the stable query projection directly. Every stream `queue_update` replaces that
snapshot without filtering or merging. Send, promote, delete, and edit mutations
apply bounded optimistic updates to this cache, use guarded rollback so newer
stream snapshots win, and always schedule detail and messages repair on
settlement. Mutation response rows are latency hints only; they never replace a
complete queue snapshot.

The active per-chat stream becomes the post-bootstrap writer for execution
fields. The global watch must not overwrite those exact-detail fields. A stale
REST metadata response must preserve stream-owned execution fields while the
stream is active.

The stream claims tokenized execution ownership when its socket opens. While the
claim is active, an exact-detail REST resolution preserves the current cached
status, `last_error`, and complete `action_required` payload while accepting
fresh metadata. Cleanup cancels the exact-detail request before releasing only
the matching ownership token, so an older stream cannot release a newer claim.
The socket starts only after both exact detail and the first messages page have
bootstrapped.

Status, error, and action events are applied through one exact-detail transition
function. Non-error statuses clear `last_error`, statuses other than
`requires_action` clear pending action state, errors store `last_error` and clear
action state, and action events store all pending calls while clearing errors.
Global watch status hints continue updating list and child projections but never
write active exact-detail execution fields.

Create-message success sets exact-detail status to `running` only when the server
reports that the message was not queued. Edit and queue promotion optimistically
set exact-detail status to `running` and clear old error and action state. Their
rollbacks are identity-guarded so a newer stream snapshot always wins.

A public execution `snapshot_version` is not required while source roles remain
non-overlapping. If it is exposed later, it can fence execution fields only. It
cannot order metadata such as title, labels, pins, ACL, context, summary, or
workspace binding because those values can change outside chatstate.

## Mutation reconciliation

Mutation responses may be useful for immediate UI feedback, but they are not
always complete state-machine snapshots. For example, a queued send can also
promote the previous queue head into committed history.

Optimistic mutations must:

1. Await cancellation of conflicting REST refetches.
2. Snapshot the previous React Query data.
3. Update the cache immutably.
4. Roll back the optimistic presentation on error.
5. Reconcile through the per-chat snapshot or targeted REST invalidation on
   both success and error.

The final reconciliation is required on error because the database transaction
can commit before a post-commit publication failure is returned to the HTTP
caller.

Metadata mutations settle through broad REST repair on both success and error.
The repair includes exact detail plus list, search, and by-workspace projections.
ACL mutations also repair the ACL projection. This policy applies to create,
archive and unarchive, title, pin and reorder, workspace binding, plan mode,
message-carried plan mode, and archive-with-workspace-deletion. Optimistic
rollback restores only the field changed by the mutation, so unrelated newer
state is preserved. Labels are not supported by the current frontend and have no
frontend mutation or reconciliation contract.

Deletion removes the complete deleted subtree query family, including detail
children such as messages, prompts, ACL, diff, and debug queries, and removes all
loaded list, search, by-workspace, and embedded-child occurrences. Access loss
evicts the complete root family. Archive removes active occurrences immediately,
then REST invalidation determines archived search and by-workspace membership.

## Query boundaries

Chat queries must be defined with co-located `queryOptions` or
`infiniteQueryOptions` factories. Keys must uniquely identify their resource and
include every changing query-function dependency. QueryClient operations must
reuse the typed key exposed by the option factory.

Imperative chat cache writes belong behind typed domain operations. Page
components and WebSocket callbacks must not construct ad hoc keys or directly
coordinate several cache shapes.

Infinite query updates must preserve `pages` and `pageParams`. A complete
`history_reset` intentionally replaces the paginated history with one complete
page and matching page parameters.

## Reconnect rules

For the per-chat stream:

- Clear connection-scoped preview and transport state on open.
- Use the greatest committed message ID only to suppress initial message
  duplication.
- Consume the fresh chatd snapshot as the durable recovery path.
- Discard an incomplete multi-frame history replacement and let the next fresh
  snapshot restart it.
- Refetch REST metadata not carried by the stream.

For the global watch:

- Revalidate active list, search, by-workspace, child, and relevant metadata
  projections through REST.
- Track dirty chat IDs and projection kinds while the baseline is unresolved.
- Revalidate dirty projections after the baseline settles.
- Apply direct patches only when the event contains authoritative fields and
  projection membership is known.

### Freshness policy

Active list, search, by-workspace, exact-detail, and root-child metadata
projections poll every 15 seconds while the document is visible and also repair on
mount, focus, and network reconnect. Under working connectivity this gives a
best-effort 15-second foreground freshness bound. There is no strict guarantee
while hidden or offline. The messages infinite query disables automatic
refetching and uses the per-chat database snapshot stream plus explicit failure
invalidation as its freshness path. Prompts, ACL, diff, and debug resources have
resource-specific stale and retry policies rather than inheriting application
defaults.

A malformed per-chat stream payload damages that connection. The frontend
fences later events from the socket, closes it to trigger a fresh chatd snapshot,
and invalidates detail, messages, prompts, list, search, and by-workspace
projections. An interrupted multi-frame history reset is discarded and triggers
the same repair path when the next connection opens.

The owner projection watch is also mounted while the Workspaces page has active
chat-by-workspace projections, avoiding a second freshness mechanism for that
route while ensuring only one route-level watch is active at a time.

## Final product boundaries

- Unread state is owner-scoped in the current backend. The frontend hides unread
  indicators and unread grouping for non-owner shared chats. A per-viewer unread
  product requires new backend state.
- Shared grant and revoke discovery uses foreground polling and focus repair.
  There is no strict hidden-document or offline freshness guarantee.
- Embed and shared child status is snapshot based through root-detail polling.
  The frontend does not open one stream per visible child.
- Chat labels are not supported by this frontend.
- Strict source ownership is sufficient today. A public execution snapshot
  version is considered only if stale REST execution writes cannot otherwise be
  fenced, and it must not become a metadata revision.

## Contract coverage

Phase 1 characterizes the current contract through the following tests:

- `AgentsPage.integration.stories.tsx` mounts the real Agents route with one
  QueryClient, REST query functions, the global watch, and the per-chat stream.
  It verifies REST message hydration gates the per-chat connection, `after_id`
  is the last committed message ID, a committed snapshot message reaches the
  rendered transcript, and a global socket open repairs a missed projection
  hint through REST.
- `components/ChatConversation/chatStreamStore.test.tsx` covers duplicate snapshot
  messages, reconnects with the latest message ID, stale preview cleanup, chat
  navigation isolation, complete history replacement across WebSocket frames,
  queue snapshots, durable versus transient event separation, and
  preview-to-committed handoff.
- `AgentChatPage.test.ts` covers queue promotion, editing, drafts, and transient
  request error presentation.
- `api/queries/chats.test.ts` covers immutable chat/message cache operations,
  edit reconciliation, queue mutations, global projection merges, deletion,
  and invalidation boundaries.
- `coderd/x/chatd/chatd_test.go` covers fresh subscription snapshots, duplicate
  suppression, `after_id` message filtering, and preview replay behavior.
- `coderd/x/chatd/stream_loop_internal_test.go` covers deterministic snapshot
  event ordering, complete history reset, full queue snapshots, status, error,
  retry, action-required, and preview reset.
- `coderd/exp_chats_test.go` covers the public stream route, authorization, and
  event chat-ID scoping.
- `coderd/x/chatd/chatstate/machine_test.go` verifies publications occur after
  commit and that a post-commit publication error can be returned while the
  state transition remains durable.

Focused frontend tests cover the final projection, mutation, polling, access-loss,
unread, deterministic-source, and `ChatStreamStore` boundaries described above.

## Related backend architecture

See `coderd/x/chatd/ARCHITECTURE.md` for the state machine, database versions,
worker ownership, stream synchronization, preview relay, and subagent lifecycle.
When a backend stream contract changes, update that document and this document
in the same change.

## Explicit unsupported behavior

`action_required` renders an explicit unsupported state from canonical exact
detail. This frontend does not execute dynamic tools or submit tool results.
Adding that behavior requires a separate product and API design covering
execution, timeout, cancellation, errors, security, and accessibility.

## Query enforcement

Coder's frontend uses Biome rather than ESLint, so the TanStack Query ESLint
rules (`prefer-query-options`, `exhaustive-deps`, `no-rest-destructuring`,
`no-unstable-deps`, and `infinite-query-property-order`) cannot be enabled in
the current lint pipeline. Chat queries use `queryOptions` and
`infiniteQueryOptions` factories instead. The `lint:chat-cache` check prevents
production Agents code from constructing chat query keys directly, which keeps
imperative cache reads and writes behind the typed cache operations in
`src/api/queries/chats.ts`.
