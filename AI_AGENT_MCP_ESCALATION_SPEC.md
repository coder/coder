# MCP Gateway Escalations: Human-in-the-Loop Tool Approval

Status: accepted plan, implementation in progress on `ais`. The desktop
bridge in `coder/sandbox` (client side of the API contract below) is done.

## Implementation status

Living checklist; update as slices land. Migration numbers are allocated
here to prevent collisions between parallel work.

| Slice | Contents                                                                                                                                                                                                          | Status              |
|-------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------|
| 1     | Tri-state tool rules (`action` on rules + `tool_default`), `mcptools.Evaluate`, gateway lists escalated tools but fail-closed denies calls, migration 000576                                                      | done (`5372142b3d`) |
| 2     | `mcp_gateway_escalations` table + tool-usage `disposition`/`escalation_id` columns + `resource_type` enum value (migration 000577), notification template (migration 000578), queries, dbauthz, audit table entry | done (`f69ed687b6`) |
| 3     | dRPC 1.7 (`CreateMCPGatewayEscalation` incl. notification enqueue, `WaitMCPGatewayEscalation`), gateway SSE hold replacing the interim denial, tool-usage dispositions recorded                                   | in progress         |
| 4     | Management API `GET /api/v2/mcp-gateway/escalations`, `POST .../{id}/approve|deny` (frozen contract below), codersdk client, lazy expiry on read, audit on resolution                                             | in progress         |
| 5     | Approvals page in the site + inbox notification deep link                                                                                                                                                         | pending             |
| 6     | Demo template example rule + docs                                                                                                                                                                                 | pending             |

V1 scope reductions (revisit later): escalation expiry is a fixed 5
minutes (no per-server `escalation_timeout` column yet); approve and deny
are both sponsor-only (no admin deny); grants ("approval sticks") remain
phase 2; the `watch` SSE endpoint remains an additive upgrade.

Companion to `AI_AGENT_MCP_GATEWAY_SPEC.md`. That spec defines two dispositions
for MCP tool calls through the gateway: permitted (forwarded upstream) and
blocked (denied locally with a JSON-RPC error). This spec adds the third:
**escalated**, where the call is held until a human approves or denies it.

## Decisions

- Only the **sponsor** (the human user the AI identity acts for, resolved
  server-side from the identity binding) may approve or deny an escalation.
  Site admins may view and deny, not approve on the sponsor's behalf.
- `tool_default` keeps `disabled` as its default. `escalate` is an explicit
  choice per rule or per server default.
- Everything **fails closed**: no answer, expired escalation, crashed
  component, or unreachable approver all resolve to a denied call.
- The AI identity's session token can never resolve escalations. Approval
  endpoints are management-plane and require real user authentication, which
  the identity's deliberately minimal scopes cannot reach.

## The core problem: holding a synchronous call

`tools/call` is a synchronous JSON-RPC request from an MCP client. Human
approval takes seconds to minutes. The gateway holds the request by answering
with `text/event-stream` (the gateway already speaks SSE for policy-filtered
responses), emitting `: keepalive` comments about every 10 seconds and MCP
`notifications/progress` when the client supplied a progress token. On
approval the call is forwarded upstream with credential injection as usual
and the result is relayed into the same stream. On denial or expiry the
stream carries a JSON-RPC error whose `data` names the escalation ID and
final status, so the model can explain what happened.

If the client gives up before the human answers, the escalation stays
pending until expiry. Approval then records a scoped grant
(sponsor, server, tool, arguments hash, short TTL of about 10 minutes) so the
model's retry of the identical call forwards immediately. Agents retry;
humans are slow. Without grants, every approval would race the client
timeout.

## Policy model

`codersdk.MCPServerToolRule` grows from `{tool, enabled}` to `{tool, action}`
with `action` one of `enabled`, `disabled`, `escalate`; the legacy boolean is
still decoded. `tool_default` accepts the same three values.
`coderd/mcptools.Allowed` becomes `Evaluate`, returning
`Permit | Block | Escalate`. Legacy allow/deny lists stay binary and are
applied first: a deny-listed tool cannot be escalated into existence.

## Data model

`mcp_gateway_escalations`: `id` (gateway-generated UUID), server config ID
plus slug and URL snapshots, `tool`, `input` (JSON arguments), `ai_agent_id`,
`sponsor_user_id`, `workspace_name` snapshot, `status`
(`pending | approved | denied | expired`), `created_at`, `expires_at`,
`resolved_at`, `resolved_by`. Snapshot columns without foreign keys, matching
the `ai_sandbox_*` retention rationale. Indexes on
`(sponsor_user_id, status)` and `(status, expires_at)`.

`mcp_gateway_escalation_grants` (phase 2): `sponsor_user_id`,
`server_config_id`, `tool`, `input_hash`, `expires_at`.

`aibridge_tool_usages` gains `disposition`
(`permitted | blocked | escalated_approved | escalated_denied |
escalated_expired`) and a nullable `escalation_id`, making the three
dispositions directly queryable.

## dRPC (aibridgedserver, proto 1.7)

- `CreateMCPGatewayEscalation`: inserts the row and enqueues the sponsor
  notification in the same transaction.
- `WaitMCPGatewayEscalation`: long-poll with a server deadline; aibridged
  re-calls until the escalation is terminal or the gateway hold expires.
- `CheckMCPGatewayEscalationGrant` (phase 2): consulted before creating a new
  escalation.

## Management API (coderd)

This contract is frozen; the desktop bridge in `coder/sandbox` already
implements the client side.

```text
GET /api/v2/mcp-gateway/escalations?status=pending
  -> 200 [Escalation]

Escalation:
  id             uuid
  server_slug    string
  tool           string
  input          string (JSON-encoded arguments)
  workspace_name string
  status         pending | approved | denied | expired
  created_at     RFC 3339
  expires_at     RFC 3339

POST /api/v2/mcp-gateway/escalations/{id}/approve -> 200; 409 if resolved
POST /api/v2/mcp-gateway/escalations/{id}/deny    -> 200; 409 if resolved
```

Authentication is the standard `Coder-Session-Token`. Listing returns only
the caller's own escalations (sponsor scope); resolution is a single
`UPDATE ... WHERE status = 'pending'` so races are idempotent and auditable
(new auditable resource type). Overdue rows are flipped to `expired` lazily
on read plus a periodic sweep.

## Notifications and web UI

A new notification template ("MCP tool call awaiting your approval") targets
the sponsor over inbox and email with tool name, server slug, and workspace,
plus an action link to the approvals page. Arguments are deliberately
excluded from notification payloads; they can embed secrets or injected
content, and render only on the authenticated approvals page.

The site gains an approvals page listing pending escalations with a JSON
arguments viewer, approve/deny buttons, and expiry countdowns. The inbox
notification deep-links to it.

## Desktop surface: `ui/escalation-ui` plus bridge (implemented)

The sandbox project ships a resident Wails desktop app that owns a per-user
unix socket and blocks `/escalate` POSTs until the human answers, failing
closed when the request context drops. Its wire contract (protocol 3)
already carries an optional MCP `ToolCall{server, name, arguments}` payload.

The app is reused **unchanged**. A new stdlib-only bridge binary
(`ui/escalation-ui/cmd/coder-escalation-bridge`) connects it to coderd:

1. Polls `GET /escalations?status=pending` with the sponsor's session token.
2. For each new escalation, holds a blocking `/escalate` POST against the
   local socket with `proto: "mcp"`, the tool payload, and
   `can_remember: false` (remember maps to grants in phase 2, never to
   admin tool rules).
3. Maps the human verdict to `POST .../approve` or `.../deny`.
4. Cancels the in-flight prompt (context cancellation, which the app already
   handles) when the escalation disappears from the pending list because it
   expired or was resolved on the web.

The web surface is the correctness baseline; the desktop app is a latency
optimization. Both act through the same API, so either can answer.

## Failure modes

| Failure                         | Outcome                                                                     |
|---------------------------------|-----------------------------------------------------------------------------|
| Notification delivery fails     | Escalation pending on approvals page; expires                               |
| aibridged crashes mid-hold      | Client request dies; escalation expires; retry re-escalates or hits a grant |
| Client times out first          | Approval recorded as grant; model retry forwards                            |
| Sponsor never answers           | `expired`; structured deny recorded                                         |
| Bridge or desktop app absent    | Web inbox and approvals page still work                                     |
| AI identity token calls approve | 403; management-plane auth required                                         |

## Phasing

1. Core loop: tri-state rules, migrations, dRPC, SSE hold, approve/deny API,
   inbox notification, minimal approvals page, tool usage dispositions.
2. Ergonomics: grants ("approval sticks"), audit logging, expiry sweep,
   disposition counts in the admin UI, client timeout env in the demo
   template.
3. Desktop: the bridge and Wails app (client side already implemented in
   `coder/sandbox`), remember-as-grant.
