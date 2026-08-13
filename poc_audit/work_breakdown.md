# Work Breakdown

Recorded 2026-08-12. This document breaks the coding work into **work
packages**. Each section below is one work package.

"Work package" is the term from work breakdown structure practice for the
lowest level element of a decomposition, sized to produce a verifiable
deliverable. It is used here in preference to "unit", which collides with unit
testing, and to "task", which collides with the `tasks` entity in this
codebase.

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

Complete. The acceptance test passes.

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
