# AI Agent Security Architecture

Status: draft (PoC scope)

This document is the master plan for improving Coder security for AI agents.
It is split into three verticals, each written as a handoff-ready
implementation plan:

1. **AI agent identity attribution** (fully specified)
2. **Workspace enhancements and execution isolation** (fully specified)
3. Auditing of agent activity (stub, to be designed)

All plans assume AGPL placement and PoC-level depth. Coder Tasks are out of
scope.

---

## Vertical 1: AI Agent Identity

### Problem

AI agents acting through Coder today are indistinguishable from the humans
they act for:

- Coder Agents (chatd) execute platform actions with the exact same
  permissions as the chat owner (`docs/ai-coder/agents/architecture.md`
  documents this contract).
- In-workspace CLI agents (Claude Code, etc.) are commonly handed the
  workspace owner's session token
  (`data.coder_workspace_owner.me.session_token`), a full-privilege user
  credential minted in `coderd/provisionerdserver` at every workspace build.
- Audit logs collapse everything to `audit_logs.user_id`; there is no way to
  ask "what did the agent do" vs "what did the human do".

### Concept

Introduce **AI agent identities**: real principals, automatically minted
per chat (and later per agent workspace), durably attributed to a human
owner, and always bounded by that human's live permissions.

| Property            | Decision                                                                       |
|---------------------|--------------------------------------------------------------------------------|
| Principal model     | Real row in `users` with `kind = 'ai_agent'` + metadata side-table `ai_agents` |
| Granularity         | One identity minted per chat / per agent workspace                             |
| Audit semantics     | Actor = agent, `on_behalf_of` = human; queryable by both                       |
| Permission ceiling  | Agent perms ⊆ owner perms, enforced structurally at request time               |
| Reduction mechanism | Owner's live roles ∩ API key scopes ∩ allow list (existing machinery)          |
| Credentials         | Plain API keys owned by the agent user                                         |
| Creation            | Automatic at chat/workspace creation; no user-facing creation flow             |
| Lifecycle           | Cascade suspend from owner; revoke on origin deletion                          |
| Surfaces (v1)       | chatd (Coder Agents), in-workspace CLI agents, aibridge attribution            |

### Terminology (important)

"Agent" is heavily overloaded in this codebase. In all schema, API, and code
introduced by this plan, use **`ai_agent`** ("AI agent identity").
Never bare "agent". Specifically do not conflate with:

- **workspace agents** (`workspace_agents`, `coderd/httpmw/workspaceagent.go`):
  the process running inside a workspace.
- **subagents** (`SubAgentAPI`): dev-container child agents.
- **aibridge / AI Gateway**: the LLM proxy.
- **Coder Agents**: the chat product built on `coderd/x/chatd`.

### Core design: identity split

The single most important design point:

- **Authorization identity = the human owner.** The RBAC subject is built
  from the owner's live roles and groups (exactly like
  `rbac.WorkspaceAgentScope` does for workspace agents today), intersected
  with the agent key's scopes and allow list. `Subject.ID` is the OWNER's
  user ID; this is required because owner-scoped permission checks match
  `resource.owner_id == subject.ID`, and agents must operate on the owner's
  resources (their chats, their workspaces).
- **Actor identity = the agent.** The API key is owned by the agent's user
  row, so `audit_logs.user_id`, `aibridge_interceptions.initiator_id`, and
  request logging all attribute to the agent. The owner is recorded in the
  new `audit_logs.on_behalf_of_user_id` column.

This decoupling gives both properties we need with no new authz machinery:

```text
effective permissions = owner's live roles/groups
                        ∩ agent key scopes
                        ∩ agent key allow list
```

Because roles are fetched live per request (`UserRBACSubject`), the ceiling
invariant holds instantly: if the owner loses a role or is suspended, every
agent attributed to them loses it on their next request. No sync jobs.

#### Request flow

```text
Agent API key (owned by ai_agent user row)
  → httpmw.ExtractAPIKeyMW / ValidateAPIKey (coderd/httpmw/apikey.go)
    → load key's user; user.Kind == 'ai_agent'
    → load ai_agents row (owner_user_id, origin)
    → check owner exists, is active (401 otherwise: cascade suspend)
    → subject = UserRBACSubject(owner.ID, key.ScopeSet())
      with Subject.Type = SubjectTypeAIAgent,
           Subject.FriendlyName = agent username
    → stash AIAgentActor{AgentUserID, OwnerUserID, OriginType, OriginID}
      in request context for audit/logging
```

### Schema changes

Follow `.claude/docs/DATABASE.md` (queries in
`coderd/database/queries/*.sql`, `make gen`, audit table updates in
`enterprise/audit/table.go`, `make gen` again).

#### 1. `users.kind`

- New enum `user_kind`: `'human'` (default), `'ai_agent'`.
- New column `users.kind user_kind NOT NULL DEFAULT 'human'`.
- PoC scope: do NOT migrate `is_system` / `is_service_account` into `kind`;
  leave them as-is and note consolidation as future work. Backfill nothing.
- Agent user rows: `login_type = 'none'` (same as service accounts and the
  prebuilds system user; structurally blocks login/password/email flows),
  generated username (see below), email follows the service-account
  precedent (migration `000433_add_is_service_account_to_users`).
- `users.kind` is a new audited field: update `enterprise/audit/table.go`
  or `make gen` will fail (known pitfall).

Username generation: `ai-chat-<8 hex>` / `ai-ws-<8 hex>`; must satisfy the
existing username format constraints (lowercase alnum + hyphen, ≤32 chars)
and be collision-retried. Visibly non-human by construction.

#### 2. `ai_agents` table (authoritative marker + attribution)

```sql
CREATE TABLE ai_agents (
    user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    origin_type   ai_agent_origin NOT NULL,     -- enum: 'chat', 'workspace'
    origin_id     uuid NOT NULL,                -- chat ID or workspace ID
    created_at    timestamptz NOT NULL DEFAULT now(),
    deleted       boolean NOT NULL DEFAULT false
);
CREATE INDEX idx_ai_agents_owner ON ai_agents (owner_user_id);
CREATE UNIQUE INDEX idx_ai_agents_origin ON ai_agents (origin_type, origin_id)
    WHERE NOT deleted;
```

Design rule: **nothing may decide "is this an AI agent" from the users row
alone**; the `ai_agents` row is the authoritative marker and must be loaded
whenever `users.kind = 'ai_agent'`. A dangling kind without a metadata row
is an auth error (fail closed, 401).

No `chats.ai_agent_id` column is needed: `(origin_type='chat',
origin_id=chat.ID)` is the link, resolved via the unique partial index.

#### 3. `audit_logs.on_behalf_of_user_id`

- Nullable `uuid`, no FK (audit rows outlive users, matching existing
  audit_logs conventions), plus index.
- Query by agent: `user_id = $agent`. Query by human principal:
  `user_id = $human OR on_behalf_of_user_id = $human`.
- PoC: single-level delegation only. No chains (human ← chat-agent ←
  sub-execution is future work for the auditing vertical).

### RBAC changes (`coderd/rbac`)

- New `SubjectTypeAIAgent = "ai_agent"` alongside existing subject types.
- New scope profile constructors (suggested home: new package
  `coderd/aiagentidentity`, see below), built from the existing granular
  scope catalog (`coderd/rbac/scopes.go`, `scopes_catalog.go`):

| Profile                                      | Scopes (starting set, tune during impl)                                                            | Allow list                                                            |
|----------------------------------------------|----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------|
| `ChatAgentProfile(chatID, ...)`              | `coder:workspaces.create`, `coder:workspaces.operate`, `coder:workspaces.access`, chat read/update | chat-associated resources where expressible; wildcard where not (PoC) |
| `WorkspaceAgentIdentityProfile(workspaceID)` | modeled on `rbac.WorkspaceAgentScope`: workspace operate/access + `no_user_data` exclusion         | that workspace's ID                                                   |

Hard exclusions in every profile: `coder:apikeys.manage_self` (agents must
not mint or manage credentials), `coder:templates.author`, user-data scopes
unless explicitly needed. Profiles are hardcoded for PoC; org-configurable
profiles are future work.

### New package: `coderd/aiagentidentity`

Shared helpers so chatd and provisionerdserver don't duplicate logic:

- `Create(ctx, db, CreateParams{OwnerID, OrganizationID, OriginType,
  OriginID}) (database.User, database.AIAgent, error)`: inserts the agent
  user row (kind, login_type none, generated username) + `ai_agents` row,
  in a transaction (`InTx`, all work on the tx handle).
- `MintKey(ctx, db, agentUserID, profile) (database.APIKey, string, error)`:
  wraps `apikey.Generate` (`coderd/apikey/apikey.go`) with the profile's
  scopes/allow list; key `user_id` = agent user.
- `Resolve(ctx, db, agentUserID) (AIAgentActor, error)`: loads metadata +
  owner for middleware/audit.

### Middleware changes (`coderd/httpmw/apikey.go`)

In `ValidateAPIKey` / subject construction path:

1. After loading the key's user, branch on `user.Kind == 'ai_agent'`.
2. `Resolve` the agent; 401 if missing/deleted (fail closed).
3. Owner liveness check: owner must exist, not deleted, `Status = active`.
   This is the enforcement point for cascade suspend; an explicit
   suspend-cascade job is optional hardening, auth-time check is the
   invariant.
4. Build subject via the existing `UserRBACSubject(ownerID, ...)` with the
   key's `ScopeSet()`; override `Type`/`FriendlyName` for logging.
5. Store `AIAgentActor` in context (new context key, mirroring how
   workspace-agent identity is stashed separately from the subject today).

### Audit changes (`coderd/audit`, `enterprise/audit`)

- `audit.InitRequest` (`coderd/audit/request.go`): today
  `userID = key.UserID` (line ~510), which already yields actor = agent.
  Add: if `AIAgentActor` present in context, set `OnBehalfOfUserID`.
- Plumb the field through `audit.Request`, the exporter/backends, and the
  audit API (`GET /api/v2/audit`): add search filter `on_behalf_of:<user>`
  and include agent/owner summaries in responses. Frontend surfacing is
  optional for PoC (note for the auditing vertical).

### Surface integrations

#### chatd (Coder Agents), primary surface

- Mint the AI agent identity + API key inside chat creation:
  `coderd/exp_chats.go` `postChats` →
  `api.chatDaemon.CreateChat(ctx, chatd.CreateOptions{...})`
  (currently around line 1446). Creation of chat + agent + key should be
  transactionally consistent.
- Platform tool calls executed by the chat must switch from the owner's
  subject to the agent-derived subject (built via the same helper the
  middleware uses), so the ceiling and attribution apply to in-process
  execution too, not just HTTP.
- The per-user synthetic attribution key
  (`coderd/x/chatd/synthetickey.go`, `chatd_<owner_id>_session_token`,
  secret discarded) is superseded for chats that have an agent identity:
  delegated AI Gateway calls should present/reference the per-chat agent
  key instead, giving per-chat (not per-user) attribution. Keep the
  synthetic-key path as fallback for chats predating agent identities.
- Chat deletion → mark `ai_agents.deleted`, delete the agent's API keys.

#### In-workspace CLI agents

- Minting point: `coderd/provisionerdserver/provisionerdserver.go`
  `regenerateSessionToken` (~line 3121) currently mints a full-privilege
  owner key exposed as `WorkspaceOwnerSessionToken`
  (`data.coder_workspace_owner.me.session_token`).
- Add a parallel, separately-plumbed value: an AI agent identity + scoped
  key per workspace (profile `WorkspaceAgentIdentityProfile`), created when
  the template opts in.
- **Dependency flag**: exposing this to templates needs a
  terraform-provider-coder change (e.g.
  `data.coder_ai_agent.me.session_token` or equivalent). For the PoC,
  gate on a template/workspace opt-in flag and document that templates
  should point AI tooling env vars (e.g. `CODER_SESSION_TOKEN` for CLI
  agents) at the agent key instead of the owner key. Do NOT change the
  existing owner-token behavior; additive only.
- **Implemented PoC convention**: the opt-in gate is a boolean rich
  parameter named `coder_ai_agent` (opts in only when its stored build
  value is exactly `"true"`); the minted token rides provisioner job
  metadata as `workspace_ai_agent_session_token` and is exported to the
  Terraform provider process as
  `CODER_WORKSPACE_AI_AGENT_SESSION_TOKEN` (empty when opted out).
  Identity is reused across rebuilds; keys rotate per opted-in start
  build; stop/delete revoke keys best-effort; opted-in mint failures
  fail the build, opt-out paths never do. The
  terraform-provider-coder data source remains the planned follow-up.

#### aibridge (attribution only, near-free)

- Agent keys are ordinary API keys, so
  `aibridge_interceptions.initiator_id` records the agent user
  automatically; human lineage = join through `ai_agents.owner_user_id`.
- `aibridgedserver.IsAuthorized` checks key + user liveness; extend it to
  also check owner liveness when the key's user is an AI agent (same
  fail-closed rule as the middleware).

### Visibility and hygiene

- Exclude `kind = 'ai_agent'` rows from default user listings, org member
  listings, group member listings, and roles queries, following the
  existing `is_system = false` filter pattern (~20 sites, greppable:
  `coderd/database/queries/{users,organizationmembers,groupmembers,roles,insights}.sql`).
  Add an `include_ai_agents` param only where needed for admin/audit UIs.
- Minimal read API for the PoC: `GET /api/v2/users/{user}/ai-agents`
  listing agents owned by a user (id, username, origin, created_at,
  deleted). Everything else goes through audit queries.
- Seat counting / licensing treatment: explicitly out of scope (AGPL PoC),
  but note that `kind` gives the hook.

### Invariants (drive tests from these)

1. **Ceiling**: agent's effective permissions ⊆ owner's permissions at all
   times; role removal from owner is reflected on the agent's next request.
2. **Cascade suspend**: agent key auth returns 401 when owner is suspended
   or deleted; also when the `ai_agents` row is deleted.
3. **Attribution**: every audited agent action has
   `user_id = agent user, on_behalf_of_user_id = owner`; audit queries by
   owner return both their own and their agents' actions.
4. **Non-interactive**: agent users can never authenticate via
   password/OIDC/GitHub (login_type none), never appear in default user
   lists, never receive emails/notifications.
5. **No self-escalation**: agent keys cannot create or modify API keys
   (scope exclusion), so an agent cannot mint itself a broader credential.
6. **Fail closed**: `users.kind = 'ai_agent'` without a live `ai_agents`
   row is an authentication error.

### Implementation order

1. Migrations: `user_kind` enum + `users.kind`; `ai_agents` table;
   `audit_logs.on_behalf_of_user_id`. `make gen`; update
   `enterprise/audit/table.go`; `make gen` again.
2. `rbac.SubjectTypeAIAgent`; scope profiles.
3. `coderd/aiagentidentity` package (Create / MintKey / Resolve) + queries.
4. `httpmw/apikey.go` agent-aware subject construction + context actor.
5. Audit plumbing: `on_behalf_of_user_id` populate + API filter.
6. User-list/org-member/group/roles filtering by kind.
7. chatd integration (mint at chat create, switch execution subject,
   supersede synthetic key).
8. provisionerdserver integration (opt-in workspace agent identity key;
   provider dependency flagged).
9. aibridge owner-liveness check.
10. Tests per the invariants above (use unique identifiers; no
    `time.Sleep` for timing, per `.claude/docs/TESTING.md`).

Steps 1 to 6 are the foundation and independently mergeable; 7 to 9 are
per-surface and parallelizable after 4.

### Non-goals (PoC)

- Org/deployment-level policy ceilings ("no agent may ever X").
- Configurable scope profiles (UI or API).
- Delegation chains deeper than one level.
- OAuth2 token exchange (RFC 8693) as the credential mechanism; plain API
  keys chosen deliberately. The OAuth2 provider's scope handling gap
  (`authorizationCodeGrant` ignores requested scopes) is noted but not
  fixed here.
- Consolidating `is_system` / `is_service_account` into `users.kind`.
- Coder Tasks integration.
- Seat/licensing semantics for agent users.
- Interactive permission-escalation approval flows (belongs to the
  auditing vertical discussion).

---

## Vertical 2: Workspace Enhancements and Execution Isolation

### Problem

Even with AI agent identities (Vertical 1), the execution environment leaks
and the network is wide open:

- Every workspace ambiently injects the owner's credentials: ALL enabled
  user secrets ride the agent manifest (fetched under
  `dbauthz.AsSystemRestricted` in `coderd/agentapi/manifest.go`, bypassing
  even `api_key_scope = no_user_data`), the owner's Git SSH private key is
  served by `coder gitssh`, and the owner's external-auth OAuth tokens are
  returned by `/workspaceagents/me/external-auth` (`coder gitaskpass`).
- There is no workspace-wide network confinement. Agent Firewall (boundary)
  is per-process, opt-in, bypassable by any unwrapped process, and Premium.
  This vertical rebuilds egress control from the ground up and does NOT
  build on boundary; the `boundary_*` tables/flows are left alone and
  eventually superseded.
- There is no first-class notion of a workspace or sub-environment that
  exists *for* an AI agent.

### Concept: two modes, one architecture

|                                     | Mode 1: agent-owned workspace                                             | Mode 2: sandbox inside human workspace                             |
|-------------------------------------|---------------------------------------------------------------------------|--------------------------------------------------------------------|
| Ownership                           | `workspaces.owner_id` = AI agent user (V1)                                | Human owns workspace; sandboxed child agent bound to AI identity   |
| Isolation boundary                  | Workspace network perimeter (netns supervisor; optionally VM-class infra) | Sandbox wall (microVM via coder/sandbox lineage, or other backend) |
| Ambient human creds inside boundary | None, structurally (owner is the agent user, who has nothing)             | None, by policy (manifest diet for AI-bound child agents)          |
| Human access                        | Auto-created workspace ACL grant (full control, no appropriation)         | Human owns the workspace already                                   |

Both modes place an AI agent identity behind an isolation boundary with
default-deny egress and credentials held outside the boundary. The sandbox
runtime interface (below) is shared; Mode 1 is the degenerate case where
the "sandbox" is the workspace interior itself.

Design for N sandboxes per workspace (the data model supports multiple
children via `workspace_agents.parent_id`); the PoC constrains to 1.

### Decisions

| Property                  | Decision                                                                                                                  |
|---------------------------|---------------------------------------------------------------------------------------------------------------------------|
| Sandbox technology        | Pluggable runtime interface; coder/sandbox (libkrun microVM) is the flagship backend, but ANY backend must be supportable |
| Workspace agent placement | A child workspace agent runs INSIDE the sandbox (keeps SSH/terminal/apps/port-forward via existing tailnet routing)       |
| Identity binding          | `workspace_agents.ai_agent_id` (nullable FK to V1 `ai_agents.user_id`)                                                    |
| Authorization             | Everything stays delegated (V1 model unchanged); Mode 1 bridged by an auto-created ACL grant                              |
| Egress                    | Rebuilt from scratch: structural default-deny, SNI/CONNECT-level policy, audited; NOT boundary-based                      |
| Credentials               | MCP-first; per-agent `ai_credential_mode = none \| injected \| brokered`, default `none`                                  |
| External auth             | Nothing fancy: denied by default, passed through as-is when opted in                                                      |
| Quota                     | Agent-owned workspaces charge the sponsoring human                                                                        |
| Human-in-the-loop         | Designed for (egress approvals), implemented post-PoC                                                                     |

### Mode 1: agent-owned workspaces

#### The authorization bridge

RBAC owner checks are literal: `resource.owner_id == subject.ID`. With
`W.owner_id = AU` (agent user) three things break: the sponsoring human
fails owner checks on W; the delegated agent subject (ID = human, per V1)
also fails them; and the workspace agent inside W builds its subject from
the owner's roles, which for AU are none.

Resolution (no new authz machinery, V1 unchanged):

1. **Auto-ACL grant**: when an agent workspace is created, coderd writes a
   workspace ACL entry granting the sponsoring human `admin`
   (`coderd/workspaces.go` ACL machinery, `db2sdk.WorkspaceRoleActions`).
   The human gets full control; `owner_id` never changes (no
   appropriation). ACL admin excludes delete: deletion goes through
   lifecycle (TTL/cascade) or org admins; direct sponsor delete is an open
   policy question, noted, not blocking.
2. **Delegated subject reaches W through the human's grant**: the agent's
   key still resolves to subject ID = human (V1), which now passes via the
   ACL path, then is narrowed by scopes + allow list (pinned to W).
3. **Workspace agent middleware** (`coderd/httpmw/workspaceagent.go`): when
   the workspace owner is an AI agent user, resolve
   `owner (AU) -> ai_agents.owner_user_id (human)` and build the sponsor's
   subject ∩ `WorkspaceAgentScope`, mirroring V1's API-key middleware.
   Fail closed if the `ai_agents` row is missing.

#### Structural credential starvation

All ambient credential channels key off `workspaces.owner_id`, so with
owner = AU they are empty by construction, not by filtering:

| Channel                                                                                                               | Behavior in Mode 1                                                                                                                                       |
|-----------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| Manifest user secrets                                                                                                 | AU owns no secrets; manifest section is empty                                                                                                            |
| `coder gitssh`                                                                                                        | Refuses in AI-owned workspaces (must never serve the sponsor's key); port-22 egress is default-deny anyway                                               |
| External auth (`gitaskpass`)                                                                                          | 403 + pointer to MCP/broker unless `ai_credential_mode` says otherwise; AU has no links regardless. Make this an explicit policy denial, not an accident |
| Owner session token (`coder_workspace_owner.me.session_token`, minted in `provisionerdserver.regenerateSessionToken`) | Minted for AU = the scoped V1 agent key. Verify the scope profile applies at this mint point                                                             |
| Template-injected env (`coder_env`, scripts)                                                                          | Admin-authored; out of our control. Template envelope lints/warns when AI-designated templates interpolate owner secrets/tokens                          |

#### Creation, lifecycle, quota

- Created by the chat agent (delegated, V1) or via API; the workspace's
  `ai_agents` origin row is `(origin_type='workspace', origin_id=W)`, or
  the owning identity is the chat's agent when chatd creates it (origin
  stays `chat`). Template must opt in (envelope below).
- Lifecycle: mandatory TTL (reuse the chat-workspace TTL precedent),
  dormancy-aggressive defaults, cascade: chat deleted or sponsor
  suspended -> workspace stopped, then auto-deleted after grace period.
  Sponsor suspension already 401s the agent's key (V1 invariant 2).
- Quota: charge the sponsor. Targeted change in the quota path
  (`enterprise/coderd/workspacequota.go`, `queries/quotas.sql`): sum
  workspaces owned by a user's sponsored agents into that user's bucket.
  AGPL PoC may stub this; flag it.

### Mode 2: sandboxed subagent in a human workspace

#### Reuse boundary (from the devcontainer/subagent machinery)

Reuse as-is: `workspace_agents.parent_id`, child token minting
(`SubAgentAPI` in `coderd/agentapi/subagent.go`), per-agent tailnet
identity + SSH routing (`coder ssh ws.child`), child apps, nested UI
(`AgentRow` / devcontainer card patterns).

Replace: everything devcontainer/Docker-specific in
`agent/agentcontainers` (labels, `devcontainer` CLI, `docker cp` binary
injection) with the sandbox runtime interface.

**Prerequisite fixes (pre-existing bugs, fix regardless):**

1. `DeleteSubAgent` does not verify the target's `parent_id` equals the
   caller; bind deletion to the calling parent.
2. Manifest user secrets bypass `no_user_data` (fetched under
   `AsSystemRestricted`). For any child agent with `ai_agent_id` set, the
   manifest is stripped: no user secrets, no external auth, no git key.
   Fail closed.

#### Sandbox runtime interface (agent-side)

New package (suggested: `agent/aisandbox`) defining:

- **Capability discovery**: backends declare what they enforce
  (`CanEnforceEgress`, `CanMountWorkspace`, `CanExec`, `CanSnapshot`,
  host requirements such as `/dev/kvm`). Coderd records declared
  capabilities so degraded confinement is visible, never silent.
- **Lifecycle**: create/start/stop/delete/reconcile (reconcile matters:
  sandboxes must survive agent restarts or fail loudly).
- **Process ops**: exec/stream/signal; mounts; env injection for the child
  agent (`CODER_AGENT_URL`, `CODER_AGENT_TOKEN` of the AI-bound child).
- **Network policy**: apply/update egress policy; emit audit events.

Backends:

| Backend                           | Isolation                     | Egress enforcement                                                                       | Status                                   |
|-----------------------------------|-------------------------------|------------------------------------------------------------------------------------------|------------------------------------------|
| `microvm` (coder/sandbox lineage) | libkrun microVM, KVM required | deny-by-default proxy + recorded DNS, per-sandbox MITM possible (CA injected into guest) | flagship; needs hardening (below)        |
| `netns` (new, shared with Mode 1) | Linux network namespace       | SNI/CONNECT proxy, default-deny                                                          | PoC-buildable everywhere; weaker than VM |
| `devcontainer` (retrofit)         | container                     | none (declares it)                                                                       | compatibility                            |

#### coder/sandbox hardening list (before production use)

Current state: ~19-commit prototype, `package main` + `internal/`, Linux
unverified at v0.1. Required work: extract an importable driver library;
bind proxy listeners to VM-only addresses or authenticate them (today
`:0` on all interfaces, unauthenticated); daemon-restart reconciliation
(today VMs orphan their egress on daemon death); confine bind mounts to a
designated workspace subtree; preseed the pinned `msb`/libkrunfw runtime
(no unauthenticated first-use downloads); surface SDK resource limits
(vCPU, memory, idle/max-duration); real KVM integration tests.

### Egress control (both modes, rebuilt)

#### Supervisor + confined child agent

The workspace agent already has a conditional two-process structure
(`cli/agent.go` ~125-148: PID-1 Linux spawns `reaper.ForkReap`, fork-execs
the real agent). Promote that slot into an explicit supervisor, decoupled
from PID-1 (new flag/env, e.g. `CODER_AGENT_CONFINE`), so it also works on
VM workspaces:

1. Supervisor creates a netns + veth (or userspace fallback where veth
   needs missing privileges) and a recorded DNS relay.
2. Fork-execs the real agent INTO the netns. Everything the agent spawns
   (SSH sessions, terminals, startup scripts, the AI process) inherits the
   confined network view. Confinement is structural, not per-process
   opt-in; nothing can run unconfined because the process spawner is
   itself inside.
3. Supervisor stays outside, runs the local egress proxy (policy +
   audit + forward), keeps reaper duties, supervises the child.

In Mode 2 the parent workspace agent plays the supervisor role for the
sandbox backend; in Mode 1 the supervisor process is it.

#### Policy: two layers

- **L4 allows for the control plane**: coderd (HTTPS/wss) and DERP (443).
  Confined agents default to relay-only (per-workspace equivalent of
  `BlockDirect`); direct P2P (STUN/UDP) is incompatible with
  default-deny-UDP. Optional narrow UDP allows for direct connections are
  a documented tradeoff, not the default. Ingress (SSH/apps) rides the
  agent's existing tailnet connection and is unaffected.
- **L7 for everything else**: SNI/CONNECT-level allow/deny/audit with NO
  decryption. Host-level rules (method/path granularity requires MITM and
  is only available in Mode 2's microvm backend where the guest CA story
  is sound; deployment-level transparent MITM is explicitly rejected: CA
  distribution, key concentration, pinned clients, mTLS).

Policy is a first-class object: default-deny; implicit allows for coder
control plane + AI Gateway; template/admin-declared allowlist; per-sandbox
overrides bounded by the template envelope.

#### Audit stream

Child agent/supervisor -> agentapi -> new tables `ai_sandbox_sessions` +
`ai_sandbox_network_events` (attributed to `ai_agent_id`, correlatable
with aibridge interceptions; modeled on but replacing the `boundary_*`
pattern). This is a Vertical 3 input.

Threat honesty for the doc: a netns beside its supervisor in the same
container is a strong fence, not a wall. Mitigations in order: AI runs as
non-root with supervisor under a different UID; drop
`CAP_SYS_ADMIN`/`CAP_NET_ADMIN` where templates allow; highest tier uses
VM-class workspaces with infra-enforced perimeter (NetworkPolicy/security
groups) and the netns as defense-in-depth + audit source. The template
envelope selects the tier.

### Credential plane (simplified for PoC)

Principle: **the credential holder always sits across an isolation
boundary from the AI process.** Rules:

1. **MCP-first.** Agent actions needing third-party auth go through the
   MCP gateway in AI Gateway: human OAuths once at the gateway; agent
   makes MCP tool calls; the gateway injects credentials upstream.
   Credentials never transit the workspace in any form.
2. **`ai_credential_mode`: one enum per agent, default `none`.** Declared
   in Terraform on `coder_agent` (precedent: `api_key_scope`), extracted
   by the provisioner, stored on the `workspace_agents` row, read
   server-side by every enforcement point.
3. **Nothing fancy for external auth.** Binary: denied (default) or passed
   through as-is when opted in. No minting, no TTL games, no dual-grant
   (all future hardening).

| Enforcement point                     | `none` (default)         | `injected`                         | `brokered`                                                                        |
|---------------------------------------|--------------------------|------------------------------------|-----------------------------------------------------------------------------------|
| Manifest user secrets                 | omitted                  | included                           | omitted                                                                           |
| External auth endpoint (`gitaskpass`) | 403 + MCP/broker pointer | stored token as-is                 | 403 + broker remote hint                                                          |
| `gitssh`                              | refuses                  | serves only if explicitly declared | refuses                                                                           |
| Git remotes/registry config           | untouched                | untouched                          | rewritten to broker hostname at agent startup                                     |
| Broker                                | rejects caller           | n/a                                | authenticates the agent's Coder key, reads THIS row server-side, injects upstream |

- **Two-keyed consent for `injected`**: template declares it, AND the
  sponsoring human consents (at chat/workspace creation or a per-provider
  "allow my agents" setting), because external-auth links and user secrets
  are the user's property, not the template's. Template-only opt-in
  suffices only for admin-owned credentials (template env).
- **Grant-time audit**: injection has no per-use choke point, so record an
  audit event at build time ("workspace W, agent AU, sponsor H received
  external-auth github via template opt-in + sponsor consent").
- **Documented risk**: an injected credential is the durable, full-scope
  token, exfiltratable by a prompt-injected agent; mitigated by egress
  default-deny (exfil needs an allowed destination), accepted by the
  admin + sponsor who opted in.
- **Broker (tier for "agent never sees it")**: a terminating endpoint, NOT
  a MITM. Workspace remotes are rewritten to
  `https://git-broker.<deployment>/github.com/org/repo.git`; the broker
  presents its own legitimate certificate, authenticates the caller by the
  AI identity's scoped Coder key, maps agent -> sponsor -> credential,
  injects upstream, enforces per-operation policy (fetch vs push is
  distinguishable in the git protocol), audits. Same shape extends to
  package registries. PoC-optional; spec it, build post-PoC if time.
- **SSH git**: not brokerable here; default-deny port 22 in AI workspaces,
  force HTTPS via `url.insteadOf`.
- **Invariant**: nothing inside the workspace can influence its own
  credential disposition. Enforcement points consult the stored row
  server-side; a lying agent only breaks itself.
- Runtime observability: disposition surfaced on the workspace agent API
  object (workspace page / `coder show`).

### Template envelope (Terraform)

Templates are the governance boundary: the confined thing must not define
its own confinement. The envelope declares what is possible; runtime
instantiates within it (mirroring the devcontainer declared/discovered
hybrid: chatd or the parent agent may create sandbox instances
dynamically, but only within the envelope).

Envelope contents: AI-workspace opt-in flag (Mode 1); allowed sandbox
backends + capability floor (e.g. "must enforce egress"); egress policy
baseline (allowlist rules); `ai_credential_mode`; resource caps; mounts
policy. Provisioner-time child agents follow the
`coder_devcontainer.subagent_id` precedent so `coder_app`/`coder_env`/
`coder_script` can attach to the sandboxed agent. Requires
terraform-provider-coder changes (new `coder_sandbox` resource or
attributes; same dependency-flag treatment as V1's provider change).

### Schema changes (summary)

1. `workspace_agents.ai_agent_id` (nullable FK -> `ai_agents.user_id`) +
   `workspace_agents.ai_credential_mode` (enum, default `none`).
2. `ai_sandbox_sessions`, `ai_sandbox_network_events` (egress audit).
3. Sandbox instance records (backend, declared capabilities, policy ref,
   state) keyed to workspace + parent agent.
4. Workspace ACL auto-grant needs no schema (existing ACL storage).
5. Quota attribution query change (sponsor bucket).

Follow `.claude/docs/DATABASE.md` end-to-end (queries, `make gen`,
`enterprise/audit/table.go`, `make gen` again).

### Invariants (drive tests from these)

1. **No ambient human credentials cross the boundary**: Mode 1 manifests
   contain zero sponsor secrets/keys/tokens; Mode 2 AI-bound child
   manifests are stripped. Fail closed.
2. **Disposition is server-authoritative**: every credential enforcement
   point reads the stored `ai_credential_mode`; no in-workspace input can
   change it.
3. **Default-deny egress**: a fresh AI workspace/sandbox can reach coderd,
   DERP, and AI Gateway, and nothing else; every allowed/denied flow
   produces an attributed audit event (when the backend declares egress
   capability).
4. **Structural confinement**: any process spawned via the confined agent
   (SSH, terminal, scripts) observes the same network policy.
5. **Sponsor control without appropriation**: sponsor holds ACL admin on
   agent-owned workspaces from creation; `owner_id` never changes.
6. **Capability honesty**: a backend that cannot enforce egress is
   recorded as such and visible via API/UI.
7. **V1 invariants all still hold** (ceiling, cascade suspend,
   attribution) for workspace-agent and child-agent paths, not just API
   keys.

### Implementation order

1. Prerequisite fixes: `DeleteSubAgent` parent check; manifest secret
   stripping for AI-bound child agents.
2. Schema: `ai_agent_id` + `ai_credential_mode` on workspace_agents;
   sandbox + egress-audit tables.
3. Workspace-agent middleware: AU-owner -> sponsor subject resolution.
4. Mode 1 minimum: create agent-owned workspace (template opt-in),
   auto-ACL grant, structural starvation checks, TTL lifecycle, scoped
   session-token mint verification.
5. Supervisor/netns confinement lib + SNI proxy + audit stream (Mode 1
   usable end-to-end here).
6. Sandbox runtime interface + `netns` backend; child-agent creation
   bound to AI identity (Mode 2 usable with netns backend).
7. Credential plane: `ai_credential_mode` enforcement points + two-keyed
   consent + grant audit events.
8. `microvm` backend (after coder/sandbox library extraction; can proceed
   in parallel from step 6).
9. Terraform provider envelope surface (dependency-flagged).
10. Quota-to-sponsor attribution.

### Non-goals (PoC) / future work

- Human-in-the-loop egress approvals (designed for: approval API + UI on
  the policy engine; implement post-PoC).
- Per-credential dispositions (single enum only), credential mint adapters
  (GitHub App installation tokens, GitLab project tokens), dual-grant
  reduced-scope OAuth links, vend-refresh machinery.
- Deployment-level transparent MITM (rejected outright).
- L7 method/path egress rules outside the microvm backend.
- More than 1 sandbox per workspace in practice (designed for N).
- Snapshots/checkpointing; Windows/macOS sandbox backends.
- Boundary/Agent Firewall integration or migration.
- Seat/licensing and billing semantics for agent-owned workspaces.

---

## Vertical 3: Auditing of Agent Activity

*Stub. To be designed.* Known anchors: `on_behalf_of_user_id` from
Vertical 1; `ai_sandbox_sessions` / `ai_sandbox_network_events` and
credential grant-time audit events from Vertical 2; aibridge interception
lineage; delegation chains deeper than one level; human-in-the-loop egress
approval flows (designed for in Vertical 2, implemented here or alongside);
frontend audit UX for agent-vs-human filtering.
