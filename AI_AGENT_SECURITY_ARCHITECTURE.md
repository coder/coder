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
at each human-to-AI boundary, durably attributed to a human owner, and always
bounded by that human's live permissions.

| Property            | Decision                                                                       |
|---------------------|--------------------------------------------------------------------------------|
| Principal model     | Real row in `users` with `kind = 'ai_agent'` + metadata side-table `ai_agents` |
| Granularity         | One per human-created chat or direct workspace opt-in; descendants reuse it    |
| Audit semantics     | Actor = agent, `on_behalf_of` = human; queryable by both                       |
| Permission ceiling  | Agent perms ⊆ owner perms, enforced structurally at request time               |
| Reduction mechanism | Owner's live roles ∩ API key scopes ∩ allow list (existing machinery)          |
| Credentials         | Plain API keys owned by the agent user                                         |
| Creation            | Automatic at those human-to-AI boundaries; no manual identity flow             |
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
    origin_type   ai_agent_origin NOT NULL,     /* enum: 'chat', 'workspace' */
    origin_id     uuid NOT NULL,                /* chat ID or workspace ID */
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
  key per directly human-opted workspace (profile
  `WorkspaceAgentIdentityProfile`). AI-created resources reuse the requesting
  agent's identity, as specified in Vertical 2.
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

### Known follow-ups (found in conformance review)

- **AI Gateway budget principal.** `aibridgedserver.IsAuthorized` returns a
  single `OwnerId` that the gateway uses for BOTH interception attribution
  (must stay the agent, per Invariant 3) and AI budget enforcement (must be
  the human owner, so agent spend counts against and is bounded by the
  sponsor). Today it returns the agent user for both, so agent Gateway
  traffic bypasses the owner's budget/override/group limits and accrues
  spend to the credential-poor agent. Fixing this correctly requires
  splitting the two concepts (e.g. add a distinct budget/governance
  principal field to the aibridge authorization response and its proto),
  rather than overloading one ID. Deferred as a deliberate design decision;
  candidate for the auditing/governance vertical.

## Non-goals (PoC)

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
  build on boundary; the `boundary_*` tables and flows are left alone and
  eventually superseded.
- There is no first-class, server-authoritative binding between a workspace
  agent process and the AI identity responsible for its actions.

### Concept: human-owned workspaces with AI-bound agents

**Workspaces are always human-owned.** `workspaces.owner_id` remains the
sponsoring human in every flow. Vertical 2 adds one binding mechanism:
`workspace_agents.ai_agent_id`, a nullable foreign key to
`ai_agents.user_id`. A workspace agent with this field set is **AI-bound**.
An unbound workspace agent retains normal behavior.

The same binding has two shapes:

| Shape                   | Binding                                                                                                   | Use case                                                                                             |
|-------------------------|-----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| AI-designated workspace | Every workspace agent in the workspace is bound to the same AI identity                                   | Workspaces created for or by AI, including chat-created workspaces and direct human workspace opt-in |
| Sandboxed agent         | Only a child agent with `workspace_agents.parent_id` is bound; the parent workspace agent remains unbound | A confined AI sandbox inside a normal human workspace                                                |

Design for N sandboxed child agents per workspace. The data model already
supports multiple children through `workspace_agents.parent_id`; the PoC
creates at most 1.

#### Identity continuity

AI identities are minted only at a human-to-AI boundary:

1. When a human creates a chat, mint a new chat-origin identity, as built in
   Vertical 1.
2. When a human opts a workspace in through the `coder_ai_agent` parameter
   with no chat involved, mint a new workspace-origin identity, as built in
   Vertical 1.
3. When an AI agent creates resources, do not mint another identity. Bind
   the created workspace, workspace agents, sandboxed child agents, or child
   chats to the requester's existing identity.

A chat-created workspace therefore remains owned by the chat sponsor, while
all of its workspace agents carry the chat identity's user ID in
`workspace_agents.ai_agent_id`. Child chats already reuse the root chat
identity; Vertical 2 extends that precedent to workspace and sandbox
creation. One delegation chain has one unbroken AI identity lineage.

### Decisions

| Property                  | Decision                                                                                                                                                        |
|---------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Workspace ownership       | Always the sponsoring human; no agent-owned workspaces in the current design                                                                                    |
| Identity binding          | `workspace_agents.ai_agent_id` is the single server-authoritative binding                                                                                       |
| Binding shapes            | All agents bound in an AI-designated workspace, or only a bound child agent in a normal workspace                                                               |
| Identity creation         | Mint only at a human-to-AI boundary; AI-created descendants reuse the requester's identity                                                                      |
| Sandbox technology        | Arbitrary admin script contract exec'd by the parent agent; platform owns binding, proxy, and stripping; netns and coder/sandbox are reference implementations  |
| Workspace agent placement | A child workspace agent runs inside a sandbox, preserving SSH, terminal, apps, and port forwarding through existing routing                                     |
| Authorization             | Sponsor's live subject intersected with the appropriate workspace-agent or scoped session-token profile                                                         |
| Egress                    | Template-settings policy object; two-phase delivery (fork-time bootstrap, live updates); supervisor is the sole consumer; SNI/CONNECT-level; not boundary-based |
| Credentials               | MCP-first; per-agent `ai_credential_mode = none \| injected \| brokered`, default `none`                                                                        |
| Human access              | Follows directly from workspace ownership; no ACL bridge or auto-grant machinery                                                                                |
| Human-in-the-loop         | Designed for egress approvals, implemented post-PoC                                                                                                             |
| Sandbox cardinality       | Designed for N sandboxed children per workspace, limited to 1 in the PoC                                                                                        |

### Binding and credential split

Implementers must keep the two workspace credentials separate. Both
attribute actions to the AI agent, but they secure different principals and
use different scope machinery.

#### Workspace agent token: daemon authentication

The workspace agent token authenticates the workspace agent daemon. When
`workspace_agents.ai_agent_id` is set, workspace-agent middleware must:

1. Resolve the bound `ai_agents` row and its sponsoring owner. Use the V1
   fail-closed resolution semantics, including an
   `GetAIAgentByOriginIncludingDeleted`-style distinction between a missing
   identity and a revoked or deleted identity.
2. Reject authentication when the identity is missing, deleted, or revoked,
   or when the sponsor is missing, deleted, suspended, or otherwise
   inactive. A dangling binding is never treated as an unbound agent.
3. Build the sponsor-owner RBAC subject, intersect it with
   `rbac.WorkspaceAgentScope`, force `no_user_data`, and set
   `Subject.Type = SubjectTypeAIAgent`.
4. Stash `AIAgentActor{AgentUserID, OwnerUserID, OriginType, OriginID}` in
   context for audit, request logging, and downstream attribution.

This subject governs the workspace agent process itself, including agent API
calls and lifecycle operations. It does not grant general CLI permissions to
programs running inside the workspace.

#### Session token: in-workspace CLI tools

The session token used by in-workspace CLI tools is the scoped AI API key
minted through the existing V1 `WorkspaceAgentIdentityProfile` and step 8
machinery. Reduced permissions for AI tool actions live in this credential,
not in the workspace agent token.

For an AI-designated workspace, coderd suppresses the full-owner session
token exposed as `data.coder_workspace_owner.me.session_token` and provides
the scoped AI session token instead. Both workspace agent token actions and
scoped session-token actions audit `actor = agent` and
`on_behalf_of = sponsor`.

For a sandboxed child, inject the scoped AI session token only into the
child boundary. The unbound parent can retain normal human-workspace
behavior outside that boundary, but it must never copy its owner session
token into the child.

The current workspace key lifetime is 24 hours. Chat keys renew on use, but
workspace keys only rotate on a start build. A renewal path is required for
workspaces or sandboxes that run for more than 24 hours; this is an explicit
implementation work item, not a reason to extend key lifetime silently.

#### Workspace-agent audit attribution

`audit.InitRequest` currently derives `user_id` from an API key or an
explicit `Request.UserID`. Workspace-agent UUID authentication supplies
neither. Every bound workspace-agent audit entry point must therefore set
the actor explicitly to the bound AI agent user and set
`on_behalf_of_user_id` to the sponsor.

This plumbing must cover synchronous HTTP request audits and background
workspace-agent events. Background producers cannot rely only on request
context; their event payloads must carry both agent and sponsor attribution.

### AI-designated workspaces

An AI-designated workspace uses the same human ownership and per-agent
binding as every other workspace. Its distinction is that every workspace
agent is bound to one AI identity.

#### Chat-created workspaces

The chat workspace-creation tool forces AI designation. It must not depend
on the selected template declaring the `coder_ai_agent` parameter or setting
it to `true`.

Creation follows these rules:

1. Keep `workspaces.owner_id` set to the chat identity's sponsoring human.
2. Reuse the requesting chat's AI identity. Do not create a workspace-origin
   identity.
3. Set `workspace_agents.ai_agent_id` to that chat identity for every agent
   produced by the workspace build, including agents added by subsequent
   builds.
4. Suppress `data.coder_workspace_owner.me.session_token`, whose
   `coder:all` owner credential must never enter an AI-designated workspace.
5. Provide the scoped AI session token instead.

The server owns this designation and propagates it through the build. A
Terraform template can add a direct human opt-in surface, but it cannot
cause a chat-created workspace to become unbound.

#### Direct human workspace opt-in

When a human starts or creates a workspace with `coder_ai_agent = true` and
no existing AI requester, Vertical 1 mints the workspace-origin identity.
Vertical 2 binds every workspace agent in that workspace to that identity,
suppresses the owner session token, and supplies the scoped AI session
token. Rebuilds reuse the identity.

### Sandboxed agent in a human workspace

#### Reuse boundary from the devcontainer and subagent machinery

Reuse as-is: `workspace_agents.parent_id`, child token minting
(`SubAgentAPI` in `coderd/agentapi/subagent.go`), per-agent tailnet identity
and SSH routing (`coder ssh ws.child`), child apps, and nested UI
(`AgentRow` and devcontainer card patterns).

Replace everything devcontainer and Docker-specific in
`agent/agentcontainers` (labels, the `devcontainer` CLI, and `docker cp`
binary injection) with the sandbox runtime interface.

**Prerequisite fix:** `DeleteSubAgent` does not verify that the target's
`parent_id` equals the caller. Bind deletion to the calling parent before
building sandbox lifecycle on this API. This compatibility-review finding
remains unfixed.

#### Sandbox script contract

The platform does not prescribe sandbox technology. A template declares a
`coder_ai_sandbox` resource carrying arbitrary admin-authored `create` and
`destroy` scripts that the PARENT workspace agent execs, plus an
`egress_enforcement` attestation (`forced | advisory | none`) and an
`ai_credential_mode`. The resource exposes `subagent_id` so `coder_app`,
`coder_env`, and `coder_script` can attach to the sandboxed child agent
(the `coder_devcontainer.subagent_id` precedent).

The platform provides the script environment:

| Variable                 | Meaning                                                                               |
|--------------------------|---------------------------------------------------------------------------------------|
| `CODER_AI_AGENT_URL`     | coderd URL for the child agent                                                        |
| `CODER_AI_AGENT_TOKEN`   | The bound CHILD agent token. Binding is server-side; the script cannot mint or rebind |
| `CODER_AI_SESSION_TOKEN` | Scoped AI session token for CLI tools inside the sandbox                              |
| `CODER_EGRESS_PROXY`     | Parent-side proxy address; policy is managed dynamically (see egress section)         |
| `CODER_SANDBOX_ID`       | Lifecycle correlation ID                                                              |

The script's job: create the isolation boundary (docker, podman, microVM,
anything), route its egress to the proxy, and launch the child agent inside
with those variables. Illustrative docker-based `create` script:

```bash
#!/usr/bin/env bash
docker run -d --name "sb-${CODER_SANDBOX_ID}" \
  --network none \
  -e CODER_AGENT_URL="$CODER_AI_AGENT_URL" \
  -e CODER_AGENT_TOKEN="$CODER_AI_AGENT_TOKEN" \
  -e CODER_SESSION_TOKEN="$CODER_AI_SESSION_TOKEN" \
  -e HTTPS_PROXY="http://${CODER_EGRESS_PROXY}" \
  -v /home/coder/project:/workspace \
  ghcr.io/example/ai-sandbox:1.2 \
  /opt/coder agent
```

**Platform-owned; the script cannot touch:** child `workspace_agents` row
creation and `ai_agent_id` binding; the parent-side egress proxy, its
policy, and its audit stream; credential stripping keyed on the binding
(all three enforcement points plus owner-token suppression); health, which
is the child agent's connection state; `destroy` runs on teardown; `create`
re-runs on reconcile after agent restarts.

**Attestation, not verification.** `egress_enforcement` is an admin
attestation. A script that claims `forced` but leaks a side channel is
undetectable at declaration time. This is consistent with the existing
trust model, because template admins already fully control workspaces, and
it is partially auditable post hoc: an attested-forced sandbox whose proxy
sees no traffic while the AI is clearly active is an anomaly signal
(a Vertical 3 input). It remains a downgrade from platform-enforced
confinement; the doc states it plainly rather than implying the platform
verifies scripts.

#### Reference implementations

Built-in confinement mechanisms are reference implementations invoked
through the same script contract, not privileged code paths:

| Reference             | Isolation                     | Notes                                                                               |
|-----------------------|-------------------------------|-------------------------------------------------------------------------------------|
| netns supervisor      | Linux network namespace       | Used for AI-designated whole-workspace confinement; also usable as a sandbox script |
| coder/sandbox         | libkrun microVM, KVM required | Flagship reference; per-sandbox MITM possible with a guest CA                       |
| devcontainer retrofit | Container                     | No egress enforcement; attests `none`                                               |

#### coder/sandbox hardening list before production use

Current state: approximately 19-commit prototype, `package main` plus
`internal/`, Linux unverified at v0.1. Required work to harden it as the
flagship reference implementation: extract an importable driver library or
stable CLI; bind proxy listeners to VM-only addresses or authenticate them
because `:0` currently listens on all interfaces without authentication;
reconcile after daemon restart because VMs currently orphan their egress;
confine bind mounts to a designated workspace subtree; preseed the pinned
`msb` and libkrunfw runtime without unauthenticated first-use downloads;
surface SDK resource limits for vCPU, memory, idle time, and maximum
duration; add real KVM integration tests.

### Credential starvation is policy, not structure

Human ownership means AI-bound agents can reach owner credential sources
unless coderd denies them deliberately. Binding must activate fail-closed
credential handling at three separate enforcement points:

1. **Manifest user secrets.** `coderd/agentapi/manifest.go` calls
   `ListUserSecretsWithValues` under `dbauthz.AsSystemRestricted` for
   `workspace.OwnerID`, bypassing normal `no_user_data` scope checks. For an
   AI-bound workspace agent, omit owner user secrets unless the stored
   credential policy explicitly authorizes injection.
2. **External auth.** `/workspaceagents/me/external-auth`, used by
   `gitaskpass`, must deny an AI-bound caller by default and consult the
   stored credential policy before returning any owner token.
3. **Git SSH.** `/workspaceagents/me/gitsshkey` must deny an AI-bound caller
   by default and consult the stored credential policy before serving any
   owner private key.

Manifest stripping alone is not sufficient. The external-auth and Git SSH
endpoints are independent credential channels and must enforce the same
server-authoritative binding. Missing binding metadata, identity resolution
errors, or unknown credential modes deny access rather than falling back to
human-agent behavior.

Owner session-token suppression for AI-designated workspaces is a fourth,
separate control. It is not governed by `ai_credential_mode`; an AI-designated
workspace never receives the ambient `coder:all` owner session token, and a
sandboxed child never receives a copy from its unbound parent.

### Egress control for both binding shapes

#### Supervisor and confined child agent

The workspace agent already has a conditional two-process structure
(`cli/agent.go` around lines 125-148: PID 1 on Linux spawns
`reaper.ForkReap`, then fork-execs the real agent). Promote that slot into an
explicit supervisor, decoupled from PID 1 through a new flag or environment
setting such as `CODER_AGENT_CONFINE`, so it also works in VM workspaces:

1. The supervisor creates a network namespace and veth, or a userspace
   fallback where veth creation lacks privileges, plus a recorded DNS relay.
2. It fork-execs the real agent into the namespace. Everything the agent
   spawns, including SSH sessions, terminals, startup scripts, and the AI
   process, inherits the confined network view. Confinement is structural,
   not per-process opt-in; nothing spawned by the confined agent can run
   outside its network policy.
3. The supervisor stays outside, runs the local egress proxy for policy,
   audit, and forwarding, keeps reaper duties, and supervises the child.

For an AI-designated workspace, the workspace-level supervisor confines the
workspace interior. For a sandboxed agent, the parent workspace agent
manages the selected sandbox backend and the bound child workspace agent
runs inside it.

#### Policy: two layers

- **L4 allows for the control plane**: coderd over HTTPS or WebSocket and
  DERP over port 443. Confined agents default to relay-only tailnet behavior,
  equivalent to a per-workspace `BlockDirect`. Direct peer-to-peer traffic
  through STUN or UDP conflicts with default-deny UDP. Optional narrow UDP
  allows are a documented tradeoff, not the default. Ingress for SSH and
  apps rides the agent's existing tailnet connection and is unaffected.
- **L7 for everything else**: SNI and CONNECT-level allow, deny, and audit
  without decryption. Host-level rules are the portable ceiling. Method or
  path rules require MITM and are available only where a sandbox backend,
  such as `microvm`, can establish a sound guest CA boundary.
  Deployment-level transparent MITM is rejected because of CA distribution,
  key concentration, pinned clients, and mTLS.

#### Policy storage and delivery

Egress policy lives in **template settings**, not in Terraform. It is a
first-class, versioned object that administrators edit through the API and
UI without a template push or workspace rebuild: default-deny; implicit
allows for the Coder control plane (coderd, DERP) and AI Gateway; a
template-level allowlist baseline; and per-workspace or per-sandbox
overrides expressed as bounded deltas on the same object. Every write is
audited (who changed which rule, when). The only writers are coderd
actors: administrators today, human-in-the-loop approval flows later.
Because the writer is always coderd and never the workspace, runtime rule
WIDENING is permitted; that is what makes future egress approvals work
without restarts.

Delivery is one mechanism applied at two moments:

1. **Fork-time bootstrap.** The supervisor fetches the policy over its
   agent connection, materializes proxy rules, creates the network
   namespace, and only then fork-execs the confined child. The child never
   runs unconfined, even briefly. If the fetch fails, the child starts
   deny-all and reports degraded status; it never starts unconfined.
2. **Runtime updates.** Policy revisions are pushed over the existing
   agent connection and applied atomically to the running supervisor
   proxy. The namespace and child process are untouched: no restart, no
   re-fork.

**The supervisor is the sole policy consumer.** The confined child never
fetches, holds, or sees policy; its only network path is the parent proxy.
This is what makes live updates safe and preserves the invariant that
nothing inside the workspace can influence its own disposition.

#### Audit stream and retention

The confined child or supervisor sends events through agentapi to
`ai_sandbox_sessions` and `ai_sandbox_network_events`. Events correlate with
aibridge interceptions and become a Vertical 3 input.

Both tables snapshot raw agent and sponsor UUIDs on each retained record.
Those UUID columns are not foreign keys to `ai_agents` or `users`; audit
history must survive identity revocation, workspace deletion, and identity
cleanup. A session-to-event relationship may enforce its own retention-safe
integrity, but deleting an identity must never delete egress audit records.

Threat honesty: a network namespace beside its supervisor in the same
container is a strong fence, not a wall. Mitigations in order are: run the AI
as non-root with the supervisor under a different UID; drop `CAP_SYS_ADMIN`
and `CAP_NET_ADMIN` where templates allow; use VM-class workspaces with an
infrastructure-enforced perimeter such as NetworkPolicy or security groups
for the highest tier; retain the network namespace as defense in depth and
an audit source. The template envelope selects the tier.

### Credential plane

Principle: **the credential holder always sits across an isolation boundary
from the AI process.** Rules:

1. **MCP-first.** Agent actions needing third-party authentication go
   through the MCP gateway in AI Gateway. The human authorizes once at the
   gateway, the agent makes MCP tool calls, and the gateway injects
   credentials upstream. Credentials never transit the workspace.
2. **One `ai_credential_mode` per workspace agent, default `none`.** Declare
   it in Terraform on `coder_agent`, following the `api_key_scope`
   precedent; extract it through the provisioner; store it on the
   `workspace_agents` row; read it server-side at every enforcement point.
3. **External auth stays simple.** It is denied by default or passed through
   as-is only after explicit opt-in. Reduced-scope token minting and refresh
   adapters remain future work.

| Enforcement point                     | `none` (default)     | `injected`                         | `brokered`                                                                 |
|---------------------------------------|----------------------|------------------------------------|----------------------------------------------------------------------------|
| Manifest user secrets                 | Omitted              | Included with consent              | Omitted                                                                    |
| External auth endpoint (`gitaskpass`) | 403 plus MCP pointer | Stored token as-is with consent    | 403 plus broker remote hint                                                |
| `gitssh`                              | Refuses              | Serves only if explicitly declared | Refuses                                                                    |
| Git remotes and registry config       | Untouched            | Untouched                          | Rewritten to broker hostname at agent startup                              |
| Broker                                | Rejects caller       | Not applicable                     | Authenticates the AI key, reads this row server-side, and injects upstream |

- **Two-keyed consent for `injected`**: the template declares the mode and
  the sponsoring human consents at chat or workspace creation, or through a
  per-provider setting such as "allow my agents." External-auth links and
  user secrets belong to the user, not the template. Template-only opt-in is
  sufficient only for administrator-owned credentials such as template
  environment variables.
- **Grant-time audit**: injection has no per-use choke point, so record an
  audit event at build time, for example: "workspace W, agent AU, sponsor H
  received external-auth github through template opt-in and sponsor
  consent."
- **Documented risk**: an injected credential is the durable, full-scope
  token and can be exfiltrated by a prompt-injected agent. Default-deny
  egress limits exfiltration to allowed destinations. The administrator and
  sponsor accept this risk when both opt in.
- **Broker for the tier where the agent never sees the credential**: use a
  terminating endpoint, not a MITM. Rewrite workspace remotes to a URL such
  as `https://git-broker.<deployment>/github.com/org/repo.git`. The broker
  presents its own legitimate certificate, authenticates the caller with
  the scoped AI Coder key, maps agent to sponsor to credential, injects the
  upstream credential, enforces per-operation policy, and audits. Git fetch
  and push are distinguishable in the protocol. The same shape extends to
  package registries. This is optional for the PoC and can land later.
- **SSH git**: it is not brokerable in this design. Default-deny port 22 for
  AI-bound execution and force HTTPS through `url.insteadOf`.
- **Invariant**: nothing inside the workspace can influence its own
  credential disposition. Every enforcement point consults the stored row
  server-side; a lying agent only breaks itself.
- **Runtime observability**: surface disposition and binding on the
  workspace agent API object, workspace page, and `coder show`.

### Template envelope: Terraform

Templates are the governance boundary: the confined process must not define
its own confinement. The envelope declares what is possible, and runtime
instantiates within it. This mirrors the devcontainer declared and
discovered hybrid: chatd or the parent agent may create sandbox instances
dynamically, but only within the envelope.

Envelope contents: direct human AI-workspace opt-in through
`coder_ai_agent`; an attestation floor (administrators can require that
sandboxes declare `egress_enforcement = forced`); and `ai_credential_mode`.
Egress rules do NOT live in the envelope or anywhere in Terraform; they
live in template settings (see policy storage and delivery). Chat-created
workspaces are designated by coderd regardless of the opt-in parameter.
The server always applies the credential-denial and default-deny
baselines; templates can only request capabilities and bounded exceptions.

Provisioner-time child agents follow the `coder_devcontainer.subagent_id`
precedent so `coder_app`, `coder_env`, and `coder_script` can attach to the
sandboxed agent. This requires terraform-provider-coder changes through the
new `coder_ai_sandbox` resource and `data.coder_ai_agent` source, and keeps
the same explicit dependency flag as the V1 provider surface.

### Rejected alternative: agent-owned workspaces

Agent-owned workspaces remain technically viable. Every owner consumer could
resolve the sponsor transitively through
`workspace.owner_id -> ai_agents.owner_user_id`. The design was rejected
because owner semantics fan out across approximately eight subsystems, each
of which would need a durable sponsor, agent, or neither decision:

1. RBAC owner-equality checks.
2. Quota allowance.
3. Quota aggregation.
4. Provisioner owner metadata.
5. Owner session-token minting.
6. Workspace sharing and ACL reconciliation.
7. Dormancy RBAC.
8. Listings and telemetry.

That creates a permanent two-notions-of-owner tax at every new owner
consumer. Policy-based credential stripping must exist anyway for sandboxed
child agents in human workspaces, so structural credential starvation in an
agent-owned workspace would be redundant insurance rather than a sufficient
security boundary.

Keep this path documented for a future requirement that truly needs
agent-owned workspaces. Such a proposal must inventory every owner consumer
again and specify the sponsor, agent, or neither semantics at each site.

### AI Gateway governance dependency

The Vertical 1 **AI Gateway budget principal** follow-up remains open.
Interception attribution must use the AI agent, while budget enforcement and
spend aggregation must use the human sponsor. Vertical 2 credential,
egress, and sandbox governance depend on that split so AI-designated
workspace traffic cannot bypass sponsor budgets. Do not treat V2 governance
as complete until the aibridge authorization response carries distinct
attribution and budget principals.

### Schema changes: summary

1. Add `workspace_agents.ai_agent_id`, nullable foreign key to
   `ai_agents.user_id`, and `workspace_agents.ai_credential_mode`, enum
   `none | injected | brokered` with default `none`.
2. Add sandbox instance records keyed to workspace and parent agent, with
   backend, declared capabilities, policy reference, and lifecycle state.
3. Add `ai_sandbox_sessions` and `ai_sandbox_network_events`. Snapshot raw
   agent and sponsor UUIDs without foreign keys to `ai_agents` or `users` so
   audit history survives identity cleanup.
4. Add no auto-ACL machinery and no sponsor quota aggregation. Human
   ownership already provides control and quota attribution.

`ai_credential_mode` does not collide with the existing
`agent_key_scope_enum ('all', 'no_user_data')`; it is a separate enum with a
separate purpose.

Follow `.claude/docs/DATABASE.md` end-to-end for implementation: queries,
`make gen`, `enterprise/audit/table.go`, then `make gen` again.

### Invariants: drive tests from these

1. **Human ownership**: every workspace remains owned by its sponsoring
   human. Sponsor control and quota attribution follow from ownership; no
   ACL bridge is required.
2. **Identity continuity**: identities are minted only at a human-to-AI
   boundary. Every AI-created descendant reuses the requester's identity.
3. **No ambient human credentials for bound agents**: an AI-bound agent is
   denied owner user secrets, external-auth tokens, Git SSH keys, and the
   ambient full-owner session token unless the design explicitly permits the
   specific channel. Missing policy or resolution errors fail closed.
4. **Binding is server-authoritative**: only coderd and provisioner data can
   set or interpret `workspace_agents.ai_agent_id` and
   `ai_credential_mode`. In-workspace input cannot unbind an agent or change
   credential disposition, and the confined child never consumes egress
   policy; the supervisor is the sole policy consumer.
5. **Credential separation**: the workspace agent token governs the daemon;
   the scoped AI session token governs in-workspace CLI actions. Neither can
   silently substitute for the other.
6. **Default-deny egress**: a fresh AI-designated workspace or sandboxed
   agent can reach coderd, DERP, and AI Gateway, and nothing else. Every
   allowed or denied flow produces an attributed audit event when the
   backend declares egress capability.
7. **Structural confinement**: every process spawned through the confined
   workspace agent, including SSH, terminal, and scripts, observes the same
   network policy.
8. **Attestation honesty**: a sandbox's declared `egress_enforcement` is
   recorded server-side and surfaced through API and UI. The platform does
   not verify scripts; mismatch detection (an attested-forced sandbox whose
   proxy sees no traffic during AI activity) is Vertical 3 work.
9. **Durable attribution**: bound workspace-agent requests and background
   events record actor = agent and on_behalf_of = sponsor. Egress audit
   records survive identity cleanup.
10. **V1 invariants extend to workspace-agent and child-agent paths**: live
    sponsor permission ceiling, cascade suspend, attribution, no
    self-escalation, and fail-closed identity resolution apply beyond plain
    API-key requests.

### Implementation order

Each phase is independently mergeable and must add tests for the invariants
it introduces.

1. **Prerequisite safety fixes**: add the `DeleteSubAgent` parent check and
   fail-closed AI-bound handling at all three credential sources: manifest
   user secrets, `/workspaceagents/me/external-auth`, and
   `/workspaceagents/me/gitsshkey`. Land the minimum binding schema and
   query support needed by those checks in the same phase.
2. **Binding and attribution foundation**: complete
   `workspace_agents.ai_agent_id` support; resolve bound identities in
   workspace-agent middleware; build the sponsor subject intersected with
   `WorkspaceAgentScope` and forced `no_user_data`; stash `AIAgentActor`;
   plumb agent and sponsor attribution through request and background audit
   events.
3. **AI-designated workspace flow**: force designation in the chat workspace
   tool; reuse the requester's identity; bind every resulting workspace
   agent; implement direct human opt-in; suppress the owner session token;
   provide the scoped AI session token; add the 24-hour key renewal path.
4. **Confinement foundation**: add the sandbox script contract and
   attestation recording, the supervisor split, the SNI/CONNECT proxy,
   the template-settings egress policy object with two-phase delivery
   (fork-time bootstrap plus live updates), relay-only tailnet default,
   sandbox records, and the retained egress audit stream. Ship the netns
   supervisor as the first reference implementation. This makes both an
   AI-designated workspace and one sandboxed child usable.
5. **Credential plane**: add `ai_credential_mode`, mode-specific exceptions
   on top of default stripping, two-keyed consent, grant-time audit, MCP and
   broker pointers, brokered Git HTTPS rewrite, and runtime observability.
6. **MicroVM reference implementation**: extract and harden coder/sandbox
   per its hardening list, then ship it as the flagship reference script.
   This can begin after the script contract stabilizes.
7. **Terraform surface**: add `coder_ai_sandbox` (create/destroy scripts,
   `egress_enforcement`, `ai_credential_mode`, `subagent_id`) and
   `data.coder_ai_agent` to terraform-provider-coder. No egress rules in
   HCL. Keep this dependency-flagged and merge only with a compatible
   provider release.
8. **Governance completion**: split AI Gateway attribution and budget
   principals, then verify sponsor budget enforcement across chat,
   AI-designated workspace, and sandbox traffic.

### Non-goals: PoC and future work

- Human-in-the-loop egress approvals. Approvals become additional coderd
  writers to the template-settings policy object (runtime widening is
  already permitted), so no new delivery mechanism is needed; the approval
  API and UI land later.
- Per-credential dispositions, reduced-scope credential mint adapters such
  as GitHub App installation tokens and GitLab project tokens, dual-grant
  OAuth links, and credential refresh machinery beyond the required Coder
  key renewal path.
- Deployment-level transparent MITM.
- L7 method or path egress rules outside a backend with a sound guest CA
  boundary, initially `microvm`.
- More than 1 sandbox per workspace in practice, although the design
  supports N.
- Snapshots and checkpointing, plus Windows and macOS sandbox backends.
- Boundary or Agent Firewall integration and migration.
- True agent-owned workspaces unless the rejected alternative is revisited
  with a complete owner-consumer inventory.

### Appendix: Terraform surface

Three annotated examples. Status labels: EXISTS TODAY (works on the current
branch), PROVIDER CHANGE (requires the dependency-flagged
terraform-provider-coder work), SERVER-SIDE ONLY (deliberately not
expressible in HCL). Egress rules appear in NO example: they live in
template settings and are managed dynamically outside Terraform.

#### Example 1: AI-designated workspace via direct human opt-in

```hcl
# EXISTS TODAY: the server detects this parameter; value "true" mints the
# workspace-origin identity (V1) and, with V2, binds every agent,
# suppresses the owner token, and enables confinement.
data "coder_parameter" "coder_ai_agent" {
  name    = "coder_ai_agent"
  type    = "bool"
  default = "false"
  mutable = false
}

# PROVIDER CHANGE: reads the existing
# CODER_WORKSPACE_AI_AGENT_SESSION_TOKEN provisioner export. Empty when
# the workspace is not AI-designated.
data "coder_ai_agent" "me" {}

# Templates serving both normal and AI-designated workspaces select
# whichever token is present. In an AI-designated workspace the owner
# token is empty by suppression (SERVER-SIDE ONLY).
locals {
  session_token = (
    data.coder_ai_agent.me.session_token != ""
    ? data.coder_ai_agent.me.session_token
    : data.coder_workspace_owner.me.session_token
  )
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  # PROVIDER CHANGE: stored on the workspace_agents row, like
  # api_key_scope today. The server clamps it; "injected" additionally
  # requires sponsor consent.
  ai_credential_mode = "none"

  env = {
    CODER_SESSION_TOKEN = local.session_token
    ANTHROPIC_BASE_URL  = "https://ai-gateway.example.com/anthropic"
  }
}
```

#### Example 2: sandboxed AI child in a normal human workspace

```hcl
# Ordinary parent agent: full owner credentials, untouched behavior.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

# PROVIDER CHANGE: the sandbox script contract. The parent agent execs
# these admin-authored scripts; the platform provides CODER_AI_AGENT_URL,
# CODER_AI_AGENT_TOKEN (bound child token), CODER_AI_SESSION_TOKEN,
# CODER_EGRESS_PROXY, and CODER_SANDBOX_ID to them. The ai_agent_id
# binding itself is SERVER-SIDE ONLY.
resource "coder_ai_sandbox" "ai" {
  agent_id = coder_agent.main.id

  create  = file("./sandbox-up.sh")
  destroy = file("./sandbox-down.sh")

  # Admin attestation, recorded server-side; not platform-verified.
  egress_enforcement = "forced" # forced | advisory | none

  ai_credential_mode = "brokered"
}

# Apps and env attach to the SANDBOXED child agent
# (coder_devcontainer.subagent_id precedent).
resource "coder_app" "sandbox_terminal" {
  agent_id     = coder_ai_sandbox.ai.subagent_id
  slug         = "ai-terminal"
  display_name = "AI Sandbox"
}
```

#### Example 3: chat-created workspaces

No Terraform at all, by design. The chat tool forces AI designation
SERVER-SIDE ONLY: identity reuse, per-agent binding, owner-token
suppression, and confinement are applied to whatever template the chat
selects, regardless of what the template declares (invariant 4). The only
template-author obligation is compatibility: do not hard-depend on
`data.coder_workspace_owner.me.session_token` being non-empty; use the
fallback pattern from Example 1.

---

## Vertical 3: Auditing of Agent Activity

*Stub. To be designed.* Known anchors: `on_behalf_of_user_id` from
Vertical 1; bound workspace-agent and background-event attribution;
retained `ai_sandbox_sessions` and `ai_sandbox_network_events` raw agent and
sponsor UUIDs; credential grant-time audit events from Vertical 2; aibridge
interception lineage; delegation chains deeper than one level; human-in-the-
loop egress approval flows designed in Vertical 2; and frontend audit UX
for agent-versus-human filtering.
