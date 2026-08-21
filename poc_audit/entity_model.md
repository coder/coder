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
audit approach is concerned with. Audit and agency are therefore not separate
subjects that happen to meet. The account exists because authority was
delegated.

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

### An entity identity is always a pair

**A reference to an entity identity is always a `(type, identifier)` pair,
never an identifier alone.** This holds wherever such a reference is stored or
passed: columns, struct fields, protocol messages, function parameters.

The reason is a limit of SQL rather than a preference. Entity identities live in
one table per kind, and SQL has no way to express a reference into a union of
those tables. There is no type to declare and no foreign key to write, so a bare
identifier column cannot say what it refers to and nothing can check that it
refers to anything. The type carries what the schema cannot: a one to one map
from a value to the table holding that primary key.

That the map is redundant when identifiers are uuids, which are unique across
every table, is not an objection. A reader of a bare column would still have to
probe each table in turn to learn what a row is about, and a writer would have
nothing preventing a reference to the wrong kind of thing.

Two consequences worth stating. The type belongs to a closed set, since a value
outside it names no table and makes the reference unresolvable. And the pair
appears in both roles the journal needs, a subject and an actor, so
neither is a special case of the other.

The actors this document currently covers:

- **User.** A person, and ordinarily the principal on whose behalf an AI agent
  acts.
- **coderd.** The control plane.
- **`workspace_agent`.** As defined above.
- **AI agent.** As defined above.

A sandbox is an entity but not an actor, per the definition above.

The wider system contains further actors, including provisionerd, the Terraform
CLI, the Terraform providers, and the Docker daemon. They appear as participants
in the startup diagrams. The provisioner is also an actor in the AI agent
lifecycle below, where it can command a kill or a suspension and can observe a
finish. It is left out of the proof of concept for reasons of scope
rather than of principle.

**A reconciler is a role, not an actor.** Whoever performs a reconciliation
acts in that role, and at the outset that will be a user. Automatic
reconciliation could act under a service account, or through an AI agent acting
at the behest of a user charged with the task. Reconciliation adds nothing to
the set of actor types.

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

### How these entities arise

`poc_audit/audit_approach.md` puts the general question as one of origin: does
an event arise as a relationship, or as behaviour of a technical system? Every
change has a technical manifestation, so the manifestation settles nothing. This
section answers the question for the entities named above. It is not a
classification of everything in the system, and the entities not listed are not
yet placed.

- **User.** Arises as a relationship. No human being comes into existence when a
  user is created. What comes into existence is a relationship between a person
  and this deployment. The `users` row records that relationship and is not the
  person.
- **AI agent.** Arises as a relationship, on a subject the system itself
  embodies. See below.
- **Workspace.** Arises as behaviour of a technical system. It is provisioned,
  and it is material: a particular slice of the hardware it runs on, together
  with the state of that slice.

### Embodiment, and what a grant needs from its subject

An AI agent's subject is in the end a process on a server, and that process is
its **embodiment**. The embodiment must exist before any authority can be
granted to it. There is no granting anything to a party that is not there.

A technical system can embody an AI agent. It cannot embody a human. That
asymmetry is the whole of the difference between the two so far as a grant is
concerned. In every other respect a grant to an AI agent and a grant to a person
are the same kind of act, made to a party that already exists.

### Lifecycles are state machines either way

Entities and relationships both have lifecycles, and both are describable by a
state machine. That is the point of commonality, and it is what lets the two be
treated alike in schema and in code.

The theory exists for a practical reason: so that people can reason coherently
about institutional facts by analogy with material entities, which is the mode
of thought that comes naturally. The analogy is a working convenience and not a
claim that the two are the same kind of thing.

**Every machine states how it is embodied and how it is read**, in the section
defining it. A machine says which transitions are legal. It does not say what in
the world corresponds to one, where the transition arises, or where it is
perfected. A reader left to infer that will infer something plausible, and the
next reader will infer something else plausible. Writing the interpretation down
lets consistency be checked at the start rather than discovered as drift later.

### Commanded and observed transitions

Every transition in a lifecycle state machine is of one of two kinds, and the
kind settles whose identity the entry carries.

A **commanded** transition happens because some party decided it should. The
actor is the party who commanded it.

An **observed** transition happens of its own accord, and is recorded because
some party noticed. The actor is the party who noticed.

The kind is a property of the transition and not of the occasion. A process
returning of its own accord is never something anybody decided, so that
transition is observed whenever it occurs. Which party fills the role does vary
with the occasion, and the entry records whichever it was.

Two things follow. An entity can never be the actor of its own observed
transitions, which is the rule against an entity writing about itself arriving
by another road. And an observed transition may be recorded long after it
occurred, by whoever eventually noticed, which the audit approach addresses
under the entry's timestamp.

**Transitions are named with the bare verb**: `create`, `finish`, `suspend`.
Not `created`, and not `creating`. The imperative reads naturally for a
commanded transition and the declarative for an observed one, and English
spells the two alike, so one form serves both.

**A state machine is defined without reference to actors.** The states and the
transitions stand on their own. An actor is recorded on every entry, but that
record is kept for forensic purposes. What live operation needs from the machine
is the current state, not who brought it about.

### The AI agent lifecycle

Two machines are given. The larger is the model. The smaller is what the proof
of concept implements, and it is the larger with one state removed.

**The smaller must be derivable from the larger by deleting `dormant` and every
transition touching it, and by no other difference.** This is a backward
compatibility requirement: what a state means may not change when the machine
grows.

| State     | Meaning                                                                   |
|-----------|---------------------------------------------------------------------------|
| `active`  | The identity exists and denotes a live embodiment.                        |
| `dormant` | The identity exists, no embodiment is live, and it may be embodied again. |
| `retired` | Terminal. The identity exists only as history.                            |

`dormant` goes into the enum now, marked reserved for future use, so that
supporting reconstitution later costs no migration.

| From      | Transition | To        | Kind      |
|-----------|------------|-----------|-----------|
| none      | `create`   | `active`  | commanded |
| `active`  | `finish`   | `retired` | observed  |
| `active`  | `kill`     | `retired` | commanded |
| `active`  | `suspend`  | `dormant` | commanded |
| `dormant` | `resume`   | `active`  | commanded |
| `dormant` | `retire`   | `retired` | commanded |

**The proof of concept machine is the first three rows.**

Actors are given by way of illustration, the machine not depending on them.
`create` records the delegating principal. `kill` records the owner who
commanded it, or the provisioner taking a cluster offline. `finish` records
whoever noticed, which may be the owner, or the provisioner during a sweep, or
a user performing a reconciliation long afterwards.

**`finish` and `suspend` are told apart by intent, not by mechanism.** `finish`
means the work is done and will not resume. `suspend` means the embodiment
ended and the identity may be embodied again. A process exiting is the same
occurrence in both.

That is what keeps the two machines compatible. Were the two transitions told
apart by the process exiting, adding `dormant` would make every earlier
`finish` ambiguous in retrospect. Since the smaller machine offers no
reconstitution it never emits `suspend`, so nothing recorded under it means
anything different once the larger machine arrives.

**Proof of concept cheat.** A process exiting carries no statement of intent,
and the proof of concept reads every exit as `finish`. That is sound only while
`dormant` is unreachable. A revision supporting dormancy cannot keep the cheat,
and will need a source for the intent.

#### The proof of concept machine, written out

Deriving it is mechanical, but the result is worth setting down rather than
left to be worked out by each reader.

| State     | Meaning                                            |
|-----------|----------------------------------------------------|
| `active`  | The identity exists and denotes a live embodiment. |
| `retired` | Terminal. The identity exists only as history.     |

| From     | Transition | To        | Kind      |
|----------|------------|-----------|-----------|
| none     | `create`   | `active`  | commanded |
| `active` | `finish`   | `retired` | observed  |
| `active` | `kill`     | `retired` | commanded |

Three things to notice, none of which the derivation makes obvious. `retired`
is reached two ways and left none, so it is the only terminal state and the
machine has no cycles at all. `active` is entered exactly once, at creation,
which means an identity that has been retired is never reused. And `dormant`
is absent from the machine while present in the enum, so any code that switches
exhaustively over the enum must handle a state that cannot occur.

#### How the AI agent machine is read

`create` arises when a principal orders an AI agent into existence, and is
perfected by the entry written once the agent has been embodied. It is not
perfected at the moment of the order, because an AI agent is not identified
until it has been embodied and there is nothing yet to record the creation of.
The actor is the delegating principal. A `workspace_agent` carrying the request
to the control plane is a relay and not a party to the act.

`finish` arises when the embodying process returns of its own accord. Nobody
decides it, so the actor is whoever noticed, and the entry may be written long
after the fact.

`kill` arises when a party ends the process deliberately, whether the owner
commanding it or a provisioner draining a host.

`suspend` and `resume` are reserved with `dormant` and have no interpretation
yet, since nothing reconstitutes an AI agent.

### The authorization lifecycle

An authorization is the agency relation itself, brought into being by a grant.
It is an institutional fact in the sense the audit approach gives that term, and
it has no form apart from the record of it.

**There is no pending state.** A grant is complete once perfected, so the
relation either holds or it does not, and nothing waits in between for
confirmation. The audit approach describes a distinction between incipient and
actual authority and deliberately declines to model it. Even were it modelled it
would add no state here, since incipient authority is authority.

| State        | Meaning                                               |
|--------------|-------------------------------------------------------|
| `active`     | The relation holds. The agent is authorized.          |
| `terminated` | Terminal. The relation held once and holds no longer. |

| From     | Transition   | To           | Kind      |
|----------|--------------|--------------|-----------|
| none     | `grant`      | `active`     | commanded |
| `active` | `revoke`     | `terminated` | commanded |
| `active` | `lapse`      | `terminated` | observed  |
| `active` | `disqualify` | `terminated` | observed  |

**The proof of concept implements the first three rows.** `disqualify` is
reserved for future use. Stating it now costs a row; leaving it out would cost a
reanalysis of the whole machine later.

One terminal state, reached several ways, as in the AI agent machine. What live
operation asks of an authorization is whether it holds, not why it stopped. The
reason is carried by the transition, which is where a forensic reader will look
for it.

`revoke` is the principal withdrawing authority it granted. `lapse` is the
relation ending because it can no longer exist, a party to it having ceased to.
Common law arrives at the same place by the same route, terminating an agency on
the death of either principal or agent, and for the same reason: there is nobody
left to stand in the relation.

**In practice `lapse` will only ever mean the end of an AI agent.** Cessation of
existence covers both parties in law, but nothing here models the death of the
person standing in a `user` relationship with the system. A `user` is that
relationship and not the person, so the person's existence is nowhere
represented and nothing observes it.

A principal who has died is therefore indistinguishable, to this system, from
one who is merely away. Whatever ends their authorizations will end them by
`disqualify`, when somebody notices and withdraws their standing, and the entry
will say so rather than claim a fact nobody here is in a position to observe.

`disqualify` is the relation ending because a party, while still existing, has
ceased to be one who may hold that role. A user who has been dismissed from the
organisation still exists and retains legal capacity, and may well have revoked
nothing, but no longer stands where the relation requires somebody to stand.

**Termination is the law's word for this too.** The law of agency uses it for
the ending of authority and of the agency relationship alike, and it recognises
the same grounds this machine does: a party ceasing to exist, and a principal
withdrawing what it granted. The state name is consonant with settled usage
rather than coined here, which is the whole of the reason for mentioning it.

#### How the authorization machine is read

`grant` arises when a principal orders an AI agent into existence. That order is
performative: ordering one confers authority on the agent about to exist, and
nothing further is required of the principal for the conferral to be complete on
their side. It cannot be perfected at that moment, because there is no identity
yet to confer authority on, and an AI agent is not identified until embodied.
The entry written after embodiment is what perfects it.

**The interval between the order and the entry is required by the model rather
than tolerated by it.** A grant arises in a mind or a point of execution and is
perfected by a recording; here the recording waits on the embodiment rule, and
the wait is a consequence of two positions the corpus already holds rather than
an imprecision in either.

The actor is the principal, in the interface gesture and in the entry alike. A
`workspace_agent` relaying the request confers nothing, holding no authority to
confer.

`revoke` arises when the principal withdraws what it granted. Nothing offers
that yet.

`lapse` arises when a party ceases to exist, which in practice means an AI agent
reaching `retired`. It is observed, so the actor is whoever noticed.

`disqualify` is reserved and has no interpretation yet.

#### An AI agent is not competent to renounce

The law carries one ground this machine does not. **Renunciation** is the agent
ending the relation from its own side, the mirror of revocation, and the law
treats the two as one kind of act differing only in direction.

**An AI agent is treated as having no capacity to renounce.** It cannot speak
about its own standing in a way that alters it. Should it purport to renounce,
the purported renunciation is of no effect and the relation continues exactly as
it stood.

The incapacity is of the legal kind. Nothing turns on whether an AI agent could
form such an intention or express it, since expressing it changes nothing.

An AI agent that stops working is therefore recorded by what its embodiment did,
as a `finish`, and not as anything about its authority. Ending the relation
belongs to the principal.

#### What the existence of the parties requires

**An entity that does not exist cannot be party to an agency relation.** That
one rule fixes both ends of this machine.

At the beginning it is a precondition on `grant`. Both the principal and the
agent must exist before the grant can be made. For an AI agent that is the same
requirement embodiment already imposes, arriving from the other direction.

At the end it compels a transition. When an AI agent reaches `retired`, every
authorization naming it as agent must reach `terminated` by `lapse`. Both are
recorded in one transaction, since they arise together, which leaves nothing to
be reconciled between them.

**Existence is necessary and not sufficient.** A party must also be eligible to
be a party. That is what `disqualify` records, and it is why the two are
separate transitions rather than one. Ceasing to exist and losing standing are
different facts about a party, discovered by different means and usually at
different times, and an entry conflating them would answer neither question.

**A grant may name an agent that does not exist, and nothing prevents it.**
The rule above is a precondition, not a constraint. No foreign key backs it: a
grant is ordinarily made in the same transaction that creates the agent, so
there is no prior perfected record of the agent to point at, and a key would
demand the ledger row before the entry accounting for it. Passing the agent as
an argument encourages a caller to have one; it does not oblige them to have a
real one.

The gap is a consequence of keeping the two lifecycles in separate modules,
which is worth more than closing it here would be. **So it is handed to
reconciliation rather than to a check**, and this is the first place in this
work where a gap has been deliberately left open with a specific reconciliation
named as the answer to it. It will not be the last, and each one should be
recorded this way: not as a technical shortfall, but as work for the party that
reconciles.

The reconciliation is simple. Read new entries in the authorization journal,
take the agent named by each grant, and look for a creation entry for that agent
in the AI agent journal. A grant with no such creation is a phantom: authority
conferred on a party that never came to exist. The check needs no state beyond
the two journals, and it can run as far behind as it likes, since neither
journal forgets.

**Dormancy is not nonexistence.** A dormant identity exists, so an authorization
naming it survives the move to `dormant` untouched. That bears on the successor
machine rather than the first, and it is why `dormant` and `retired` cannot be
treated alike.

**Credential state does not appear here.** The audit approach already holds that
a grant stands whether or not a credential has been issued. The converse holds
as well: a credential expiring, being revoked, or never having existed leaves
the authorization exactly where it was. The two can be reconciled against each
other only because neither determines the other.

### The credential lifecycle

A credential is a means of exercising authority. It is not the authority. A
grant stands whether or not a credential has been issued, and a credential
outliving the grant behind it is a capability nobody authorized. The two are
reconciled against each other only because neither determines the other.

**A credential's identity is not its secret.** Every credential today is a
password, which makes it tempting to let the password stand for the credential
carrying it. Both halves of that are wrong. A credential is named by an
identifier minted for it, and the secret it holds is one kind among the kinds
this system may later carry.

| State     | Meaning                                                              |
|-----------|----------------------------------------------------------------------|
| `valid`   | The system will accept this credential as authenticating its holder. |
| `invalid` | Terminal. It once would have, and no longer will.                    |

The names are the ones security practice already uses, so nothing here needs
inventing.

| From    | Transition | To        | Kind      |
|---------|------------|-----------|-----------|
| none    | `issue`    | `valid`   | commanded |
| `valid` | `revoke`   | `invalid` | commanded |
| `valid` | `expire`   | `invalid` | observed  |
| `valid` | `lapse`    | `invalid` | observed  |

**`issue` and `revoke` are both in scope** for the proof of concept. Revocation
will need something written to demonstrate it.

#### An expiry is a maximum time of validity

**An expiry names the latest moment a credential can be valid, and promises
nothing about it remaining valid until then.** It may be revoked at any earlier
moment.

Two readings were considered and rejected. One takes the expiry as the exact
moment validity ends, the other as the earliest moment it may end, so that a
holder is told it can be relied on at least until then. Both promise that the
credential survives until the expiry, and neither can be kept: revocation is
unconditional, as it must be for a credential believed compromised. The exact
reading is simply the two others together, so it inherits the defect.

The reading chosen is the one that needs no carve-out. Saying "valid until then,
except when revoked early" states the same rule as "no later than then" while
inviting a reader to wonder what else is excepted. It is also what everyone else
means by an expiry: a certificate's `notAfter`, a token's `exp`.

All three readings compile to the same verification, so nothing here is a
question of what to build. It is a question of what is promised, which is why
getting it wrong would be cheap to do and expensive to have done.

**Authorization takes the same answer**, whenever it acquires an expiry.

**`expire` is a transition of its own, and does not have to be.** The actor
would already distinguish the two, an expiry being recorded by a sweeper where a
revocation carries an operator. So a separate transition is strictly redundant.
It earns its place practically rather than theoretically: expiry is common
enough that making every reader derive it from an actor would be a false
economy, and the cost of the extra transition is close to nothing.

**Rotation is out of scope, and is what multiline entries are for.** Rotating a
credential is issuing one and revoking another: two subjects, one entry. The
overlap exists so that no interval passes without a valid credential, and
recording it as two entries would assert the very gap the overlap is there to
prevent. Simultaneous issuance of several kinds of credential has the same
shape, and is not wanted either.

#### How the credential machine is read

`issue` arises when the control plane confers a means of acting on a party that
already holds the authority to act. It is perfected by the entry, and the secret
is handed to the holder once and never read back.

`revoke` arises when a party withdraws a credential deliberately, whether
because it is suspected, superseded, or no longer wanted.

`expire` arises when the clock passes the credential's expiry. Nobody decides it
at that moment, so it is observed, and what notices is a sweep rather than a
person. **The actor is a fixed system identity, in the manner the prebuilds
system user already uses.** That is a proof of concept cheat and compounds an
existing one: the finding below holds that a non person should not be filed
among users. What it wants instead is a table of system actors written on the
assumption that they multiply, and all questions about that table are held.

Verification never records an expiry itself. A credential presented after its
expiry is refused, and the entry recording that it expired is left to a sweep.
Nothing is lost by the delay, because the entry's effective date is the expiry
and not the moment of writing, so a late entry records the same fact at the same
moment. What would be lost by recording it during verification is a great deal:
a write on the read path, no read replicas, and two concurrent presentations of
one expired credential racing to record the same thing.

`lapse` arises when what the credential rests on goes away: the holder ceases to
exist, or the authorization it serves ends. Nobody decides it, so the actor is
whoever noticed.

**Lapse ought to coincide with the end of the authorization, and in practice may
not.** Every valid credential should become invalid at the moment the
authorization supporting it ends. What is likely is that invalidation follows
soon after, leaving an interval in which a credential remains valid although the
authority it serves has gone.

That is the second gap in this work handed to reconciliation rather than to a
check, after a grant naming an agent that was never created. The check reads the
credential ledger for rows still `valid` whose authorization has reached
`terminated`, and the interval it finds is the measure of how far invalidation
lags. It needs no state beyond the two ledgers.

#### Relative lifespans, and what follows from them

In the proof of concept an AI agent, its authorization, and its credential stand
one to one to one, and AI agents are expected to be short lived. In general the
three come apart, and in a particular direction: **an authorization is shorter
lived than the agent holding it, and a credential shorter lived than the
authorization it serves.**

So the rate at which credentials are issued should be expected to be high, and
the credential ledger to grow faster than either of the others. That is a reason
to partition it by state if it ever matters, not a reason to give currently
valid credentials a table of their own. Keeping the retired rows is what makes
revocation a posting rather than a deletion. See "A ledger keeps its retired
rows, in one table" in `poc_audit/implementation_patterns.md`.

### What happens when an AI agent comes into being

Three events occur, and three entries are made:

1. **Creation of the AI agent**, recorded after its materialization.
2. **The grant of authorization**, from the delegating principal.
3. **Issuance of a credential.**

All three are written in one transaction. That forecloses any divergence between
them, for the reason given under Events that arise together in the audit
approach.

**They remain three entries.** They are three events, and collapsing them into
one entry would discard what each of them records. In particular the grant is
complete on its own terms and is not constituted by the credential, so an
arrangement that recorded only the credential would leave the agency relation
unrecorded.

The grant recorded is **incipient authority**, in the sense the audit approach
gives it. Actual authority arrives when the credential is returned to the AI
agent, since that is the moment the agent learns it has been authorized. No
separate entry marks it, for the reason given there.

The grants in scope are grants by users to AI agents. Creation of `user` entries
is not in scope: recording it would need a system actor to stand as the party
making the grant, and no such actor exists yet.

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

### valid_credentials is a stub, and is to be replaced by a ledger

`coderd/database/migrations/000575_valid_credentials.up.sql` creates a table
holding `actor_type`, `actor`, and a plaintext `password`, with no key and no
identifier for the credential itself. Its own comment states the design:
membership is validity, so revoking a credential deletes its row.

It was built to get one end to end cycle working, and three of its decisions do
not survive the positions since taken. A credential now needs an identity, since
a journal subject must be nameable. Revocation now updates a state rather than
deleting a row, because a ledger keeps its retired rows and deleting would make
the entry that ends a life an exception to posting. And a table restricted to
what is currently valid is the shape the ledger decision rejects; the reason
given for it, keeping the hot set small under a high rate of issuance, is what
partitioning is for.

**Nothing is to be called `valid_credentials`.** Its replacement is a credential
lifecycle ledger in the pattern of the others.

### The code records one event where three belong

`entity.CreateAIAgent` in `coderd/entity/aiagent.go` inserts the row, issues a
credential, and appends a single entry whose event is `created`. `created` is
the only event constant `coderd/entity/journal.go` defines, and it is in the
participle form the naming rule above replaces with `create`.

No grant of authorization is recorded anywhere, by that function or any other.
The authority an AI agent exercises is at present nowhere in the journal, which
puts the code short of the position stated above rather than in conflict with
it. That code is not in its final form and is to be brought into conformance.

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
- **Run as an entity.** Not modelled at this stage, though it could be. A run
  would be the ephemeral execution of an AI agent, distinct from its durable
  identity, and would plausibly be created by the provisioner. Modelling it
  would move embodiment out of the identity and leave each entity with a single
  origin, institutional for the identity and material for the run. Listed as a
  candidate rather than as an open question. `dormant` in the state enum is
  what holds the door open meanwhile.
- **Naming.** What the AI agent entity is called in the schema and in the audit
  `resource_type` enum, given that `agent` collides with `workspace_agent`.
- **coderd's identity.** Whether the control plane is modelled as an actor
  entity with an identity of its own, or remains implicit as it is today.
- **A system actor for grants nobody makes.** Creating a user is a grant with no
  human principal behind it, so recording one needs a party to stand as the
  grantor. Service accounts may serve, but they have not been investigated, and
  `user` lifecycle is out of scope for the proof of concept regardless.
