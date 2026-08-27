# AI Agent Audit Trail: Owner-Scoped End-to-End Timeline

Assembles what an AI agent is, what authority and credentials it holds, and
what it did, into one owner-filterable read surface: per-source query
plumbing, a merged timeline API in coderd, and an AI Activity page.

This is a feature spec, not part of the `poc_audit/` design corpus. It builds
a read surface over records that corpus defines. Throughout this document
"audit" never means the `audit_logs` table; where that mechanism is meant it
is named explicitly.

## Implementation status

| Slice | Contents                                                                                                                                                      | Status                             |
|-------|---------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------|
| 1     | `ai_audit_trail` RBAC resource, owner-scoped time-windowed queries over the three lifecycle journals, the use journal, and the egress logs (migration 000593) | done (`522f28120e`)                |
| 2     | `GET /api/v2/ai-audit/timeline` merged event feed (frozen contract below), codersdk client, tests                                                             | done (`ab9c14d4cb`)                |
| 3     | AI Activity page in the site (`/ai-activity`)                                                                                                                 | done (`dcc4d65881`)                |
| 4     | Docs                                                                                                                                                          | this spec is the PoC documentation |

## Grounding in the design corpus

Positions this feature is built to respect (see `poc_audit/AGENTS.md` for
standing conventions):

- **Journals and logs compose; ledgers do not sequence.** A timeline is
  assembled from the chronological journals plus the activity logs. Ledgers
  are current-state folds used only to resolve identity and enrich display
  (`journal_vs_log.md`, Established).
- **Rendering is on-demand**, shaped by the request. No materialized event
  table exists; the endpoint merges bounded per-source queries at read time
  (`audit_approach.md`, Established).
- **Both dates are preserved.** Journal-sourced events carry `occurred_at`
  (effective date) and `recorded_at` (recording date). Merged cross-source
  ordering by effective date is presentation, not a total-order claim;
  within one journal, entry ids order entries (Established).
- **Owner is a pair.** The filter resolves to `(owner_type, owner_id)` from
  `ai_agent_ledger`. v1 accepts user owners, which is the only owner type
  minted today (`entity_model.md`, Established).
- **Current-owner semantics.** `owner=` filters by present ownership: the
  owner's agent set is resolved from the ledger, then journals are read by
  those subjects. Nothing transfers ownership today, so this coincides with
  event-time ownership; if transfers arrive, an event-time filter is a new
  parameter, not a change to this one (`entity_model.md`, Open, flagged).
- The egress records (`ai_sandbox_sessions`, `ai_sandbox_network_events`)
  are activity logs, not journals; they carry server-resolved `ai_agent_id`
  and `sponsor_user_id` snapshots, which for the log sources stand in for
  the ledger owner.

## Decisions

- New site-level RBAC resource **`ai_audit_trail`** (action `read`), granted
  to the `auditor` and `owner` roles. `owner=me` is always allowed and needs
  no permission; naming another owner requires `ai_audit_trail:read`. The
  existing `audit_log` resource is untouched.
- **Credential presentations are individual events**, both accepted and
  refused. No aggregation: refusals are a security signal and acceptances
  are the agent's authentication trace.
- **Egress network events are aggregated** per (session, host, action)
  bucket; raw events stay behind the existing per-session drill-down.
- Merge in Go, not SQL UNION. Poll, not SSE.

## Sources and event vocabulary

| Timeline `type`           | Source                                                         | `detail.event` values                           |
|---------------------------|----------------------------------------------------------------|-------------------------------------------------|
| `ai_agent_lifecycle`      | `ai_agent_lifecycle_journal` (+ `_create` line)                | `create`, `finish`, `kill`                      |
| `authorization_lifecycle` | `authorization_lifecycle_journal` join ledger by agent         | `grant`, `lapse`                                |
| `credential_lifecycle`    | `credential_lifecycle_journal` (+ lines) join ledger by holder | `issue`, `revoke`, `lapse`, `discharge`         |
| `credential_use`          | `credential_use_journal` join ledger by holder                 | `presentation_accepted`, `presentation_refused` |
| `sandbox_session`         | `ai_sandbox_sessions` (log)                                    | `started`, `ended`                              |
| `egress`                  | `ai_sandbox_network_events` (log, aggregated)                  | `allowed`, `denied`                             |

Journal event words are exposed verbatim; the timeline does not rename them.

## Timeline API (frozen contract)

```text
GET /api/v2/ai-audit/timeline
  ?owner=<user id | username | "me">     default "me"; current-owner semantics
  &after_time=<RFC3339>                  optional lower bound (exclusive)
  &before_time=<RFC3339>                 optional upper bound (exclusive)
  &ai_agent_id=<uuid>                    optional single-agent filter
  &types=<comma-separated types>         optional; default all
  &limit=<1..1000>                       default 100
```

Response:

```jsonc
{
  "events": [
    {
      "id": "<source row uuid or synthetic journal:entry:line id>",
      "type": "credential_use",
      "occurred_at": "2026-08-27T12:00:00Z",  // effective date
      "recorded_at": "2026-08-27T12:00:02Z",  // recording date; log sources: creation time
      "ai_agent_id": "<uuid>",
      "owner": { "type": "user", "id": "<uuid>", "username": "..." },
      "workspace_id": "<uuid, omitted when unknown>",
      "summary": "presentation accepted (api_key)",
      "detail": { /* type-specific, content-free */ }
    }
  ],
  "count": 123 // events returned; no total across heterogeneous sources
}
```

Semantics:

- Newest-first by `occurred_at`, tiebroken by `(type, id)`. This order is
  presentation; within one journal, `detail.entry_id` is authoritative.
- Pagination is time-windowed: pass `before_time` = the `occurred_at` of the
  last event received. Per-source queries are capped at `limit`, so a page
  is complete down to its oldest returned event.
- All queries are system-guarded with handler-level owner scoping; the
  endpoint sits behind the standard `apiKeyMiddleware`. An AI agent
  credential is not a user session and cannot reach the management plane.

Event `detail` payloads (v1):

| Type                      | Detail fields                                                                                                     |
|---------------------------|-------------------------------------------------------------------------------------------------------------------|
| `ai_agent_lifecycle`      | `event`, `entry_id`, `actor_type`, `actor`, `creation_site_type`, `creation_site_id` (create only)                |
| `authorization_lifecycle` | `event`, `entry_id`, `actor_type`, `actor`, `authorization_id`, `principal_type`, `principal_id`                  |
| `credential_lifecycle`    | `event`, `entry_id`, `actor_type`, `actor`, `credential_id`, `credential_type`, `token_name` (api_key issue only) |
| `credential_use`          | `event`, `entry_id`, `actor_type`, `actor` (the verifier), `credential_id`, `source` (annotation, may be empty)   |
| `sandbox_session`         | `event`, `session_id`, `egress_enforcement`, `duration_ms` (ended only)                                           |
| `egress`                  | `event`, `session_id`, `host`, `port`, `protocol`, `count`                                                        |

## Registry

No new registry endpoint: `GET /api/v2/users/{user}/ai-agents` already
serves the owner's agents from the ledger and anchors the UI's agent picker.

## Web UI (slice 3)

`/ai-activity`: owner picker (default "me"; other owners visible only with
`ai_audit_trail:read`), the owner's agents with ledger state and creation
site, type filters, time range, timeline list with per-type icons. Journal
events render both dates when they differ. Egress rows link to the
workspace's per-session drill-down. Poll every 10s.

## Failure modes

| Failure                               | Outcome                                                |
|---------------------------------------|--------------------------------------------------------|
| Source unused / feature not exercised | Timeline simply lacks those event types                |
| Owner user deleted                    | Ledger rows persist; owner block carries id only       |
| Agent retired                         | Full history remains readable (journals are permanent) |
| Non-auditor names another owner       | 403                                                    |
| AI agent credential calls the API     | 401 (not a user session)                               |

## Explicitly out of v1

Event-time ownership filtering (requires ownership history; nothing
transfers today), non-user owner types in the filter, SSE/live updates, CSV
export, total counts, `credential_expired` events (expiry sweeps are pending
in the corpus work breakdown), and lifecycle journals for sessions,
sandboxes, and workspaces (Absent in the corpus; the egress logs stand in).
