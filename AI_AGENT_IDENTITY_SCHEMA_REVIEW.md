# AI Agent Identity: Schema Review

Adversarial review of the Vertical 1 identity schema (migration `000565`
and companions) against the goals in `AI_AGENT_SECURITY_ARCHITECTURE.md`.
Produced by a review pass that was asked to challenge the design rather
than confirm it, and to propose alternatives with concrete DDL.

Status: review artifact. Nothing here is implemented. Findings are ordered
by severity; the recommendation section separates what is worth doing at
PoC stage from what should wait for production.

## Overarching goal, in one paragraph

AI agents acting through Coder must be attributable, permission-bounded,
and execution-isolated. Identity is the load-bearing piece: an AI agent is
a real `users` row (`kind='ai_agent'`) with an `ai_agents` metadata row
naming its sponsoring human and its delegation origin. Authorization uses
the sponsor's live roles, so an agent can never exceed its sponsor and
loses access the moment the sponsor is suspended; attribution follows the
agent's own API key, so audit records name the agent with the sponsor as
`on_behalf_of`. Everything downstream consumes that identity: workspace
designation (`workspaces.ai_agent_id`), per-agent binding
(`workspace_agents.ai_agent_id`) which gates all four credential
starvation enforcement points, sandbox lifecycle, and the retained egress
audit stream that deliberately snapshots raw UUIDs with no foreign keys so
history survives identity cleanup.

## What the design gets right

1. **The identity split.** Subject is the human, actor is the agent. Owner
   equality RBAC checks depend on the human ID, so an agent subject would
   fail every check against its sponsor's own resources. Replacing this
   would disrupt human ownership, quotas, and ACLs across Vertical 2.
2. **One identity per delegation boundary.** Stable agent user IDs are
   already depended on by workspace designation and sandbox lifecycle.
3. **Real user rows, at PoC.** API keys, audit actors, and aibridge
   initiators already key on user UUIDs. This is a migration-cost argument
   rather than proof that users are the clean long-term domain model.
4. **Raw UUID snapshots without FKs for retained audit.** Correct. Do not
   "improve" those tables by referencing live identity rows.

## Findings

### S1: `users.kind` and `ai_agents` are split brain, and one direction fails open

The schema permits an `ai_agents` row whose user is `kind='human'`, and a
`kind='ai_agent'` user with no metadata row
(`coderd/database/migrations/000565_ai_agent_identity.up.sql:1-28`).
Middleware branches on `users.kind` before loading `ai_agents`
(`coderd/httpmw/apikey.go:500`), so:

- `kind='ai_agent'` with metadata missing: denied. Fail closed.
- `ai_agents` row present with `kind='human'`: delegation is skipped
  entirely and a direct human subject is built, using that row's own
  roles. This is the dangerous direction.

The architecture document calls `ai_agents` authoritative, but the runtime
discriminator is `users.kind`. Those must be made to agree structurally.

Recommended fix, keeping real user rows:

```sql
ALTER TABLE users
    ADD CONSTRAINT users_id_kind_key UNIQUE (id, kind),
    ADD CONSTRAINT users_ai_agent_no_roles CHECK (
        kind <> 'ai_agent' OR cardinality(rbac_roles) = 0
    );

ALTER TABLE ai_agents
    ADD COLUMN agent_kind user_kind
        GENERATED ALWAYS AS ('ai_agent'::user_kind) STORED,
    ADD COLUMN sponsor_kind user_kind
        GENERATED ALWAYS AS ('human'::user_kind) STORED;

ALTER TABLE ai_agents
    DROP CONSTRAINT ai_agents_user_id_fkey,
    DROP CONSTRAINT ai_agents_owner_user_id_fkey;

ALTER TABLE ai_agents
    ADD CONSTRAINT ai_agents_agent_user_fkey
        FOREIGN KEY (user_id, agent_kind)
        REFERENCES users (id, kind) ON DELETE RESTRICT,
    ADD CONSTRAINT ai_agents_sponsor_user_fkey
        FOREIGN KEY (owner_user_id, sponsor_kind)
        REFERENCES users (id, kind) ON DELETE RESTRICT,
    ADD CONSTRAINT ai_agents_not_self_sponsored
        CHECK (user_id <> owner_user_id);
```

Cost: low migration complexity, minimal query change, one redundant
composite unique index. Note this also changes `ON DELETE CASCADE` to
`RESTRICT`, which is desirable: today a hard-deleted sponsor cascades away
identity metadata that retained audit rows still reference.

### S2: the runtime origin relationship points the wrong way

`ai_agents(origin_type, origin_id)` is a polymorphic association with no
FK (`000565_ai_agent_identity.up.sql:21-32`). Workspaces carry
`ai_agent_id` (`000567_workspace_ai_designation.up.sql:4-7`); chats do
not. Chat execution therefore reconstructs the relationship by root-chat
convention and treats a missing row as a legacy chat
(`coderd/x/chatd/chatd.go:3869-3890`), which is precisely why a corrupted
modern chat falls back to full owner tools. Purge must separately scan for
orphaned origins.

The asymmetry is accidental. Origin tables should point at identity, with
provenance columns on `ai_agents` retained as an immutable snapshot rather
than the runtime join, plus an explicit legacy discriminator so NULL is
never ambiguous:

```sql
CREATE TYPE chat_ai_identity_mode AS ENUM ('delegated', 'legacy_unscoped');

ALTER TABLE chats
    ADD COLUMN ai_agent_id uuid,
    ADD COLUMN ai_identity_mode chat_ai_identity_mode
        NOT NULL DEFAULT 'legacy_unscoped';

-- backfill from the polymorphic origin, then:
ALTER TABLE chats
    ALTER COLUMN ai_identity_mode SET DEFAULT 'delegated',
    ADD CONSTRAINT chats_ai_identity_shape CHECK (
        (ai_identity_mode = 'delegated' AND ai_agent_id IS NOT NULL)
        OR
        (ai_identity_mode = 'legacy_unscoped' AND ai_agent_id IS NULL)
    ),
    ADD CONSTRAINT chats_ai_agent_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agents (user_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX chats_ai_agent_id_idx ON chats (ai_agent_id);
```

Child chats store the root identity directly instead of inferring it. This
makes "delegated chat with no identity" unrepresentable. Cost: medium
migration, medium to high chat code churn (model, creation input, actor
resolution, gateway key lookup, tests). Rejected alternatives: per-origin
nullable FK columns on `ai_agents` (cannot require exactly one, still
permits a chat with no identity); separate link tables (avoid polymorphism
but need triggers for total participation and add a join everywhere);
supertype/subtype origin tables (overbuilt for two origin types).

### S3: sponsorship and organization scope are under-represented

`ai_agents.owner_user_id` is independent of `chats.owner_id` and
`workspaces.owner_id`, so a workspace may reference an identity sponsored
by a different human. Separately, `CreateParams` validates an
`OrganizationID` (`coderd/aiagentidentity/aiagentidentity.go:28-34`) that
the table never stores, so an identity is sponsor-global rather than bound
to the delegation's organization, while the chat profile carries wildcard
template and organization-member allow-list entries
(`coderd/aiagentidentity/profile.go:43-49`).

```sql
ALTER TABLE ai_agents RENAME COLUMN owner_user_id TO sponsor_user_id;
ALTER TABLE ai_agents
    ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT,
    ADD CONSTRAINT ai_agents_scope_key
        UNIQUE (user_id, sponsor_user_id, organization_id);

ALTER TABLE workspaces
    ADD CONSTRAINT workspaces_id_owner_org_key UNIQUE (id, owner_id, organization_id);
ALTER TABLE workspaces
    DROP CONSTRAINT workspaces_ai_agent_id_fkey,
    ADD CONSTRAINT workspaces_ai_designation_scope_fkey
        FOREIGN KEY (ai_agent_id, owner_id, organization_id)
        REFERENCES ai_agents (user_id, sponsor_user_id, organization_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
```

Sponsor must stay immutable; ownership transfer remains revoke and
recreate, because aibridge lineage recovery joins the current row.

### S4: workspace-origin identity and workspace designation are conflated

A workspace can have a workspace-origin identity without being
AI-designated: a human-declared sandbox in a normal workspace resolves or
creates one (`coderd/aisandboxes.go:74-96`). `workspaces.ai_agent_id`
means designation, so origin resolution still falls back to the
polymorphic lookup. Split them:

```sql
ALTER TABLE workspaces RENAME COLUMN ai_agent_id TO designated_ai_agent_id;
ALTER TABLE workspaces ADD COLUMN workspace_origin_ai_agent_id uuid;
CREATE UNIQUE INDEX workspaces_origin_ai_agent_unique_idx
    ON workspaces (workspace_origin_ai_agent_id)
    WHERE workspace_origin_ai_agent_id IS NOT NULL;
```

Semantics: direct human opt-in sets both to the same identity; a
sandbox-only normal workspace sets origin with designation NULL; a
chat-created workspace sets designation to the chat identity with origin
NULL.

### S5: binding is denormalized and unconstrained

`workspaces.ai_agent_id` claims every agent in the workspace is bound,
while `workspace_agents.ai_agent_id` is independently nullable and
populated by application code
(`coderd/provisionerdserver/provisionerdserver.go:2229-2249`). The
database permits a designated workspace with unbound agents, which
disables all four credential starvation enforcement points, since they key
on the per-agent binding. A plain FK cannot express this because agents
reach workspaces through resource, job, and build tables; a deferred
constraint trigger validating the committed state can. Replacing the
column with a binding table is cleaner in isolation but has large Vertical
2 blast radius; do not do it at PoC.

Missing FK support index, worth adding immediately:

```sql
CREATE INDEX idx_ai_sandboxes_ai_agent_id ON ai_sandboxes (ai_agent_id);
```

### S6: `deleted boolean` is the wrong revocation model

`UpdateAIAgentDeleted` can set the flag either direction, so revocation is
not terminal. Revocation leaves the agent's `users` row active, which is
why generic human-user mutation paths still reach agents. There is no
revocation timestamp or reason, which Vertical 3 will want. And
`ResolveOnBehalfOf` calls the live-only `Resolve`
(`coderd/audit/request.go:564-571`), so a delayed background audit after
revocation silently loses sponsor lineage.

```sql
CREATE TYPE ai_agent_state AS ENUM ('active', 'revoked');

ALTER TABLE ai_agents
    ADD COLUMN state ai_agent_state NOT NULL DEFAULT 'active',
    ADD COLUMN revoked_at timestamptz,
    ADD COLUMN revocation_reason text,
    ADD CONSTRAINT ai_agents_state_consistent CHECK (
        (state = 'active' AND revoked_at IS NULL AND revocation_reason IS NULL)
        OR
        (state = 'revoked' AND revoked_at IS NOT NULL AND revocation_reason IS NOT NULL)
    );
-- backfill from deleted, then drop it.
```

Plus a BEFORE UPDATE trigger making provenance immutable and revocation
monotonic, an AFTER UPDATE trigger deleting the identity's API keys
atomically on revocation, and origin-side triggers that revoke a
workspace-origin identity when its workspace is deleted (which is finding
5 from the conformance audit, enforced structurally rather than by
remembering to call a helper).

Then split the resolution API in two, which is the key insight:

- `ResolveForAuthorization`: requires active state and a live sponsor.
- `ResolveForAttribution`: returns immutable sponsor and provenance
  regardless of state.

That fixes lineage loss without weakening authorization. Do not add an
`expired` state; keys expire, identities are durable lineage.

### S7: real user rows need database guards, not a growing exclusion list

Listing queries filter `kind='human'`, but mutation queries accept
arbitrary user IDs, which is why admin APIs can still assign organization
membership, group membership, and roles to agent users. Make it
structural with typed composite FKs:

```sql
ALTER TABLE organization_members
    ADD COLUMN member_kind user_kind
        GENERATED ALWAYS AS ('human'::user_kind) STORED,
    ADD CONSTRAINT organization_members_human_user_fkey
        FOREIGN KEY (user_id, member_kind)
        REFERENCES users (id, kind) ON DELETE CASCADE;
```

Same pattern for `group_members` and human-only notification recipient
tables. A true `principals` supertype is cleaner if Coder expects several
machine principal kinds, and the architecture document's fail-open
objection to it is really a migration-risk objection rather than an
inherent property. It is still a multi-release migration touching API
keys, audit DTOs, aibridge proto, request logging, and RBAC loaders. Do
not attempt at PoC.

### S8: indexes do not match implemented query patterns

`GetAIAgentByOriginIncludingDeleted` filters by origin and orders by
`created_at DESC`, but the only origin index is partial
`WHERE NOT deleted`, so it cannot serve that lookup, and chatd calls it on
platform tool resolution. Owner listing orders by created time against an
owner-only index.

```sql
CREATE INDEX idx_ai_agents_origin_history
    ON ai_agents (origin_type, origin_id, created_at DESC, user_id DESC);
CREATE INDEX idx_ai_agents_sponsor_created
    ON ai_agents (sponsor_user_id, created_at DESC, user_id DESC);
CREATE INDEX idx_audit_logs_user_time ON audit_logs (user_id, "time" DESC);
CREATE INDEX idx_audit_logs_on_behalf_time
    ON audit_logs (on_behalf_of_user_id, "time" DESC)
    WHERE on_behalf_of_user_id IS NOT NULL;
```

The audit filter is `user_id = X OR on_behalf_of_user_id = X` ordered by
time, which may still need a BitmapOr plus sort; a `UNION ALL` shape may
be required for strict ordered index use. Unverified without
representative data.

## Not schema solvable

- **Stale subjects in live DRPC and WebSocket sessions.** No DDL fixes
  this. A monotonic identity state gives a source of truth, but live
  connections need per-message revalidation or revocation pubsub that
  closes sessions.
- **Request logger omitting the AI agent subject type.** Code only.
- **Background audit producers omitting `on_behalf_of`.** Schema can
  retain the snapshot; callers must populate it.

## Recommendation

Do now, at PoC:

1. `chats.ai_agent_id` with an explicit delegated/legacy mode; stop
   resolving chat identity by polymorphic origin.
2. Store identity `organization_id`; composite sponsor and organization
   FKs from chats and workspace designation.
3. Separate `workspace_origin_ai_agent_id` from designation.
4. Typed composite FKs enforcing agent and sponsor kinds, and blocking
   roles and human-only memberships at the database level.
5. Replace reversible `deleted` with immutable active/revoked state plus
   timestamp, reason, and atomic key deletion; split resolution into
   authorization and attribution variants.
6. Add the missing history, sponsor, and FK support indexes.

Do before production:

1. Deferred trigger enforcing workspace-agent binding consistency.
2. Origin-side deletion triggers revoking origin identities.
3. Aibridge sponsor snapshots, or a formal guarantee that identity rows
   are never hard deleted.
4. Live-session invalidation.
5. Validate audit index plans against production-like cardinality.

Do not:

1. Do not make sponsor mutable; revoke and create instead.
2. Do not use per-origin nullable FK columns as the primary relationship.
3. Do not hard delete agent users or identity rows.
4. Do not add an identity `expired` state while key lifetime models it.
5. Do not introduce a principal supertype at PoC.

## Could not verify

Production row counts and query plans; whether out-of-repo tooling hard
deletes users (in-repo deletion is soft); whether cross-organization
delegation is intentionally permitted; backfill feasibility for identities
whose polymorphic origin was already purged; external
terraform-provider-coder consumers; Vertical 3 requirements beyond the
current stub.
