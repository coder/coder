# Tier 3 Data Protection Mode — Database Analysis

## The Question

For Tier 3 (irreversible, at-rest protection), should we:
- (A) Store data with permanently obfuscated user IDs, or
- (B) Not store user-identifying data at all, or
- (C) Keep storing it as-is (because the system breaks without it)?

## Table-by-Table Classification

### Category 1: CONFIGURATION — Cannot touch (system breaks)

These tables store user identity for access control, routing, or
core system operation. Tier 3 cannot obfuscate or omit this data.

| Table | Column | Why It's Essential |
|---|---|---|
| `users` | `id`, `username`, `email` | Authentication, RBAC, session management |
| `organization_members` | `user_id` | Org membership, authorization checks |
| `group_members` | `user_id` | Group-based RBAC |
| `workspaces` | `owner_id` | Workspace access control, RBAC ownership |
| `templates` | `created_by` | RBAC ownership checks |
| `chats` | `owner_id` | Chat isolation (SQL WHERE filters), pin ordering |

**Recommendation:** Leave as-is. Tier 3 obfuscation happens at
read time (as in Tier 1/2) for these tables. The data must exist
for the system to function.

### Category 2: PURE ANALYTICS — Can stop storing

These tables exist solely for reporting, insights, and analytics.
The system functions identically without per-user-level data in
them.

| Table | Column | Current Use | Tier 3 Approach |
|---|---|---|---|
| `template_usage_stats` | `user_id` | Feeds `/insights/user-activity` and `/insights/user-latency`. Also used as `COUNT(DISTINCT user_id)` for template-level DAU counts. | **Option B: Stop storing per-user rows.** Instead, aggregate into a single row per `(start_time, template_id)` with summed metrics and a distinct user count. Individual user activity becomes invisible at rest. |
| `connection_logs` | `workspace_owner_id` | Display-only in the connection log viewer. Not used for routing or billing. Retention-purged. | **Option B: Store NULL or a sentinel value** instead of the real owner ID. The connection event is still logged (workspace_id, type, time) but the owner identity is gone. |

### Category 3: AUDIT TRAIL — Can obfuscate at write time

These tables record "who did what" but the identity is not used for
any system decision after insertion. The data is write-once,
read-for-display.

| Table | Column | Current Use | Tier 3 Approach |
|---|---|---|---|
| `audit_logs` | `user_id` | Display in audit log viewer. Nullable (system actions have no user). Filtering by user in the UI. Not used for compliance exports or access control. Retention-purged. | **Option A: Store HMAC-obfuscated user ID at write time.** Use a server-scoped (non-rotating) HMAC key so the same user always maps to the same pseudonym, preserving "same actor" correlation without revealing identity. Alternatively **Option B** (NULL) if even pseudonymous correlation is unwanted. |
| `workspace_builds` | `initiator_id` | "Started by" display in UI. Not used for RBAC, provisioner routing, or lifecycle management. | **Option A: Store HMAC-obfuscated ID.** The provisioner doesn't need the real initiator to function. Build attribution becomes pseudonymous. |

### Category 4: MIXED — Functional + Analytics

These tables have user identity that serves both a functional
purpose AND an analytics/display purpose. This is the nuanced
case.

| Table | Column | Functional Use | Analytics Use | Tier 3 Approach |
|---|---|---|---|---|
| `aibridge_interceptions` | `initiator_id` | **AI seat billing/quota tracking** via `aiSeatTracker.RecordUsage()`. Also used for API access control filters. | Display in AI Bridge log viewer and session list. Telemetry aggregation (`COUNT(DISTINCT initiator_id)`). | **Cannot fully remove.** Two sub-approaches: **(1)** Store the real ID but strip it from the record after the seat tracker has consumed it (async, within minutes). **(2)** Maintain a separate `ai_seat_usage` ledger table that records seat consumption without tying back to individual interceptions, then NULL out `initiator_id` on the interception row. |
| `chat_messages` | (via `chats.owner_id`) | Chat delivery, access control. | Cost reporting via `/chats/cost/users`. | **Cannot remove `chats.owner_id`** (breaks chat routing). But the cost reporting joins through it. Tier 3 cost reporting should return aggregate-only data (total cost across all users, no per-user breakdown). |

## Recommended Tier 3 Design

```
Tier 3 = "No individual monitoring data persists at rest"
```

| Data Category | Action | Mechanism |
|---|---|---|
| **Config tables** (users, orgs, workspaces, chats) | No change at DB level | Obfuscation at read time (same as Tier 2) |
| **template_usage_stats** | Stop storing per-user rows | Modify `UpsertTemplateUsageStats` to aggregate into `(start_time, template_id, NULL)` rows. User-level insights endpoints return empty. Template-level DAU counts use a separate `distinct_users` counter column. |
| **connection_logs** | NULL out owner identity | Set `workspace_owner_id` to a well-known sentinel UUID at insert time. Connection events are still logged for operational monitoring. |
| **audit_logs** | Obfuscate at write time | Store HMAC(user_id) using a deployment-scoped persistent key. Preserves "same actor" correlation for security investigation without revealing identity. |
| **workspace_builds** | Obfuscate at write time | Store HMAC(initiator_id) using the same persistent key. Build history is still navigable. |
| **aibridge_interceptions** | Decouple billing from identity | (1) Record seat usage in a separate ledger at interception time, (2) NULL out `initiator_id` after seat tracking completes (async, < 1 min). Interception records remain for aggregate analytics without user attribution. |
| **Prometheus metrics** | Remove user labels | Strip `workspace_owner` label entirely (not just pseudonymize). Metrics become `{status, template_name, template_version, workspace_transition}` only. |
| **Telemetry exports** | Already not obfuscated by design | No change. Telemetry sends aggregate counts, not individual user data. |

## Key Design Decision: HMAC Key for Tier 3

Unlike Tier 1/2 where the HMAC key rotates per server restart
(pseudonyms change each boot), Tier 3 needs a **persistent
deployment-scoped key** stored in the database or derived from a
deployment secret. This is because:

- Data is obfuscated at WRITE time (not read time)
- The same user must produce the same pseudonym across restarts
- Without this, audit log correlation breaks after restart

This key should be generated once at Tier 3 enablement and stored
in `site_configs` or derived from the deployment's
`CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS`.

## What Tier 3 Does NOT Touch

- User CRUD (accounts, profiles, org membership) — functional
- Workspace CRUD (create, start, stop, delete) — functional
- Template CRUD (create, update, delete) — functional
- Group CRUD (create, manage) — functional
- Chat delivery and routing — functional
- Provisioner job execution — functional
