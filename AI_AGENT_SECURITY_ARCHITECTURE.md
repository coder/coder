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
alone**; the `ai_agents` row must be loaded whenever
`users.kind = 'ai_agent'`. A dangling kind without a metadata row is an
auth error (fail closed, 401).

Known deviation: in the current implementation `users.kind` is the
runtime discriminator, not `ai_agents`. Authentication branches on the
kind and only then loads the metadata row
(`coderd/httpmw/apikey.go:500`), so the two markers can disagree in a
direction that fails OPEN: an `ai_agents` row whose user is
`kind = 'human'` skips delegation entirely and builds a direct human
subject. Nothing in the schema ties the two together. The fix is typed
composite foreign keys referencing `users (id, kind)`, which makes the
mismatch unrepresentable; see `AI_AGENT_IDENTITY_SCHEMA_REVIEW.md`.

This design originally held that no `chats.ai_agent_id` column was
needed, on the grounds that `(origin_type='chat', origin_id=chat.ID)` is
the link and is resolved through the unique partial index. That was
wrong. Without a durable reference on the chat, "no row" is ambiguous
between a chat that predates identities and a chat whose identity is
missing, and chatd resolves that ambiguity by falling back to the
sponsor's full subject. Workspaces already carry `ai_agent_id`; the
asymmetry with chats is accidental rather than principled.

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

| Profile                                      | Scopes (starting set, tune during impl)                                                                         | Allow list                                                                              |
|----------------------------------------------|-----------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| `ChatAgentProfile(chatID, ...)`              | `coder:workspaces.create`, `coder:workspaces.operate`, `coder:workspaces.access`, chat read/update, `user:read` | exact chat ID; typed wildcards for workspace, template, organization-member, user (PoC) |
| `WorkspaceAgentIdentityProfile(workspaceID)` | modeled on `rbac.WorkspaceAgentScope`: workspace operate/access + `no_user_data` exclusion                      | that workspace's ID, no wildcard                                                        |

Hard exclusions in every profile: `coder:apikeys.manage_self` (agents must
not mint or manage credentials), `coder:templates.author`, user-data scopes
unless explicitly needed. Profiles are hardcoded for PoC; org-configurable
profiles are future work.

Profile scopes and allow lists cannot express "only this agent's
workspaces". One action-independent allow list applies to the union of all
selected scopes, it is fixed at mint time, and it must include a workspace
wildcard for creation to authorize an object that has no ID yet. The chat
profile's wildcard therefore reaches the sponsor's ordinary workspaces,
including SSH. The next section closes that reach inside `coderd/rbac`
itself; `AI_AGENT_RBAC_PROFILE_REVIEW.md` holds the full reachability
analysis that derived it.

### Designation as an RBAC authorization boundary

Status: specified, not implemented. This is Vertical 1 step 10 in the
implementation order below.

Scope profiles alone cannot confine an agent to its own workspaces, for
the structural reasons stated above: the allow list is static subject
state fixed at mint time, one action-independent list applies to the
union of every selected scope
(`coderd/database/modelmethods.go:283-339`), and the list must contain a
workspace wildcard because creation authorizes an object that has no ID
yet (`coderd/workspaces.go:944-976`). Meanwhile the sponsor's ordinary
workspaces hold exactly what Vertical 2's credential starvation withholds
from designated ones: the ambient `coder:all` owner session token,
external-auth tokens, and the Git SSH key. Without a further rule,
`workspace:ssh` against the wildcard walks into an ordinary workspace and
reads those credentials, and `workspace:start` rebuilds one with an
attacker-selected template version. Note that start, not update,
authorizes the version switch on a rebuild
(`coderd/wsbuilder/wsbuilder.go:1253-1279`); waking a dormant workspace
is the exception that authorizes through update.

This section specifies the authorization boundary that closes that
reach. The marker it reads, `workspaces.ai_agent_id`, belongs to
Vertical 2: its two triggers and its one-way ratchet are specified there
under "AI-designated workspaces".

`AI_AGENT_RBAC_PROFILE_REVIEW.md` derives the boundary from a
reachability matrix of both profiles (eleven OPEN rows, all through the
workspace wildcard), argues an object-side policy attribute against a
dynamic allow list, and recommends the former. The dynamic allow list is
rejected as unworkable rather than merely inferior: it cannot authorize
creation before an ID exists, cannot differ per action, and turns
credential rows into mutable request-time state with append, rotate, and
revoke races. This section is the resulting specification.

#### Mechanism

The boundary is one new conjunct on the single existing `allow` rule in
`coderd/rbac/policy.rego`, beside `permission_allow` and `scope_allow`.
There is no middleware veto and no second authorization layer; the policy
remains the only decision point, and list-query filtering continues to be
derived from the same policy by partial evaluation.

```rego
allow if {
    permission_allow
    scope_allow
    ai_workspace_designation_allow
}

# Passes unconditionally for non-AI subjects; passes for exempt actions;
# otherwise requires exact designation lineage. Sketch:
ai_workspace_designation_allow if {
    not subject_is_ai_agent
}

ai_workspace_designation_allow if {
    subject_is_ai_agent
    not ai_workspace_action_requires_designation
}

ai_workspace_designation_allow if {
    subject_is_ai_agent
    not input.subject.ai_agent_id = ""
    input.object.ai_agent_id = input.subject.ai_agent_id
}
```

The rule consumes two new policy-input attributes:

- `input.object.ai_agent_id`: the workspace's designation marker,
  populated from `workspaces.ai_agent_id` in the RBAC object converters.
  Empty string means undesignated.
- `input.subject.ai_agent_id`: the acting AI identity, populated wherever
  an AI actor is resolved. Empty string means a non-AI subject.
  `Subject.ID` remains the sponsoring human; the identity split above is
  unchanged.

#### Specified behavior

1. **Non-AI subjects are unaffected.** A subject with a non-AI type and an
   empty acting ID passes the rule unconditionally. Human and system
   access to every workspace, designated or not, is unchanged.
2. **Exempt actions: `read` and `create`.** An AI subject may read
   workspace metadata sponsor-wide (inventory UX, "what workspaces do I
   have?") and may create, which necessarily authorizes an ID-less
   object. Creation safety comes from the chokepoint designating every
   AI-created workspace before its first build, not from this rule.
3. **Protected actions: everything else.** Any other action on a
   workspace-typed object (`workspace`, `workspace_dormant`,
   `prebuilt_workspace`), including `ssh`, `application_connect`,
   `start`, `stop`, `update`, and `delete`, requires
   `object.ai_agent_id == subject.ai_agent_id`, exact match. The
   protected set is defined by exclusion so that future workspace actions
   are protected by default.
4. **Fail closed.** An undesignated object never matches a non-empty
   acting ID. An AI-typed subject whose acting ID is empty is denied
   protected actions rather than treated as human; a subject counts as
   AI-delegated when either marker says so (subject type `ai_agent` or a
   non-empty acting ID). Aggregate objects (`Object.All()`) carry no
   designation and are denied.
5. **Cross-agent isolation.** Exact equality denies agent A protected
   actions on a workspace designated to agent B, shared sponsor
   notwithstanding.

#### Decided defaults

| Decision                                           | Default                                                                           |
|----------------------------------------------------|-----------------------------------------------------------------------------------|
| Read/list of the sponsor's undesignated workspaces | Allowed. Metadata reach is accepted for chat inventory UX.                        |
| SSH and application connect                        | Designation match required.                                                       |
| Start, stop, update                                | Designation match required. Start covers rebuilds and template-version selection. |
| Cross-agent access under one sponsor               | Denied. Exact identity equality, not merely non-null designation.                 |
| Future workspace actions                           | Protected by default via the exemption-list structure.                            |
| Workspace create                                   | Allowed without a match; the chokepoint designates the result before it builds.   |

#### Implementation requirements

Status: none of this is implemented. `coderd/rbac` contains no
designation attribute today. Unlike citations elsewhere in this document,
which describe current behavior, the `file:line` references in this
subsection are **change targets**: they name the declaration or call site
each requirement modifies. The review artifact carries fuller snippets
for each.

1. **Object attribute.** `rbac.Object` gains `AIAgentID string`
   (`json:"ai_agent_id"`) and a `WithAIAgentID` builder; the struct is
   declared at `coderd/rbac/object.go:25-43`. The eight builders that
   reconstruct the struct (`coderd/rbac/object.go:141-239`) must
   preserve it, `Equal` (`coderd/rbac/object.go:93`) must compare it,
   and `All()` must deliberately clear it so aggregate authorizations
   fail closed.
2. **Subject attribute.** `rbac.Subject` gains `AIAgentID string`, and
   `Type` becomes a functional policy input rather than logging-only.
   The struct is declared at `coderd/rbac/authz.go:101-123`, where
   `Type` is currently documented as "not used in any functional way,
   only for logging". An `AsAIAgent(id, name)` helper sets
   both and rebuilds `cachedASTValue`; mutating a functional field
   without invalidating the cached AST poisons authorization. The field
   stays exported and `Subject.Equal` compares it: the global
   authorization cache hashes the JSON-encoded subject, and an
   unexported field would let two different agents share cache entries
   (`coderd/rbac/authz.go:37-59`).
3. **Rego input.** Both attributes enter the manually built AST values in
   `coderd/rbac/astvalue.go:83-100,114-143`. Empty strings are emitted,
   not omitted; fail-closed equality depends on them.
4. **Object population happens in converters only.**
   `WorkspaceTable.RBACObject` sets the attribute when `AIAgentID.Valid`
   (`coderd/database/modelmethods.go:538-555`). The same population
   applies to `DormantRBAC`, `AsPrebuild`, and `WorkspaceIdentity`, and
   `Workspace.WorkspaceTable` must copy the column when reducing.
   Handlers never set it.
5. **Subject population at all three resolution sites.** The HTTP agent
   path (`coderd/httpmw/apikey.go:524-530`), in-process chat tool
   subjects (`coderd/x/chatd/chattool/subject.go:56-70`), and
   workspace-agent middleware
   (`coderd/httpmw/workspaceagent.go:156-191`) call `AsAIAgent` after
   building the sponsor subject. These sites construct input only; the
   decision stays in the policy.
6. **Partial evaluation and SQL.** `input.object.ai_agent_id` joins
   `rego.Unknowns` (`coderd/rbac/authz.go:349-370`) and gets a
   registered converter, `COALESCE(workspaces.ai_agent_id::text, '')`,
   in `coderd/rbac/regosql/configs.go`; `regosql` rejects unmapped
   variables outright (`coderd/rbac/regosql/compile.go:171-199`).
   Because the action is ground at partial-evaluation time and `read` is
   exempt, list queries produce no new SQL residual; workspace listing
   is unchanged.
7. **Refresh the build object at the chokepoint.** `createWorkspace`
   designates the row but passes the earlier in-memory workspace to
   `wsbuilder` (`coderd/workspaces.go:763-784,795-823`). The value
   returned by `SetWorkspaceAIAgentID` must be copied back before the
   initial build's `start` authorization, or every AI-created workspace
   fails its own first build under the new rule.

#### Test derivation

- `Authorize` quadrants: AI subject on an undesignated workspace is
  denied every protected action; AI subject on its own designated
  workspace is allowed; agent A on agent B's workspace is denied; a human
  subject is unchanged on all of the above.
- Empty-input hardening: an AI subject type with an empty acting ID is
  denied protected actions; `Object.All()` is denied.
- `Prepare`/`CompileToSQL` on workspace list queries: identical SQL
  before and after for `read`; protected-action prepared queries compile
  with the designation predicate.
- End to end: a chat-created workspace's first build succeeds (chokepoint
  refresh); AI SSH to a sponsor's ordinary workspace is denied.

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

### Design decisions and rejected alternatives

The sections above describe the design. This section records why it has
this shape, so a reviewer does not have to reconstruct the argument, and so
a future change knows which constraint it is breaking.

#### Real `users` rows, not a new principal type

**Chosen:** AI agents are ordinary `users` rows marked `kind = 'ai_agent'`,
with an `ai_agents` side table as the authoritative marker.

**Rejected:** a separate principals table, or reusing service accounts.

**Why:** every attribution surface in the product already keys on a user
ID. `audit_logs.user_id`, `aibridge_interceptions.initiator_id`, API keys,
and request logging all take a `uuid` that must resolve to a user. A new
principal type would require touching each of those, and any surface
missed would fail open by attributing agent activity to a human. Reusing
the row type makes attribution work by default and makes a missed surface
a visible bug rather than a silent misattribution.

The cost is that agent rows appear in tables that assume humans, which is
why they are excluded from user listings, organization and group member
lists, dormancy, AI seat counts, and notifications. That exclusion work is
real and was found by review rather than by design; a separate principal
type would have avoided it. We judged silent misattribution to be the worse
failure.

#### Sponsor intersection, not roles of their own

**Chosen:** the RBAC subject is built from the sponsoring human's live
roles and groups, intersected with the agent key's scopes and allow list.

**Rejected:** granting agent users their own roles and organization
membership.

**Why:** an agent with independent roles is a principal that can outlive,
exceed, or drift from the human who created it. Intersection makes
"an agent can never do more than its sponsor" a structural property rather
than an operational promise. It also means there is no new authorization
machinery to get wrong: the ceiling is enforced by the same code path that
already evaluates human permissions.

The consequence is that agents cannot be granted anything their sponsor
lacks, including for legitimate reasons. If a future case needs that, it is
a deliberate departure from this model, not a configuration change.

#### Live role lookup, not a permission snapshot

**Chosen:** roles are fetched per request through the normal subject
construction path.

**Rejected:** copying the sponsor's permissions onto the agent at mint
time.

**Why:** a snapshot has a revocation window. If a human is suspended or
loses a role, any agent holding a snapshot keeps that access until
something reconciles it, and reconciliation is exactly the kind of job that
fails quietly. Live lookup makes suspension propagate on the agent's next
request with no background work. Cascade suspend is a consequence of the
design rather than a feature that has to be maintained.

#### Authorization identity is the human, actor identity is the agent

**Chosen:** `Subject.ID` is the sponsoring human; the API key belongs to
the agent user.

**Rejected:** making the agent the subject and bridging access through
ACLs or ownership transfer.

**Why:** owner-scoped permission checks match `resource.owner_id ==
subject.ID`. An agent subject would fail every such check against its
sponsor's own chats and workspaces, so it would need an ACL bridge on
every owned resource. That bridge is both more code and a second place for
access to be wrong. Splitting the two identities gets the permission
behavior for free and still attributes actions to the agent, because
attribution follows the key rather than the subject.

This is the decision with the widest blast radius: it is why workspaces
stay human-owned in Vertical 2, and why agent-owned workspaces were later
rejected on compatibility grounds.

#### One identity per delegation boundary

**Chosen:** mint an identity when a human crosses into AI execution, and
reuse it for everything that execution creates.

**Rejected:** one identity per agent process, per session, or per
workspace.

Concretely, identities are minted only at a human-to-AI boundary: when a
human creates a chat (a chat-origin identity), and when a human opts a
workspace in through the `coder_ai_agent` parameter with no chat involved
(a workspace-origin identity). When an AI agent creates resources, no new
identity is minted; the created workspace, workspace agents, sandboxed
child agents, and child chats bind to the requester's existing identity.

**Why:** the question an auditor asks is "which delegation did this come
from," not "which process ran it." Per-process identities fragment one
human decision across many principals and make the audit trail harder to
follow, not easier. Reuse gives one unbroken lineage per delegation: a
chat that creates a workspace produces agents carrying the chat's identity,
so the workspace's activity is traceable to the conversation that caused
it.

The tradeoff is coarser granularity. Two concurrent tasks in one chat share
an identity and are distinguished by other event fields, not by principal.

#### Non-interactive by construction

**Chosen:** `login_type = 'none'`, empty email, generated username, no
password, no organization membership.

**Why:** an agent user that can authenticate interactively is a credential
that can be phished, shared, or left behind. Making the login path
structurally absent is stronger than relying on policy, and the empty email
also keeps agents out of notification paths that assume a reachable human.

#### Revocation is a soft delete

**Chosen:** revoking an identity marks `ai_agents.deleted` and removes its
keys; the user row and its audit history remain.

**Rejected:** deleting the row.

**Why:** audit records reference the agent by user ID. Hard deletion would
either break those references or force the audit trail to be rewritten,
and an audit trail that can be erased by revoking the principal it
describes is not an audit trail. Soft deletion also lets the system
distinguish "this identity was revoked" from "this identity never existed",
which is what makes fail-closed resolution possible.

Three qualifications the current implementation does not yet meet:

- **Survival is conventional, not structural.** `ai_agents.owner_user_id`
  is `ON DELETE CASCADE`, so hard-deleting a SPONSOR erases the identity
  metadata that retained audit rows still reference. Nothing survives
  because the schema guarantees it; it survives because no hard-delete
  path currently exists. The V2 and V3 audit tables were deliberately
  built with raw UUIDs and no foreign keys for exactly this reason, and
  V1 was not held to the same standard. `ON DELETE RESTRICT` is the fix.
- **Revocation is reversible.** `deleted` is a boolean that can be set
  either direction, there is no revocation timestamp or reason, and the
  agent's `users` row stays active afterwards. An immutable
  `active | revoked` state with a timestamp and reason is the better
  model, and is what Vertical 3 will want to query.
- **Revocation is not applied on origin deletion.** Deleting a workspace
  revokes its scoped key but leaves the workspace-origin identity
  `deleted = false`. The lifecycle coupling is application code that was
  never written rather than a database constraint.

- **Resolution conflates two questions.** `Resolve` answers "may this
  identity act", and `ResolveOnBehalfOf` reuses it to answer "who was
  this identity acting for", so a delayed background audit written after
  revocation silently loses sponsor lineage. These should be separate
  entry points: authorization requires a live identity and sponsor,
  attribution returns immutable provenance regardless of state.

See `AI_AGENT_IDENTITY_SCHEMA_REVIEW.md` for the proposed DDL.

#### Fail closed on resolution, everywhere

**Chosen:** a `users.kind = 'ai_agent'` row without a live `ai_agents`
record is refused, as is a deleted identity or a deleted, suspended, or
non-human sponsor.

**Why:** the conformance review found the opposite behavior in practice:
chatd failed open to full-owner tools when identity resolution errored, and
in-process tools discarded sponsor status so a suspended human could still
act through their agent. Both passed their tests. Fail-closed resolution is
stated as an invariant precisely because the failure mode is invisible
until someone looks for it.

#### Accepted deviation: agents may read their own owner

`ChatAgentProfile` permits `user:read`, because an agent must be able to
resolve the human it acts for. It does not permit `user:read_personal`,
`user:update`, `user_secret:*`, `user_skill:*`, `api_key:*`,
`coder:templates.author`, `coder:apikeys.manage_self`, `coder:all`, or a
global `*:*` allow list. The validator rejects those; an earlier revision
accepted `coder:all` and `*:*`, which the review caught.

Two further qualifications from the security review:

- **The validator is allow-by-default and has drifted.** The generated
  API key scope enum has 236 values and the RBAC catalog has 50 concrete
  resource types, while `validateProfile` names four scopes explicitly
  and applies semantic rules to five resource families
  (`coderd/aiagentidentity/profile.go:92-133`). Everything else is
  accepted. Composite `coder:*` scopes are also checked by string prefix
  rather than expanded permission by permission. It should be an
  allowlist.
- **The chat profile's allow list is broader than "the chat's own
  resources".** It uses typed wildcards for workspaces, templates,
  organization members, and users, because a workspace being created has
  no ID to pin in advance. The scope machinery applies one
  action-independent allow list to the union of selected scopes, so a
  compromised chat bearer key can read, start, stop, update, SSH into,
  and connect to every workspace its sponsor can reach, not only the one
  the chat created. The sponsor ceiling still applies, so this is
  bounded by the human's own access, and chatd discards the key
  plaintext after minting, which makes the surface latent rather than
  live. It is nonetheless wider than the design implies. The designation
  boundary above is the accepted fix: protected workspace actions
  require an exact designation match, which collapses the wildcard's
  reach to metadata read. See `AI_AGENT_RBAC_PROFILE_REVIEW.md`.

### Invariants (drive tests from these)

1. **Ceiling**: agent's effective permissions ⊆ owner's permissions at all
   times; role removal from owner is reflected on the agent's next request.
2. **Cascade suspend**: agent key auth returns 401 when owner is suspended
   or deleted; also when the `ai_agents` row is deleted.
3. **Attribution**: every audited agent action has
   `user_id = agent user, on_behalf_of_user_id = owner`; audit queries by
   owner return both their own and their agents' actions.
4. **Non-interactive**: agent users can never authenticate via
   password/OIDC/GitHub (login_type none) and do not appear in default
   user lists. Note the last two properties are enforced by query filters
   rather than by the schema: listing queries filter on kind, but
   mutation paths still accept agent user IDs, so an administrator can
   assign organization or group membership and roles to an agent user,
   and chat sharing can enqueue an inbox notification to one. Those
   assignments do not affect agent authorization, which always uses the
   sponsor's roles, but they are representable.
5. **No self-escalation**: generic session-key and token routes reject
   `kind = 'ai_agent'` targets, so those public routes cannot mint AI agent
   credentials outside `aiagentidentity.MintKey`. Profile validation permits
   only the exact scopes used by the built-in profiles, expands composite
   `coder:*` scopes through RBAC, checks every resulting resource-action
   permission, and rejects global allow-list entries. The built-in profiles
   exclude API key permissions, so a built-in-profile credential cannot mint
   itself or its sponsor a broader credential.

   This invariant was previously proven FALSE. Before these checks, the
   generic key APIs bypassed `validateProfile` and could create a full-scope
   token for an agent user. Because the delegated subject ID is the sponsor,
   that token could then mint a normal human token for the sponsor. The
   demonstrated path required site-owner-level privilege to create the
   initial unsafe key; a built-in chat agent key could not call the generic
   routes. A security review demonstrated the escalation with an executable
   test. See `AI_AGENT_IDENTITY_SECURITY_REVIEW.md`.
6. **Fail closed**: `users.kind = 'ai_agent'` without a live `ai_agents`
   row is an authentication error. This holds for both authentication
   middlewares. It does NOT currently hold in chatd, which treats a
   missing row as a chat that predates identities and falls back to
   running platform tools with the sponsor's full subject
   (`coderd/x/chatd/chatd.go:3880-3890`). A durable per-chat identity
   reference is required to distinguish a legacy chat from a corrupted
   one.
7. **Cascade suspend applies per connection, not per message**: subjects
   are built when a request or connection is authenticated. A long-lived
   DRPC or WebSocket session keeps the subject it was created with, so a
   sponsor suspended mid-session retains access until that session ends.
   New requests and reconnections see the change immediately.
8. **Designation boundary**: an AI subject is denied every workspace
   action except `read` and `create` unless the workspace's designation
   (`workspaces.ai_agent_id`, Vertical 2) exactly matches its acting
   identity. Undesignated workspaces, empty acting IDs, aggregate
   objects, and workspaces designated to a different agent all deny.
   Non-AI subjects never evaluate the rule.

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
10. RBAC designation boundary per "Designation as an RBAC authorization
    boundary" above: object and subject designation attributes, the
    `policy.rego` conjunct, partial-evaluation and `regosql` support,
    subject population at all three resolution sites, and the
    `createWorkspace` build-object refresh.
11. Tests per the invariants above (use unique identifiers; no
    `time.Sleep` for timing, per `.claude/docs/TESTING.md`).

Steps 1 to 6 are the foundation and independently mergeable; 7 to 9 are
per-surface and parallelizable after 4. Step 10 needs Vertical 2's
designation flow (its phase 3) for end-to-end effect, though the
`coderd/rbac` changes themselves only require the `workspaces.ai_agent_id`
column.

### Review artifacts

Four independent reviews of this work exist as separate documents. They
are the source for the corrections and qualifications recorded above, and
each states what it could not verify:

| Document                                               | Question asked                                                                                                   | Headline finding                                                                                                                                                                                                                                                                                 |
|--------------------------------------------------------|------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| (conformance pass, findings folded into this document) | Does the code do what this document claims?                                                                      | Eight disagreements; a chat key could create an unmarked workspace and receive the sponsor's ambient credentials. Fixed at the `createWorkspace` chokepoint.                                                                                                                                     |
| `AI_AGENT_IDENTITY_SCHEMA_REVIEW.md`                   | Is the schema the right shape, and what would a better one be?                                                   | `users.kind` is the runtime discriminator rather than `ai_agents`, and the two can disagree in a direction that fails open. Includes proposed DDL and costs.                                                                                                                                     |
| `AI_AGENT_IDENTITY_SECURITY_REVIEW.md`                 | What can a compromised or misused agent credential actually reach?                                               | Invariant 5 is false for keys minted outside the built-in profiles; demonstrated by executable test. Also inventories all 236 scopes and 50 resources against the validator.                                                                                                                     |
| `AI_AGENT_RBAC_PROFILE_REVIEW.md`                      | What do the scope profiles reach, and how should "only this agent's workspaces" be modeled inside `coderd/rbac`? | Eleven OPEN rows in the chat profile's reachability matrix, all through the workspace wildcard, including SSH into the sponsor's ordinary workspaces. Recommends an object-side designation boundary in `policy.rego`; specified in Vertical 1, "Designation as an RBAC authorization boundary". |

Findings still open are listed in those documents rather than duplicated
here. The ones that change a stated invariant are marked inline above.

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

The minting rule is an identity decision and is stated in Vertical 1 under
"One identity per delegation boundary". Its consequences for this vertical:

A chat-created workspace remains owned by the chat sponsor, while all of
its workspace agents carry the chat identity's user ID in
`workspace_agents.ai_agent_id`. Child chats already reuse the root chat
identity; Vertical 2 extends that precedent to workspace and sandbox
creation, so a workspace, its agents, and any sandboxed child inherit the
identity of whichever delegation asked for them. One delegation chain has
one unbroken AI identity lineage.

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

Designation is recorded on the workspace row itself:
`workspaces.ai_agent_id` (nullable foreign key to `ai_agents.user_id`,
migration 000567) names the single AI identity the workspace belongs to.
It is not ownership; `owner_id` stays the human, and the sponsor's own
access is never affected by it. The marker has three consumers:

1. **Credential starvation** at build time: a designated workspace never
   receives the ambient full-owner session token
   (`coderd/provisionerdserver/provisionerdserver.go:586-607`).
2. **Workspace-agent binding**: at build completion the provisioner copies
   the marker into `workspace_agents.ai_agent_id` for every agent of that
   build (`provisionerdserver.go:2234-2245`), so binding survives rebuilds.
3. **The RBAC designation boundary** specified in Vertical 1
   ("Designation as an RBAC authorization boundary").

Two events set the marker, and nothing clears it:

1. **Creation by an AI actor.** The shared `createWorkspace` chokepoint
   designates the new workspace with the requesting identity before its
   first build, regardless of what the template declares
   (`coderd/workspaces.go:776-782`).
2. **Human opt-in.** A start build carrying `coder_ai_agent = true`
   designates a not-yet-marked workspace with its workspace-origin
   identity (`provisionerdserver.go:3292-3305`).

The marker is deliberately a one-way ratchet, not a per-build derivation
from the template declaration. Template version and build parameters are
attacker-selectable inputs for an agent allowed to start its own
workspace; if qualification were recomputed each build, the agent could
rebuild its own workspace with a version or parameter set lacking the
declaration, and the next build would mint a fresh full-owner session
token into an environment the agent controls. The token decision
therefore treats the stored marker as dominant over the current build's
parameters (`provisionerdserver.go:589-591`), and the build that
designates a previously ordinary workspace deletes any owner token minted
by earlier undesignated builds. Un-designation has no API; the recovery
path for a mistaken opt-in is workspace recreation. This asymmetry is
intentional: un-designating is dangerous because a workspace that hosted
an agent may carry agent-influenced state that would then receive full
credentials, while recreation is cheap.

#### Chat-created workspaces

The chat workspace-creation tool forces AI designation. It must not depend
on the selected template declaring the `coder_ai_agent` parameter or setting
it to `true`.

Creation follows these rules:

1. Keep `workspaces.owner_id` set to the chat identity's sponsoring human.
2. Reuse the requesting chat's AI identity. Do not create a workspace-origin
   identity.
3. Set `workspaces.ai_agent_id` to that chat identity at the shared
   `createWorkspace` chokepoint, before the first build, so designation
   precedes any provisioning.
4. Set `workspace_agents.ai_agent_id` to that chat identity for every agent
   produced by the workspace build, including agents added by subsequent
   builds.
5. Suppress `data.coder_workspace_owner.me.session_token`, whose
   `coder:all` owner credential must never enter an AI-designated workspace.
6. Provide the scoped AI session token instead.

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
binary injection) with a generic parent-agent script executor and
reconciler for the sandbox script contract below.

**Prerequisite fix (landed in phase 1):** `DeleteSubAgent` previously did
not verify that the target's `parent_id` equals the caller. Deletion is
now bound to the calling parent (`coderd/agentapi/subagent.go`), so
sandbox lifecycle can safely build on this API.

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
# An internal-only docker network with no gateway: the sandbox can reach
# the parent-side proxy and the control plane through it, and nothing
# else routes out. This is what an "egress_enforcement = forced"
# attestation is claiming.
docker network create --internal "sbnet-${CODER_SANDBOX_ID}" >/dev/null

docker run -d --name "sb-${CODER_SANDBOX_ID}" \
  --network "sbnet-${CODER_SANDBOX_ID}" \
  -e CODER_AGENT_URL="$CODER_AI_AGENT_URL" \
  -e CODER_AGENT_TOKEN="$CODER_AI_AGENT_TOKEN" \
  -e CODER_SESSION_TOKEN="$CODER_AI_SESSION_TOKEN" \
  -e HTTPS_PROXY="http://${CODER_EGRESS_PROXY}" \
  -e HTTP_PROXY="http://${CODER_EGRESS_PROXY}" \
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

#### Sandbox identity and key lifecycle

The child's `ai_agent_id` and the owner of its `CODER_AI_SESSION_TOKEN`
are always the SAME identity. Which identity, per identity continuity:

- **AI-requested sandbox** (a chat or bound agent asks for one): reuse the
  requester's existing identity (`AIAgentActor.AgentUserID`). No new
  identity is minted.
- **Human-declared sandbox** (template declares `coder_ai_sandbox`, no AI
  requester): this is a human-to-AI boundary. Mint or reuse the V1
  workspace-origin identity for the enclosing workspace, exactly as the
  `coder_ai_agent` opt-in path does.

The sandbox lifecycle (coderd, when creating the child agent row) calls
`aiagentidentity.MintKey` with a sandbox-unique token name for that
identity. The key is revoked when `destroy` runs or the sandbox record is
deleted, and it participates in the same 24-hour renewal work item as
other workspace keys. V1's `regenerateAIAgentSessionToken` cannot be
reused unchanged: it looks up or creates a workspace-origin identity,
which is wrong for AI-requested sandboxes that must reuse the requester's
identity.

**Attestation, not verification.** `egress_enforcement` is an admin
attestation. A script that claims `forced` but leaks a side channel is
undetectable at declaration time. This is consistent with the existing
trust model, because template admins already fully control workspaces, and
it is partially auditable post hoc: an attested-forced sandbox whose proxy
sees no traffic while the AI is clearly active is an anomaly signal
(a Vertical 3 input). It remains a downgrade from platform-enforced
confinement; the doc states it plainly rather than implying the platform
verifies scripts.

Confinement guarantees are therefore SCOPED, not universal: the platform
proxy is always default-deny for traffic that reaches it; the netns
reference implementation guarantees forced routing for AI-designated
workspaces; an arbitrary script only attests its routing coverage. The
structural-confinement invariant applies to `forced` shapes; `advisory`
and `none` do not satisfy it and are recorded as such. Requiring
`forced` through the envelope is an admission-control claim, not added
enforcement.

#### Reference implementations

Built-in confinement mechanisms are reference implementations invoked
through the same script contract, not privileged code paths:

| Reference             | Isolation                     | Notes                                                                               |
|-----------------------|-------------------------------|-------------------------------------------------------------------------------------|
| netns supervisor      | Linux network namespace       | Used for AI-designated whole-workspace confinement; also usable as a sandbox script |
| coder/sandbox         | libkrun microVM, KVM required | Flagship reference; per-sandbox MITM possible with a guest CA                       |
| rootless podman       | Container, user namespace     | Daemonless, so its processes stay descendants and inherit platform confinement      |
| devcontainer retrofit | Container via a daemon socket | No egress enforcement; the daemon escapes confinement, so it attests `none`         |

Placement in that table follows from the backend compatibility rule in
"Host-side enforcement" below: a backend is confinable when every process
that originates egress is a descendant the platform launched. Daemonless
runtimes qualify; anything reached through a pre-existing privileged
helper socket does not.

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

#### Host-side enforcement: where the rules live

The confinement above places the child in a namespace and the proxy
outside it. That is necessary but not sufficient, because it leaves open
where the firewall rules that force traffic to the proxy are installed.
Putting them inside the child's namespace is the obvious choice and the
wrong one.

**Principle: enforcement rules live on the HOST side of the veth, in the
supervisor's own namespace. Rules inside the confined namespace exist only
to make traffic flow to the proxy, never to decide whether it may leave.**

Netfilter rules are enforced by the kernel, so nothing inside the namespace
can send a packet past them by being uncooperative. But a process holding
`CAP_NET_ADMIN` **in the namespace that owns the rules** can simply delete
them. If the rules are inside the child's namespace, the child's own
privilege defeats them. If they are on the host side, the child may flush
its own namespace freely: its traffic then leaves the veth undirected,
meets the supervisor's default-deny, and is dropped. The child can break
its own connectivity. It cannot widen it.

This inversion has a consequence worth stating plainly, because it decides
which sandbox technologies the platform can support:

**`CAP_NET_ADMIN` inside the confined namespace becomes tolerable.** Sandbox
backends that must create bridges, veths, or TAP devices to build their own
boundary (rootful podman with netavark, TAP-based VMMs) stay compatible,
because the privilege they need is confined to a namespace whose egress the
platform controls from outside. A design that enforced from inside would
have to strip that privilege and would therefore exclude them.

Rules are installed on both sides, with different jobs:

| Side                  | Purpose                                                                                                             | If the child subverts it                                             |
|-----------------------|---------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------|
| Child namespace       | Transparent redirection: DNAT outbound TCP to the supervisor's listeners, DNAT port 53 to the DNS relay             | Child loses interception and its traffic is dropped by the host side |
| Host side of the veth | Enforcement: accept only the listener ports from the peer address, drop everything else, drop forwarding, drop IPv6 | Not reachable from inside the namespace                              |

##### Routing: the child needs a default route

An earlier form of this design specified a namespace with no default route,
on the reasoning that no route means no egress. That is fail-closed but
nonfunctional: for locally generated TCP, Linux resolves the route before
the packet reaches the `OUTPUT` chain, so `connect()` fails with
`ENETUNREACH` before any DNAT rule can redirect it. Transparent
interception requires a default route through the veth, with the host-side
rules providing the deny.

##### Datapath

The confined namespace gets a default route via the host veth address,
plus DNAT rules that send port 80 to the transparent HTTP handler, port
443 to the SNI listener, and port 53 to the DNS relay. Traffic already
addressed to the supervisor's listeners returns early so proxy-aware
clients are unaffected.

The host side accepts, on that veth only, connections from the peer
address to those listener ports, and drops everything else inbound and
forwarded. IPv6 is disabled in the namespace or given equivalent rules;
leaving it implicit creates an uncovered path because `iptables` is IPv4
only. UDP other than the DNS relay and all ICMP are dropped, which closes
QUIC on UDP 443 and ICMP as an unrecorded channel.

##### Privilege: a separate problem from the rules

Host-side enforcement removes the need to strip `CAP_NET_ADMIN` to protect
the *rules*. It does not remove the need to protect the *supervisor*, which
is a distinct concern with a distinct mitigation.

The supervisor holds sockets created in the host network namespace and the
agent's credentials. A socket carries the namespace it was created in, so
obtaining one of the supervisor's file descriptors yields unfiltered egress
without a packet ever crossing the veth. A confined process running as the
same UID as the supervisor can reach them: `ptrace`, `process_vm_readv`,
`pidfd_getfd`, `/proc/<pid>/mem`, or simply opening a credential file whose
permissions assume a trust boundary that is not there.

The requirement is therefore: **the confined process must not be able to
attach to, read, or open anything belonging to the process enforcing its
confinement.** A dedicated UID is the cheapest lever that closes all of
those at once; a seccomp filter denying the FD-theft calls plus a PID and
mount namespace hiding the supervisor and its sockets is the equivalent
without the UID change. `no_new_privs` is required either way so setuid
binaries cannot restore privilege after the drop, and inherited file
descriptors must be closed explicitly at exec.

This applies per shape. In the microVM shape the AI is behind a VM
boundary and cannot reach the supervisor at all, so the drop is
unnecessary. In the netns shape the AI shares a kernel and a process table
with the supervisor, so it is load bearing. This is the same mitigation
listed first in the threat-honesty note under "Audit stream and
retention", restated here as a requirement rather than an aspiration.

Note also the interaction with rootless container runtimes: `newuidmap`
and `newgidmap` are setuid binaries, and `no_new_privs` makes the kernel
ignore setuid bits. A rootless backend either prepares its user namespace
before the drop or runs with a single-UID mapping.

##### DNS is mandatory, and is a policy chokepoint

Cooperative proxying never needed DNS in the namespace: the client hands
the proxy a name and the proxy resolves it. Transparent interception
inverts that. The client must resolve a name to an address before it can
open the connection the platform intends to intercept, so without a
working resolver there is nothing to intercept.

The relay is therefore required, not optional, and it is a second policy
decision point rather than a forwarder with logging. Names outside the
policy are refused rather than resolved, queries are recorded as an
activity signal that precedes any connection, and malformed or unnecessary
query types are rejected. A relay that forwards by default is an egress
channel in its own right: query names alone carry data out. Recording is
not prevention.

##### Failing closed and preflight

Confinement must not degrade silently. The current supervisor falls back
from namespace mode to advisory proxy-only mode when setup fails, which
converts a structural claim into a cooperative one at runtime. For a shape
that attests `forced`, setup failure is fatal.

Because the mechanism depends on binaries and privileges that a workspace
image may not carry (`ip`, `iptables` and `ip6tables` or `nft`, the ability
to create namespaces and switch UID), the platform preflights them and
reports the shape as unsupported when they are absent, rather than
claiming an enforcement it is not performing.

##### Backend compatibility

The determining question is whether every process that originates egress
is a descendant the platform launched, or whether the backend delegates to
something that already exists outside the namespace.

| Backend                                     | Confinable                      | Why                                                                                                                                                         |
|---------------------------------------------|---------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Plain process, chroot, bubblewrap           | yes                             | Direct descendants                                                                                                                                          |
| microVM with user-mode networking (libkrun) | yes                             | The VMM's host sockets are in the confined namespace; needs only `/dev/kvm` access                                                                          |
| Rootless podman (pasta, slirp4netns)        | yes                             | Daemonless: `conmon` and the runtime are descendants; the userspace network helper holds its sockets in the confined namespace                              |
| Rootful podman (netavark), TAP-based VMMs   | yes, with host-side enforcement | Requires `CAP_NET_ADMIN` inside, which host-side rules tolerate; nested interfaces need matching `PREROUTING` and `FORWARD` coverage                        |
| Docker via a daemon socket                  | **no**                          | `dockerd` creates the container in its own namespace; a unix socket is not isolated by a network namespace, so the request escapes before any packet exists |

Docker through a shared daemon socket cannot be confined by this
mechanism and must be recorded as `advisory` or unsupported rather than
`forced`. The same reasoning applies to any backend reached through a
pre-existing privileged helper, including a sandbox runtime whose own
daemon was started outside the namespace.

##### Implementation status

Landed:

- Enforcement rules are installed on the host side of the veth, scoped to
  that interface, and recorded as exact deletion specs so cleanup removes
  precisely what was added. Child-side rules are redirection only.
- The namespace has a default route through the veth, without which
  `connect()` fails before DNAT can act.
- Namespace addressing is allocated per sandbox from a configurable pool
  (`100.115.92.0/24` carved into `/30`s), skipping prefixes that collide
  with an existing host route or interface address.
- IPv6 is disabled in the namespace where sysctl allows, and `ip6tables`
  DROP policies are applied unconditionally on both sides so v6 is never
  an implicitly uncovered path.
- The DNS relay enforces policy: default deny by name, a query-type
  allowlist that excludes the classic tunneling records, rate limiting,
  and refusal without contacting upstream.
- The proxy accepts origin-form requests, which is the shape DNATed port
  80 traffic arrives in, while keeping absolute-form for explicitly
  proxied clients. TLS connections with no readable SNI now produce a
  deny event instead of closing silently.
- Destinations are resolved once and validated against loopback,
  link-local, private, and metadata ranges before dialing, so an allowed
  hostname cannot rebind the proxy onto the supervisor's own network
  position.
- The privilege-dropping launcher clears supplementary groups,
  capabilities, and the bounding set, sets `no_new_privs`, closes
  inherited descriptors, and switches to a dedicated UID, with a preflight
  that reports why a drop is impossible rather than proceeding.
- Forced shapes fail closed: preflight failure, namespace setup failure,
  listener failure, and an unconfigured namespace all abort instead of
  downgrading to advisory.

Not yet landed:

- `SandboxController` still runs the admin script cooperatively. Wiring it
  to open a namespace and launch through the privilege-dropping helper is
  the remaining step; the primitives it needs are in place.
- DNS decisions are logged but not retained. `ai_sandbox_network_events`
  constrains protocol to `connect`, `http`, `sni`, and `tcp`, so keeping
  them server side needs a schema change.
- Only the datapath rule construction is unit tested. Namespace creation,
  firewall insertion, and packet-level redirection require root and were
  not exercised end to end.

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
fetches, holds, or sees policy; in a `forced` shape its only network path
is the parent proxy. This is what makes live updates safe and preserves
the invariant that nothing inside the workspace can influence its own
disposition.

Who fetches and applies policy, per shape:

- **AI-designated workspace**: the outer workspace-level supervisor (the
  parent process of the main agent) establishes and retains the
  egress-control stream to coderd BEFORE fork-execing the confined main
  agent, and keeps it for revision pushes. The confined agent's normal
  connection is separate and carries no policy. The supervisor reports
  degraded state.
- **Sandboxed child**: the unbound PARENT agent's existing coderd
  connection receives policy revisions; the parent-side sandbox
  controller and proxy apply them. The child's connection carries no
  policy. The parent agent reports degraded state.

#### Audit stream and retention

The parent-side supervisor or sandbox controller, which owns the proxy,
emits network events through agent-authenticated ingestion endpoints
(`POST /workspaceagents/me/ai-sandbox-sessions`,
`PATCH /workspaceagents/me/ai-sandbox-network-events`) into
`ai_sandbox_sessions` and `ai_sandbox_network_events`. Session and event
attribution is resolved server-side from the agent binding; the reporter
cannot assert it, and a reporter may only append events to sessions it
created. The confined child never emits network policy
events; its connection state supplies health and it may produce non-network
activity events only. Events correlate with
aibridge interceptions and become a Vertical 3 input.

Both tables snapshot raw agent and sponsor UUIDs on each retained record.
Those UUID columns are not foreign keys to `ai_agents` or `users`; audit
history must survive identity revocation, workspace deletion, and identity
cleanup. A session-to-event relationship may enforce its own retention-safe
integrity, but deleting an identity must never delete egress audit records.

Threat honesty: a network namespace beside its supervisor in the same
container is a strong fence, not a wall. Mitigations in order are: run the
AI under a UID distinct from the supervisor, so it cannot attach to that
process or open its credentials and host-namespace sockets; drop
`CAP_SYS_ADMIN` and `CAP_SYS_PTRACE`; use VM-class workspaces with an
infrastructure-enforced perimeter such as NetworkPolicy or security groups
for the highest tier; retain the network namespace as defense in depth and
an audit source. The template envelope selects the tier.

`CAP_NET_ADMIN` is deliberately absent from that list. Once enforcement
rules live on the host side of the veth (see "Host-side enforcement"), a
confined process holding it can only reconfigure its own namespace, which
breaks its connectivity rather than widening it. Keeping it available is
what allows sandbox backends that build their own network boundary to run
inside platform confinement.

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
baselines. Templates declare the script contract, the attestation, and the
credential mode. Only coderd-admin template-setting writes, and later
human-in-the-loop approval writes, may change egress rules or overrides;
templates cannot author egress exceptions.

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

1. Add `workspaces.ai_agent_id`, nullable foreign key to
   `ai_agents.user_id`: the sticky designation marker (one-way; set at AI
   creation or first human opt-in build; never cleared) consumed by
   credential starvation, workspace-agent binding, and the RBAC
   designation boundary. Add `workspace_agents.ai_agent_id`, nullable
   foreign key to `ai_agents.user_id`, copied from the workspace marker at
   build completion, and `workspace_agents.ai_credential_mode`, enum
   `none | injected | brokered` with default `none`.
2. Add sandbox instance records keyed to workspace and parent agent, with
   parent and child agent IDs, sandbox ID, create/destroy script version
   or digest, the `egress_enforcement` attestation, credential mode, the
   effective policy revision reference, health, reconciliation, and
   lifecycle state. Sandbox records never contain Terraform-derived egress
   rules; they only reference the effective policy revision.
3. Add server-side storage for the versioned template-settings egress
   policy and its bounded per-workspace or per-sandbox override revisions,
   including writer identity and audit linkage. This is the authoritative
   policy model; nothing about it is Terraform-derived.
4. Add `ai_sandbox_sessions` and `ai_sandbox_network_events`. Snapshot raw
   agent and sponsor UUIDs without foreign keys to `ai_agents` or `users` so
   audit history survives identity cleanup.
5. Add no auto-ACL machinery and no sponsor quota aggregation. Human
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
   shape attests `forced` routing. `advisory` and `none` shapes record
   their weaker coverage instead of claiming confinement.
7. **Structural confinement (`forced` shapes only)**: in an AI-designated
   workspace confined by the netns supervisor, and in any sandbox attesting
   `forced`, every process spawned through the confined agent, including
   SSH, terminal, and scripts, observes the same network policy. Shapes
   attesting `advisory` or `none` make no such guarantee and must be
   recorded and surfaced as weaker.
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
11. **Designation is a one-way ratchet**: no code path clears
    `workspaces.ai_agent_id`, and the token decision treats the stored
    marker as dominant over the current build's parameters. The
    authorization boundary built on the marker is Vertical 1's
    designation boundary invariant.

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

   *Implementation notes (landed).* The pieces below are on the branch;
   deviations and remaining gaps are listed afterward.
   - **Policy object**: `template_ai_egress_policies` (migration 000568),
     an insert-only per-template revision history (revision 0 with no
     rules is the implicit deny-all default; `created_by` is a plain UUID
     so attribution survives user cleanup). Admin API:
     `GET/PUT /api/v2/templates/{template}/ai-egress-policy` (template
     `ActionRead`/`ActionUpdate`, audited with old/new rules and
     revisions in `AdditionalFields` because child-row changes produce no
     template diff). Writes publish on pubsub channel
     `template-ai-egress-policy:<template_id>`.
   - **Delivery**: agent-authenticated
     `GET /api/v2/workspaceagents/me/ai-egress-policy` (bootstrap) and
     `.../watch` (SSE: subscribe first, send current, then full
     replacement policy per revision). Reads are materialized: coderd
     appends implicit control-plane allow rules (access URL host and
     effective port) to the admin rules.
   - **Supervisor** (`agent/confine`, wired via `coder agent
     --confine=netns|proxy`, env `CODER_AGENT_CONFINE`): fetches policy
     before fork (fetch failure means deny-all plus degraded, never
     unconfined), runs the CONNECT/absolute-form HTTP proxy and, in netns
     mode, an SNI passthrough listener; applies watch revisions
     atomically; keeps the last policy on stream failure (never widens).
     Matcher: exact host or single leading-label wildcard, empty ports
     imply 80/443, case- and trailing-dot-insensitive, coderd host
     implicitly allowed. The child env gets
     `CODER_AGENT_EGRESS_PROXY_URL` plus, in forced mode, HTTP(S)_PROXY
     and NO_PROXY, and the agent propagates proxy variables to every
     spawned process.
   - **netns reference**: namespace `coder-confine-<8hex>`, veth /30
     (100.115.92.0/30), no default route inside, `/etc/netns` resolv.conf
     pointing at the host veth; created with the external `ip` binary;
     any setup failure cleans up and falls back to proxy-only advisory
     mode with a degraded signal.
   - **Audit stream**: `ai_sandbox_sessions` and
     `ai_sandbox_network_events` (migration 000569) with raw-UUID
     snapshots and no FKs, ingestion endpoints as described under "Audit
     stream and retention", and dbpurge retention (events by age,
     sessions only when ended and event-free).
   - **Read surface and UI (follow-up)**: workspace-scoped reads
     `GET /api/v2/workspaces/{workspace}/ai-sandbox-sessions` and
     `.../{session}/network-events` (keyset paginated on the event row
     ID, which is returned so clients can page), both authorized against
     the workspace. `ai_agent_id` is exposed on the workspace and
     workspace-agent API objects, annotated in `coder show`, and
     surfaced in the UI as an AI badge on the workspace topbar, an
     AI-bound badge on the agent row, and an egress activity section
     listing sessions with their attestation and each allowed or denied
     destination.
   - **Sandbox lifecycle (landed)**: `ai_sandboxes` (migration 000570)
     records a sandbox created by a parent agent from an admin
     declaration, with the declaration name unique per parent while
     live. Agent-authenticated `POST/GET/DELETE
     /api/v2/workspaceagents/me/ai-sandboxes` create, list, and destroy
     sandboxes. Create resolves the AI identity server-side (the
     parent's own identity when the parent is AI-bound, otherwise the
     workspace-origin identity through the now-exported
     `aiagentidentity.ResolveWorkspaceOrigin`), inserts and binds the
     child agent row, and mints the child agent token plus a
     sandbox-scoped session token (`ai-sb-<sandbox id>`) in one
     transaction. Re-posting the same name reconciles to the existing
     sandbox and rotates the session token, because its plaintext cannot
     be recovered. Delete soft-deletes the child row and revokes the
     scoped token, so the child stops authenticating even if its process
     survives. Sandbox children are excluded from `ListSubAgents`: they
     have their own lifecycle owner, and the devcontainer reconciler
     deletes any listed child it does not track, which would reap a live
     sandbox on every agent restart.
   - **Sandbox controller (landed)**: `agent/confine.SandboxController`
     completes the script contract. The interim declaration surface,
     until the Terraform resource exists, is agent process environment:
     `CODER_AI_SANDBOX_CREATE_SCRIPT` (required to enable),
     `_DESTROY_SCRIPT` (optional), `_NAME` (reconciliation key, default
     `sandbox`), `_EGRESS_ENFORCEMENT` (default `none`: the attestation
     defaults to claiming nothing, and an invalid value refuses startup
     rather than degrading the claim), and `_PROXY_ADDRESS` (default
     parent loopback; isolation technologies that cannot reach parent
     loopback, such as containers on a bridge network, must declare a
     reachable address). The controller deletes stale same-parent
     records under other names, creates or reconciles the sandbox,
     starts the deny-all-bootstrapped proxy and policy watch BEFORE the
     create script runs so the child never has a proxyless window,
     posts the session with `ChildAgentID`, and executes the script
     with the platform variables appended last and deduplicated so a
     template environment cannot override `CODER_AI_AGENT_TOKEN` and
     friends. `CODER_EGRESS_PROXY` is a bare host:port, matching the
     example script that prepends the scheme. Create-script failure is
     degraded, not fatal: the proxy and session stay up and health
     remains the child's connection state. Teardown runs the destroy
     script (without the possibly rotated session token), closes the
     session with `EndedAt`, flushes events, and closes the proxy.
     Covered by unit tests against a fake coderd and a Linux
     integration test driving a real agent process end to end.
   - **Deviations and open gaps**: relay-only
     tailnet (`BlockDirect`) for confined agents was evaluated and
     deliberately skipped: in a `forced` netns shape the namespace has no
     route for direct paths, so the flag would only suppress attempts
     that already cannot succeed, and tailnet peers are coordinated
     through coderd rather than being an open exfiltration channel. In
     `advisory` shapes it would add no guarantee the attestation does not
     already disclaim. There is no DNS relay in the netns yet, so direct
     DNS inside the namespace times out (proxied traffic resolves at the
     proxy). Degraded state is surfaced through the supervisor init log
     and a best-effort external agent log entry; a first-class degraded
     agent health field does not exist and reusing `start_error` would
     mislead the UI. Confinement runs for any agent whose command opts
     in; server-side enforcement that an AI-designated workspace MUST
     run confined is future admission-control work.
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

The RBAC designation boundary that consumes this vertical's designation
marker is specified and scheduled in Vertical 1 (its step 10); it depends
only on phase 3's designation flow here.

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
# EXISTS TODAY: parameter detection, workspace-origin identity mint, and
# the scoped-token provisioner export (V1).
# SERVER-SIDE V2: per-agent binding, owner-token suppression, and
# confinement are added on top of the same signal.
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
