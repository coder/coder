<!-- markdownlint-disable-file MD036 -->
<!-- Bold labels stand in for level 4 headings inside a work package, so the
     heading index stays a usable table of contents. Scoped to the proof of
     concept. Revisit if this document outlives it. -->

# Work Breakdown

Recorded 2026-08-12. This document breaks the coding work into **work
packages**. Each section below is one work package.

"Work package" is the term from work breakdown structure practice for the
lowest level element of a decomposition, sized to produce a verifiable
deliverable. It is used here in preference to "unit", which collides with unit
testing, and to "task", which collides with the `tasks` entity in this
codebase.

**A package's number is an identifier and nothing else.** It is neither a
priority nor a position in a sequence, and the packages are not meant to be
worked in numeric order. What a package depends on is stated in its own Status,
and that is the only ordering the document asserts.

Each work package is sized so that completing it produces a passing acceptance
test. Subsections are fixed for now and may grow later:

- **Summary.** A sentence or two.
- **Status.** Whether the package is done, and what remains if not. Absent
  until work starts.
- **New behavior.** What the package adds or changes in the running system.
- **New data.** One item per new datum: table, column, type, interface.
- **Acceptance tests.** What whole-package tests must pass for it to be done.
- **Implementation.** The code elements needed, separating existing locations
  to alter from green field locations.
- **PoC cheats.** Every compromise made for proof of concept scope. All of
  these are in the definitely keep away from production category.

## WP1. Create AI agent identity

### Summary

A `workspace_agent` calls an API endpoint in the control plane to notify it of
the creation of an AI agent somewhere within its workspace. The control plane
mints an identifier for the AI agent, issues it a credential, and records both.

### Status

Complete as of 2026-08-13, at commit `f5a9eff1fc`. The acceptance test passed
then.

That is a historical statement rather than a standing one. Nothing re-runs it
against the design as it has since developed, and confirming it would mean
checking out code from that date, which nobody should have reason to do. Read it
as a record of when this package was considered finished, not as a claim about
the tree in front of you. The commit is given for precision and may not survive
a rebase of this branch; the date will.

### New behavior

- A mock agent creation call that occurs during workspace initialization. This
  mock persists for the duration of the PoC, so that this test continues to run
  even after proper agent creation code is in place.
- A write to the entity journal recording the creation of a new AI agent, in
  the same transaction as the row it accounts for.
- Issuance of a credential for the AI agent to use for subsequent calls to the
  control plane, written in the same transaction as the identity and its entry.
- Verification of a presented credential against the ones currently valid for an
  entity.

### New data

- **A new drpc method on the `AgentSocket` service**, the local socket that
  processes inside a workspace use to reach the `workspace_agent`.
- **A new drpc method on the `Agent` service**, the same service that already
  serves the manifest call shown in the proposal diagram. The
  `workspace_agent` calls this one to reach the control plane. That API is
  versioned and negotiated, so one rpc also costs a version bump, a client
  interface, two connect helpers, and a method on every test double.
- **One journal for every entity**, `entity_journal`, whose entries name their
  subject and actor by a `(type, identifier)` pair. The approach in
  `poc_audit/audit_approach.md` is stated independently of any entity, so one
  journal serves all of them.
- **A table for AI agent identities**, `ai_agents`. An entry's subject has to
  name something that exists.
- **A table of currently valid credentials**, `valid_credentials`, holding an
  actor pair and a password. Membership is validity: revoking deletes the row.
  It has no key, so an actor may hold several at once and a rotation can overlap
  rather than requiring a moment with no valid credential.

### Acceptance tests

A test that follows the sequence in
`poc_audit/workspace_startup_proposal.d2`.

Test Scenario:

1. The workspace starts and its `workspace_agent` reaches the ready state.
2. The startup script runs the minimal executable, which asks the control
   plane, by way of the `workspace_agent`, to create an AI agent.
3. The control plane returns an AI agent identifier and a credential.
4. Verify that the identifier names a row in `ai_agents` owned by the owner of
   the workspace the request came from.
5. Verify that the journal contains one creation entry whose subject is that
   identifier and whose actor is the `workspace_agent` the control plane
   authenticated.
6. Verify that the credential the executable received is the one on record,
   compared by digest.
7. Verify independently, from the script timings the agent reports, that the
   executable exited zero.

Notes:

- The minimal executable makes that request and does little else.
- **Verification is by reading from the database directly.**
- The test builds its workspace with
  `dbfake.WorkspaceBuild(...).WithAgent(...)`, which attaches the startup
  script without running a provisioner, so no template that real workspaces use
  is involved.
- The identifier is taken from the executable's own output rather than from the
  database, because that is the route a real caller receives it by. Reading it
  from the database instead would leave the returned value unchecked, and a
  handler that persisted one identifier and returned another would pass.
- **There is nowhere yet to present the issued credential**: the endpoints that
  would accept it belong to collaborator work that is not available. A live test
  of the credential is therefore deferred. That verification works at all is
  covered separately, as a unit test.
- **The credential is compared by digest, never printed.** The executable's
  standard output becomes a startup script log which the control plane stores,
  so printing a credential would put it somewhere it does not belong. The
  executable reports a digest instead, which shows the credential arrived intact
  and is useless to a reader of the log.
- Step 7 is independent of the steps above it on purpose. A call that succeeded
  and was then followed by a failure is still a failure.

### Implementation

**Existing locations to alter**

- `agent/proto/agent.proto`, adding one rpc to `service Agent` along with its
  request and response messages, then regenerating.
- `coderd/agentapi/api.go`, embedding the new sub API in `type API struct` at
  line 47, beside `*ManifestAPI`, and constructing it in the same place the
  others are constructed at line 122.
- `coderd/database/migrations/`, up and down pairs for the journal, its
  indexes, the AI agent table, and the table of valid credentials.
- `coderd/database/queries/`, new queries, followed by `make gen`.
- `coderd/database/dbauthz/dbauthz.go`, authorization wrappers for each new
  query.
- `coderd/rbac/policy/policy.go` is **not** touched. The journal and the AI
  agent table reuse `rbac.ResourceSystem` rather than taking a resource of their
  own. See the cheats.
- `agent/agentsocket/`, adding one rpc to the `AgentSocket` service in the
  proto and implementing it in the service, so that the `workspace_agent`
  relays the call onward. `UpdateAppStatus` on that service is the model to
  follow, since it already forwards to the control plane's agent API.

Because this is drpc rather than REST, four things a new HTTP endpoint would
need are **not** needed: no route registration in `coderd/coderd.go`, no
`codersdk` HTTP types, no swagger annotations, and no `coderd/apidoc`
regeneration.

**New locations**

- `coderd/entity/`, the package owning lifecycle, identity, and the journal. The
  handler needs somewhere to call that is neither an HTTP handler nor a
  generated query, and `coderd/wsbuilder` is the precedent for a package owning
  transactional creation.
- `coderd/agentapi/aiagent.go`, the handler, following the shape of
  `manifest.go` in the same package.
- `coderd/database/queries/`, one query file per table.
- The migration up and down pairs.
- The minimal executable that the startup script runs.
- The acceptance test.

**Test harness available**

The end to end shape already exists and does not need building:

- `coderdtest.New` for the control plane.
- `dbfake.WorkspaceBuild(...).WithAgent(...)` to build a workspace with an
  agent without running a provisioner.
- `agenttest.New(t, client.URL, agentToken)` to run a real `workspace_agent`
  against the test control plane. See `coderd/workspaceagents_test.go:778` for
  the pattern in use.

**The test harness must not run the executable directly.** It is run from the
manifest, as it would be in a real workspace, so that the test exercises all
three hops of the call path rather than stubbing the first.

The harness already supports this:

- The provisioner `Agent` message carries `repeated Script scripts = 21`, and
  `WithAgent` takes mutators over `[]*sdkproto.Agent`, so a script can be
  attached to the built workspace.
- `agenttest.New` constructs the real agent, which wires `agentscripts.New`
  and calls `scriptRunner.Init` with the scripts from its manifest.
- `agenttest.New` also sets a socket path, so the local `AgentSocket` service
  the executable calls is stood up per test.

**Who may call this**

Anything running in the workspace can make this call, because the call
authenticates with the `workspace_agent` credential, which is present in the
workspace environment and readable by any process there. The caller is proved
to hold that credential, not to be the `workspace_agent`.

This is not a compromise made for the PoC. It follows from the security model,
and specifically from the possibility that a workspace may spin up sandboxes
after provisioning is complete. Whether to allow that is not yet fully decided
by the collaboration. For this work package it is allowed.

**Call path**

Three hops, matching the proposal diagram, where the call to the control plane
originates from the `workspace_agent` rather than from whatever prompted it:

1. The minimal executable calls the local `AgentSocket` service.
2. The `workspace_agent` relays that to the control plane over the `Agent`
   service, on the same session it already uses for the manifest.
3. The control plane mints the identifier and the credential, writes the row
   and both entries in one transaction, and returns.

The executable therefore needs only a local socket client, not a session to the
control plane. `UpdateAppStatus` already follows this path and is the model.

### PoC cheats

- **The credential is a plaintext password in the database.** The mandates in
  `poc_audit/security_findings.md` require that only a non reversible form be
  stored, and this violates that knowingly. Every credential here is assumed to
  be a password, which is a simplification and not a position on what
  credentials should be.
- **Credential lifecycle is not journaled.** Issuance is not recorded, and
  neither rotation nor revocation exists to record. This reproduces P7, no
  auditability of credential events, deliberately and to limit scope. The table
  is shaped so that adding the journal later does not require reshaping it:
  revocation will delete a row, and the account of when that happened belongs to
  the journal rather than to a column.
- **The mock call is not real AI agent creation.** An identity is minted and a
  row exists, so the entity is real and the journal accounts for something.
  What does not exist is any AI agent process the identity names. **This is a
  cheat only for this work package.** Open decisions elsewhere make doing more
  than this at this point premature.
- **The journal has no matching current status table.** `ai_agents` holds an
  identity and its owner, not a status. Current state must be derived by
  folding the journal, and there is no denormalized answer to check the fold
  against.
- **Authorization is coarse.** Both the journal and `ai_agents` are guarded by
  `rbac.ResourceSystem`, which does not tell writing the journal apart from any
  other internal write. It was chosen for one property worth keeping: an
  entity's own credential is workspace-scoped and carries no system permission,
  so nothing inside a workspace can write journal entries or create an AI
  agent, and the rule that an entity may not write about itself is enforced by
  the database rather than trusted to code. A dedicated resource is production
  work.
- **The socket call's credential check proves less than it looks like.** The
  `workspace_agent` compares the presented credential against its own, in
  constant time, but it exports that credential to every process it starts. A
  caller that passes has shown it is inside this workspace, not which process
  it is.
- **The event vocabulary is open text**, and the actor and subject types are
  text with no database `CHECK`. The type set is closed in Go and refused at
  the point of writing. Two notes for later sit in the code: a periodic sweep
  for values outside the set is the intended replacement for a constraint, and
  the `Type` for a user currently absorbs system actors such as the account
  that creates prebuilt workspaces.
- **`AppendEntry` validates almost nothing** beyond those types, deliberately,
  and what a production implementation would have to check is not specified
  either. See its comment for why.
- **The executable prints the minted identifier to standard output** so the
  test can read it out of the script log. Nothing in a real workspace would
  want that line.

## WP2. Bring all three journals to mature form

### Summary

Every journal and ledger in the proof of concept is built to the patterns in
`implementation_patterns.md`, and the three entities that have lifecycles each
have their own pair. Completing this package means no journal in the tree is
still in a shape the corpus has moved past.

### Status

Complete as of 2026-08-22. Every schema change this package describes has
landed, `entity_journal` and `entity_ai_agents` are gone, and each of the three
entities has its own journal and ledger written to the current patterns. The
blocker recorded here for a time, the credential lifecycle, was designed and
built in WP4.

The last item was the one this package called required regardless: retiring an
AI agent now lapses every authorization naming it and invalidates every
credential it holds, in the transaction that retires it.

That forced a question the entity model had left open, and answering it moved
the model past what this package built. **The lapse entries name an actor, and
the model now says an entailed operation has none.** That misalignment is known,
deliberate, and WP6's to close. It is recorded there rather than by reopening
this package, whose deliverable stands: the endings happen, in one transaction,
and stop the credential verifying.

### What forces the work

`authorization_lifecycle_journal` and `authorization_ledger` were
written to the current patterns. `entity_journal` was not, and predates most of
them. It is one journal for every entity rather than one for each, so it carries
a `subject_type` that per-entity journals make unnecessary; it has a single
`recorded_at` where two dates are now required; its `id BIGSERIAL PRIMARY KEY`
is a row identifier where an entry identifier and a line number are now
required;
and it has no lines, so no entry in it can be a multiline entry.

`entity_ai_agents` has no state column, so the AI agent's states are recorded
nowhere, and no posting reference, so nothing says which entry last posted to
a row.

None of that is a defect in what was built. It is the distance between the first
thing written and the patterns arrived at since, and closing it is the point of
this package.

### New behavior

- One journal and one ledger for each of the three entities: AI agent,
  authorization, and credential.
- The AI agent lifecycle recorded as transitions against the machine in
  `entity_model.md`, rather than a single `created` event.
- The event vocabulary settled once across all three machines rather than grown
  one entity at a time, and written in the bare verb form the naming rule asks
  for.
- Creation of an AI agent writing three entries in one transaction, one to each
  journal, as `entity_model.md` describes.

### Acceptance tests

Mostly undecided, and they should be written before the implementation is. The
WP1 test is extended to assert that the grant of authorization is recorded,
which is the smallest thing that cannot pass under the present code.

**One is required regardless: a unit test exercising the termination of an AI
agent.** Whether termination is wired into the running system during the proof
of concept is undecided, and this test does not wait on that answer. It exists
so that whoever wires it in finds it works the first time, and it is the only
thing that makes the `lapse` transitions of the other two machines reachable,
since both fire on an AI agent reaching `retired`.

### PoC cheats

Not yet enumerated. They will be, before this package is called complete.

## WP3. Credential expiry

Not yet written. Covers the sweep and its three triggers, the fixed system
actor, the clock check on the verification path, and the optional enqueue. The
expiry column itself lands with the credential schema in WP4, defined there and
left unused, so this package changes no schema.

## WP4. Credential foundations

### Summary

Credentials become normalized, typed, and able to stand behind the credential
`api_keys` already issues. The package ends with a running system whose api key
issuance goes through a journal, which is what makes it a foundation rather than
a rehearsal.

### Status

Complete as of 2026-08-22. All four milestones landed, and both acceptance
tests named below pass, the second of them against a running server.

Four things were not foreseen when the package was written.

**A credential type fixes the shape of its authenticator.** An api key token is
`<key id>-<secret>` at exactly ten and twenty two characters, because
`httpmw.SplitAPIToken` parses nothing else. So the api_key type mints something
structurally different from a password, and the token is a wire format packing a
declaration and an authenticator output rather than an authenticator. Splitting
it is the verifier's work.

**The ledger had to record the public half.** `credential_api_key.key_id` was
added, uniquely indexed, so that a token arriving over the wire resolves to the
credential it names and so that the mirror is traceable back from `api_keys`.
Without it the column that connects the two records would be derived rather than
recorded.

**The mirror has to write an expiry the ledger does not hold**, because
`api_keys.expires_at` is `NOT NULL`. Expiry remains out of scope in the sense
the package intended: nothing reads or writes the ledger's own column. What the
mirror writes is a stand-in, and the finding it produced is recorded in
`rewrite_rbac.md` under "api_keys cannot express a credential that does not
expire".

**The second milestone left five methods untested in the dbauthz suite**, which
asserts that every method is exercised at least once and so had been failing
whenever it ran unfiltered. Found here and fixed here. The lesson is the one
already learned once on this branch: a filtered test run is not evidence about
the suite.

### What forces the work

The credential ledger is denormalized. `credential_value` holds a hex digest for
one type and the empty string for another, which is already two meanings in one
column and would become a junk drawer at the third. `api_keys` is what that
becomes when it is left alone long enough: `login_type` and `ip_address` are
meaningful for a credential a person obtained by logging in and fiction for one
minted for an agent, and nothing in a row says which it is.

Normalizing is cheaper here than the actor case that motivated the `(type, id)`
pair, because the direction of reference reverses. The ledger mints the
identifier, so a per-type table keys on the ledger's own id and points at it
with an ordinary foreign key. There is no union to join into: read the ledger
row, learn the type, go to one table.

Separately, the two credential stores are unconnected. Ours mints, stores and
verifies over the agent socket; `api_keys` does the same over HTTP. Both now
carry a `(holder_type, holder_id)` pair, identically shaped, and no code crosses
between them. Until one of them produces the other, the credential ledger
describes credentials rather than recording them, which
`rewrite_rbac.md` argues is not a record at all.

### New behavior

- A journal and a ledger per credential type, replacing the single denormalized
  pair. The null type needs no table, having no value to hold.
- An `api_key` credential type, carrying what an API key is: its scopes, its
  allow list, its token name.
- Entries carrying type specific particulars for issuance only. Other
  operations on a credential do not need what issuance needs, and a journal
  shaped to hold the largest entry leaves most rows mostly empty.
- A `credential_use` entity, described in the corpus before it is built. It is
  a variable rather than a state machine: entries record assignments and posting
  overwrites. Two variables are wanted, `last_presented` for every presentation
  and `last_used` for accepted ones, and the difference between them is the
  security value.
- Issuance of an api key posting to `api_keys` in the same transaction as the
  ledger, so that the existing table becomes what the journal produces.

### Milestones

1. **Normalize the existing credential journal and ledger.** Per-type tables,
   the existing test still passing. Done.
2. **Add the `api_key` credential type**, with its journal and its ledger. Done.
3. **Model and build `credential_use`.** The corpus entry comes first, since
   this is the first entity that is not a state machine and the general form was
   only settled recently. Done.
4. **Mirror `api_key` credentials into `api_keys` on issuance.** Posting is
   already a transaction, so a third write joins it. Done.

### What this package does not do

**Expiry is out of scope, with one exception.** The schema defines the column
and nothing reads or writes it. Expiry is generic to credentials rather than a
property of any one type, so the column belongs with the entry table and not
with any of the line tables carrying type specific particulars; the ledger
follows the same division for the same reason. Defining it now and leaving it
unused costs nothing and saves a migration later. WP3 covers the behaviour.

It replaces the write path for issuance and no other. Revocation, expiry and
last use still write `api_keys` directly, so for the duration the ledger is
complete about beginnings and silent about endings. **That is an interim and
should not be read as the ledger being authoritative**, which it becomes only
when every path that changes a credential goes through it.

The divergence this leaves is asserted rather than described. A test issues a
credential, revokes it through the ledger, and requires that the server still
accepts the token, because revocation does not reach `api_keys`. It passes
today; it is written so that the day revocation joins the mirror it fails and
has to be rewritten, which is the notice wanted.

### Acceptance tests

To be written before the implementation, and at least these.

**A credential of each type round trips**, including one whose type carries no
value, which is the case a denormalized column handles by convention rather than
by structure.

**An api key issued through the journal is accepted by the existing
authentication path.** This is the test that makes the package a foundation: it
fails if the mirror is wrong in any way that matters, and it needs no new
endpoint to write.

**A presentation that is refused is recorded, and a presentation that is
accepted updates a different variable.** The two are separate assertions because
conflating them is the defect this entity exists to remove.

**Posting occurs in journal order.** `entity_model.md` makes this policy, and a
variable is where violating it gives a visibly wrong answer, so the test belongs
with the first variable rather than with the policy.

### PoC cheats

Not yet enumerated. Two are already visible. The subsequence predicate selecting
which presentations are journaled will start as a constant rather than as state
on the ledger row, which defers the ability to order that everything be
recorded. And the mirror into `api_keys` is one way, so nothing detects the two
diverging on the paths this package does not replace.

## WP5. Authenticate a holder of either kind

### Summary

An AI agent authenticates with its own credential without being a `users` row.
The subject the request runs under is built from the agent's ledger row and its
owner's roles, and the authentication path routes to it on the holder type the
credential already carries.

### Status

Not started. Depends on WP4, which put the holder pair on `api_keys` and made
issuance record it, and on the AI agent ledger carrying a state, which landed
with WP2's schema work. Nothing else blocks it.

### What forces the work

**An AI agent's key authenticates today only because the agent is a `users`
row.** `coderd/httpmw/apikey.go:480` reads `GetUserByID` on the holder before
any AI agent specific code runs, so the moment the agent stops being a user that
call fails first and everything after it is unreachable. This is therefore not
preparation for rewriting the identity code. It is the thing that has to be true
before that rewrite can be tested at all.

**The subject needs less than expected, and the rest is computed.**
`coderd/httpmw/apikey.go:524` builds an AI agent's subject from **the owner's**
roles and then relabels it with the agent's subject type and username. The
agent's own users row supplies exactly three things: a username, a status, and a
deleted flag. Two of those are already `ai_agent_ledger.state`.

The username is nowhere else, `ai_agents` having no name column and the ledger
none either, and **it is not going to be given one**. Eric's position, stated
before this package was written: a live function should generate names for AI
agents rather than a column persisting one.

**What the function reads is the origin the agent was created in**, which the
ledger now records as a pair. Storing a rendering and storing the fact it
renders are different things, and only the first was ever objected to. So the
subject is assembled from the ledger row, the owner's roles, and a computation
over a fact the ledger holds.

### New behavior

- A subject built for an AI agent from its ledger row and its owner's roles,
  with no users row for the agent involved anywhere in the construction.
- A display name for an AI agent, computed from its identity rather than stored,
  so that nothing persists it and nothing has to keep it in step.
- The four holder reads in `coderd/httpmw/apikey.go` branching on holder type
  rather than assuming a user.
- `AsUserIDUnchecked` retired on the authentication path, which is the first
  place the marked cheat is removed rather than counted.

### New data

**An origin, as a `(type, identifier)` pair, on the AI agent's creation entry
and folded onto its ledger row.** It is a fact about the creation event rather
than a description of where the agent currently runs, so an agent that moved
would keep the one it was created in. That is what keeps it clear of the `run`
candidate under Derived in `entity_model.md`, which exists to hold embodiment
apart from durable identity: if runs become entities, each run has its own
origin and the identity keeps this one.

The pair is required by "A reference standing where a foreign key would stand
is a pair", the thing an agent is embodied in being a workspace or a chat and
no single table holding both. It is `text` with a `CHECK` rather than the
`ai_agent_origin` enum that already exists, which is the standing proof of
concept call, and reusing that enum would couple this schema to a definition
this work does not own without buying anything.

**The display name is not data.** It is computed from the origin and the
agent's identifier. The question this package originally posed, whether a name
belongs on a ledger or on a creation entry, does not arise for a name nothing
stores.

### What the origin forced

**The AI agent journal takes the normalized form.** Recording an origin makes
`create` an operation with particulars where the others have none, which is the
heterogeneity that decides the form. More is coming: `transfer` carries a new
owner, a different shape again.

This was not foreseen when the package was written and is the largest single
piece of it. The cost was smaller than it looked, because the multiline
machinery had exactly one query and no caller.

### Milestones

1. **Build the subject.** An AI agent gets an `rbac.Subject` constructed from
   its ledger row, its owner's roles, and a computed display name. Verifiable on
   its own, without the authentication path calling it.
2. **Route to it.** The four holder reads branch on holder type, and a
   credential held by an AI agent reaches the subject built in milestone 1.

### What this package does not do

**It does not gate on the grant.** Whether an authorization exists is a
different question from which subject to build, and the position that an
unauthorized agent should get no subject at all changes what authentication
returns rather than how it resolves a holder. Three sites build an agent subject
and they differ, per that finding in `rewrite_rbac.md`, so the gate wants its
own package or wants to land with the identity rewrite.

**It touches no table originating in the AI identity code.** The agent it
authenticates is one this work created, through `entity.CreateAIAgent`, which
writes a ledger row and issues a credential and never writes `users`. That is
what makes the acceptance test possible before that code is rewritten.

**It retires `AsUserIDUnchecked` on the authentication path and nowhere else.**
The remaining sites stay, and the count in `rewrite_rbac.md` stops being a
single number at that point.

### Acceptance tests

**An AI agent created by this work authenticates against a running server.** No
users row exists for it. The request reaches a handler, and the subject it runs
under carries the owner's roles, the AI agent subject type, and the agent's
display name. This is the whole package in one test, and it fails on either
milestone being wrong.

**A credential held by a user authenticates exactly as before.** The branch must
not change the path it did not add. Existing tests cover this, and the point of
naming it is that no new test is owed.

### PoC cheats

One is visible: the subject is built from the
owner's roles, which is what the code does today and is not obviously right. An
agent inherits everything its owner can do, narrowed only by scope, and nothing
records that the inheritance happened. Keeping it is deliberate, since changing
it is a question about authorization rather than about authentication.

## WP6. Record entailed operations without an actor

### Summary

The two lapse paths stop naming an actor and start naming the entry that
entailed them, and `entity.SystemActor` goes away.

### Status

Not started, and deliberately deferred. Eric, 2026-08-23: the naming question
below is not worth thinking through while the riskier work is outstanding, and
it does not block that work.

**A known misalignment stands until this package runs.** The code writes an
actor on lapse entries and the model says an entailed operation has none. The
constant carries a comment saying so, so that nothing comes to depend on a
position the corpus has left.

### What forces the work

WP2 built the lapses before the entity model recognised entailment. It
attributed them to a fixed system actor, the best answer available under a model
with two kinds of operation and is the wrong answer under one with three. See
"Commanded, observed and entailed operations" in `entity_model.md`.

Nothing is broken in the running system. What is wrong is what the journal says
about who acted, which is the one thing a journal exists to be right about.

### New behavior

- Lapse entries carrying no actor.
- Lapse entries naming the entry that entailed them.
- `entity.SystemActor` deleted, no caller remaining.

### New data

- Actor and actor type nullable on the credential and authorization journals,
  with the `NOT NULL` replaced by a check tying nullness to the event, per "The
  actor column is nullable" in `implementation_patterns.md`. The AI agent
  journal needs no change, having no entailed transitions.
- A column on each of those journals holding the entailing entry. On the
  authorization journal it references the AI agent journal. On the credential
  journal it references the AI agent journal for `lapse`, and would reference
  the authorization journal for `discharge`, which nothing can reach yet.

### Milestones

None. The package is one change and is too small to divide.

### What blocks it

**The name `discharge` is unconfirmed.** It was coined rather than borrowed from
a source Eric has read, and he has not passed on it. The schema does not need it
until `discharge` is reachable, so this package can run without the answer, but
running it while the answer is outstanding risks writing a line table that has
to be renamed.

### Acceptance tests

**Retiring an AI agent writes lapse entries with no actor, each naming the entry
that retired it.** The existing WP2 tests assert the opposite today and are the
thing to change, which is the intended signal.

**A commanded entry still cannot be written without an actor.** This is what
`NOT NULL` was buying, and the check that replaces it has to be shown buying the
same thing, in both directions.

### PoC cheats

Expected to remove one rather than add any. The fixed system actor filed among
users was a cheat compounding a cheat, and no entailed operation wants it. The
need for system actors elsewhere is untouched.
