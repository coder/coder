# Entity Model

Recorded 2026-08-06. This document states the entities this work deals with,
what identifies them, and how they relate. It is the entity-specific
counterpart to `poc_audit/audit_approach.md`, which is deliberately independent
of any particular entity.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built
on top of them, which is offered for challenge rather than settled.
**Findings** records verifiable facts about the existing codebase.
**Open** records questions not yet answered.

The scope is currently the entities the audit work has had to reason about.
A document of this kind should eventually encompass the whole system, but
extending it that far is not attempted here.

## Established

### Terminology

Terms used with a specific meaning. The three senses of "agent" are defined
here rather than in the audit approach document, since they are facts about
entities rather than about audit.

**Actor.** An entity that can act and interact with other actors, in the sense
the term carries in a use-case description. The defining category is anything
that can act and be held responsible. A user is an actor. The control plane,
coderd, is an actor. A sandbox is not an actor: it holds actors, but it does
not act.

**Sandbox.** A process that runs inside a workspace and holds an actor. It is
a mechanism rather than a role: it can hold any actor, it may transiently hold
none during its own creation, and it may hold a user rather than an AI agent.
A sandbox sits at a lower level of abstraction than the workspace startup
diagrams represent, and so does not appear as a participant in them.

**`workspace_agent`.** The long standing Coder concept: the process that runs
inside a provisioned workspace and provides that workspace's services,
including SSH access, port forwarding, terminal connectivity, application
serving, health reporting, and resource statistics. It authenticates to the
control plane with its own token and reaches it over the tailnet. Sub-agents,
such as the one created for a devcontainer, are also `workspace_agent` entities
carrying a reference to their parent.

This name is pervasive. It is the prefix on eleven tables and appears in nine
enum types in the schema, it is used throughout Go identifiers, and it is the
value `workspace_agent` in the audit `resource_type` enum.

`workspace_agent` is always written in full and is never shortened to "agent".
Its name reaches the codebase through the general software sense of an agent as
a process acting for a system, which itself derives from the legal sense below.
The codebase's usage does not depend on that relation.

**AI agent.** A software system that pursues goals over multiple steps with
latitude in choosing its own actions, typically driven by a large language
model. Claude Code is one. Competing examples include GitHub Copilot and OpenAI
Codex.

An AI agent ordinarily stands in the relation of principal and agent described
below: it acts on behalf of a person, within some scope of authority, and its
acts have effects attributed to that person. The two senses therefore overlap
in substance and not merely in spelling. That is what makes fixing this
vocabulary necessary rather than merely tidy.

**Agent, in the relation of principal and agent.** From the law of agency in the
English legal tradition. An agent is a party authorized to act on behalf of
another party, the principal, such that acts taken within the scope of that
authority bind the principal as though the principal had acted directly.
Authority may be actual, whether express or implied, or apparent, where the
principal's own conduct leads a third party reasonably to believe that authority
exists. A principal may also ratify an unauthorized act after the fact and
thereby become bound by it.

Three parties are in view, not two: the principal, the agent, and the third
party with whom the agent deals.

The relation is fiduciary. Among the duties the agent owes the principal is the
**duty to account**: to keep and render a faithful account of what was done
under the delegated authority. That duty is the origin of the obligation the
audit approach is concerned with. Double entry bookkeeping arose in the same
mercantile tradition, as the means by which agents and factors accounted to
their principals. Audit and agency are therefore not separate subjects that
happen to meet. The account exists because authority was delegated.

**Usage of the bare word.** Unqualified, "agent" means an agent in the relation
of principal and agent. That sense is the oldest of the three and the origin of
the others, so it holds the unmodified word. The other two senses are always
qualified: `workspace_agent` written in full, and "AI agent" written in full.

### Identifiers in source code

The terminology rules above govern prose and rendered text. They are
**recommended for source code identifiers as well**, in both senses:

- `workspace_agent` written in full, in table names, column names, Go
  identifiers, and package names.
- `ai_agent` written in full, in the same places, for the AI agent entity.
- **Where a short form is genuinely needed, it is `ws_agent`.** Not `agent`.
  A short form that still says which kind of agent is meant costs four
  characters and removes the ambiguity entirely.
- **Unadorned uses of `agent` should be renamed for clarity over time**, each
  to whichever of `ws_agent` or `ai_agent` it actually means. This is expected
  to happen gradually, as the code containing them is touched for other
  reasons, rather than as a single sweep.

For `workspace_agent` the recommendation completes an existing practice for
table names and establishes a new one for columns. See the finding below: all
eleven tables in the family carry the full prefix, but among columns the short
form is the more common of the two, and package names use it throughout. That
is why this is a recommendation rather than a mandate, since honouring it means
renaming.

Some unadorned uses are out of reach and should be excluded from the
recommendation rather than left as apparent violations. `coder_agent` is a
Terraform resource type owned by the provider and written by template authors,
and `CODER_AGENT_TOKEN` and `CODER_AGENT_URL` are part of the interface the
workspace environment presents. Renaming any of those breaks users rather than
clarifying anything.

For `ai_agent` the cost is nil, because no such entity exists yet. Fixing the
identifier form before the first table is written is the cheapest this decision
will ever be, and it is the only one of the three senses that can still be got
right for free.

### Actors in scope

The actors this document currently covers:

- **User.** A person, and ordinarily the principal on whose behalf an AI agent
  acts.
- **coderd.** The control plane.
- **`workspace_agent`.** As defined above.
- **AI agent.** As defined above.

A sandbox is an entity but not an actor, per the definition above.

The wider system contains further actors, including provisionerd, the Terraform
CLI, the Terraform providers, and the Docker daemon. They appear as participants
in the startup diagrams but are not of immediate relevance to audit in the PoC.

### Sandbox occupancy

A sandbox holds an actor. The relationship is not fixed to a kind:

- It may hold an AI agent.
- It may hold a user, such as the workspace owner.
- It may hold none, transiently, during its own creation.

A sandbox could, as a technical possibility, hold more than one actor, but that
would defeat the whole point of a sandbox, which is to isolate actors from each
other. A workspace may hold both a user and a sandbox containing an agent, but
by contrast a workspace is not an isolation mechanism.

### Identity independence

**An AI agent has its own identity, independent of any sandbox and independent
of any workspace.**

At present the lifespan of an AI agent will fall entirely within that of a
particular sandbox. That may change, for instance by retaining a session
identifier so that the state of a previous AI agent can be reconstituted.

**The initial implementation may assume containment. The database structures
may not.** No schema may be built that assumes an AI agent belongs to exactly
one sandbox, or to a workspace at all.

### Attribution of auditable actions

Any AI agent with the capacity to act externally will have received prior
authorization from a delegating principal, ordinarily a user.

**That delegation is recorded elsewhere and does not need to be recorded per
event.** An auditable event needs to record only the actor behind the action.

## Derived

Reasoning built on the positions above. Offered for challenge.

### Occupancy is a relationship with its own lifecycle

If a sandbox can hold different actors over its life, and an AI agent can
outlive or move between sandboxes, then which sandbox holds which actor cannot
be a column on either entity. A nullable sandbox reference on the agent, or an
actor reference on the sandbox, records only the present and silently loses
history when it changes. That is precisely the assumption the identity
independence position forbids.

What the position implies instead is a distinct representation of occupancy
with its own start and end, so that several may exist for one AI agent over
time and none may exist at a given moment.

This is structurally the same problem as P5 in
`poc_audit/security_findings.md`, where a one to one column for the
`workspace_agent` credential makes overlap impossible and leaves no lifecycle
to record. Same shape, same remedy. Worth noting that it is an easy mistake to
make twice.

It also pays off for audit. Placement and removal become persistent state
changes with somewhere to be recorded, and the question of which sandbox held
which actor at a given time becomes answerable, which is the stronger
coherence property listed as open in the audit approach.

### One actor per entry, not two

The attribution position settles a question the audit approach lists as open.
Since delegation is recorded once and separately, an entry does not need to
carry both a principal and an agent. It carries the actor.

This also disposes of a complication that would otherwise arise from a sandbox
being able to hold either an AI agent or a user. If entries had to name both
parties, the two cases would need different shapes, because a user occupying a
sandbox is the principal rather than an agent of one. Recording the actor alone
is uniform across both.

### Durable identity and ephemeral execution

Retaining a session identifier so that a previous AI agent's state can be
reconstituted implies that the identity persists while the running thing does
not. That is two entities rather than one, even if the first implementation
only ever creates one execution per identity.

This is flagged for the same reason as the occupancy point: a schema that
treats an AI agent and its run as a single row forecloses reconstitution as
surely as a sandbox reference forecloses movement.

## Findings

Verifiable facts about the existing codebase, recorded for reference.

**No sandbox entity exists.** There is no table named for a sandbox or a
microVM in the schema.

**No AI agent entity exists.** There is extensive AI machinery, including
`chats`, `chat_messages`, `aibridge_interceptions`, `boundary_sessions`,
`tasks`, and `ai_seat_state`, but nothing in the schema that *is* an agent.
Both entities are green field.

**The name "agent" is taken in the schema.** It refers exclusively to the
`workspace_agent`, across eleven tables and nine enum types, and the audit
`resource_type` enum already contains a `workspace_agent` value. An audit
resource type named `agent` would sit next to it and require every reader to
disambiguate.

**Writing `workspace_agent` in full is the practice for table names and not
for columns.** All eleven tables in the family carry the full prefix. Column
names are close to evenly split, and the short form is in fact the more common
of the two: seven are `workspace_agent_id` and eight are `agent_id`, across
fifteen tables.

There is no discernible rule behind the split. It does not even hold within the
`workspace_agent_*` family itself, where `workspace_agent_scripts`,
`workspace_agent_devcontainers`, `workspace_agent_log_sources`, and the two
context tables use the full form, while `workspace_agent_stats` and both
resource monitor tables use the short one. Outside the family it is likewise
mixed: `boundary_sessions` and `task_workspace_apps` use the full form, while
`chats`, `workspace_apps`, `workspace_app_stats`, `workspace_app_statuses`, and
`jfrog_xray_scans` use the short one.

Go package names use the short form throughout: `agent`, `agent/agentscripts`,
and `coderd/agentapi`.

**The product already uses "agent" in the AI sense.** For example
`site/src/pages/AgentsPage` is the AI chat UI. The collision is already live
between frontend vocabulary and schema vocabulary.

**A system actor is stored as a user because there was nowhere else to put
it.** The account that creates prebuilt workspaces is a row in `users`, added
by `000308_system_user.up.sql` and referred to as
`database.PrebuildsSystemUserID`. It is not a person and has no credential a
person uses.

It is a user because `users` was the only table holding identities that other
tables could point at. That is the same gap the `(type, identifier)` pair works
around: there is no union of identity tables to refer into, so an identity with
no home is filed under the nearest one that exists.

**Post proof of concept: it should not be a user.** Any work that gives system
actors their own identity should take this row with it, rather than leaving a
non-person filed among people and every query about users carrying an exception
for it.

## Open

- **Sandbox ontology.** Whether a sandbox is a resource the workspace provides,
  like an app or a `workspace_agent`, or an independent entity that happens to
  live in a workspace, like a task whose workspace reference is nullable.
- **Sandbox state machine.** The legal states and transitions. Whether a
  sandbox is stop and start able or create once and destroy once. Whether
  failure states are distinguishable from terminal states. Whether there is an
  idle reaping analogue, which would be a third independent clock alongside
  user dormancy and workspace dormancy.
- **Creators and authority.** Which parties may create a sandbox, and what
  actor is recorded for each: the provisioner during a build, a user through
  the API, or something inside the workspace acting on a user's behalf.
- **Ownership transfer.** `ClaimPrebuiltWorkspace` rewrites `owner_id` and
  `name` on an existing workspace row, transferring it from the prebuilds
  system user to a real user while keeping its identifier and build history. If
  a prebuilt workspace can carry sandboxes, their audit trail spans two owners,
  one of which cannot log in.
- **Run as an entity.** Whether the ephemeral execution of an AI agent is
  modelled separately from its durable identity, per the derived section above.
- **Naming.** What the AI agent entity is called in the schema and in the audit
  `resource_type` enum, given that `agent` collides with `workspace_agent`.
- **coderd's identity.** Whether the control plane is modelled as an actor
  entity with an identity of its own, or remains implicit as it is today.
