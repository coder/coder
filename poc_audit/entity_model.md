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

**Authenticator.** Something a holder possesses and controls, used to
authenticate it. A password is one. The term is NIST's, from SP 800-63, along
with the three below.

**Authenticator output.** The value an authenticator produces and sends to the
verifier, which is not the authenticator itself. For a password the two
coincide; for a challenge and a response it is the response; for a key pair it
is a signature. **What is presented is never called a secret**, because in
general it is not one, and calling it that would build the password case into
the vocabulary.

**Verifier.** The party that establishes a holder controls an authenticator.

**Relying party.** The party that acts on the verifier's answer. Access control
is one relying party and signing a document is another, so keeping the two roles
apart is what stops verification being confused with a grant of access. The two
usually happen together, which is exactly why they need separate names.

**Session.** A grouping, by convention, of a sequence of interactions. The
grouping is arbitrary in itself: a session may span several connections, and one
connection may be divided into several sessions. So a session is not defined
until its convention is written down, as a rule saying which interactions belong
to it and which do not. The convention adopted here is below.

**Claimant.** A party whose identity is not yet established, and which becomes a
subscriber once it is. See below: an AI agent is never one.

### What is an entity, and which things are

**Everything whose state deserves a complete history is an entity.** Three
consequences decide most hard cases. An entity **need not be an actor**: a
sandbox is one and does not act, an authorization is one and cannot. An entity
**need not be material**: an authorization is purely institutional, which is
what the question of origin above is for. And an entity **need not have a
lifecycle**, which is one thing an entity may have rather than what makes it
one.

#### A subject-entity and a model-entity are both entities

Two things are called an entity and the difference matters in a few places, of
which one is the rule about how many journals and ledgers there are.

A **subject-entity** is the thing itself: the credential as it travels through
the system, the AI agent as it acts. It exists outside any journal or ledger and
would exist if none had been written. It is an entity in the ordinary sense of
the word.

A **model-entity** is what a journal records through: a state machine, a
variable, an account. It is a model of some subject-entity, chosen because it
makes that subject tractable, and it is not the subject. The map is not the
territory.

**One subject-entity may have more than one model.** A credential has a
lifecycle, which is a state machine, and it has a record of use, which is a pair
of variables. Neither model is the credential and neither is more truly it than
the other; they answer different questions about the same thing.

Saying "entity" without the qualifier is normal and usually unambiguous. Use the
longer form only where the two could be confused, as they can wherever the count
of journals or ledgers is at issue.

Corpus maturity has three levels. `named` means the term is defined and glossed
and no more. `modelled` means identity, states and operations,
and relations to other entities are stated. `settled` means modelled, plus how
those operations are read, plus the reconciliations they generate. How much of
an entity exists in code is a separate question, answered in
[Implementation of Entities](implementation_of_entities.md) and nowhere here.

| Entity            | Corpus  |
|-------------------|---------|
| Authorization     | settled |
| Credential        | settled |
| AI agent          | settled |
| Sandbox           | named   |
| Session           | named   |
| User              | named   |
| `workspace_agent` | named   |
| Workspace         | named   |
| coderd            | named   |

A session's lifecycle is not journaled today and should become so. coderd's may
never need to be; see the Open section.

**coderd is identified by a fixed singleton.** One identifier denotes any
process running an instance of coderd, and no attempt is made to tell one
embodiment apart from another.

**The control plane is not the same thing as coderd**, though the two are used
interchangeably today. A coderd is a process; the control plane is a category of
such processes. That category is unmodelled, and the singleton is what papers
over the difference until it is modelled.

**System actor is a category of entity, not an entity.** coderd, service
accounts, the identity that creates prebuilt workspaces, the provisioner, and
the **custodian** are all system actors. The custodian is the one that runs
periodically and records what the passage of time has made true, expiries among
them. It fills the **sweeper** role, and a sweep is a reconciliation.

**Derived, and needing discussion rather than settled.** Only one stands: a
**run**, the ephemeral execution of an AI agent, distinct from its durable
identity. It is declined for now and listed as a candidate.

### The negative space around entities

A list of things that are **not** entities, kept for as long as it is useful and
no longer. Ideally the corpus becomes clear enough that a reader works this out
unaided, and this section is scaffolding toward that rather than part of the
finished shape. Its present use is to hold conclusions reached about the status
of unfinished things, so that each is reached once.

**Roles.** Principal, agent, claimant, subscriber, verifier, relying party,
recorder, auditor, reconciler, sweeper. An entity takes a role on for the
duration of an interaction; a role has no identity and no lifecycle. Actor is
**not** in this list, being a kind of entity rather than a role.

**Events and transitions.** `create`, `grant`, `revoke`. They are what a journal
records, not what it is about.

**Attributes and facts about entities.** An expiry, a scope, a state. An
expiration is a fact about a credential rather than a thing with a life.

**Records and the mechanisms holding them.** Journal, ledger, entry, log, audit
trail. These are removed from entities by at least one step: a record is derived
from the activity of entities, so it is downstream of them rather than one of
them. An audit trail is removed by at least two, being a collection of records
under the reading the audit approach gives it.

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

### An AI agent is never a claimant

A claimant is a party whose identity is not yet established. **An AI agent is
never in that position**, and that is a property to preserve rather than an
accident to observe.

The sandbox holding an AI agent's embodiment knows which AI agent it holds, so
an identity can be supplied by the context of a presentation instead of asserted
within it. An AI agent is **subscribed at birth**: from the moment its identity
exists it is bound to a known embodiment, and no interval passes in which that
identity is an unproven claim.

The gain is structural rather than a convenience. An AI agent cannot claim to be
a different AI agent, because it makes no claim at all. Impersonating one would
mean subverting the binding the sandbox holds, which is a different problem from
defeating a check, and not one any credential guards against.

**What the authenticator is then for is a separate question and is not settled
here.** Possession still has to be shown across boundaries the sandbox's own
knowledge does not cross.

### The session convention adopted here

**Every session here has the same participants throughout.** That is the rule
which makes the grouping something rather than nothing, and it is a restriction
rather than an observation: interactions with a different participant are a
different session.

A second restriction holds at present, that a session has exactly **two**
participants. So a session is described by two `(type, identifier)` pairs. That
is narrower than the same-participants rule requires and may be relaxed later.

The **participants rule** says who takes part, and is the same for every session
here.

| Kind          | Participants                 |
|---------------|------------------------------|
| An AI agent's | The AI agent and its sandbox |
| A sweep       | The custodian and coderd     |

The second is the pattern for tracking the activity of any background system
actor: the actor and whatever contains it.

**A session's bounds are constraints, not definitions.** A session cannot
outlive either participant, because a session is a relationship and a
relationship cannot survive a party to it. So an AI agent's session is bounded
by the agent's lifespan and by its sandbox's, both at once, ending with
whichever comes first. The two do not compete, and neither defines the session.

That is the rule the authorization machine states for the parties to an agency
relation and the credential machine for a holder, arriving here a third time.

**What delimits a session is its own transitions.** Constraint is not
delimitation: two sessions may share participants, so participant existence
cannot say where one ends and the next begins. A session is therefore opened and
closed by command, and a participant ceasing to exist forces it closed, which by
the tests above is entailed rather than observed: it follows from the record of
that party's ending.

So a session machine needs more than one kind of transition, which is the shape
the other three machines already have. A sweep's session is the simple case, opened
and closed by the command that starts and stops the sweep, its participants
outliving it comfortably.

The point of having sessions at all, at this stage, is that **an AI agent's
logged activity is always subordinate to one**. An action is recorded as
belonging to a session rather than floating free, which is what lets it be
placed against the authorization in force at the time.

**Cheat: the sandbox is named contextually, not identified.** Neither sessions
nor sandboxes are journaled entities yet, so there is no sandbox identity to
name. The second participant is written as the pair
`("sandbox", "sandbox-of:<ai agent uuid>")`, meaning the sandbox this AI agent
runs in, whichever that is.

**This form belongs in a log and nowhere else.** The `(type, identifier)` rule
with uuid identifiers is a discipline of journals and ledgers, whose identifiers
name rows in identity tables. A log carries no such obligation, so a text
identifier costs nothing there. It must never appear in a journal's or a
ledger's identifier column.

Three things recommend the form over an empty string or a sentinel uuid. It is
**self evidently not a real identifier**, so a reader meets an unfinished thing
rather than a plausible one. It **distinguishes the sessions of different AI
agents**, which an empty string would have collapsed together. And it carries a
fixed prefix, so the tool that reads these records can recognise it without
matching on prose.

It cannot be a nil uuid, that value already meaning absent and being rejected as
such in `coderd/entity`.

The cost is that the sandbox identifier is derived from the AI agent's, so one
fact is written twice and the two copies could disagree. That is tolerable only
because the value is always computed and never authored, so a disagreement needs
a hand written row rather than an ordinary mistake.

The cheat costs nothing else today, because an AI agent's lifespan falls
entirely within one sandbox, so an AI agent has exactly one session and the pair
is determined by the AI agent alone. **It stops working the moment either an AI
agent outlives a sandbox or a sandbox holds more than one session**, and it is
not expected to survive sandboxes becoming journaled entities.

### Entities are described by their states and their operations

An entity has state, and a set of operations that change it. A journal entry
records that a particular operation occurred; posting applies it to the ledger.
**That is the whole of what entities have in common**, and it is what lets them
be treated alike in schema and in code.

**An entity's definition includes an initial state**, the value its ledger holds
before anything has been posted to it. For a state machine that state is
nonexistence, which is analytical: nothing in code need represent it, but a
machine that omits it cannot say what its first transition proceeds from. An
account begins at zero. A variable begins uninitialized.

The theory exists for a practical reason: so that people can reason coherently
about institutional facts by analogy with material entities, which is the mode
of thought that comes naturally. The analogy is a working convenience and not a
claim that the two are the same kind of thing.

#### A state machine is the case where every operation is a transition

Where the operations available to an entity are exactly the transitions of a
state machine, everything the machine formalism offers comes with them: a finite
set of states, a legality condition on each operation, and diagrams. That is a
great convenience and it suits lifecycles particularly well, which is why most
of the entities here are described that way.

**It is a special case and not the definition.** A state machine can describe
things that are not lifecycles, and a lifecycle can be described without one.
Every entity here that has a lifecycle happens also to be described by a state
machine, and **that coincidence is historical rather than principled**. Nothing
should be inferred from it.

#### Not every entity has a lifecycle

A variable does not. Its journal records operations upon it and nothing about
its beginning or its end.

An upper bound on when it began could be read off its first entry, and a lower
bound on when it ended off its last. **That is inference from the journal rather
than something the journal records**, and the difference decides what is in the
model. Direct evidence is what an entry says. Indirect evidence is what can be
worked out from a body of entries, and it is not part of the model however
reliable it is.

To bring a variable's creation into the model, the **container** it is created
in has to be modelled first, and creation then becomes an operation on that
container rather than on the variable. Nothing here needs that yet.

#### Entities described by operations that are not transitions

These form a category rather than a single shape. What its members have in
common is only what the section above gives every entity: a ledger row holding
a **value**, and entries each recording an operation to apply to it. An entry is
pending until posted; posting applies the operation. There is no transition, no
state and no diagram, because the machine vocabulary has nothing to attach to.

**Two members of that category are described below, and they are the two this
work needs rather than an exhaustive list.** What unites them is not a further
principle to be stated here; it is the account given under "What an entity is,
stated formally", which covers transitions equally. The nearest familiar thing
to each is an account in a conventional ledger.

**A variable takes destructive assignment.** Its row holds the value last
assigned, and an entry says what to assign. Posting overwrites. The word carries
the sense it has in a programming language rather than in mathematics.

**An account takes a debit or a credit.** Its row holds a balance, and an entry
says what to add. Posting accumulates, so the current value is the sum of what
has been posted rather than the last thing posted.

**They differ in whether posting can fail.** An assignment always succeeds,
since nothing about the value being replaced can refuse the replacement. A debit
can be refused, by an insufficient balance or any other condition on the result.
The pending to posted step is therefore trivial for a variable and consequential
for an account, and the two want separate posting code rather than one
implementation with a flag.

Both correct an error the same way, by a further entry, and the two dates
distinguish a correction from a later genuine operation: a correction carries
the effective date of the entry it corrects, and a genuine later operation
carries its own.

#### What an entity is, stated formally

This restates the whole of the section, transitions included, and not only the
two kinds described just above. It is brief because its purpose is to let a
reader who knows the vocabulary of functional programming arrive quickly at the
same place, not to do work the prose has not already done. A reader who does not
recognise the vocabulary loses nothing by skipping it.

**Take `S` for the state a ledger row holds, and `M` for a monad expressing
whether an operation may decline to produce one. Then an operation is a Kleisli
arrow `S -> M S`, the journal is a sequence of them, and the ledger is their
fold under bind, starting from the initial state the entity's definition
gives.**

**Posting is where that fold advances.** It applies bind for one entry, so the
ledger at any moment is the fold over the entries posted so far. An entry not
yet posted is one the fold has not reached, which is what pending and posted
name.

**The fold takes journal order**, which is neither the order posting happened to
run in nor the order the operations occurred in the world. For a state machine
the difference rarely shows, since a late transition is usually illegal from the
state reached without it. For a variable it decides the answer outright, the
value being whatever the last operation folded assigned.

Journal order must therefore be total, which is what the entry identifier taken
from a sequence is for. A journal whose order is not settled leaves its ledger
undefined.

Deriving a ledger in journal order is universal bookkeeping practice, with
perhaps some exception we have not looked for.

**The policy here is to post in journal order, always.** In most cases that
costs nothing, being had by posting an entry in the same transaction that
creates it: an entry that is never pending cannot be posted out of order. Where
that does not hold, and it will not always, either the rule or its
implementation needs more care than is given here, and the case should be
settled where it arises rather than by weakening the rule in advance.

**The choice of `M` is the difference between a variable and an account.** Take
`M` to be the identity monad and every operation is total, so posting cannot
fail. Take it to be one carrying failure and an operation may refuse, which is
what a debit needs.

Associativity of Kleisli composition is the useful law: posting entries in
batches gives the same ledger as posting them one at a time. It is worth
checking against any new kind of operation, since a kind that broke it would
make the ledger depend on when posting happened to run.

**Where to start, for a curious reader.** Begin with the distinction described
just above, between an operation that cannot fail and one that can. Then read
what a monad is in the two instances used here and no others: the identity
monad, where nothing can go wrong, and the option or maybe monad, where an
operation may decline to produce a result. Those two are the formal statement of
that one distinction, and everything else the word carries elsewhere is unused
here.

### Commanded, observed and entailed operations

Every operation is of one of three kinds, and the kind settles whether the entry
carries an actor and whose identity it is. A transition is the case this was
first written for, and nothing in it is particular to transitions.

A **commanded** operation happens because some party decided it should. The
actor is the party who commanded it.

An **observed** operation happens of its own accord, and is recorded because
some party noticed. The actor is the party who noticed.

An **entailed** operation happens by necessity, from something already recorded.
**It has no actor**, and this is not a gap to be filled later. There was no act.

The kind is a property of the operation and not of the occasion. A process
returning of its own accord is never something anybody decided, so that
operation is observed whenever it occurs. Which party fills the role does vary
with the occasion, and the entry records whichever it was.

Two things follow for the first two kinds. An entity can never be the actor of
its own observed operations, which is the rule against an entity writing about
itself arriving by another road. And an observed operation may be recorded long
after it occurred, by whoever eventually noticed, which the audit approach
addresses under the entry's timestamp.

#### Recognising an entailed operation

Two tests, and either suffices.

**It happens by operation of law.** The phrase is the law's own, for rights and
duties arising automatically from a rule rather than from any party's act. An
agency relation terminates when a party to it ceases to exist, and nobody has to
do anything for that to be so.

**It follows by logical necessity from what is already recorded.** Given the
entry retiring an AI agent, and the rule, the lapse of its authorizations
follows. Given a credential's recorded expiry and the clock, its expiration
follows.

The positive form of both is one property: **an entailed operation is derivable
from the record, given the rule.** No party contributed anything, which is
exactly why no party can be named. There is nobody to name.

**Ambient facts count as available, and the clock is the one met so far.** An
expiry needs the hour as well as the record, and the hour is nobody's testimony:
it is not perceived, reported, or asserted by a party, and two derivations
performed independently reach the same answer. That is the test an ambient fact
has to pass. A fact only some party is in a position to know is testimony
however mechanically it arrives, and an operation depending on one is observed
rather than entailed, its actor being whoever was in that position.

**The three kinds are exclusive by construction.** If a party decided it, the
operation is commanded, and whatever follows by necessity is a different
operation. If a party perceived something the record did not already contain,
the operation is observed. An operation cannot be both derivable from the record
and dependent on somebody having been there.

#### An entailed operation records what entailed it

**Every entailed operation is defined to carry a reference to each entry that
entailed it.** This is part of defining the operation, not an option exercised
per entry, so its cardinality is fixed when the operation is.

Single cause entailment is what this work has met so far: an authorization
lapses because one entry retired its agent. It is not the general case. Some
entailments are the intersection of situations, and hold only when two or more
recorded facts hold together, so the reference is to entries rather than to an
entry.

What that reference buys is the question a reader of the entry actually has.
"Why did this lapse" is answered by naming the entry that retired the holder,
and the derivation becomes checkable rather than asserted. An actor, had one
been invented, would have answered a question nobody asked.

**An entailed entry is in principle redundant**, and this should be recognised
rather than discovered. It could be recomputed from its references and the rule.
Writing it anyway is a choice and a good one, the ledger being a fold over
entries and the alternative being a reader who must run the rules to know the
state. No other class of entry has this property.

#### Two attributions that were considered and rejected

**Not the actor of the entailing operation.** Their act caused it, so the
reading is tempting, and it is coherent while the entailing operation is
commanded: somebody who kills an AI agent cannot disclaim what follows. It fails
where the entailing operation is itself observed. The party who noticed an AI
agent finish did not cause its authorization to lapse, did not notice the lapse,
and stands in no relation to it beyond having been looking in the right
direction. Chaining an actor propagates a noticer into a slot that then reads as
responsibility.

**Not the custodian, nor any other party responsible for the record.** Something
did write the entry, and naming the writer is a real option, which is why it
takes an argument to decline. The argument is that a custodian's acts bear on
the journal, and a journal is not an entity: it is a record, removed from
entities by a step, per "Records and the mechanisms holding them" above. An
entry about a credential naming the custodian as actor says the custodian did
something to the credential, which it did not. The parallel is "A reconciler is
a role, not an actor" in the same section.

Recording the author of an entry, as distinct from its actor, remains available
and is not what the actor column is for. `audit_approach.md` keeps the two
apart under segregation of duties, and nothing here records an author yet.

#### One name, different kinds in different machines

`lapse` is entailed on the authorization and credential machines and observed on
the AI agent's. The word is the same and the kind differs, which follows from
the kind being a property of the operation rather than of the name.

**What differs is where the ending happens.** An authorization lapses in the
institutional world: a party recorded as ended cannot go on standing in a
relation, and the conclusion follows from the record. An AI agent lapses
materially: the chat it was made for was purged, that purge is in no journal,
and something has to go and look. One is derived and the other is perceived.

The lesson generalises past this pair. **A transition's kind cannot be read off
its name**, and a machine that borrows a name from another machine borrows the
meaning and not the kind.

#### An actor belongs to an operation, not to a transaction

Bringing an AI agent into being writes three entries that all name the owner,
because all three operations are commanded and the owner commanded all three.
Ending one writes three that do not share an actor, and two of which have none,
because only the first is commanded. The entries are as related as they ever
were, and what relates them is arising together.

**Operations are named with the bare verb**: `create`, `finish`, `suspend`,
`assign`. Not `created`, and not `creating`. The imperative reads naturally for
a commanded operation and the declarative for an observed one, and English
spells the two alike, so one form serves both.

**Operations are defined without reference to actors.** The states and the
operations upon them stand on their own. An actor is recorded on every entry,
but that record is kept for forensic purposes. What live operation needs is the
current value, not who brought it about.

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
| `active`  | `transfer` | `active`  | commanded |
| `active`  | `finish`   | `retired` | observed  |
| `active`  | `kill`     | `retired` | commanded |
| `active`  | `lapse`    | `retired` | observed  |
| `active`  | `suspend`  | `dormant` | commanded |
| `dormant` | `resume`   | `active`  | commanded |
| `dormant` | `retire`   | `retired` | commanded |

**The proof of concept machine is the first five rows**, and implements all but
`transfer`, which nothing performs yet.

`lapse` is the thing the agent was made for ceasing to exist: a chat purged by
retention, and by the same reasoning a sandbox outlived. The agent has no
occasion left, so it ends.

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
| `active` | `transfer` | `active`  | commanded |
| `active` | `finish`   | `retired` | observed  |
| `active` | `kill`     | `retired` | commanded |

Three things to notice, none of which the derivation makes obvious. `retired`
is reached two ways and left none, so it is the only terminal state and an
identity that reaches it is never reused. `active` is where an AI agent spends
its life, entered once at creation and left once at retirement, with `transfer`
returning it to itself. And `dormant` is absent from the machine while present
in the enum, so any code switching exhaustively over the enum must handle a
state that cannot occur.

**`transfer` makes the machine cyclic, and something depended on its not
being.** An entity's entries were held to be bounded by the sequences an acyclic
machine allows, which is what justified reading them under a limit and treating
an overflow as an error. A self transition removes that bound: ownership may
change any number of times. The limit survives as an expectation about how often
that happens rather than as a property of the machine, and it should be read
that way wherever it is relied on.

#### Ownership is not authorization

An AI agent's **owner** is the principal it belongs to. Its **authorization** is
what it may do and on whose behalf. Today the two coincide closely enough to
look like one fact, and they are not.

They agree only coarsely, and only over the overlap of their lifespans. Every
owned AI agent is authorized and every authorized one is owned, which holds
right up until an AI agent exists outside that overlap.

A prebuilt AI agent is the case that pulls them apart. It is **owned by the
prebuild actor when created and authorized for nothing at all**. Ownership
transfers later, and the authorization that follows is granted by the new owner,
not by the party that owned it first. At creation there is an owner and no
authorization; after transfer the owner has changed while the identity has not.

So the ledger keeps an owner of its own, and it is not a copy of the
authorization's principal.

**An owner is an actor, so it is always a `(type, identifier)` pair.** The
prebuilt case is what makes that more than caution: the prebuild actor is a
system actor, so the owner position holds something other than a user on the
first day it matters.

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

`transfer` arises when an AI agent's owner changes. It is commanded, and its
actor is whoever commanded it. Nothing performs it yet, and the case it exists
for is the prebuilt AI agent described under ownership above: created owned by
the prebuild actor, transferred later, and authorized by the owner it acquires
rather than the one it began with.

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
| `active` | `lapse`      | `terminated` | entailed  |
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
reaching `retired`. It is entailed: nobody performs it, and the entry carries no
actor and names the entry that retired the agent.

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

| From    | Transition  | To        | Kind      |
|---------|-------------|-----------|-----------|
| none    | `issue`     | `valid`   | commanded |
| `valid` | `reissue`   | `valid`   | commanded |
| `valid` | `revoke`    | `invalid` | commanded |
| `valid` | `expire`    | `invalid` | entailed  |
| `valid` | `lapse`     | `invalid` | entailed  |
| `valid` | `discharge` | `invalid` | entailed  |

**`issue`, `revoke`, `expire` and `lapse` are all in scope** for the proof of
concept. Revocation needs something written to demonstrate it. Expiry earns its
place by costing little on top of what is being built anyway and showing more
for that cost than most of what was considered beside it, and it brings the
sweep in with it. `discharge` is reserved: nothing can reach it until a grant
can be revoked while its holder lives.

**Three entailed transitions on one machine is not redundancy.** They differ in
what they follow from, which is what the splitting rule in
`implementation_patterns.md` separates, and each carries the references its own
rule needs: `lapse` names the entry ending the holder, `discharge` names the
entry ending the authorization, and `expire` names nothing.

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

`reissue` arises when a credential's validity is pushed forward rather than
replaced, which the chat gateway does because an in flight generation may
already hold the current identifier. It is a self transition, and seeing why is
quicker with a predicate than with a narrative: a credential is valid when it
has been issued and has not since been revoked, and both remain true across an
extension. Something happened and nothing changed state, which is what a self
transition is for.

`revoke` arises when a party withdraws a credential deliberately, whether
because it is suspected, superseded, or no longer wanted.

`expire` arises when the clock passes the credential's expiry. Nobody decides it
and nobody notices it: it follows from the expiry the record already holds, so
it is entailed, and the entry carries no actor.

**That both of this machine's non commanded transitions turned out to be
entailed is what the third kind was worth.** Both were classed observed until
the kind existed, and both were to be attributed to a fixed system identity
filed among users because there was nowhere else to put one.
That was a proof of concept cheat compounding an existing one, and recognising
entailment retires it here rather than working around it. The need for system
actors does not go away, since a grant nobody makes still wants a party to have
made it, but no entailed operation wants one.

Verification never records an expiry itself. A credential presented after its
expiry is refused, and the entry recording that it expired is left to a sweep.
Nothing is lost by the delay, because the entry's effective date is the expiry
and not the moment of writing, so a late entry records the same fact at the same
moment. What would be lost by recording it during verification is a great deal:
a write on the read path, no read replicas, and two concurrent presentations of
one expired credential racing to record the same thing.

`lapse` arises when the credential's holder ceases to exist. The credential then
authenticates nobody: there is no party left for it to speak for. It references
the entry recording that ending.

`discharge` arises when the authorization the credential serves ends while the
holder still exists. The credential would still identify its holder correctly;
what has gone is the authority there was any point in exercising. It references
the entry ending the authorization.

**The word is borrowed from suretyship**, where a surety is discharged when the
principal obligation is discharged. The structure is the one wanted: a thing
accessory to another ends because that other has ended, without anybody acting
on the accessory thing. A credential is accessory to an authorization in the
same way, being a means of exercising it.

**Both grounds hold at once when an AI agent is retired, and the transition
recorded is `lapse`.** The retirement ends the holder and lapses the
authorization in the same instant, so neither of those endings caused the other.
They are consequences of a common cause, and **a sibling is not referenced**:
the reference names what an operation follows from, and following from the same
thing is not following from each other. `discharge` is therefore reachable only
where the authorization ends and the holder does not, which is `revoke` or
`disqualify` of the grant.

**Both are entailed, and neither carries an actor.**

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

### The credential use model

A second model of the credential, beside its lifecycle. The lifecycle says
whether a credential is one the system will accept; this says when it was last
offered and when it was last accepted. Neither is more truly the credential than
the other, and each is silent about what the other records.

It is not a state machine. Its values are two moments, and its operations
assign them, so it is a pair of variables in the sense "Entities described by
operations that are not transitions" gives that word.

| Value            | Meaning                                                |
|------------------|--------------------------------------------------------|
| `last_presented` | When the credential was last offered, however it went. |
| `last_used`      | When it was last offered and accepted.                 |

Both begin unassigned, which is the initial value every variable has and here
means the credential has never been offered. Nothing distinguishes that from a
credential issued a moment ago, because nothing needs to.

| Operation               | Assigns                       | Kind     |
|-------------------------|-------------------------------|----------|
| `presentation_accepted` | `last_presented`, `last_used` | observed |
| `presentation_refused`  | `last_presented`              | observed |

Both name a presentation, because both are one: what differs is how it went.

#### How the credential use model is read

A **presentation** is one offering of a credential to a verifier. It carries two
things, and their being two is the point: the presenter **declares which
credential** they are presenting, and supplies an **authenticator output** for
it. Verifying the output establishes possession; the declaration says what
possession is being claimed of. A password style exchange conflates them by
sending one blob, and a challenge response protocol separates them visibly.
Without the declaration a refusal names no credential, and there is nothing for
`presentation_refused` to be about.

**A wire format may pack the two into one string, and often does.** An API key
token is a key identifier joined to a secret. Nothing about that makes it one
thing: the first half is the declaration and the second is the authenticator
output, and the verifier's first act is to take them apart. Which half is which
is not discoverable from the string, so it is settled by the credential's type.

Two things follow. **A credential type fixes the shape of its authenticator**,
because the authenticator has to be readable by whatever verifies it. The shape
is part of what the type is, alongside whatever the ledger holds for it. And
something that cannot be parsed cannot be refused either: it fails before there
is a credential for a refusal to be about, so it leaves no entry, which is the
same gap a declaration naming no credential leaves.

**The wire has its own name for the credential.** A key identifier is not the
identifier the ledger minted. Resolving one into the other is the verifier's
work rather than the presenter's, the presenter having supplied only what the
wire carries, and the ledger has to have recorded the wire's name for its own
credential or the resolution has nothing to go on.

**All of this model's operations are observed.** The actor is the verifier: the
party the presentation was made to, and so the party that noticed. Nobody
commands a presentation into the record. The nearest ordinary picture is an
officer at a control point recording the decisions they make on what is put in
front of them.

**The model records that a presentation occurred and how it went, not who made
it**, and the reason is the reason credentials exist at all.

A credential is issued because the system has no reliable knowledge of who is
presenting. Had it such knowledge the credential would be redundant, there being
nothing left for possession of a secret to establish. **So a record of the
presenter's identity is unreliable by construction**, and a record claiming
otherwise would be claiming the credential was unnecessary. For an accepted
presentation the holder is established by the acceptance, which is the
credential doing its work. For a refused one nothing is established at all.

Particulars of the presenter are therefore annotations at best: kept because an
investigator may want them, never read by posting, and never to be mistaken for
findings.

**Particulars of the presentation are annotations too, for the opposite
reason.** Which process, container or software version performed the
verification, and what connection the presentation arrived on, are things the
verifier knows about itself and knows reliably. They are annotations not because
they cannot be trusted but because they bear on nothing the operation assigns.
Where both kinds are recorded they sit in annotative fields alike, and the
shared treatment should not be read as a shared justification.

**Presenter particulars are not recorded at present, and the reason is worth
keeping.** The declaration of which credential is being presented is the only
claim a presentation carries, and it is already the entry's subject. A separate
record of the presenter would need a claim distinct from that declaration, which
arises where one party presents on behalf of another and does not arise here. A
field standing empty until then would be filled with whatever identity came to
hand, and the nearest one is the holder established by acceptance, which is a
finding and belongs nowhere near an annotation.

#### What this model does not record

**It says nothing about the credential's existence.** When a credential began,
whether it is still valid, and when it ended are the lifecycle model's, and this
model neither duplicates nor contradicts them. A variable's own creation and
destruction are not modelled at all, per "Not every entity has a lifecycle".

**Its journal records a declared subsequence rather than every presentation.**
Recording all of them is recording every authenticated request, and whether that
is affordable is a customer's judgement. What the journal records is whatever
its predicate selects, and the predicate is part of its definition, per
"Completeness is measured against what a journal purports to record" in
`journal_vs_log.md`.

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

### Occupancy belongs to the lifecycle of the occupant

If a sandbox can hold different actors over its life, and an AI agent can
outlive or move between sandboxes, then which sandbox holds which actor cannot
be a column on either entity. A nullable sandbox reference on the agent, or an
actor reference on the sandbox, records only the present and silently loses
history when it changes. That is precisely the assumption the identity
independence position forbids.

What the position implies instead is a representation of occupancy with its own
start and end, so that several may exist for one AI agent over time and none may
exist at a given moment.

**That does not make occupancy an entity.** It is a relationship tracked with
the lifecycle of the **content**, not of the container and not of itself. A
container's lifecycle is born empty, at least conceptually, so it has nothing to
say about what it holds; the occupant is what enters and leaves, and occupancy
is the relationship the occupant has with whatever contains it.

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
- **A journal for coderd.** coderd is an entity, settled below. Whether its
  lifecycle is ever journaled is not. It would become worth doing once the rest
  of the system is modelled explicitly enough that coderd's process lifetime is
  visible and salient to other activity, and until then it is an ambient
  singleton.
- **A system actor for grants nobody makes.** Creating a user is a grant with no
  human principal behind it, so recording one needs a party to stand as the
  grantor. Service accounts may serve, but they have not been investigated, and
  `user` lifecycle is out of scope for the proof of concept regardless.
