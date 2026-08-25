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

Complete, 2026-08-23, in one increment rather than milestones.

**Three things went differently from the plan.**

**`aiagentidentity.Create` keeps its name and composes the two steps inside
itself**, rather than splitting into `MirrorAIAgent` at the call site. Nineteen
call sites, almost all tests, would each have had to name the owner and the
creation site twice, in two vocabularies. The function is deleted whole in
phase 3, so the split would have been scaffolding for scaffolding. The two
steps are named and commented where they are, which keeps what the split was
for.

**The username retry loop is gone now rather than in phase 3.** It could not
survive joining the caller's transaction: a unique violation aborts a
transaction, so a retry inside one cannot succeed. The name is derived from the
ledger's identifier instead, half of it, the whole not fitting a username.
Collision is not defended against and Eric accepted that for the proof of
concept.

**The ledger gains a creation time, reversing an absence its own table comment
argued for.** That comment said a second copy could disagree with the first.
The argument holds equally against every other column there, all of which are
folds, so folding one more is the consistent move and the absence was the
inconsistent one. The comment is rewritten to claim two absences.

**Deleted agents are not backfilled.** There is no actor to attribute a legacy
retirement to: the system actor is superseded and nothing new is to use it, and
naming the owner would say the owner ended an agent the orphan sweep ended. A
referent still pointing at a deleted agent fails the new foreign keys loudly
rather than being papered over. Nothing in the fixtures does.

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

## WP10. The AI agent ledger becomes the only AI agent

### Summary

Every reader of `ai_agents` moves to the ledger, retirement is recorded rather
than flagged, and the table is dropped.

### Status

Complete, 2026-08-23, in six commits after WP9: `cdc7819a57`, `e379c05dd4`,
`1033c5f9df`, `de105359b2`, `171d1905c7`, `3404685fb3`.

**Written after the fact, and not after the decision.** Eric named removing
`ai_agents` as the goal while the work was still several steps away, and chose
each step toward it: retire `Resolve` first, then the write of the mirror's
flag, then the by-creation-site readers, then the sweep. So this package records
a direction that was set, rather than one inferred from what happened to get
done.

What was not known in advance is that it is one package. WP9 ended with the
table reduced to a mirror and this looked like several; it turned out to be one
shape applied six times, and is recorded as one.

### What forces the work

WP9 left the ledger authoritative and the mirror still read by eight queries.
While anything reads the mirror, two records describe one agent and only one of
them is maintained by the entity functions.

### What was done

**Resolution reads the ledger.** `aiagentidentity.Resolve` returns the ledger
row in place of the mirror's, and checks `state` where it checked `deleted`.

**Retirement was moved first, because it had to be.** Resolution moved and
revoked agents began authenticating, all three revocation paths having written
only the mirror. See "A fact's writes move to its new home before its reads do"
in `implementation_patterns.md`.

**Two by-creation-site queries** carry the readers that looked an agent up by
what it was created in: one for the live agent of a site, one for the latest
whatever its state, the second being what tells a chat tree that never had an
agent from one whose agent is gone.

**The AI agents endpoint** reads a by-owner ledger query joined to `users` for
the username, and reports `state != active` as `deleted`. Its response is
unchanged in every field.

**The sweep reads the ledger** and takes its idempotency from `state = 'active'`
rather than from the mirror's flag. Key revocation became a delete by holder,
which works because `api_keys.holder_id` is deliberately not a foreign key.

**The table and the `ai_agent_origin` enum are dropped**, migration 000589, with
no data carried across: every row was written by code that exists only on this
branch.

### Acceptance tests

**A concurrent resolution of one workspace yields one agent.** Eight racers,
and without the workspace row lock all eight create their own, every run.

**A retired agent does not resolve, and does not authenticate.** Asserted for
each of the three revocation paths.

**The sweep retires an orphan in the ledger and revokes its keys**, through a
real purge run rather than against the queries.

**The endpoint's response is unchanged**, which its existing test asserts field
by field.

Each of the two tests written for this had its negative control run: removing
the behaviour makes the assertion it exists for fail.

### PoC cheats

**The event is `kill` at all three revocation paths**, which overstates every
one of them. An ownership change is noticed lazily by the next resolution, a
prebuild claim is not an order to end the previous agent, and an orphaned agent
is the corpus's `lapse` almost by definition. Eric, 2026-08-23: use `kill` and
record it.

**The sweep's actor is `SystemActor`**, whose own comment says nothing new
should use it. Eric: this sounds like custodian territory and the analysis is
deferred. It is the third use WP6 has to undo.

**The derived username is 42 characters**, over the 32 `codersdk.NameValid`
states. Nothing enforces it: the column is plain text and the paths that
validate are a user-supplied rename and OIDC login, neither of which an AI agent
reaches. The name is now one derivation shared with the authorizer's friendly
name, which is what the length buys.

### What this package does not do

**It does not remove the agent's `users` row.** Six places route on
`users.kind = 'ai_agent'`, and the mirrored username is read by the endpoint and
by the authorizer. Removing it means collapsing the two agent paths in
`httpmw/apikey.go`, which means `MintKey` minting with
`holder_type = 'ai_agent'`, after which the subject is built from the ledger
identity rather than from the owner's roles and decoration. **That is a change
to what an AI agent is authorized to do**, and is held pending a security
finding Eric has not yet released.

## WP11. The credential ledger issues

### Summary

The credential an AI agent actually presents is minted through the ledger, and
then the key it is carried on names its holder.

### Status

Complete. Milestones 1 and 2 on 2026-08-23, milestones 3 and 4 on 2026-08-25.
Milestone 4's minting change went with milestone 2 and was not noticed until
2026-08-24; only its removal was left.

### What forces the work

**The ledger records a credential nobody presents, and does not record the one
everybody does.** `entity.CreateAIAgent` issues a password credential, and
`aiagentidentity.MintKey` writes `api_keys` directly, so every agent carries two
credentials: an inert one in the ledger and the `api_key` that authenticates
every request, absent from it.

**The mirror that would fix this is already written and has never run.**
`IssueCredential` takes `CredentialTypeAPIKey` and writes both the ledger and
`api_keys` in one transaction, which is WP4's work. `CredentialTypeAPIKey`
appears in two test files and nowhere else, so that path is exercised by tests
alone.

### Four milestones, and why

Each lands on its own and each is separately revertible. The last was meant to
be the only one that changes an authorization decision, standing alone so that
when it went wrong nothing else was in the same change.

**That did not hold.** The holder type rides on issuance, so milestone 2 carried
it and the authorization change landed there. The separation was a property of
the plan and not of the code, and nothing in the milestone's own description
would have revealed it.

**The first is about where new work lands, not about the ledger.** An AI agent's
credential is minted in one place and ended in four, so issuance is already
protected against a new call site going somewhere wrong and revocation is not.
Until that is fixed, work on the identity code entrenches `api_keys` further
every time something new has to end a credential, there being nowhere else for
it to go.

### Milestone 1: revocation gains a single door

**Done, 2026-08-23.** The sites that delete an AI agent's keys call
`aiagentidentity.RevokeKey` or `RevokeAllKeys`, which do exactly what those
sites did.

**No ledger write, no behaviour change, nothing moved.** A refactor, and first
because it is the cheapest thing that stops the surface widening: new code
ending an agent's credential has an obvious right place, and milestone 3 becomes
a change to one function rather than to four.

**Four sites, not the five this package first said.** Of the two in
`provisionerdserver`, only `deleteAIAgentSessionToken` is an agent's; the other
is `deleteSessionTokenForUserAndWorkspace`, which ends the workspace owner's
token and is a human credential. The four are `aiagentidentity/workspace.go`,
`aisandboxes.go`, `dbpurge.go`, and that one site in `provisionerdserver.go`.

**Two functions rather than one**, because there are two operations. Three sites
end one named credential and the sweep ends every credential an agent holds, and
collapsing those into one call with an optional name would hide which was meant.

**Six deletion sites remain outside the door and all are human**: the token
endpoint, two in login, the workspace owner's session token, and two in the
OAuth2 provider.

### Milestone 2: issuance moves

**Done, 2026-08-23.** `MintKey` issues through `IssueCredential` with
`Type: CredentialTypeAPIKey`, and the mirror keeps `api_keys` correct behind it.
The credential every AI agent request carries is in the ledger for the first
time.

**Verification is untouched.** It still reads `api_keys`, splits a token and
compares a digest, and knows nothing of any ledger. See "Issuance can move to
the journal before authentication moves" in `rewrite_rbac.md`, which is the
position this milestone realises.

**But the subject built from the credential did move, and that was not
intended.** `MintKey` issues with an AI agent holder, `apiKeyHolderType` maps
that to `ai_agent`, and the mirror writes it. `httpmw/apikey.go` routes on
exactly that, so every key minted from this point reaches the branch WP5 added
and gets its subject from the ledger identity rather than from the owner's roles
with decoration. **This is milestone 4's change and it arrived here.**

It was reported at the time as touching no authorization, which was wrong. What
was checked is that verification still reads `api_keys`; what was not checked is
which branch the key then takes. The tests cover it, minting through `MintKey`
and asserting the AI agent subject, so the change is exercised. It was simply
not announced.

**Nothing is lost by dropping `apikey.Generate` from this path.** It defaults an
empty scope set and allow list and validates scopes; `validateProfile` is
stricter on all three, requiring both rather than defaulting them, rejecting the
broad scopes, and refusing a wildcard.

**The actor is the agent's owner**, which is what creation records and for the
same reason. Nobody commands this minting as such, a build or a sandbox creation
reaching it as a consequence, so the owner is the closest true attribution until
an actor kind exists for the party that does.

**The mirrored row is read back through the ledger**, which knows the key id the
credential was mirrored under. Reading it back by token name would fail for a
profile that has none, and `validateProfile` does not require one.

### The expiry cheat this milestone required

**`MintKey` expires an AI agent's key after 24 hours and the ledger holds no
expiry at all**, its mirror writing a stand-in for never. Routing minting
through the ledger unchanged would have converted every AI agent credential from
a token that expires in a day into one that never does, silently and with every
test still passing.

`APIKeyCredential` therefore gained `MirrorLifetime`, read by the mirror and by
nothing else: not folded, not journaled, and held by no ledger row.

**It preserves function until expiry gets a proper treatment, which supersedes
it.** Expiry was raised and deliberately left unsettled; when the model holds
it, the fact moves to the ledger and the field goes. Eric, 2026-08-23.

The acceptance test asserts the 24 hours survive, and was checked against its
absence: with the lifetime removed the key expires in 9998 and the assertion
fails. Every other assertion in that test passes either way, which is what makes
the check worth having.

### Milestone 3: endings move

**Most of this arrived without being written.** `RetireAIAgent` lapses every
valid credential of an agent whatever its type, so once milestone 2 put the
api_key credential in the ledger, all three retirement paths began posting its
ending. What remains is endings where the credential dies and the agent lives.

**Between milestone 2 and this one the ledger is complete about beginnings and
silent about deaths**, which `rewrite_rbac.md` names as the mistake the staging
makes available. Splitting them takes that state deliberately rather than by
oversight, and this milestone is what bounds how long it lasts. **Nothing should
read the ledger as authoritative in between.**

### The four sites, and what each is

Enumerated on 2026-08-24 after the milestone twice named the wrong number. Each
is stated with what the credential is, since as of milestone 2 every credential
here is journalled in the ledger and mirrored into `api_keys`, and ending one
means posting to the ledger and deleting the mirror.

**Site 1, sandbox deletion. A `discharge`.** `deleteWorkspaceAgentAISandbox` in
`aisandboxes.go`, `tigre` only, Jon Ayers 2026-08-11. The credential was
accessory to the sandbox it was issued for. **This is the only site whose cause
is written in the same transaction**, the sandbox's soft delete being one of the
handler's own writes.

**Sites 2 and 3, workspace stop and workspace delete. A `revoke`.** One call to
`revokeAIAgentSessionTokens` in `provisionerdserver.go` serving both
transitions, Jon Ayers 2026-08-10 and 08-11.

These were first classed `discharge` and are not. The call runs in
`acquireProtoJob`, when the build job is picked up, so the workspace has neither
stopped nor been deleted and **the material ending has not happened yet**. What
has happened is the decision, captured in the build's transition. Eric,
2026-08-24: the captured intent is the better explanatory hook, and `revoke` is
then more accurate. It is commanded and carries an actor, for which the
candidate is the build's initiator; **that it is reachable at this site is
unverified.**

**Site 4, the opted-out start build. Dead, and to be deleted.** The `default:`
arm of the start switch, introduced with that switch in `9071fd979b` as the else
of a two-case decision rather than for its own sake.

It can never end anything, which was checked rather than assumed. The
`ai-ws-<workspace>` credential is minted in exactly one place; every path to
that mint designates the workspace first; designation is never cleared, its only
writer setting a value and never null. The arm runs only when the workspace is
not designated, so the credential cannot exist when it runs. A workspace whose
agent was made by the sandbox route is the near case, and that agent holds only
`ai-sb-` credentials, so the lookup finds nothing.

### What this milestone builds

**Implement `discharge`.** Done. `entity.DischargeCredential`, entailed, taking
no actor, over the shared `invalidateCredential`. It does not copy
`LapseCredential`'s signature, which takes an actor and refuses to run without
one; a status comment on that function records the rework this anticipates.

**The two-field entailing reference.** Done, migration 000590. An entry
reference and an annotative text, exactly one set on a discharge, with the actor
made nullable and a check tying its absence to the event. Site 1 fills the text,
a sandbox not being an entity.

**One transition, and the conflation is recorded rather than avoided.** The
grounds for discharge are four rules reaching one state, so a complete model has
four transitions. This milestone writes one, with the annotation distinguishing
them, which "Transitions that reach one state may be conflated, and a complete
model splits them" permits while the model is being made. The conflation is
recorded in the credential machine's reading in `entity_model.md`.

**The endings get their own function, and `RevokeKey` is unchanged.** Eric,
2026-08-24, choosing this over moving the mirror deletion into the lapse. A
discharge cannot be posted inside `RevokeKey`, which is also how a retirement
deletes the mirrored key after the credential has already lapsed, and a
retirement must not post both. With sites 2 and 3 reclassified, `RevokeKey`'s
remaining callers are two revocations and two retirement cleanups, and its name
says what it does.

**Site 1's transaction and ordering.** Done. The handler's three writes are one
transaction and the sandbox's ending is recorded before the endings that follow
from it, so each follows something already true. Nothing inside reads what the
earlier writes wrote, so the order carries meaning and not behaviour.

**Sites 2 and 3 need no transaction with their cause**, the cause being a
decision already recorded elsewhere rather than a write in this request. What
must be atomic is the posting and the mirror's deletion.

**No handling for a key without a ledger credential.** Every AI agent key is
minted by `MintKey` and every `MintKey` goes through the ledger, so a key
without one cannot arise. The lookup's own error is the loud failure.

**The scope is AI agent credentials only.** The six human deletion sites stay as
they are, which keeps this package to the one holder kind the proof of concept
scopes.

**Both rotations are excluded**, and extracting their common function belongs
with the rotation work in WP13. This milestone does not depend on it.

### Acceptance tests for milestone 3

**Sandbox deletion posts a discharge**, whose entry carries no actor and an
annotation naming the sandbox that was destroyed.

**Workspace stop and workspace delete post a revocation**, whose entry carries
the build's initiator.

**A retirement posts a lapse**, and neither a discharge nor a revocation, so
that the three endings stay distinguishable in the record.

**Every ending deletes the mirrored key as well as posting**, the two being one
act, and a credential ended in the ledger while `api_keys` still authenticates
is the divergence this milestone exists to prevent.

**A discharge says what entailed it, or is refused.** `EntailedBy` takes exactly
one form, and neither both nor neither is accepted.

**Nothing outside the endings and the drops deletes a key an AI agent holds**, a
structural assertion and what milestone 1 bought.

**Three of these were stated wrongly before the sites were enumerated.** The
list said three discharges, said a lapse names the retirement entry when it
names nothing, and said a rotation leaves a usable credential throughout when it
deletes before minting and leaves a real gap. They are corrected here rather
than quietly, because each was a claim about the code that the code did not
support.

### Milestone 4: the key names its holder

**The minting half is done and was not meant to be done here.** `MintKey` mints
with `holder_type = 'ai_agent'` because the holder type rides on issuance, so
this landed with milestone 2 on 2026-08-23. Every key minted since routes to the
branch WP5 added, `AIAgentRBACSubject`, and takes its subject from the ledger
identity.

**So the authorization change is live**, in the part of this work that was to be
taken slowly. The surviving branch builds the subject from the ledger identity;
the branch it displaced builds it from the owner's roles and decorates the
result. The two do not produce the same subject, and the difference is the point
rather than a detail to be preserved. What is different from the plan is only
that nobody chose the moment.

**Done, 2026-08-25.** The branch that fetched a user and read its kind is gone,
along with the direct-assignment subject construction inside it. The user path
now reads the user for existence alone, so that a key naming a user who is not
there is answered as an invalid credential rather than as a server error.

**Nothing can reach it again.** `MintKey` is the only minting path for an AI
agent and issues through the ledger, whose mirror maps an AI agent holder onto
`ai_agent`. There is no path that gives an agent a user-holder key.

**One test changed what it asserts.** `TestAIAgentMissingMetadataFailsClosed`
built a users row of kind `ai_agent` holding a user-holder key and asserted the
refusal. That state is no longer expressible, a key with a user holder being a
user's key whatever its user's kind. The fail-closed property is asserted where
it now lives: a key naming an AI agent the ledger does not have.

**The finding recorded in `security_findings.md` about the API key subject's
cached AST is carried by those same lines**, and deleting them is where it is
answered rather than merely moved.

**How this was found is worth keeping.** It was reported at the time as touching
no authorization, and the error was checking that verification still reads
`api_keys` and inferring from that that nothing downstream had moved. The
question that would have caught it is not "does authentication still work" but
"which branch does this key now take". That is the second consequence in this
package to arrive unannounced, after milestone 3's endings, and both were found
by being asked rather than by looking.

### Acceptance tests

**An AI agent's credential is ended in one place**, which is a structural
assertion rather than a behavioural one and is what the first milestone buys.
Met: no deletion of a key an AI agent holds occurs outside those two functions.

**Every credential an AI agent presents has a ledger row**, asserted for a
workspace agent's session token and a chat agent's key.

**An ending on an AI agent path reaches the ledger**, so that the ledger is not
merely complete about issuance. One assertion per ending rather than per site,
there being one site left.

**One authentication path for AI agents, not two.** Asserted by the older branch
being gone rather than by behaviour, since behaviour is what changes.

### What this package does not do

**It does not move human credentials.** The six deletion sites outside AI agent
paths keep writing `api_keys` directly, and `api_keys` remains what
authenticates a request. This package makes the ledger authoritative for one
holder kind, which is what the proof of concept scope asks for.

## WP12. An AI agent stops being a users row

### Summary

The agent's `users` row goes, and with it the last thing the `ai_agents` table
left behind.

### Status

**Complete**, 2026-08-25. An AI agent has no `users` row, `users` has no `kind`
column, and `user_kind` is gone. `aiagentidentity.Create` returns
`entity.NewAIAgent`, the ledger's own value, rather than a mirrored row.

The window between milestones 2 and 3 lasted one commit and is closed. It is
recorded under milestone 2 rather than removed, because what closed it is worth
keeping: user administration cannot reach an agent, there being no row to name.

### What forces the work

The ledger is authoritative for what an AI agent is, and a `users` row still
supplies its name, carries a status two sites check, and is what five sites read
to decide an agent is an agent. Facts about one entity live in a table the
corpus holds should not file it. See "A system actor is stored as a user because
there was nowhere else to put it" in `entity_model.md`, which is the same shape.

### The five sites are four different problems

Re-measured 2026-08-25, after milestone 4 removed the sixth. Treating them as
one list is what hid the sequencing.

**`aibridgedserver.go:823` is the same problem WP11 milestone 4 solved**, in
another file: it fetches a user and reads its kind to decide whether to resolve
an AI agent. The answer is the same, to decide by the credential's holder type.
This is the only remaining instance of that pattern.

**`aiagentidentity.go:377`, inside `Resolve`, is the keystone.** The kind check
and the users-row fetch behind it are what produce `ResolvedIdentity.AgentUser`,
and two consumers still read it: `chattool/subject.go` and
`httpmw/workspaceagent.go`, each for the agent's status and its name. `Resolve`
cannot stop reading a users row until they stop reading `AgentUser`.

**`apikey.go:67` and `:221` are guards, not routing.** They refuse to create an
API key for an AI agent user. They are live today, an administrator being able
to name the agent's row, and become unreachable when the row goes rather than
needing a replacement.

**`users.go:1158` is a notification detail.** It suppresses the personal
notification about a status change, an agent having nobody to notify. It goes
with the row, and is the one site where a second, similar suppression elsewhere
in notifications is worth looking for.

**A fifth consumer is not in that list.** `GetAIAgentsByOwner` joins `users` for
the username, so the AI agents endpoint reads the row too, for a value rather
than for a decision.

### Milestone 1: aibridged decides by holder type

**Done**, 2026-08-25. `aibridgedserver.go` branches on `key.AIAgentID()` rather
than on `user.Kind`, which was the last site with that pattern.

**A test caught the difference, which is the best evidence the change does
something.** `TestAuthorizationAIAgentOwnerLiveness/AI agent identity missing`
built a users row of kind `ai_agent` and a key whose holder type said `user`.
The old code believed the row and refused; the new code believes the key and did
not. The setup now states the holder type, so the case verifies what it always
meant to: an agent whose ledger row is missing is refused.

### Milestone 2: status moves to the ledger

`chattool/subject.go` and `httpmw/workspaceagent.go` read the agent's `state`
rather than the users row's `Deleted` and `Status`, and take the name from
`entity.DisplayName` rather than the row's username.

**This is what frees `Resolve`.** With `AgentUser` unread, the kind check and
the users-row fetch inside it go, and `ResolvedIdentity` loses a field.

**Done**, 2026-08-25. Both consumers take liveness and the name from the ledger,
`ResolvedIdentity` lost `AgentUser`, and `Resolve` no longer fetches a users row
for the agent or checks its kind. The name comes from `ResolvedIdentity.Name()`,
which calls the same `entity.DisplayName` the mirror's username was written
from, so it is the same string by construction.

**Neither consumer needed a replacement check.** Both were testing the users
row's `Deleted` and `Status` after `Resolve` had already refused anything but an
active ledger state. A second check against a mirror is a second opinion able to
disagree with the authority, so they went rather than moved.

**The status values are not the same fact, and the answer was that they are not
interchangeable.** A users row can be suspended or deleted; a ledger row is
active or retired. `users.go:1158` proves an agent's row is reachable: it
suppresses the personal notification on a status change *because* `PUT
/users/{user}/status` reaches an agent. Nothing there retires the agent.

**Milestone 3 dissolves that rather than answering it.** With no users row there
is nothing to suspend and the ledger's state is the only status there is. Hence
the window recorded under Status.

### Milestone 3: the row goes

**Done**, 2026-08-25, with migration 000592. `Create` writes no row and returns
`entity.NewAIAgent`; the AI agents endpoint computes the username; the two
`apikey.go` guards and the `users.go` notification suppression went with the
row, all four having existed only because an agent could be named as a user.

**Three things came up that this package had not recorded.**

**`aibridge_interceptions.initiator_id` referenced `users` and had to stop.** It
is `NOT NULL` and, for an AI agent's key, the initiator *is* the agent, so the
reference could not survive the row. Dropping it is the change
`api_keys.holder_id` already took and for the same reason: the column names
whoever acted, and not every actor is a person. **What is given up is the
database's guarantee that an initiator exists**, and reconciling interceptions
against the two ledgers is what replaces it. Nothing does that yet.

**Dropping the column silently took two check constraints with it**, neither
about AI agents. `users_email_not_empty` and `users_service_account_login_type`
both named `kind` only to exempt an agent from a rule meant for people, and
Postgres drops any constraint depending on a dropped column. Both are restored
without the exemption, which makes them stricter than they were: there is no
longer a kind of user they do not apply to. Nothing would have reported the
loss; the generated `check_constraint.go` shrinking is what caught it.

**`aibridgedserver.IsAuthorized` fetched a users row for every key holder.** For
an agent that fetch would now fail, so the agent path returns before it, with
the name computed from the ledger. The user path is unchanged below it.

**Three tests changed what they assert, and the reason is the same in each.**
The AI seats and dormancy exclusions used to rest on a `kind = 'human'` filter;
they now rest on an agent having no users row at all, and the seats case asserts
the foreign key refusing it, which a filter could never do. The notification
case asserted that suspending an agent skipped the personal notification; it now
asserts that suspending one is not possible. The api key guard case asserted
403; it now asserts 404, the endpoint resolving no user before a guard would
have run.

**There are seventeen `kind = 'human'` filters, not six.** Six are in
`users.sql`; the rest are in `groupmembers.sql` (five), `insights.sql`,
`organizationmembers.sql`, `aiseats.sql` and `aiseatstate.sql`. Dropping the
column touches six files.

**The rows must go before the filters do.** A filter removed while agent rows
still exist would hand those agents group membership, roles and seats. Deleting
the rows first makes every filter vacuous before it is deleted, which is what
makes the removal a no-op rather than a change of behaviour.

**`GetAuthorizationUserRoles` is answered.** Its filter, `users.sql:662`, makes
the query return no row for an AI agent, which is how "an AI agent has no roles
of its own" is enforced in SQL. With no users row the query returns no row
anyway, for a better reason, so the filter is redundant rather than load
bearing. That holds only under the ordering above.

**What this frees.** Those filters become no-ops. **The `kind` column and the `user_kind` type go entirely**, not just the
`ai_agent` value: the type is declared on this branch and has exactly two
values, so with one gone the column is a constant. Removing an enum value is
ordinarily expensive; this one is not, there being no deployment that has it.

### New data

None, and nothing structural holds the row in place: `ai_agent_ledger.id` has no
foreign key to `users`.

### Acceptance tests

All met.

**No AI agent has a `users` row**, and `InsertAIAgentUser` is gone.

**The four consumers work without one**: the AI bridge, the chat tool subject,
the workspace agent middleware, and the AI agents endpoint.

**An agent's displayed name is unchanged**, which is what makes the username
column's removal invisible rather than merely tolerable.

**A retired agent is refused everywhere a suspended or deleted users row was
refused before**, which is the assertion that the status substitution in
milestone 2 was sound.

## WP13. The credential journal's structure

### Status

**Complete**, 2026-08-25. Four milestones: the atomic group, `reissue` left
documented rather than built, the rotation rewrite, and a retirement becoming one
event. The fourth was the further journal structure work this package was opened
expecting to gather.

**Milestone 1 has landed**, 2026-08-25, with migration 000591. An entry now
carries the party and the moment, and a line carries the credential and what
happened to it.

**Milestone 2 is closed and wrote no code**, 2026-08-25. What it would have
journalled turned out to be unsound throughout, so it became documentary. See
its section, and `credential_expiration.working_state.md` for the reasoning.

**Milestone 3 has landed**, 2026-08-25. `RotateKey` exists, both sites call it,
and a rotation is now one entry naming both credentials.

**Milestone 4 has landed**, 2026-08-25, and closes the package. A retirement is
one event on both journals rather than an entry per thing ended.

**One item has left, complete.** The two-form entailing reference was pulled
into WP11 milestone 3 and shipped with migration 000590: `entailed_by_entry` and
`entailed_by_annotation` on the credential journal, never both, and required on
a discharge. The position it needed is in `implementation_patterns.md` under
"The reference has two forms, and one of them is words".

### The order these are taken in

**Milestone 3's first step is available now** and depends on nothing: extracting
the common rotation function is a pure refactor. Its second step needs the
atomic group, so the cheap independent piece can go first. The package's shape
is one refactor, one restructuring, and two things that sit on the
restructuring.

**Milestone 2's placement is the open question below**, not its content.

### Milestone 1: an entry becomes an atomic group

**Still true after migration 000590**, which added the entailing reference and
made the actor nullable without touching the entry's identity: the primary key
is `(entry_id)`, and `subject` and `event` remain single and `NOT NULL`.

**The schema claims a capability its primary key forbids.** The comment on
`credential_lifecycle_journal.entry_id` reads: "An entry may occupy several
lines sharing this value, expressing an atomic group: rotation issues one
credential and revokes another as a single event, so that no interval passes
without a valid one." The table has `PRIMARY KEY (entry_id)`, one `subject` and
one `event`, so an entry names exactly one credential and exactly one thing that
happened to it. Two subjects in one entry is not expressible.

**What forces it.** `entity_model.md` holds that rotation is issuing one
credential and revoking another as one entry, and that recording it as two
entries would assert the gap the overlap exists to prevent. The bar against
rotation in the proof of concept was lifted on 2026-08-24, so the position now
has to be implementable.

**The shape this implies.** The entry carries when and who; the lines carry
which credential and what happened to it. That moves `subject` and `event` off
the entry, which touches every credential write rather than only rotation.

**One actor per entry survives untouched, and that is a position rather than an
artifact.** `entity_model.md`, "One actor per entry, not two": delegation being
recorded once and separately, an entry needs no principal beside its agent, and
recording the actor alone stays uniform whether a sandbox holds an AI agent or a
user. One subject per entry is not a position. It is what the table happens to
do, and it is the only thing standing between the schema's own comment and what
the schema permits.

**`subject` and `event` are a pair and move together.** Moving the subject alone
looks sufficient and is not: a rotation is an `issue` and a `revoke`, so a single
`event` column leaves it inexpressible however many subjects an entry can name.

**So an atomic group is one party, one moment, several subjects, each with its
own event.** The corpus's own phrasing implies the split, calling rotation "two
subjects, one entry" and never suggesting two actors.

**It pays beyond rotation.** `RetireAIAgent` lapses a holder's credentials as
one entry each. Under lines that is one entry with a line per credential, which
is what happened: one event ending several credentials.

#### What landed, and the one thing given up

**Migration 000591.** `credential_lifecycle_journal_line` keyed `(entry_id,
line)` carries `subject` and `event`; the entry keeps both dates, the actor pair
and the entailing reference. The subject index moved with the column. The
`api_key` line table's foreign key moved from the entry to the line, which is
what stops a type specific line claiming a number the entry never issued.

**It matches a position already held.** `implementation_patterns.md`, under
"Entry level values are written once, on line zero", already says line level
means the event and the subject. The credential journal was the outlier.

**Two checks were dropped and not reproduced.** Migration 000590 made a
discharge carry no actor and name what entailed it, as checks over `event`,
`actor` and the entailing reference. With `event` on another table no check can
see both sides. Eric, 2026-08-25: **drop them and consign the property to
reconciliation.** The door in `coderd/entity/credential.go` still writes only
what the model permits, so enforcement is what was lost. The surviving check
tests only columns that stayed, an entailed entry naming its cause in one form
and never in both.

**The reader returns a flat row rather than an embedded entry.** `sqlc.embed`
would have made every caller write `.CredentialLifecycleJournal.Actor` for
nothing. Naming the columns keeps the row a view of one line with its entry,
which is what a reader of a credential's history wants.

**Acceptance test**: `TestAnEntryIsAnAtomicGroup` in `coderd/entity`. One entry,
two lines, an `issue` and a `revoke`, one actor and one moment. Each credential's
history returns the line naming it and the entry identifier that says the two
happened together. It exercises the store directly, because the schema is what
this milestone changed and nothing in production writes a two line entry yet.

#### What the other two journals want, corrected on measurement

I first recorded that the authorization journal needed the same schema change.
**It does not.** It is already in the denormalized form with `PRIMARY KEY
(entry_id, line)`, entry level values on line zero under checks, and two insert
statements expressing that. It has been able to carry a multiline entry all
along; nothing wrote one. The credential journal took the normalized form
because it has a type specific line table, which is what
`implementation_patterns.md` means by heterogeneity deciding the form.

So the remaining work was never schema. It was that **a retirement is one event
and was written as many**, on both journals. That became milestone 4.

**The AI agent journal wants neither**, one agent being one subject per event.

#### A pre-existing gap this surfaced

`TestMigrateUpWithFixtures` fails because **no migration fixtures exist for any
credential or authorization table**. It listed eight such tables before this
change and nine after, so the new table joins the gap rather than causing it.
The gap dates to WP2 and WP4 and is not this package's to close, but nothing was
recording it.

### Milestone 2: `reissue` is documented and left alone

**This milestone no longer writes code.** Eric, 2026-08-25, withdrew for this
item only the requirement that the solution use the journal, on the evidence
that the mechanism it would have journalled is unsound throughout and that
repairing one part of it ahead of the rest would misrepresent the whole. What
remains is to say so, accurately, in the places a later reader will look.

`reissue` is on the credential machine, `valid` to `valid`, commanded, and
nothing posts it. The site is `coderd/x/chatd/synthetickey.go:57`, which extends
a chat agent key's expiry with `UpdateAPIKeyByID`, writing a new `ExpiresAt`
straight to `api_keys`.

**The exemption is narrower than it sounds, and it is checkable.** No state
transition escapes the ledger; what escapes is a change to an expiry that
changes no state. **The ledger holds no expiry to be falsified**: rows are
inserted with a null `expires_at`, both folds write other variables, and nothing
updates it. So the extension contradicts nothing the ledger says.

**Expiry is provisionally treated as a fact about how a credential is managed
rather than as part of the credential**, which is why it moves without an entry.
The reasoning, the alarm clock story that supplies it, the corpus passage now in
dispute, and the findings behind the judgement that the mechanism is unsound are
all in `credential_expiration.working_state.md`. They are not repeated here.

**Acceptance is documentary.** A comment at the site saying the extension is
outside journal control, why that is consistent, and that it is provisional; the
working state file recording the rest; and P11 in `security_findings.md` for the
one finding that is a defect rather than a design gap.

**What was going to be here, and is now deferred with the expiry work.** Whether
`reissue` becomes a line like the operations in milestone 1, which decides
ordering rather than membership. Whether the definition is too narrow for having
been written from one instance. Whether reissuance covering more than an expiry
is a conflation. Whether the material form of an authenticator distinguishes
anything. And the `last_used` race at the same site.

### Milestone 3: the rotation rewrite

**Step one is done**, 2026-08-25. `regenerateAIAgentSessionToken` in
`provisionerdserver` and `rotateAISandboxSessionToken` in `aisandboxes` were
substantially the same: `DropKey` by the profile's token name, then `MintKey`
with that profile, differing only in which profile and how the agent is reached.
`aiagentidentity.RotateKey` is that pair, and both call it.

**What it buys is that the reason is in the name.** Written out at each site,
what the two statements amounted to was legible only by inference, and `DropKey`
cannot be told why it was called. Saying it by which function is called is the
same rule the endings door follows.

It was split out of WP11 milestone 3 on 2026-08-24, having been put there on the
mistaken view that rotation was a kind of ending.

**It is behaviour preserving, and the escalation is the part worth checking.**
`provisionerdserver` escalated for the drop and not the mint, which one function
cannot reproduce. Escalating the whole call is safe because `MintKey` escalates
internally for everything it does, `Resolve` included, so the mint sees no
context it did not already construct for itself.

**One latent divergence closed with it.** The sandbox drop derived its token
name from `sandbox.WorkspaceID` while the mint beside it used the `workspaceID`
argument. They agree at the only call site. Had they ever differed the drop
would have missed and the mint would have left a second key, so unifying them on
the argument removes a way for the pair to come apart rather than introducing
one.

**`deleteAISandboxSessionToken` went with it**, having had no other caller.
`deleteAIAgentSessionToken` stays: `revokeAIAgentIdentity` is its second caller,
and that one is dropping a key on a retirement rather than rotating.

**Step two is done**, 2026-08-25. `entity.RotateCredential` issues a credential
and revokes the one it replaces as a single entry: one party, one moment, line
zero revoking and line one issuing. `RotateKey` routes through it, so both call
sites got the change without knowing about it.

**It needed a refactor the milestone did not name.** `IssueCredential` opened
its own entry, so nothing could issue on a line of an entry someone else opened.
It is now `prepareIssuance`, which mints and validates outside the transaction,
and `postIssuance`, which writes one issuance as a line of an entry already
open. `invalidateCredential` split the same way into `postInvalidation`. Both
splits are behaviour preserving and the existing tests carried unchanged.

**Nothing to supersede is treated as an issuance rather than refused.** A
profile whose key has been swept, or which has never been minted, reaches
`RotateKey` the same way, and refusing it would make callers ask a question they
have no reason to ask.

**Acceptance test**: `TestRotationIsOneEntry` in `coderd/aiagentidentity`. A
mint then a rotation; the superseded credential is invalid with a `revoke`, the
replacement valid with an `issue`, and both name the same entry. Negative
control run: reverting `RotateKey` to the drop and mint pair fails it.

#### Why the code deletes before it inserts, which is an implementation note

`api_keys` carries a unique index over a holder and a token name for minted
tokens, so two rows of one name cannot sit there at once. The superseded row
therefore goes before the replacement arrives, and being in one transaction that
ordering is invisible to every other reader.

**This says nothing about the overlap `entity_model.md` describes.** Eric,
2026-08-25: mirrored tables are transient, present only while the code is being
worked on, and will not appear in any final version of a journaled system. A
constraint of the scaffolding cannot bear on a position about the record. The
ledger holds both credentials and the overlap is the ledger's.

The note is here to explain the write order and goes when the mirror does.

### Milestone 4: a retirement is one event

**Done**, 2026-08-25, and it is what milestone 1 was for.

**What forced it.** Retiring an AI agent ends every authorization naming it and
every credential it holds. A holder ceasing does not end its credentials one at
a time, so those endings are one event, and both journals recorded them as an
entry apiece. `audit_approach.md` defines an atomic group as one event and not
several that happen to coincide; this was several that coincided, asserted as
such.

**What changed.** `LapseCredential` and `LapseAuthorization` became
`LapseCredentials` and `LapseAuthorizations`, each taking the set the one ending
ends and writing one entry with a line apiece. Both had exactly one caller, the
retirement, so this replaced them rather than sitting beside them. They take
ledger rows rather than identifiers, the posting being conditioned on the
reference each row was read at and the caller having read them already.

**An empty set writes nothing.** No credential ended means no event happened,
and an entry with no line would assert one did.

**It made a documented dead statement live.**
`InsertAuthorizationLifecycleJournalSubsequentLine` carried a comment saying
nothing called it, that the proof of concept wrote no multiline entry, and that
it should be read as documentation rather than a tested path. A retirement is
now that path, and the comment says so.

**Acceptance test**: `ARetirementIsOneEvent` in `coderd/entity`. An agent with
two credentials and two authorizations is retired; each journal shows one entry
with both subjects on separate lines, and the authorization journal's later line
carries null where the entry level values sit, which is the denormalized rule
holding. Negative control run: writing an entry per credential fails it.

### Not written yet

Nothing. Everything this package opened with is done, and the further journal
structure work it expected to gather turned out to be milestone 4.

## WP14. An AI agent's own endpoints

### Summary

An `/aiagents/` namespace paralleling `/users/`, whose first and possibly only
mutating route retires an agent.

### Status

Not started, opened 2026-08-25. **Not part of WP12**, which removes a row where
this adds a surface. Mixing them would make the row's removal wait on an
authorization decision.

### What forces the work

**WP12 blocks the user-administration routes against an AI agent target**, which
is right: those routes are about people, and one of them renames, which would
break a name the system now derives. But suspending an agent's users row is
today the only manual way to stop one, so blocking it leaves no off-switch.

The absence is not created by that guard. It is made explicit by it: the
capability was an accident of an agent being a users row, was indiscriminate,
being available to any user administrator with no notion of the agent's sponsor,
and had already stopped working on the API key path.

**`kill` has no honest caller.** Its three sites, an ownership change, a prebuild
claim and the orphan sweep, are all recorded cheats: none is a party ordering an
agent's death. An administrator retiring one is what the transition was defined
for, so this gives an existing concept its first real use rather than adding a
concept.

### The shape, which is syntax and not yet a definition

```text
GET    /api/v2/aiagents                          list, admin-scoped
GET    /api/v2/aiagents/{aiagent}                one agent
PUT    /api/v2/aiagents/{aiagent}/status/retire  the kill switch
```

`GET /users/{user}/ai-agents` stays as the owner-scoped list, as
`/users/{user}/workspaces` coexists with `/workspaces`.

**A route and a verb do not define an endpoint**, which is why the first
milestone below is about semantics and writes no code. Two questions are already
apparent, and neither is visible in the syntax above.

**`{aiagent}` resolves an identifier and not a name.** An agent's name is
derived and exists for logs and display; admitting it as a lookup key would make
it an identifier again, which is what removing it from `users` was for.

### Only a commanded transition gets a route

The machine has `create`, `finish`, `kill` and `lapse`. `finish` is observed and
`lapse` entailed, so neither is anybody's to request and neither can have one.
`create` has its paths already. **That leaves exactly one mutating route.**

The surface falls out of the model rather than being designed against it. The
rule generalises as a **candidate**: an entity's commanded transitions are the
ones that could have a route, and which of them should is a separate question.
The observed and entailed ones are excluded outright, which is the part the
model settles.

### Milestone 1: what these endpoints mean

**No code.** The syntax above says what may be called and nothing about what
happens, who may call it, or what the system will decline to do.

**Two questions are already apparent.**

**Whether retiring is refused when it would break something.** A retire bricks a
designated workspace's next build, for the reason under Risks below. The system
could perform it anyway, could refuse it, or could refuse it unless overridden.
Eric, 2026-08-25: refusing is a live option, and choosing it means **setting out
a policy about when an operation is declined for what it would break**, which is
a general position and not a property of this endpoint.

**Whether an AI agent may call any of these. It may not.** Eric, 2026-08-25:
definitely not, on any of them, with future analysis able to permit it later.
This settles the privilege-laundering risk by rule rather than by scope
arithmetic, and it applies to the reads as much as to the retire: an agent has
no business enumerating its siblings.

**What else this milestone owes.** Who may call each endpoint, given that
ownership is not authorization. What a retire takes with it, the mirrored keys
being the known case. What is returned, and whether a retired agent is still
readable, which it must be if the record is to be worth keeping. Whether a
second retire is an error or a no-op.

### Milestones 2 to 4: one endpoint each

**One milestone per endpoint, implemented separately.** Eric, 2026-08-25: the
analysis requirements make discoveries certain, and separate landings are what
let each be addressed where it arises rather than at the end of a batch.

**Milestone 2, `GET /aiagents/{aiagent}`.** The smallest, and it is where the
`{aiagent}` extractor and the authorization decision first have to be real. Both
are needed by everything after it, and this is the cheapest place to be wrong
about them.

**Milestone 3, `GET /aiagents`.** Adds listing, and with it whatever filtering
and paging the answer to "admin-scoped" turns out to require.

**Milestone 4, `PUT /aiagents/{aiagent}/status/retire`.** The motivating route
and the only mutating one, taken last because it is the only irreversible one
and because the two before it settle the extractor and the resource.

**The order is discovery-first, not value-first.** The kill switch is why the
package exists, so if the absence of an off-switch starts to matter, milestone 4
can move ahead of the reads at the cost of settling the extractor and the
authorization under the riskiest route.

### Risks

**A retire bricks a designated workspace's next build, and this is
demonstrable.** `workspaces.ai_agent_id` is set once and never cleared. A
designated workspace whose agent is retired reaches `resolveDesignatedAIAgent`,
which returns the retired row and reports designated, then
`regenerateAIAgentSessionToken` calls `MintKey`, which calls `Resolve`, which
refuses a retired agent, and the job fails. The same sequence is documented in
`provisionerdserver.go` for the prebuild claim path. **This endpoint would make
a latent defect reachable on purpose**, so clearing the designation is part of
retiring, not a follow-up.

**An agent could reach its own kill switch, or another's.** An AI agent acts on
its owner's roles, narrowed by scope and allow list, so a route authorized by the
owner's permissions may be satisfied by an agent's credential. An agent that can
retire an agent is the privilege-laundering shape `AI_AGENT_IDENTITY_SPEC.md`
warns about for issuance.

**Answered by rule in milestone 1**: no AI agent may call any of these
endpoints. The risk is not that the rule is hard to state but that it is easy to
implement as scope arithmetic and thereby to leave a path open. It wants a check
that does not depend on getting an allow list right.

**The authorization story does not exist and touches the policy.** An AI agent
has no RBAC resource of its own; today it is covered by `ResourceUserObject`
because it is a users row, and after WP12 it is not. Deciding who may retire
one, its owner or an administrator or both, is a decision the corpus will not
make for us: ownership is not authorization. Adding a resource reaches
`policy.rego`, which is the riskiest surface in this work.

**Retirement is terminal and there is no un-retire.** The machine has no
transition out of `retired`, deliberately. So the endpoint is irreversible by
construction, and an accidental or hostile call cannot be undone; the remedy is
a new agent, which is not the same agent. That is an argument for a narrow
authorization rather than against the endpoint.

**It is new public surface.** Swagger annotations, `codersdk` methods, generated
TypeScript, and a contract someone may build against. The `/aiagents/` namespace
also invites the two read routes to grow filtering and pagination, which is how
a one-route package becomes four.

### Not in scope

**Un-retiring, and anything resembling it.** If reconstituting an agent is ever
wanted it is `dormant`, which the machine reserves and does not implement.

**A composition to write once.** Retiring should take the mirrored keys with it,
as the orphan sweep does with `DropAllKeys`, or the agent stays authenticable
until somebody notices its credentials. That is the same composition the sweep
performs and it belongs beside it.
