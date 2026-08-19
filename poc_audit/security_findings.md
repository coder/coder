# Security Findings and Mandates for Post PoC Work

Recorded 2026-08-06.

This document records **mandates and recommendations for work after the proof
of concept**. It prescribes no code. Nothing here is scheduled, and nothing
here is implemented on the `tigre` branch.

Terminology follows `poc_audit/audit_approach.md`. `workspace_agent` is always
written in full. The bare word "agent" is reserved for the principal and agent
sense and is not used here.

Problems are numbered on first statement and referred to by number afterward
rather than restated. A problem may appear in more than one section when its
resolution has several parts.

## 1. Issuance authority

### Problem

**P1. The credential for workspace_agent is minted outside coderd.** The
Terraform provider generates it during apply, in `provider/agent.go:516`,
`uuid.NewString()`. coderd receives it over the provisioner protocol and writes
it, validating only that it parses as a UUID
(`coderd/provisionerdserver/provisionerdserver.go:2924`). coderd's own
`uuid.New()` is only the fallback for resources that carry no auth at all.

coderd is therefore in the position of recording, as the credential it will
later accept, a value produced by another party.

### Solution

**coderd must be the only party that can issue a `workspace_agent`
credential.** The Terraform provider must not generate one, and coderd must
reject any credential presented to it for storage.

The mechanism already exists and needs generalizing rather than inventing.
`coderd/provisionerdserver/provisionerdserver.go:711` already pushes
credentials down to the provisioner, in the `RunningAgentAuthToken` proto
message, for prebuilt workspaces being claimed. Its comment explains the
reason it was built, which is unrelated: the credential is baked into
immutable resource attributes, so changing it would force replacement and
defeat the prebuild.

Mechanically, though, that is the required direction: coderd mints, the
provisioner receives. The channel, the proto message, and the provider side
plumbing are all in place. Extending it from the claim path to every build is
a change of policy, not of architecture.

## 2. Storage of the credential

### Problem

**P2. The credential is stored in plaintext.**
`workspace_agents.auth_token` is a bare `uuid` column. The deployment level
encryption feature in `enterprise/dbcrypt` covers `user_links`,
`external_auth_links`, `crypto_keys`, and `ai_providers`. It does not cover
`workspace_agents`.

**P3. The credential is stored at all.** It is a bearer secret, so the rules
that apply to passwords apply to it. A presentable credential should not be
recoverable from the database in any form, encrypted or otherwise. Encryption
at rest protects against one threat and leaves the stored value recoverable by
anything holding the key.

P1 compounds both: the value coderd stores in the clear is one it did not
create.

### Solution

**Store only a one way hash of the credential, never the credential itself.**
This is not a novel practice for this codebase. `api_keys` already does it:

- `api_keys.hashed_secret bytea` holds the hash
- `api_keys.id text` is a separate, non secret lookup key
- comparison is `apikey.ValidateHash(key.HashedSecret, keySecret)`
- the hash is `sha256.Sum256` (`coderd/apikey/apikey.go:155`)

The `workspace_agent` credential should follow the same shape: a public
identifier used to find the row, and a hash used to verify the presented
secret. This also removes the need to bring `workspace_agents` under
`dbcrypt`, because there is no longer a secret at rest to encrypt.

A note on salting, offered for the implementer rather than as a
contradiction of policy. Salting defends against precomputation and against
shared low entropy secrets. It earns its cost for human chosen passwords,
which is why `users.hashed_password` uses a slow key derivation function. For
a randomly generated high entropy credential, this codebase's own practice for
`api_keys` is unsalted SHA-256 with a separate lookup identifier, and a slow
KDF would impose its cost on every authenticated request an agent makes. The
requirement that matters is that the stored value must not be reversible to a
presentable credential. Whether a per credential salt is added on top should
be decided on that basis.

## 3. The second copy, in Terraform state

### Problem

**P4. The credential is also stored, in plaintext, inside
`workspace_builds.provisioner_state`.** Terraform state records all resource
attributes, and `Sensitive: true` on the provider's `token` attribute governs
display rather than storage. That state is persisted wholesale into a `bytea`
column (`coderd/provisionerdserver/provisionerdserver.go:1254` and `:2081`).

This copy is worse than P2 in three ways:

- It is not reachable through the entity it belongs to. Rotating or clearing
  `workspace_agents.auth_token` leaves it untouched.
- It is retained per build. Workspace rows are never hard deleted, so
  historical credentials accumulate for the life of the workspace.
- Nothing treats that column as containing secrets, so no existing control
  applies to it.

### Solution

**The credential must never enter Terraform state.** Hashing the stored copy,
per section 2, does not help here: this copy is inside an opaque blob that no
credential handling code touches.

Two directions are worth investigating, and neither is verified:

1. **Keep it out of state at the Terraform level.** Recent Terraform versions
   provide ephemeral values and write only arguments, intended for exactly
   this: values needed during apply that must not be persisted. The pinned
   version in `mise.toml` is well past the point at which these appeared.
   Whether the provider and the `coder_agent` resource can be reshaped to use
   them needs checking.
2. **Do not route the credential through Terraform at all.** Have the
   `workspace_agent` bootstrap with something that is not a long lived
   credential, such as the existing cloud instance identity path, and exchange
   it with coderd for a session credential held only in memory. This satisfies
   the ephemerality policy directly, and it removes the reason the credential
   was in state to begin with.

The second is the larger change and the cleaner outcome. The first may be a
useful intermediate step.

## 4. Credential lifecycle and schema shape

### Problem

**P5. The credential is one to one with the entity.**
`workspace_agents.auth_token` is a single column. There is no issuance time,
no expiry, no revocation time, and no link to a predecessor.

Two consequences follow, and the second is the more practical:

- No lifecycle can be recorded. Under the definitions in
  `poc_audit/audit_approach.md`, issuance, rotation, and revocation are
  persistent state changes and therefore auditable events. There is currently
  nowhere to record them.
- **Rotation cannot be performed without a gap.** A single column means the
  new value invalidates the old one at the instant of the write, while the
  holder still has the old one. Overlapping validity requires one to many.

**P6. Uniqueness of the credential is assumed but not enforced.** The index is
not unique:

```sql
CREATE INDEX workspace_agents_auth_token_idx ON workspace_agents USING btree (auth_token);
```

The lookup, `GetAuthenticatedWorkspaceAgentAndBuildByAuthToken`, is generated
as `:one` and carries the comment "This should only match 1 agent, so 1
returned row or 0". A collision would authenticate as whichever row was
returned first, silently rather than as an error.

For contrast, `workspace_agent` names did get deliberate enforcement, with a
documented reason:

```sql
CREATE TRIGGER workspace_agent_name_unique_trigger ...
COMMENT ON TRIGGER ... IS 'Use a trigger instead of a unique constraint because existing data may violate ...'
```

Names got rigour. The credential did not.

### Solution

**Move credentials into their own table, one to many with the entity they
authenticate.** The structure must support both ends of the range:

- **More than one valid credential at a time**, so rotation can overlap and
  the holder of the old credential is never locked out mid rotation.
- **No valid credential at all**, so revocation is expressible as a state
  rather than as deletion of a column value, and so an entity can exist before
  or after ever holding one.

Each row should carry at minimum the hash from section 2, a non secret
identifier, an issuance time, an expiry, and a revocation time. Uniqueness
must be enforced by the schema rather than assumed by the query, addressing
P6.

This is what makes the events in P5 recordable. Once issuance, rotation, and
revocation are rows with times, they are persistent state changes with a
place to be recorded, which is the precondition for section 5.

## 5. Auditability of credential events

### Problem

**P7. There is at present no auditability of credential events.**

`workspace_agent` is not wired into the mechanism currently called `audit_logs`
at all, with zero mentions in `coderd/audit/diff.go` and zero in
`enterprise/audit/table.go`, despite the `resource_type` enum already
containing a `workspace_agent` value.

Combined with P5, this means that credential events are neither representable
nor recordable today. Neither half is sufficient alone: a lifecycle table with
no coverage records state without provenance, and coverage with no lifecycle
table has nothing to record.

### Solution

**Treat credential issuance, rotation, and revocation as auditable events in
the audit system described by `poc_audit/audit_approach.md`.**

Which subset is actually audited remains a design choice, per that document.
But these are unambiguously persistent state changes, and a credential whose
issuance cannot be attributed defeats the purpose of controlling who may issue
it, which is what section 1 sets out to fix.

The correspondence key requirement from the audit approach applies directly
here. An entry recording that a credential was issued must name which
credential, by its non secret identifier and never by its value.

## 6. Names and identifiers

### Problem

**P8. Names of variables that hold credentials do not always say that they are
credentials.** Two habits account for most of it. A credential is named for how
it is presented, as with `token`, or for how it is encoded, as with `uuid`.
Neither says what the value is, and both invite the reader to reason about the
wrong thing.

Naming by presentation is the more common. `workspace_agents.auth_token` is the
stored column, `RunningAgentAuthToken` is the proto message that carries a
credential to the provisioner
(`provisionersdk/proto/provisioner.proto:331`, whose field is simply `token`),
and `GetExternalAgentTokensByTemplateID` selects
`workspace_agents.auth_token AS agent_token`
(`coderd/database/queries/workspaceagents.sql:395`), renaming one presentation
to another.

Naming by encoding shows up here as a type rather than as an identifier: the
column is `uuid`. That is worse than a bad name, because it commits the schema
to the encoding. Policy goal 3 requires that what persists be a one way hash,
and a `uuid` column cannot hold one. The encoding was recorded as though it
were the nature of the value, and it now obstructs P2 and P3.

**`CODER_AGENT_TOKEN` is the clearest offender, and is wrong twice.**
It is set at `agent/agent.go:1704` and read at `cli/root.go:93`. `token` names
the presentation. `agent` names the wrong principal.

The second half is worth stating precisely, because the credential does have a
genuine relationship to a `workspace_agent`. It is stored per `workspace_agent`
row and identifies which one presented it. But the authority it confers is not
that agent's. `coderd/httpmw/workspaceagent.go:112` builds the RBAC subject
from the workspace **owner's** roles, scoped by
`rbac.WorkspaceAgentScope`, whose allow list
(`coderd/rbac/scopes.go:63`) names the workspace, the template, the version,
the owner, and optionally the task. **No element of it names a
`workspace_agent`.** Two agents in one workspace hold different values and
receive identical authority.

So it identifies a `workspace_agent` and authenticates as the workspace. The
name captures neither.

### Solution

**Rename poorly named variables.** A credential is named for what it is:

- Not for how it is presented. `token` is a presentation.
- Not for how it is encoded. `uuid` is an encoding, and should not appear in
  the name or, per P2 and P3, in the stored type.
- A more specific name is welcome where the kind is known and fixed.
  `password` is a good name for a password.
- Where the kind is unknown, or makes no difference to the code holding it,
  `credential` is the right word.

The same discipline applies to what the name claims about the principal. A
credential's name should not assert an authority it does not confer, which is
the second defect in `CODER_AGENT_TOKEN`.

This is cosmetic only in appearance. Sections 1 through 5 are all concerned
with treating this value as a bearer secret with a lifecycle. A name that says
`token` invites the reader to think about a string being passed, and a type
that says `uuid` forecloses storing a hash. Names that describe encodings and
presentations are how the wrong model gets propagated.

## Policy

The goals below are what future work on the code must satisfy. They are stated
as outcomes rather than as implementations.

1. **coderd is the sole issuer.** No other party may create a credential that
   coderd will accept for a `workspace_agent`. coderd must never record, as a
   credential it will honour, a value produced elsewhere.
2. **The credential is ephemeral outside coderd.** A credential passed out of
   coderd is never written to Postgres. It exists only in the process memory
   of the parties that transmit or use it, and specifically never in Terraform
   state.
3. **Only a non reversible form is stored.** What persists is a one way hash
   and a non secret identifier. Encryption at rest is not a substitute,
   because it leaves the value recoverable.
4. **The schema supports the full range of credential validity.** More than
   one credential may be valid at once, so rotation can overlap. No credential
   may be valid, so revocation is a state rather than an absence. Uniqueness
   is enforced by the schema, not assumed by a query.
5. **Credential lifecycle events are auditable**, in the sense given by
   `poc_audit/audit_approach.md` rather than by the existing mechanism of the
   same name. Issuance, rotation, and revocation are persistent state changes
   and are treated as such, referring to credentials by identifier and never by
   value.
6. **Credentials are named for what they are.** Not for how they are
   presented, not for how they are encoded, and never for an authority they do
   not confer. Where the kind of credential is known and fixed, the specific
   word is used; otherwise the word is `credential`.

## Origin

P1 through P7 came out of tracing the `register and fetch manifest` arrow in
`poc_audit/workspace_startup_asbuilt.d2`, not out of a security-specific review.
It was a design trace that kept turning up the same class of issue. They concern
the credential a `workspace_agent` uses to authenticate to coderd.

P8 has a different origin. It came out of naming a field while writing proof of
concept code, where the question of what to call the value made it necessary to
establish what the value actually authenticates.

## Before acting on any of this

A documented threat model may already accept some or all
of these, particularly if provisioner daemons are considered to be inside the
trust boundary. That question is load bearing for the first section: with only
built in provisioners the exposure is small, and external provisioner daemons
are the case that matters.

These are old code paths. The credential handling dates to 2022. Check for
existing issues before reporting any of this as new.
