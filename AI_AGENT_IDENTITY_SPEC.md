# Agent Identities

Status: design specification. Nothing described here should be assumed to
exist. Every section is written as work to be done, in normative voice, so
that an implementer can build it from this document alone. Where the
document refers to behavior that Coder already has independently of this
feature, it says so explicitly.

## Problem

AI agents acting through Coder today are indistinguishable from the humans
they act for:

- Coder Agents (`chatd`) execute platform actions with the exact same
  permissions as the chat owner, as documented in
  `docs/ai-coder/agents/architecture.md`.
- Claude Code and similar agents are hosted in environments identical to a
  human-designated workspace, granting them access to all of the owner's
  credentials and secrets.
- Audit logs collapse everything to `audit_logs.user_id`. There is no way
  to ask "what did the agent do" as distinct from "what did the human do".

Two properties of the solution are worth stating up front, because they are
the objections this design attracts:

- **It adds no new privilege surface.** An agent's permissions are always a
  subset of its human sponsor's. There is no configuration in which an
  agent can do something its sponsor could not.
- **The AI-mode marker on a workspace is one-way.** It can be set but never
  cleared. The reasoning is in "The designation ratchet" below; the short
  version is that un-designating would re-expose ambient credentials to an
  environment an agent has already influenced.

## Solution

Introduce Agent Identities as a first-class concept. An agent identity has
the following properties.

1. **First-class citizens of the `users` table.** An agent is a real user
   row, not a synthetic subject or a bare token, so it flows through
   audit, RBAC, and API key machinery without special cases.
2. **Every agent maps to exactly one human user.** There are no orphan
   agents. The human is the accountable party.
3. **One agent identity per chat or per workspace.** If a chat creates a
   workspace, that identity propagates to the workspace and to its
   AI-bound workspace agents rather than a new identity being created.
4. **Agents are non-interactive.** They cannot log in. This must be
   enforced by database constraint, not by application convention.
5. **Agent permissions are the intersection of their human principal's
   permissions and their own granted API key scopes.**
   1. An agent can never hold a privilege its human sponsor lacks, and
      loses a privilege as soon as the sponsor does.
   2. Key scopes are minted from **profiles**. The scopes granted to a
      chat differ from those granted to an AI-bound workspace agent.
      1. AI subjects hold workspace scopes including `ssh`, `start`,
         `stop`, and `update`, but every workspace action other than
         `read` and `create` resolves successfully only against
         workspaces attributed to that agent.
6. **An agent's designated workspace never receives the sponsor's ambient
   credentials.** No owner session token, no user secrets, no external
   auth tokens, no Git SSH key.
7. **Every audited action records both identities.** `audit_logs.user_id`
   is the agent that acted; `audit_logs.on_behalf_of_user_id` is the human
   accountable for it. Querying by the human returns both their own
   actions and their agents'.

## Identity generation

### Semantics: mint at a delegation boundary

An agent identity is created at a **delegation boundary**: the point where
a human's intent first becomes autonomous execution. The test to apply is
"would a human have to newly consent here?" Creating a chat is a new
delegation. A chat agent then creating a workspace is not, because consent
was already given upstream.

This yields one rule with two halves:

> Mint only at a human-to-AI boundary. Everything downstream of a boundary
> reuses the identity that crossed it.

Exactly two identity boundaries exist, and they correspond to the two
values of the origin enum:

| Boundary                              | `origin_type` | `origin_id`  | Sponsor             |
|---------------------------------------|---------------|--------------|---------------------|
| A human creates a chat                | `chat`        | chat ID      | the chat owner      |
| A human opts a workspace into AI mode | `workspace`   | workspace ID | the workspace owner |

Alternatives considered and rejected:

- **Per action or per session.** Identity sprawl makes audit unqueryable:
  "what did this agent do" becomes a join across thousands of principals,
  and revocation has no single handle.
- **One identity per human.** Loses the ability to distinguish concurrent
  agents, and revoking one agent revokes all of them.
- **Let agents mint their own.** Any agent that can create an identity can
  create one with a different sponsor or a wider profile, which is a
  privilege-laundering path. Agents must never mint identities.

### Credential boundaries are separate, and more numerous

One identity may hold several API keys, one per **credential** boundary.
This is what allows a sandbox's credential to be rotated and revoked
independently of its enclosing workspace's while still attributing to the
same principal.

| Credential            | Profile                                          | Named for     | Lifecycle                                                |
|-----------------------|--------------------------------------------------|---------------|----------------------------------------------------------|
| Chat key              | `ChatAgentProfile(chatID)`                       | the chat      | renewed while the chat lives                             |
| Workspace session key | `WorkspaceAgentIdentityProfile(workspaceID)`     | the workspace | rotated on every start build; deleted on stop and delete |
| Sandbox session key   | `SandboxIdentityProfile(workspaceID, sandboxID)` | the sandbox   | rotated on every start build; deleted on stop and delete |

A sandbox therefore does **not** receive its own identity. It resolves the
enclosing workspace's identity, creating the workspace-origin identity on
first use if the workspace has none, and mints a sandbox-named key under
it. Two sandboxes in one workspace share one identity and differ only by
key.

Sandboxes are declared in the template and created by the build, never
created at runtime, so a sandbox key's lifecycle matches the workspace
key's: minted during a start build, rotated on each subsequent start,
deleted on stop and delete. The key is named per declared sandbox so that
revoking one sandbox's credential does not disturb the enclosing
workspace's, which is the only reason the two are separate keys rather than
one.

### Package ownership

A single package owns identity creation and credential minting. Suggested
home: `coderd/aiagentidentity`.

```text
coderd/aiagentidentity
  Create(ctx, db, CreateParams{OwnerID, OrganizationID, OriginType, OriginID})
      -> (database.User, database.AIAgent, error)
  MintKey(ctx, db, agentUserID, profile) -> (database.APIKey, plaintext, error)
  Resolve(ctx, db, agentUserID)          -> (ResolvedIdentity, error)
  ResolveWorkspaceOrigin(ctx, db, workspace) -> (database.AIAgent, error)
  ChatAgentProfile / WorkspaceAgentIdentityProfile / SandboxIdentityProfile
  AIAgentActor, WithActor, ActorFromContext
```

**No other package may insert into `ai_agents` or construct an agent API
key.** `Create` must insert the `users` row and the `ai_agents` row in one
transaction, doing all work on the transaction handle, and `MintKey` must
apply profile validation. Both guarantees are lost the moment a caller
bypasses the package, so enforce the constraint with a test asserting that
no package outside the identity package references the underlying insert
queries.

Two callers, and no others:

| Caller                     | Boundary it handles                                                                                         | When it runs                             |
|----------------------------|-------------------------------------------------------------------------------------------------------------|------------------------------------------|
| The chat surface (`chatd`) | chat creation; chat key renewal                                                                             | at the API request that creates the chat |
| The provisioner server     | workspace opt-in detection; designation; agent binding; per-build key rotation for workspaces and sandboxes | during build transitions only            |

The split is not an assignment of responsibilities; it follows from one
rule: **each mint runs in whatever component is alive when its boundary
event occurs.** Chat creation is an API request, so it belongs to the chat
surface. Everything workspace-shaped happens during a build, so it belongs
to the provisioner server.

Sandboxes are declared in the template, so a sandbox is created by the
build that creates its enclosing workspace agents, and its identity
resolution, binding, and key mint belong to the provisioner caller exactly
as the workspace opt-in path does. There is deliberately no runtime
sandbox-creation API and no third caller.

Separate the two things the word "create" covers here, because only the
first is an identity concern:

| Step                                                              | When             | By what                                     |
|-------------------------------------------------------------------|------------------|---------------------------------------------|
| The sandbox's workspace-agent row exists, is bound, and has a key | during the build | the provisioner server                      |
| The sandbox process, container, or VM is brought up               | at agent startup | the host agent, running the declared script |

The host agent therefore never mints a credential. It consumes one that the
build already minted for a row that already exists, which is the same
pattern Terraform-declared devcontainer children follow: the agent updates
configurable fields on a pre-provisioned row rather than creating one. This
is what removes the runtime creation path without removing the host agent's
role in actually standing the sandbox up.

This is a narrowing of an earlier design that allowed sandboxes to be
created dynamically by the running parent agent. Declaring them upfront
costs the ability to add a sandbox to a running workspace without a
rebuild, and buys three things that matter more:

1. **One creation path for agent rows.** Credential starvation is enforced
   per workspace-agent row, so every path that creates such a row must set
   the binding column. Removing the runtime path removes an entire class of
   bug in which a new creation path forgets to bind and silently produces
   an unstarved agent inside a designated workspace.
2. **Provisioner-attested declarations.** A sandbox's declared properties,
   including any egress-enforcement attestation, arrive as build output
   rather than as input from a process running inside the workspace. Build
   output is server-trusted; in-workspace input is not.
3. **One mint cadence.** Sandbox and workspace keys rotate and are revoked
   on the same build transitions, so there is no separate reconcile path
   that can mint a replacement credential after a delete has already
   removed the old one.

### Conditions: when each path fires

**Chat creation.** When a chat is created with identity creation
requested, `Create` an identity with `origin_type = 'chat'`, then
`MintKey` with the chat profile. The chat row, the identity, and the key
must commit or roll back together. A chat whose identity failed to create
must never be observable, so any state notifications must be buffered
until the transaction commits.

**Workspace opt-in by a human.** During a start build, if the workspace
carries no designation marker and the build declares AI opt-in, resolve or
create the workspace-origin identity, record the designation marker on the
workspace, and mint the workspace key. The declaration surface is a
template concern; the server must treat the stored marker as dominant over
it.

**Workspace created by an agent.** Reuse the requesting agent's identity.
Do not create a workspace-origin identity, and do not consult the
template's declaration. The designation must be written at the single
shared workspace-creation chokepoint, before the first build is created,
so that build can bind its agents. Writing it in individual handlers is
incorrect: a handler that forgets leaves an undesignated workspace, which
then receives the sponsor's ambient credentials.

**Declared sandbox in a build.** For each sandbox the template declares,
resolve the enclosing workspace's identity, mint a sandbox-named key, and
bind the sandbox's workspace agent to that identity. In an ordinary
undesignated workspace this is the first use of the workspace-origin
identity and therefore creates it, which is the case that makes
first-use resolution idempotency (below) load-bearing: several declared
sandboxes in one workspace resolve the same identity concurrently within
one build.

Bind at agent insert rather than by a follow-up update. The build creates
the sandbox's agent row, so the binding is known at insert time, and
deferring it to a second statement is what allows a creation path to omit
it.

**Rebuild.** Reuse the identity. Only keys rotate. A rebuild that adds or
removes declared sandboxes mints or revokes their keys accordingly; the
enclosing workspace's identity is unaffected.

Paths that must **not** mint an identity:

| Situation                          | Correct behavior                                     |
|------------------------------------|------------------------------------------------------|
| An agent creates a workspace       | reuse the requester's identity                       |
| A build creates a declared sandbox | reuse the enclosing workspace's identity; mint a key |
| An agent creates a sub-agent       | inherit the parent agent's binding                   |
| A workspace is rebuilt             | reuse; rotate keys only                              |
| An agent makes any API call        | nothing; the key already exists                      |
| A chat key expired                 | re-mint under the same identity and token name       |

### Required properties of the resolution logic

Four failure modes that an implementation will hit. All must be handled
explicitly.

1. **Origin uniqueness is the concurrency control.** The partial unique
   index on `(origin_type, origin_id) WHERE NOT deleted` is what makes
   "one identity per boundary" true under concurrency. First-use
   resolution must therefore treat a unique violation on insert as a
   signal to re-read and reuse, not as an error to propagate. Two
   concurrent first-use callers must both succeed with the same identity.
   If one receives an error, resolution is not idempotent and that
   caller's build or sandbox fails for no user-visible reason.
2. **A mint must not outlive a revocation.** A mint that resolved a live
   identity can otherwise insert a key after that identity is revoked. The
   liveness check and the key insert must share one transaction.
3. **Re-sponsoring on ownership change.** A workspace-origin identity
   records its sponsor. If workspace ownership transfers, the stored
   `owner_user_id` no longer matches the workspace owner, and continuing
   would delegate the previous owner's permissions to an agent running in
   the new owner's workspace. Resolution must compare the two and
   re-sponsor, or refuse, rather than proceeding.
4. **Deletion must revoke.** Workspace deletion must revoke that
   workspace's keys, and revocation must resolve the identity from the
   **designation marker** first rather than from the origin. A
   chat-created workspace's key belongs to a chat identity that has no
   workspace-origin row, so resolving by origin alone leaves that key live
   until expiry.

### The designation ratchet

The designation marker on a workspace names the single agent identity the
workspace belongs to. It has three consumers: credential starvation at
build time, workspace-agent binding at build completion, and the RBAC
designation boundary at request time.

It must be one-way. No code path may clear it, and the stored marker must
dominate the current build's declaration. The reason is an escalation
path: an agent legitimately holds `start` on its own designated workspace,
and a start build can select a different template version, so if
designation were re-derived per build from the template declaration, an
agent could rebuild its own workspace with a version that omits the
declaration and the next build would mint a full-owner session token into
an environment the agent controls.

Recovery from a mistaken opt-in is workspace recreation, not
un-designation. This asymmetry is deliberate: a workspace that has hosted
an agent may carry agent-influenced state that must not subsequently
receive full credentials, while recreation is cheap.

## Schema changes

### `users`

- New column `kind`, enum `human | ai_agent`, default `human`.
- Rows with `kind = 'ai_agent'` must also have `login_type = 'none'` and
  an empty email, enforced by CHECK constraint rather than by application
  code, so non-interactivity is structural.

```sql
CREATE TYPE user_kind AS ENUM ('human', 'ai_agent');

ALTER TABLE users
    ADD COLUMN kind user_kind NOT NULL DEFAULT 'human';

-- Fold kind into whatever constraints already restrict non-human users.
-- AI agents must never hold an interactive login method.
ALTER TABLE users
    ADD CONSTRAINT users_ai_agent_login_type CHECK (
        kind != 'ai_agent' OR login_type = 'none'
    );
```

### `ai_agents`

```sql
CREATE TYPE ai_agent_origin AS ENUM ('chat', 'workspace');

CREATE TABLE ai_agents (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    origin_type ai_agent_origin NOT NULL,
    origin_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted boolean NOT NULL DEFAULT false
);

CREATE INDEX idx_ai_agents_owner ON ai_agents (owner_user_id);

-- One live identity per delegation boundary. This index is also the
-- concurrency control for first-use resolution.
CREATE UNIQUE INDEX idx_ai_agents_origin ON ai_agents (origin_type, origin_id)
    WHERE NOT deleted;
```

Two refinements to consider, both raised by review of an earlier draft:

**Typed composite foreign key.** `REFERENCES users(id)` permits an
`ai_agents` row whose user has `kind = 'human'`. That state fails open,
because authentication discriminates on `users.kind`: such a key would
skip delegation and build a direct human subject. Making the state
unrepresentable requires a composite reference, which Postgres allows only
against a unique constraint:

```sql
ALTER TABLE users ADD CONSTRAINT users_id_kind_key UNIQUE (id, kind);

-- Then, in ai_agents:
--   kind user_kind NOT NULL GENERATED ALWAYS AS ('ai_agent') STORED,
--   FOREIGN KEY (user_id, kind) REFERENCES users (id, kind) ON DELETE CASCADE
```

Note that `REFERENCES users(id, kind)` cannot be written as a column
constraint without both the unique constraint above and a `kind` column on
`ai_agents`.

**Revocation state.** A boolean `deleted` is reversible and records
neither when nor why. A `state` enum of `active | revoked` plus a
`revoked_at` and reason is preferable if attribution history matters,
which it does for audit.

### `audit_logs`

```sql
ALTER TABLE audit_logs ADD COLUMN on_behalf_of_user_id uuid;

CREATE INDEX idx_audit_logs_on_behalf_of_user_id
    ON audit_logs (on_behalf_of_user_id);
```

Deliberately not a foreign key, so audit history survives identity
cleanup. Note that if `ai_agents.owner_user_id` cascades on user deletion,
survival of the agent metadata itself is conventional rather than
structural.

### `workspaces` and `workspace_agents`

These two columns are what the permission model reads. Without them the
Permissions section below is unimplementable.

```sql
-- The designation marker: the single AI identity this workspace belongs
-- to. Sticky and one-way. Consumed by credential starvation, agent
-- binding, and the RBAC designation boundary.
ALTER TABLE workspaces
    ADD COLUMN ai_agent_id uuid REFERENCES ai_agents (user_id);

CREATE INDEX workspaces_ai_agent_id_idx ON workspaces (ai_agent_id);

-- Per-build copy, written from the workspace marker at build completion.
-- Credential starvation is enforced per agent row, so every path that
-- creates a workspace agent must set this.
ALTER TABLE workspace_agents
    ADD COLUMN ai_agent_id uuid REFERENCES ai_agents (user_id);

CREATE INDEX workspace_agents_ai_agent_id_idx
    ON workspace_agents (ai_agent_id);
```

Prefer making `ai_agent_id` a parameter of the workspace-agent insert
rather than a follow-up `UPDATE`. If binding is only ever a post-insert
update, every current and future agent-creation path must remember to
perform it, and one that forgets produces an unbound agent inside a
designated workspace, which silently defeats credential starvation.
Setting it at insert makes that class of bug unrepresentable.

## Permissions

### RBAC

Three additions to `coderd/rbac`:

- **`SubjectTypeAIAgent = "ai_agent"`** alongside the existing subject
  types. If `Subject.Type` is currently documented as non-functional and
  used only for logging, that changes: it becomes a policy input.
- **`Subject.AIAgentID`** carries the acting AI identity. `Subject.ID`
  must remain the sponsoring human, because RBAC owner checks compare
  `resource.owner_id` to `subject.ID`. Provide a single helper that sets
  type and acting identity together and invalidates any cached policy
  input, since a stale cached subject would authorize as the
  pre-decoration subject. Keep the field exported if the authorization
  cache key is derived from a serialization of the subject: an unexported
  field is omitted from the hash, which would let two different agents
  share a cached decision.
- **`Object.AIAgentID`** carries a workspace's designation. Populate it
  only in the database-to-RBAC object converters, never in handlers, and
  route every workspace-typed object through one helper so that a newly
  added converter cannot silently omit it. An "all objects of this type"
  aggregate object must clear the field: it covers every workspace and
  cannot claim one designation, so designation-protected authorizations
  against it must fail closed.

Effective permission is the intersection of four independent things, of
which only the last is new:

```text
sponsor's live roles  ∩  key scopes  ∩  scope allow list  ∩  designation boundary
```

Roles must be resolved live, per request. A permission snapshot taken at
mint time would let an agent outlive its sponsor's access.

**Profiles.** Scopes are chosen for the boundary being crossed and are
never inherited from the acting credential:

| Profile                                          | Scopes                                                                                                                      | Allow list                                                                        |
|--------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------|
| `ChatAgentProfile(chatID)`                       | `coder:workspaces.create`, `coder:workspaces.operate`, `coder:workspaces.access`, `chat:read`, `chat:update`, `user:read`   | exact chat ID; typed wildcards for workspace, template, organization member, user |
| `WorkspaceAgentIdentityProfile(workspaceID)`     | `workspace:read`, `workspace:update`, `workspace:start`, `workspace:stop`, `workspace:ssh`, `workspace:application_connect` | that one workspace ID, no wildcard                                                |
| `SandboxIdentityProfile(workspaceID, sandboxID)` | as the workspace profile                                                                                                    | as the workspace profile, with a distinct token name                              |

The workspace profile is the chat profile's workspace actions minus
`create`, pinned to one ID instead of a wildcard, with every non-workspace
resource dropped. A workspace token therefore cannot create workspaces, so
it cannot manufacture new designated environments.

The chat profile's workspace wildcard is unavoidable. One
action-independent allow list applies to the union of all selected scopes,
it is fixed at mint time, and `create` must be authorized against an
object that has no ID yet. The designation boundary is what makes that
wildcard safe, and it is the reason the boundary cannot be expressed as
scope data.

**Profile validation must be an allowlist.** Enumerate the permitted
scopes, derived from the profiles above, and reject everything else. A
denylist drifts: a catalog of a few hundred scopes checked against a
handful of named prohibitions accepts almost everything by default.
Composite scopes must be expanded and checked permission by permission,
not matched by name prefix. Reject a global `*:*` allow-list entry
outright. Hard exclusions to check regardless of the allowlist: any
all-permissions scope, self key management, template authoring, API key
resources, user secrets, and personal user reads.

**Minting must be the only path to an agent credential.** Generic
key-creation routes that accept a target user must refuse targets whose
`kind` is `ai_agent`, mirroring any guard that already exists for system
users. Otherwise a privileged caller can create a default full-scope key
for an agent user, and because the delegated subject is the sponsor and
organization members hold key actions on their own keys, that key can then
mint an ordinary human token with no `ai_agents` linkage to revoke. Making
this a property of the identity rather than of the mint path additionally
requires rejecting, at authentication, any agent-owned key whose scope and
allow-list shape matches no known profile.

### Rego

One new conjunct on the single existing `allow` rule, plus the rules it
depends on. There is no second authorization layer and no middleware veto:
a request that was previously allowed can now also be denied by the same
policy evaluation.

```rego
allow if {
    permission_allow
    scope_allow
    ai_workspace_designation_allow
}

# Read the acting identity through a defaulted rule, never directly. In
# rego, `not input.subject.missing_field = ""` evaluates to true, so
# reading the field raw would classify a subject whose field is absent as
# an AI agent and deny it every protected workspace action. Defaulting to
# the empty string makes an absent field mean "not an AI agent", which
# fails in the safe direction.
default acting_ai_agent_id := ""

acting_ai_agent_id := input.subject.ai_agent_id

# Either marker is sufficient. Checking both means a half-populated
# subject fails closed instead of being treated as a human.
subject_is_ai_agent if {
    input.subject.type = "ai_agent"
}

subject_is_ai_agent if {
    acting_ai_agent_id != ""
}

# All three types address workspace rows.
is_workspace_object if {
    input.object.type in {"workspace", "workspace_dormant", "prebuilt_workspace"}
}

# Read supports workspace inventory for a chat. Create must be authorized
# before the workspace has an ID to designate, and is covered instead by
# the server designating every AI-created workspace before its first
# build.
ai_designation_exempt_action if {
    input.action in {"read", "create"}
}

# Defining the exempt actions rather than the protected ones means any
# workspace action added later is protected by default.
ai_workspace_action_requires_designation if {
    is_workspace_object
    not ai_designation_exempt_action
}

# Human and system subjects never evaluate designation.
ai_workspace_designation_allow if {
    not subject_is_ai_agent
}

# AI subjects may perform exempt actions, and are unaffected on every
# non-workspace resource.
ai_workspace_designation_allow if {
    subject_is_ai_agent
    not ai_workspace_action_requires_designation
}

# Protected actions require a populated acting identity and an exact
# match. An undesignated workspace carries the empty string and never
# matches; a workspace designated to another agent never matches.
# Unification is used on the object side because the object may be a
# partial value during partial evaluation.
ai_workspace_designation_allow if {
    subject_is_ai_agent
    acting_ai_agent_id != ""
    input.object.ai_agent_id = acting_ai_agent_id
}
```

Resulting behavior:

| Subject                       | Object                       | Action                      | Result    |
|-------------------------------|------------------------------|-----------------------------|-----------|
| Human or system               | any workspace                | any                         | unchanged |
| AI                            | any workspace                | `read`                      | allow     |
| AI                            | workspace with no ID yet     | `create`                    | allow     |
| AI                            | its own designated workspace | any                         | allow     |
| AI                            | undesignated workspace       | anything but read or create | deny      |
| AI                            | another agent's workspace    | anything but read or create | deny      |
| AI with empty acting identity | any workspace                | anything but read or create | deny      |
| AI                            | aggregate workspace object   | anything but read or create | deny      |
| AI                            | any non-workspace resource   | any                         | unchanged |

**Partial evaluation.** If the policy is also compiled to SQL for list
filtering, the new object field must be declared unknown and given a SQL
converter, mapping NULL to the empty string so it can never unify with a
populated acting identity. An unmapped unknown will be rejected by the
compiler rather than silently ignored. Because the action and object type
are known at partial-evaluation time, the `read` exemption resolves during
compilation and workspace list filtering emits no designation predicate;
assert that with a test comparing an AI actor's compiled list filter to a
human's.

### Authentication

`users.kind` must be the runtime discriminator, not the presence of an
`ai_agents` row. On a key whose user has `kind = 'ai_agent'`,
authentication must:

1. Resolve the `ai_agents` row. A missing or revoked row is an
   authentication error, never a fallback to human behavior.
2. Liveness-check the sponsor: exists, not deleted, active. This is the
   enforcement point for cascade suspend. A background suspend-cascade job
   is optional hardening; the auth-time check is the invariant.
3. Build the subject from the sponsor's ID with the key's own scope set,
   then decorate it with the acting agent identity.
4. Store the resolved actor on the request context for audit.

The same sequence applies at every other site that constructs an agent
subject, including workspace-agent middleware for a bound agent and
in-process tool execution inside the chat surface. A construction site that
skips the decoration silently disables the designation boundary on that
path, so enumerate the sites and test each one.

### Audit

Request-scoped audit already derives the actor from the API key's user,
which yields actor equals agent without additional work. Add: when an AI
actor is present on the request context, set `on_behalf_of_user_id` to the
sponsor. Plumb the field through the audit request struct, the exporter
backends, and the audit query API, and add a search filter so an operator
can ask for a human's delegated activity.

Background and provisioner-originated events carry no HTTP request
context and must be handled separately, or agent-caused background effects
will attribute to nobody.

## Invariants

Drive tests from these.

1. **Ceiling.** Agent permissions are a subset of the sponsor's at all
   times. A role removed from the sponsor is gone on the agent's next
   request, which requires live role resolution rather than a snapshot.
2. **Cascade suspend.** Agent authentication fails when the sponsor is
   suspended or deleted, or the identity is revoked. This holds per
   connection, not per message: a long-lived streaming session retains the
   subject it was built with, so a sponsor suspended mid-session keeps
   access until that session ends. Decide deliberately whether that is
   acceptable, because terminating live sessions has blast radius beyond
   AI agents.
3. **Attribution.** Every audited agent action records actor equals agent
   and on-behalf-of equals sponsor. Querying by sponsor returns both their
   own and their agents' actions.
4. **Non-interactive.** Agent users cannot authenticate interactively,
   enforced by database constraint. Note that mutation paths may still
   accept agent user IDs, so an administrator can assign roles or group
   membership to one. Such assignments do not affect authorization, which
   always uses the sponsor's roles, but they are representable and should
   be filtered from listing surfaces.
5. **No self-escalation.** Agent credentials can be created only through
   profile-validated minting.
6. **Fail-closed resolution.** `kind = 'ai_agent'` without a live
   `ai_agents` row is an authentication error on every path, including
   in-process surfaces. A surface that treats a missing row as "this
   record predates identities" and falls back to the sponsor's full
   subject violates this. Distinguishing a legacy record from a corrupted
   one requires a durable per-record identity reference rather than an
   inference.
7. **Designation boundary.** An AI subject is denied every workspace
   action except read and create unless the workspace's designation
   exactly equals its acting identity. Undesignated workspaces, empty
   acting identities, aggregate objects, and workspaces designated to a
   different agent all deny. Non-AI subjects never evaluate the rule.
8. **Monotonic narrowing.** Any credential minted as a side effect of an
   agent-initiated action carries a profile whose expanded permissions and
   allow-list reach are a subset of the acting profile's. Test it
   mechanically by expanding both through the scope catalog and asserting
   containment. Without that test, adding one scope to the workspace
   profile silently turns workspace creation into a privilege-gain
   primitive.
9. **Non-materialization.** A chat identity's credential never exists
   outside its mint transaction. The identity is used by reference only: a
   subject for authorization and a key ID for attribution. Enforce
   structurally with a mint variant that returns no plaintext, because
   possession is the single precondition for relocating a chat credential
   into an environment the agent controls.
10. **One-way designation.** No code path clears a workspace's designation
    marker, and the stored marker dominates the current build's
    declaration.

## Implementation order

1. **Migrations.** `user_kind` and `users.kind` with the login-type
   constraint; `ai_agents` with the origin partial unique index;
   `audit_logs.on_behalf_of_user_id`; `workspaces.ai_agent_id`;
   `workspace_agents.ai_agent_id`. Regenerate models, update the audit
   table map, regenerate again.
2. **RBAC primitives.** Subject type, subject acting identity, object
   designation, profile constructors, allowlist validator.
3. **Identity package.** `Create`, `MintKey`, `Resolve`, and
   workspace-origin resolution with the idempotency and re-sponsoring
   requirements above.
4. **Authentication.** Agent-aware subject construction plus context
   actor, at every construction site.
5. **Audit plumbing.** Populate on-behalf-of on request and background
   paths; add the query filter.
6. **Designation boundary.** The rego conjunct, subject and object policy
   input, partial-evaluation unknowns and the SQL converter, converter
   population, and subject decoration everywhere.
7. **Guardrails.** Refuse agent targets on generic key routes; filter
   agent users out of user, organization member, group, and role listings.
8. **Surfaces.** Chat (mint at creation, execute under the agent subject,
   renew keys); provisioner (detect opt-in, designate, bind every agent
   including declared sandboxes' agents, suppress the owner session token,
   mint and rotate workspace and sandbox keys per build); AI gateway
   (sponsor liveness, plus a separate budget principal so agent spend
   counts against the sponsor rather than the credential-poor agent).

Steps 1 to 5 are the foundation and are independently mergeable. Step 6
needs only the designation column from step 1. Step 8 is parallelizable
per surface once step 4 lands.

## Known gaps and decisions still open

- **Sub-agent binding propagation.** Declaring sandboxes upfront removes
  one runtime agent-creation path but not all of them: a runtime sub-agent
  API remains for devcontainer autodetection, and it is authenticated by
  the parent agent's token, which is present in the agent environment and
  inherited by spawned processes. Such a path that inherits the parent's
  other fields but omits `ai_agent_id` produces an unbound agent inside a
  designated workspace, which passes every starvation check, reverts
  attribution to the human, and is treated as non-AI by the designation
  boundary. It is reachable deliberately by a process in the workspace and
  accidentally by opening a devcontainer. Setting the column at insert, and
  propagating the parent's binding on every child-creation path, is the
  durable fix.
- **Authentication-side credential shape validation.** Until
  authentication rejects agent-owned keys whose shape matches no profile,
  no-self-escalation is a property of the mint path rather than of the
  identity.
- **Other key-issuance paths.** Audit every path that mints a credential
  for a target user, not just the obvious key-creation routes. A workspace
  created for an agent user but left undesignated, for example, would have
  a full-scope owner session token generated for that agent user.
- **Live session termination on suspension.** See invariant 2.
- **Sibling workspace isolation.** All workspaces designated to one chat
  identity form a single trust boundary. They are all starved and all
  audit to one identity, so a jump between them yields no credential gain
  and no attribution loss, but a compromise in one reaches the others.
  Isolating them would require per-workspace sub-identities.
- **Broad read.** An agent can enumerate its sponsor's workspace metadata.
  This is a deliberate product choice for chat inventory UX, not a
  resource-bounded guarantee.
