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

**A number means a package exists. It does not mean anybody has scheduled
it.** Several here are written and unstarted, and one is written and unlikely to
be started before a demo. Work analysed far enough that redoing the analysis
would be waste, but which nobody has decided to do at all, would sit at the end
under a Candidates heading and take no number until somebody decided. Nothing is
in that state today.

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

Complete as of 2026-08-23. An AI agent created by this work, with no row in
`users` anywhere, authenticates against a running server and has a subject built
for it from the ledger.

**Two agent paths now coexist, and that was not foreseen.** The identity code's
agents are still `users` rows, and their keys carry `holder_type = 'user'`, so
routing on the holder alone bypassed them and broke three of their tests. Both
branches therefore stand: a key whose holder is an AI agent reaches the ledger,
and a key whose holder is a user whose kind is `ai_agent` reaches the path that
was already there, unchanged. The second goes when the identity code does.

**"me" cannot name an AI agent**, which the acceptance test found and which is
larger than one route. See the finding below.

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

### What the acceptance test found: "me" cannot name an AI agent

`me` is a path or filter value standing where a user is named, alongside a
username and a user id. It expresses self reference: the user this request is
made by. **Seven places in coderd resolve it**, each independently taking the
credential's holder for a user id, in `httpmw/userparam.go`, `workspaces.go`,
`searchquery/search.go` twice, `audit.go` twice, and `httpapi/queryparams.go`.

For an AI agent the three things that word carries disagree. The slot demands a
user, the self reference denotes the agent, and the resolution looks for a row
that does not exist. **So the question is not what to return. It is that `me`
sits in a user typed slot and the requester is not a user.**

**The characteristic failure is silence, not an error.** Only the route that
fetches a row gives a 404. The rest filter: `owner:me` becomes an id that owns
nothing, the query succeeds, and the answer is an empty list. An agent asking
for its own workspaces is told it has none, which is indistinguishable from
having none. Both are asserted in the acceptance test, the silent one because it
is the one that misleads.

**Nothing has to ask for this.** The credential is delivered into the workspace
for the `coder` CLI to use, whose own comment calls it the key for in workspace
CLI actions. The CLI names things as the requester's by default: `cli/ssh.go`
and `cli/configssh.go` set `Owner` to `me`, and forty one `codersdk.Me` sites
stand in that package. An agent running `coder list` writes no filter and sees
nothing.

**One candidate can be ruled out now.** Resolving `me` to the owner would make
the word denote somebody other than the requester, and it would do so on a
filter the agent never wrote: `coder list` inside an agent's workspace would
print the owner's entire inventory. Substituting the sponsor is worse when the
word arrives from a tool rather than from a caller.

**What is not ripe** is the choice between rendering an agent into the user
shaped answer and refusing the slot to it. That turns on whether an AI agent
owns user scoped resources of its own, settings and keys and preferences among
them, which the entity model has not reached. Deciding it here would decide it
by side effect on one route.

### Acceptance tests

**An AI agent created by this work authenticates against a running server.** No
users row exists for it, which the test asserts before proceeding rather than
assuming. It then reaches an endpoint authorized by the subject and asking for
no user by name, and finally retires the agent and requires the same token to
stop working, which puts ledger state and an HTTP response at two ends of one
assertion.

**A credential held by a user authenticates exactly as before.** The branch must
not change the path it did not add. Existing tests cover this, and the point of
naming it is that no new test is owed.

### PoC cheats

**Two agent paths coexist**, and nothing prevents an agent existing in both
senses at once. Nothing creates one that way, and nothing checks.

One more: the subject is built from the
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

## WP7. An AI agent addresses itself

### Summary

`/api/v2/aiagents/me`, answering an AI agent's questions about itself from the
ledger. The half of a pair whose absence is why the other half is ambiguous.

### Status

Not started, and unscheduled. Written now for what it settles rather than for
what it builds: with this route in play, an agent reaching `/users/me` has
somewhere else it could have asked, and the meaning of both becomes a
consequence of having two slots rather than a rule imposed on one.

**WP8 depends on this package being written rather than built**, for the reason
given there. So writing it has already done the work it was written to do, and
building it is a separate question that nothing is waiting on.

### What it settles

**The ambiguity of `/users/me` is caused by the absence of this route.** With
one self reference slot, that slot carries two meanings and picks the wrong one
for an AI agent. With two, neither needs a rule: asking under `/users` says the
question is about the party acted for, and asking under `/aiagents` says it is
about the party acting.

**The two routes resolve from the two fields the subject already has.**

| Route          | Resolves from       | Denotes             |
|----------------|---------------------|---------------------|
| `/users/me`    | `subject.ID`        | the party acted for |
| `/aiagents/me` | `subject.AIAgentID` | the party acting    |

Both from the subject, neither from the credential, so one principle produces
both rather than two decisions producing one each. They are the same pair as the
`on-behalf-of` and `performed-by` headers discussed for MCP: the headers are
those fields crossing a wire and these are those fields addressed over HTTP.

**`/api/v2/workspaceagents/me` is the working precedent**, mounted at
`coderd.go:1871`, where a party that is plainly not a user addresses itself with
the same word, in a slot that is not user typed. The shape is established rather
than invented.

### Impersonation is the mechanism, agency is the fact

An AI agent reaching `/users/me` gets the owner because its subject carries the
owner's identity, and the request is indistinguishable from the owner's own.
That is impersonation, and it is the right word for the mechanism.

**It is not the right word for the relation.** An agent acting within conferred
authority is not pretending to be its principal; its acts bind the principal
because a grant says they may, and impersonation is precisely acting without
that. The two descriptions sit at the two levels this corpus already separates:
impersonation is what capability looks like, agency is the institutional fact,
and the authorization ledger is what makes the first legitimate and the second
revocable.

**That is practical rather than terminological.** An agent whose grant has
lapsed is still mechanically impersonating and is no longer institutionally an
agent. The gap between those two is what a gate on the grant closes, and this
route is where an agent could find out which side of it that agent is on.

### New behavior

- `/api/v2/aiagents/me` returning what the ledger holds about the requesting
  agent: its identifier, its owner, its origin, its state, and its computed
  display name.
- **A user asking is refused.** A user acts as no agent, and the asymmetry is
  the direction of the relation showing through rather than an awkwardness.
  Agents act for users; users do not act for agents.

### New data

None in the database. The ledger already holds every field, and the display
name is computed.

**An RBAC resource does not exist**, however. Nothing in `rbac/policy/policy.go`
names an AI agent, so there is no resource type to authorize a read of one
against. What that resource is, and whether reading one's own record needs a
permission at all, is the one real design question in this package.

### Use cases that would earn it

- **Am I still authorized?** The state and the grant. An agent checking before
  a long or irreversible action, and the case most likely to justify the route.
- **What am I designated to?** The origin, which is what the workspace
  designation boundary compares against.
- **Who am I acting for?** The owner, so an agent can say so in its own output
  rather than inferring it.
- **What am I called?** The display name, so that logs an agent writes match
  logs the server writes about it.

### An open question this route raises about itself

If a gate on the grant refuses unauthorized agents everywhere, an agent that has
lost its grant cannot ask whether it still has one. **This may be the single
endpoint that should answer anyway**, so that an agent learns why it is being
refused rather than only that it is. Deciding that is deciding what an
unauthorized agent may still be told, which is a question about disclosure and
not about this route.

### Acceptance tests

**An AI agent asking about itself is told its state, its owner and its origin**,
none of which comes from a users row.

**A user asking is refused**, and refused as a category error rather than as a
missing record.

### PoC cheats

Not enumerated; nothing is built. One is foreseeable: the route would resolve
its subject through `subject.AIAgentID`, which is set by `Subject.AsAIAgent` on
two of the three paths that build an agent subject and not on the third. Until
that is uniform, the route would work on some credentials and not others.

## WP8. Resolve "me" from the subject

### Summary

The seven places that resolve `me` take it from the credential's holder. They
should take it from the request's subject, and refuse when that subject is not a
user.

### Status

Not started, and unscheduled. Eric, 2026-08-23: probably better after the demo,
and the demo may need it, so the analysis is kept rather than the conclusion
rederived.

**It depends on WP7, and the dependency is unusual enough to state exactly. It
depends on WP7 having been written, not on WP7 having been built.** No code here
calls anything there.

What WP7 supplies is the reason. On its own, resolving `me` to the owner reads
as a choice to discard the agent's identity, and the obvious objection is that
an agent can then no longer refer to itself. With `/aiagents/me` written down,
the answer is that it still can, elsewhere, and the meaning of each route
follows from there being two slots rather than from a rule imposed on one. The
rule is the same either way; only its justification is missing without WP7, and
a rule whose justification is missing is one a reviewer is right to refuse.

### What forces the work

`me` stands where a user is named, alongside a username and a user id, and
expresses self reference: the user this request is made by. Every site resolves
that by taking the credential's holder for a user id.

**For an AI agent the word's parts disagree.** The slot demands a user, the self
reference denotes the agent, and the resolution looks for a row that may not
exist. Which of those gives way is the question, and this package answers it
with a principle that already governs elsewhere.

**The principle: an AI agent interacting outward uses its owner's identity.**
Eric's collaboration group holds this, and the code already implements half of
it. `AIAgentRBACSubject` sets `subject.ID` to the owner, so every authorization
decision for an agent is already made as the owner. Attribution rides a separate
channel, `subject.AIAgentID`, which is the inverse of an on behalf of header and
was discussed as a `performed-by` field for MCP.

**So the current state is incoherent rather than merely incomplete.**
Authorization says the owner and `me` says the agent, and those are two rows.
That is the `users` table's overloading surfacing in a public facing word: `me`
means the row holding the credential, where everything else means the party
whose roles apply.

### The sites, and which is already right

| Site                           | Resolves from | Origin    |
|--------------------------------|---------------|-----------|
| `httpmw/userparam.go:91`       | holder        | pre tigre |
| `workspaces.go:171`            | holder        | pre tigre |
| `searchquery/search.go:126`    | holder        | pre tigre |
| `searchquery/search.go:131`    | holder        | pre tigre |
| `audit.go:67`                  | holder        | pre tigre |
| `httpapi/queryparams.go:198`   | holder        | pre tigre |
| `audit.go:74` (`on_behalf_of`) | **subject**   | Jon Ayers |

**The last one is the model.** It reads `httpmw.UserAuthorization(ctx).ID`,
which for an agent is the owner, and it is the newest of the seven, written
during the attribution work. The other six are older code that had no reason to
consider a non user holder. `audit.go` therefore contains both meanings of `me`
three lines apart.

### What the check found

`subject.ID` is a users row for every subject that reaches a handler today. The
workspace agent path sets it to the workspace owner, or to a bound AI agent's
owner, then builds through `UserRBACSubject`.

**It is not a users row for every subject that could.** `UserAuthorization`
reads the same context slot `dbauthz.As` writes. Most system subjects carry
`uuid.Nil`, but `SubjectTypeFileReader` carries a well formed UUID that is not a
user, and `SubjectTypePrebuildsOrchestrator` carries one that is.

So the rule is **resolve from the subject and refuse when the subject is not a
user**, not merely resolve from the subject. A system subject in a `me` filter
would otherwise parse cleanly and match nothing, which is the same silent
failure by another route. This is the `AsUserIDUnchecked` lesson again: an
identifier does not say what it denotes.

### A workspace_agent has its own `me`, in a slot that is not user typed

**`me` already denotes two kinds of party in this codebase.** Under
`/users/{user}` it is the API key's holder. Under `/api/v2/workspaceagents/me`,
mounted at `coderd.go:1871`, it is the bearer of an agent token, and the whole
agent API hangs off it: `/me/rpc`, `/me/ai-egress-policy`, the rest.

That one works, for a reason that names the defect in the other. **The segment
is not in a user typed slot.** It is a literal in a path about workspace agents,
so nothing has to be widened, synthesized or refused, and a party that is
plainly not a user addresses itself with the same word every day.

So a workspace_agent is excluded from the *user* `me`, not from `me`. It is
excluded twice over: `ExtractWorkspaceAgent` puts no API key in the request
context, so `userparam.go` answers 400, "Cannot use \"me\" without a valid
session", and the filter sites read the key through `APIKey`, which panics when
it is absent and is therefore unreachable rather than wrong. And it has no
reason to want the user slot, having its own.

**The contrast with the AI agent is the whole lesson.** An AI agent fails
quietly precisely because it does carry an API key, and a plausible looking one,
and because it was routed through the user slot rather than given one of its
own. `/aiagents/me/...` would have been unremarkable.

**Something anticipated an agent reaching user scoped resources.**
`ScopeNoUserData` exists, is `allPermsExcept(ResourceUser)`, and is applied when
a workspace agent is bound to an AI agent. An unbound agent gets `ScopeAll` and
is permitted user reads it has no route to make. **The fence is a scope and not
an identity**: it removes the permission and says nothing about who `me` would
mean, so it would not have helped had the plumbing allowed the call.

The occasion that does exist is one level down. A workspace agent hands a
session token to something inside the workspace, which then uses the CLI. That
token is an API key held by an AI agent, which is the case above. **So the
workspace agent is the delivery mechanism for that case rather than a case of
its own.**

### The general shape: a keyword in an identifier slot

A third instance settles what the family is.
`httpmw/organizationparam.go:78` resolves the literal `default` in the
`{organization}` slot, "to make it easier for single org deployments". The same
block also resolves a nil UUID to the default organization, as a workaround for
legacy provisioners, under a TODO from March 2024 saying it should have gone by
now.

**All of these are a slot typed for one kind of thing accepting a keyword that
means work it out from context.** That is sound while the context can produce
only one answer of the right type, and it is what fails when a new kind of party
appears and the slot's type does not move with it.

`default` differs in one respect worth keeping: it is implied by the deployment,
where `me` is implied by the credential. A deployment cannot disagree with the
subject and a credential can, which is why only one of the two has a problem.
The nil UUID is the same overloading again in a slot typed as an identifier
rather than as a keyword, which is why it needed a TODO and `default` did not.

**The fix this package proposes is the one `/workspaceagents/me` already
demonstrates**: resolve the keyword against the party the route is about. Under
`/users` that party is a user, so `me` is the user the request acts as, which
for an AI agent is its owner.

### Why the failure is worth fixing rather than tolerating

**It is silent.** Only `userparam.go` fetches a row and gives a 404. The rest
filter, so `owner:me` becomes an id owning nothing, the query succeeds, and the
answer is an empty list. Demonstrated in `poc_tests/credential_test.go`:
`owner:me` for an agent returns no error and no workspaces.

**Nothing has to ask for it.** The credential is delivered into the workspace
for the `coder` CLI, whose comment calls it the key for in workspace CLI
actions. The CLI names things as the requester's by default: `cli/ssh.go` and
`cli/configssh.go` set `Owner` to `me`, among forty one `codersdk.Me` sites
there. An agent running `coder list` writes no filter and sees nothing.

### What it does not decide

Whether an AI agent owns user scoped resources of its own. It makes that
question smaller: if `me` always means the owner, agent owned settings could
never be reached by that word anyway and would need a vocabulary that does not
exist yet.

### Scope against the identity code

**Five of the six edits are in code predating this branch.** The sixth,
`audit.go:67`, is not Jon's either; his line sits three lines below it and is
the reference. So this is a sweep over old code with his line as the model, and
it is separable from the identity rewrite in both directions.

**It does change behaviour for his agents**, which stop getting their own
synthetic user record back from `me`. That is the intended correction and he
should hear it as a decision rather than meet it as a surprise.

### Acceptance test

An AI agent asking for its own workspaces receives the owner's, and asking who
it is receives the owner. The assertions currently recording the opposite in
`poc_tests/credential_test.go` are what change, which is the intended signal.

## WP9. The AI agent ledger becomes the identity

### Summary

The ledger holds everything `ai_agents` holds, every column that refers to an AI
agent refers to the ledger, and the constraint the identity code enforces over
agents moves to the container it is really about.

### Status

Not started. First of the work replacing the AI identity code, and the largest
piece of data model change in it.

### What forces the work

`ai_agents` is keyed on `users.id` and duplicates the ledger almost entirely:
owner and creation site are in both, `deleted` is a coarse `state`, and
`created_at` is the creation entry's effective date. **Nothing in it is unique
except a constraint**, and that constraint is stated over the wrong entity.

Until the ledger holds the rest and the referents point at it, the ledger
describes an AI agent while another table is the one the system believes.

### New behavior

- The ledger answering owner, creation site, creation time and state without
  `ai_agents` being consulted.
- **The ledger minting the identifier**, with the users row and `ai_agents` row
  written under it by `aiagentidentity.MirrorAIAgent`. Today they mint and the
  ledger has nothing to do with them.
- Four columns referring to an AI agent referring to the ledger's identifier.
- A chat tree refusing a second live occupant.
- **Creating a sandbox occupying it, and soft deleting one vacating it.**
  Vacating has no representation today, so a sandbox that is gone and a sandbox
  that is empty are the same row.

### New data

**Rename `origin_type` and `origin_id` to `creation_site_type` and
`creation_site_id`**, on `ai_agent_ledger` and on
`ai_agent_lifecycle_journal_create`. Origin is Jon's word for the same thing and
creation site is ours, per the term list. **Define the term in
`entity_model.md` before the rename lands**, so the schema does not carry a word
the model has never defined; this is the failure mode that produced a wrong
paragraph about sandboxes.

**Add `creation_time` to `ai_agent_ledger`**, folded from the `create` entry's
effective date. Nothing is added to the journal: the effective date of a
creation is when the agent came into being, and a second column beside it would
record one fact twice.

**Add `occupancy_count` to `chats`**, `integer NOT NULL DEFAULT 0`, with
`CHECK (root_chat_id IS NULL OR occupancy_count = 0)`.

This replaces `idx_ai_agents_origin`, which enforces one live agent per creation
site as a uniqueness rule over agents. It is a fact about the site, and belongs
there per "Capacity belongs to the container" in `entity_model.md`.

**The container is the chat tree, not the chat.** `chatd.go`'s
`chatAIAgentOriginID` resolves a sub-chat to its root, so one agent has always
served a whole tree. The tree is an entity with no data structure, and the root
chat's identifier stands in for an identifier it does not have, which is why
data about the tree lives in the root chat's row. **That goes in the code as a
comment carrying a note that the comment can migrate to corpus.**

**Enforcement is a conditional update aimed at the root**, not a constraint:
incrementing where the count is zero, and no rows affected meaning occupied.
The `CHECK` is a backstop against a caller that failed to resolve to the root,
which is a bug rather than a legitimate posting, per "A ledger constraint that
can refuse a posting is the wrong mechanism".

**Add `occupancy_count` to `ai_sandboxes` on the same terms.** A sandbox is a
container of the same kind and gets the same treatment, with no root and
non-root split because there is no tree: one row is one sandbox. Enforcement is
the same conditional update, aimed at the sandbox itself.

**No `CHECK` on the sandbox count.** A ceiling constraint was considered, on
the ground that a sandbox's ceiling of one follows from what a sandbox is for
and is therefore Established rather than policy. Eric, 2026-08-23: omit it, the
table not being one we need to worry about for long. The conditional update is
the mechanism in both cases and the backstop buys little on a table expected to
become a ledger within the week.

**Today the sandbox count coincides with `deleted`, and that is a reason to
record it rather than not.** No query updates `ai_sandboxes.ai_agent_id`: a
sandbox is created with an occupant and soft deleted, never emptied. So the
count is one while live and zero after, which `deleted` already tells you.

They are different facts that currently coincide. **A soft deleted sandbox is
gone; an unoccupied sandbox is empty**, and they coincide only because nothing
empties a sandbox without deleting it. Recording both says which is which, so
that the day something does empty one, nobody has to work out which of the two
`deleted` had been standing for.

It also states the occupancy that `ai_agent_id NOT NULL` currently implies,
which is what would let that constraint go without losing the fact.

**For a chat tree the count enforces; for a sandbox it records**, and the
difference is worth knowing before someone reads the two as one mechanism. A
sandbox's occupant is a single column set at insert, so a second occupant is
structurally impossible and the ceiling protects nothing. What the count adds
there is **vacating**, which has no representation at all today: soft deleting a
sandbox empties it, and nothing says so.

### The ledger mints, and `ai_agents` mirrors

**The package does not work without this**, which was not obvious when it was
scoped. Repointing a referent at the ledger while `aiagentidentity.Create` still
writes a users identifier into it fails on the next agent created. The two
halves, repoint the referents and do not replace `Create`, cannot both hold as
stated.

**So `entity.CreateAIAgent` mints, and the two legacy rows are written under
the identifier it returns.** One identity, three tables, one transaction.

**`aiagentidentity.Create` splits rather than being renamed.** After this change
it no longer creates the identity, so a name saying it does would describe what
it used to do. What remains of it is writing two rows that mirror what the
ledger holds, and `MirrorAIAgent` says that. Its callers, `chatd.go` and
`createWorkspaceOrigin`, then read as two steps:

```go
created, _ := entity.CreateAIAgent(ctx, tx, params)
user, agent, _ := aiagentidentity.MirrorAIAgent(ctx, tx, created.ID, params)
```

Which is worth more than tidiness. **The call site shows which of the two is the
identity**, and later work deletes a line rather than untangling a function.

Three things follow, and together they are why this is the fix rather than
dropping either half.

- **The two identifier spaces become one.** A referent pointing at
  `ai_agents.user_id` points at `ai_agent_ledger.id`, they being the same value.
- **Backfill stops being a decision.** One ledger row per existing `ai_agents`
  row, under the same identifier, and every referent is already valid.
- **It is a step toward the substitution rather than a detour.** Later work
  deletes the second and third writes; nothing gets rewired.

**This is WP4's mirror with the authority reversed.** There the ledger mirrored
into `api_keys`; here the legacy tables mirror from the ledger. It carries the
same known cost, one way and silent about divergence on the paths it does not
cover, and the same mitigation: it is bounded to one increment and recorded as
an interim rather than read as the ledger being authoritative.

### The four referents

| Column                            | Today                      |
|-----------------------------------|----------------------------|
| `workspaces.ai_agent_id`          | FK to `ai_agents(user_id)` |
| `workspace_agents.ai_agent_id`    | FK to `ai_agents(user_id)` |
| `ai_sandboxes.ai_agent_id`        | FK to `ai_agents(user_id)` |
| `ai_sandbox_sessions.ai_agent_id` | column and index, no FK    |

All four come to hold a ledger identifier. `audit_logs.on_behalf_of_user_id` is
not among them: it holds the owner, who is a user, and does not move.

**Existing rows are handled by the backfill above** rather than by a decision
between backfill and discard. That choice existed only while the two identifier
spaces were separate, and the minting change removes it.

### What this package does not do

**It does not drop `ai_agents`.** Its writers are `aiagentidentity.Create` and
the orphan sweep, and replacing those is later work. What changes is which of
them is authoritative: after this package the ledger mints and `ai_agents`
follows, so dropping it later removes a mirror rather than an identity.

**It does not touch the sandbox `NOT NULL`.** `ai_sandboxes.ai_agent_id` cannot
be null, which contradicts the Established position that a sandbox may hold
none transiently. It may never bite, the sandbox row being created where the
agent is already in hand, and the table is expected to become a ledger shortly.
The occupancy count above is what makes dropping it cheap when the time comes.

**It does not give `workspaces` an occupancy count.** A workspace is a creation
site by the recorded type set, so the omission is an asymmetry rather than a
principle. It is the heaviest table in the schema, nothing in this work needs
the count there, and adding it can wait for something that does.

### Acceptance tests

**Every question `ai_agents` answers, the ledger answers**, for an agent this
work created: owner, creation site, creation time, state.

**A chat tree admits one live agent, and so does a sandbox.** A second is
refused by the posting returning no rows, not by an error from the storage
engine.

**A non-root chat cannot carry an occupancy count**, which the constraint
enforces and a test asserts, since the constraint is the only thing keeping the
column meaningful on the rows it is meaningless for.

**A soft deleted sandbox is distinguishable from a live one by occupancy**, and
not only by `deleted`. This is the assertion that makes the sandbox count worth
having rather than a symmetry with chats, and today it fails for want of
anything to assert against.

**An agent created through the identity code has a ledger row under the same
identifier**, which is what makes every referent valid and is the assertion the
mirror exists to satisfy.

**The four referents resolve against the ledger**, which is a schema assertion
rather than a behavioural one.

### PoC cheats

**A credential is issued that nobody will ever present.**
`entity.CreateAIAgent` does three things in one transaction: records the
creation, grants authorization, and issues a password credential. Once the
identity code mints through it, every AI agent acquires that credential, and
`MintKey` then issues an `api_key` credential beside it that is the one actually
used. So each agent carries two, one of them inert.

Eric, 2026-08-23: leave it. **This pass is about the AI agent entity**, and
pulling the credential apart to avoid one unused row would drag part of a later
phase into this one.

**The bundling is scaffolding, not a design position**, which matters for how it
comes apart. Eric: `CreateAIAgent` was built to exercise end to end flow of
control for WP1, and **there is no design reason to keep credential issuance in
it long term.** So the eventual split removes something that was never argued
for, rather than reversing a decision.

**The grant stays, and only for a reason that has an expiry.** Eric: it can stay
there for now, but only because all grants are universal. A universal grant
takes no parameters, so creation can imply one without deciding anything. **A
scoped grant is a decision, and a decision cannot be a side effect of creating
the thing it is about.** When a scoped grant exists, the grant leaves
`CreateAIAgent` the same way the credential does.

That conditional is easy to lose, so it goes in the code as well, on
`CreateAIAgent` itself, which is where somebody introducing a scoped grant will
be standing.

**A pre-existing gap is made more visible by this package without being caused
by it.** `MintKey` writes `api_keys` directly and never reaches the credential
ledger, so the credential an agent actually uses is absent from the ledger while
the inert one is in it. WP4 moved our own issuance onto the ledger and this path
was never ours. It is Phase 3's to close.
