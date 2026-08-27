# AI Agent Audit: Sponsor-Scoped End-to-End Activity Timeline

Everything an agentic identity does is already recorded with a
`sponsor_user_id` snapshot (the accountable human). This spec unifies those
records into one sponsor-filterable audit surface: per-source query plumbing,
a merged timeline API in coderd (AGPL, not the enterprise aibridge handler),
and an AI Activity page.

## Implementation status

| Slice | Contents                                                                                                                                                                   | Status              |
|-------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------|
| 1     | Sponsor becomes RBAC owner of AI-initiated interceptions (`RBACObject` + rego converter + authorized SQL filter), `sponsor:` filter in the bridge sessions search language | done (`cb00ca1f41`) |
| 2     | Sponsor-scoped list queries: sandbox sessions, aggregated network events, escalations (sponsor param), AI agent registry endpoint                                          | done (`a4d7e40c35`) |
| 3     | `GET /api/v2/ai-audit/timeline` merged event feed (frozen contract below), codersdk client                                                                                 | done (`d2714f9429`) |
| 4     | AI Activity page in the site (`/ai-activity`)                                                                                                                              | done (`910ddfd8f2`) |
| 5     | Docs                                                                                                                                                                       | done                |

## Decisions

- **Sponsor is the RBAC owner** of AI-initiated bridge interceptions. The
  agent initiated the request; the sponsor is the accountable principal.
  Human-initiated interceptions (no sponsor) keep the initiator as owner.
- **Auditors can cross sponsors**: `sponsor=me` is always allowed; naming
  another user requires `audit_log:read` (auditor/owner), mirroring
  `/api/v2/audit` semantics.
- **The timeline lives in coderd**, not the enterprise aibridge handler, so
  the audit surface does not depend on an aibridge license entitlement even
  though some sources (interceptions) are populated by enterprise code.
- **Merge in Go, not SQL UNION**: the sources are heterogeneous tables, each
  already indexed by sponsor. The handler runs bounded per-source queries and
  merges by timestamp.
- **Content stays out of the timeline**: prompts, model thoughts, and tool
  inputs are not in the envelope. The timeline answers "what happened";
  drill-down surfaces (bridge session threads, escalation viewer, per-session
  egress table) answer "what was said".
- **Network events are aggregated** per (session, host, action) bucket in the
  timeline. Raw events stay behind the existing per-session drill-down.
- Poll, not SSE. Live updates are an additive upgrade.

## Sources

| Event types                                        | Table                                            | Sponsor column    |
|----------------------------------------------------|--------------------------------------------------|-------------------|
| `sandbox_session_started`, `sandbox_session_ended` | `ai_sandbox_sessions`                            | `sponsor_user_id` |
| `egress` (aggregated allowed/denied per host)      | `ai_sandbox_network_events`                      | `sponsor_user_id` |
| `bridge_session_started`                           | `aibridge_interceptions` (grouped by session_id) | `sponsor_user_id` |
| `tool_call` (with disposition)                     | `aibridge_tool_usages` join interceptions        | via interception  |
| `escalation_created`, `escalation_resolved`        | `mcp_gateway_escalations`                        | `sponsor_user_id` |

The identity registry (`ai_agents.owner_user_id`, query
`GetAIAgentsByOwnerID`) lists which agentic identities a sponsor owns and
anchors the sponsor picker plus per-agent filtering.

## Timeline API (frozen contract)

```text
GET /api/v2/ai-audit/timeline
  ?sponsor=<user id | username | "me">   default "me"
  &after_time=<RFC3339>                  optional lower bound (exclusive)
  &before_time=<RFC3339>                 optional upper bound (exclusive)
  &ai_agent_id=<uuid>                    optional single-agent filter
  &types=<comma-separated event types>   optional; default all
  &limit=<1..1000>                       default 100
```

Response:

```jsonc
{
  "events": [
    {
      "id": "<source row uuid or synthetic uuid>",
      "type": "egress",                       // see event types above
      "occurred_at": "2026-08-27T12:00:00Z",
      "ai_agent_id": "<uuid>",
      "sponsor": { "id": "...", "username": "...", "avatar_url": "..." },
      "workspace_id": "<uuid, omitted when unknown>",
      "workspace_name": "<name, may be empty>",
      "summary": "denied tcp github.com:443 (x12)",
      "detail": { /* type-specific, content-free */ }
    }
  ],
  "count": 123 // events returned; no total (heterogeneous sources)
}
```

Semantics:

- Events are returned newest-first, merged across sources by
  `(occurred_at, type, id)`.
- Pagination is time-windowed: pass `before_time` = the `occurred_at` of the
  last event received to fetch the next page. Per-source queries are capped
  at `limit`, so a page is complete down to its oldest returned event.
- `sponsor` other than the caller requires `audit_log:read` on site level;
  otherwise 403. Unknown sponsor usernames return 400.
- Egress aggregation: one event per (session, host, action) bucket within the
  window, `occurred_at` = latest occurrence, `detail.count` = occurrences,
  `detail.protocol`/`detail.port` from the latest event.
- All queries are system-guarded with handler-level sponsor scoping (same
  pattern as the escalations API); the endpoint sits behind the standard
  `apiKeyMiddleware`, so AI identity tokens (scoped, no `audit_log` perms and
  not the sponsor) cannot read their sponsor's timeline unless the sponsor is
  the caller.

Event `detail` payloads (v1):

| Type                      | Detail fields                                                             |
|---------------------------|---------------------------------------------------------------------------|
| `sandbox_session_started` | `egress_enforcement`                                                      |
| `sandbox_session_ended`   | `egress_enforcement`, `duration_ms`                                       |
| `egress`                  | `session_id`, `host`, `port`, `protocol`, `action`, `count`               |
| `bridge_session_started`  | `session_id`, `client`, `providers`, `models`                             |
| `tool_call`               | `interception_id`, `server_url`, `tool`, `disposition`, `escalation_id`   |
| `escalation_created`      | `escalation_id`, `server_slug`, `tool`, `expires_at`                      |
| `escalation_resolved`     | `escalation_id`, `server_slug`, `tool`, `status`, `resolved_by` (user id) |

## RBAC change (slice 1)

`AIBridgeInterception.RBACObject()` owner becomes `sponsor_user_id` when set,
else `initiator_id`. The rego-to-SQL converter
(`regosql.AIBridgeInterceptionConverter`) matches owner against
`COALESCE(sponsor_user_id, initiator_id)::text`. Effects:

- Ownership follows accountability. Note the member role deliberately grants
  only create/update on interceptions ("cannot read them back"), so plain
  member sponsors still cannot list their agents' sessions in the AI Bridge
  UI; auditors can, including with the new `sponsor:` filter. The
  sponsor-facing surface is the timeline, whose system-guarded queries are
  sponsor-scoped in the handler (like the escalations API).
- AI identities lose implicit read on their own interception rows (they are
  no longer the owner). Their tokens carry no aibridge read scope, so nothing
  observable changes for them; this is fail-closed in the right direction.

`searchquery.AIBridgeSessions` gains `sponsor:` (parsed with `parseUser`,
supports `me`), threaded through `ListAIBridgeSessions` /
`CountAIBridgeSessions` as `@sponsor_user_id` CASE filters.

## Registry API (slice 2)

```text
GET /api/v2/ai-audit/agents?sponsor=<user|me>
```

Returns the sponsor's agentic identities from `ai_agents`
(`GetAIAgentsByOwnerID`): `{user_id, username, owner_user_id, origin_type,
origin_id, created_at, deleted}`. Same authorization rule as the timeline.

## Web UI (slice 4)

`/ai-activity`: sponsor picker (default "me"; other sponsors visible only
with the audit permission), agent and type filters, time range, timeline list
with per-type icons and drill-down links (bridge session threads, workspace
egress table, approvals page). Poll every 10s. Reuses AIBridgePage filter
primitives and the MCPEscalationsPage layout conventions.

## Failure modes

| Failure                              | Outcome                                                       |
|--------------------------------------|---------------------------------------------------------------|
| Source table empty / feature unused  | Timeline simply lacks those event types                       |
| Sponsor user deleted                 | Snapshot IDs still returned; sponsor block carries id only    |
| Workspace deleted                    | `workspace_name` snapshot (escalations) or id-only (sessions) |
| Non-auditor names another sponsor    | 403                                                           |
| AI identity token calls the timeline | 403 (scoped token, not sponsor, no audit perm)                |

## Explicitly out of v1

SSE/live updates, CSV export, retention policy changes, total counts across
sources, linking interceptions to the exact sandbox session
(`agent_firewall_session_id` exists upstream on interceptions as a v2 hook),
and per-event deep links into third-party MCP servers.
