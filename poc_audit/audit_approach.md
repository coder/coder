# Audit Approach

Recorded 2026-08-04. This document states an approach to audit. It is
independent of any particular entity being audited.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built
on top of them, which is offered for challenge rather than settled.
**Findings** records verifiable facts about the existing codebase.
**Open** records questions not yet answered.

Material specific to particular entities belongs in separate documents
alongside this one.

## Established

### Terminology

Terms used in this document with a specific meaning.

**Audit.** An action. To audit is to examine an account and satisfy oneself that
it is faithful. The word is from Latin `audire`, to hear: accounts were read
aloud and the auditor listened, which is why the office is an auditor rather
than an inspector, and why `audience` is a cognate.

Two parties are always in view. One keeps the records and renders them. The
other receives them, examines them, and is owed the answer. The stance is
skeptical and demands proof. It is not adversarial by design, though it could
become so were the party rendering trying to get away with something and the
party receiving isn't going to let them.

One focus of this document is what the system must do for auditing to be
possible. It never refers to the existing database table named `audit_logs`,
which does something different and is discussed under the relationship section
below. Where the existing mechanism is meant, it is named explicitly.

As a noun, "audit" names one performance of the action, as in "at the next
audit". **It is (almost) never used to modify another noun.** Not "audit
journal", and not "audit log". Naming a record for the activity it supports
turns an action into a kind of thing, which is how "audit log" came to name a
log that nobody audits. Accounting does not do this either: it qualifies a
journal by its contents, as in a sales journal or a cash receipts journal, and
never by its purpose or by who will read it. (The exception is "audit trail";
see below.)

Where purpose or audience matters, the prose says so rather than the name: the
journal is kept for auditing, or is rendered to an auditor. "Auditable" is
permissible, being the adjective of the verb.

**Journal.** A book of original entry. The distinction between a journal and a
log, and why this work needs the former, is the subject of
`poc_audit/journal_vs_log.md`.

**Book of original entry.** The accounting term for a journal's position in the
sequence of recordkeeping, and the reason the journal is not merely one record
among several. An entry is made there **first**, at the very beginning of the
process, before anything is derived from it. Every other view, including any
statement of current state, is downstream of it and owes its content to it.

A single database transaction that writes an entry and a current state row
together collapses that sequence in practice, and ours does. The order is then
not observable in time. It remains true in dependency: the entry is the origin
and the state is the derived thing, whichever the storage engine writes first.

**Journalizing, posting, and auditing.** Three actions, deliberately
distinguished from each other. Journalizing is making the original entry.
Posting is carrying entries into the derived view. Auditing is the examination
of what results. They are performed at different times and, in a mature
arrangement, by different parties. Collapsing all three into "a database write"
loses every distinction this document depends on, which is why the words are
kept.

**Audit trail.** A collection of data sufficient to follow a transaction from
end to end, almost always heterogeneous in source: records from several systems,
plus documents and other evidence that are not records at all. **It is not a
data structure.** No table is an audit trail, and no table becomes one by being
named after it.

The question worth asking is therefore never "is this table an audit trail?" but
**"can a complete audit trail be constructed from what we have?"** That question
has an answer that can be wrong, and finding it wrong is useful. The journal
described here is one contribution toward such a trail. So, for what it covers,
is the mechanism named `audit_logs`.

**Entry.** An element of the journal. The journal is composed of entries. An
entry is a reflection and recording of an event.

**Record.** The stored representation of something, typically a row in a
database table. An entry is usually implemented as a record in some table, but
that is a fact about implementation and not part of the model. Where this
document says "entry" it is speaking about the journal; where it says "record"
it is speaking about storage.

The restriction on "record" applies to its use as a noun standing in for an
entry. As a verb it remains ordinary usage: an entry records an event.

**Agent**. The three senses of "agent", and the terms for the entities this work
deals with, are defined in `poc_audit/entity_model.md` rather than here, since
they are facts about entities rather than about audit. The bare word "agent"
refers to the legal sense of principal vs. agent.

### Recordkeeping, and where auditing stops

Auditing needs a boundary. Without one the word expands until it names
everything the system does with its own history, and a word that names
everything distinguishes nothing.

**Recordkeeping** is the name for the rest of it: making entries, carrying them
into whatever derived view exists, and holding both for as long as the
obligation lasts. It is what the party holding the records does, and it goes on
whether or not anyone ever asks to see the result. (Accounting calls its own
version bookkeeping. The word is not used here, because most of what this system
keeps is not a book.)

Recordkeeping covers:

- **Journalizing**, making the original entry.
- **Posting**, carrying entries into a derived view.
- **Retention**, holding both for as long as the obligation to account lasts.

Auditing covers:

- **Rendering**, producing the account for the party entitled to it.
- **Examination** of what was rendered.
- **Forming a view** about whether it is faithful.

Rendering is performed by the party that keeps the records, and belongs to the
audit all the same. It stands at the beginning of that interaction rather than
at the end of recordkeeping: it happens because someone asked, and what is
produced is shaped by what was asked for. Recordkeeping is unconditional and
rendering is not.

Together the two are what the duty to account requires. One party keeps, and
renders when called upon. The other receives and examines.

### What this work is, under that distinction

**The proof of concept is primarily recordkeeping. Auditing is a stretch goal.**

The journal, the entries, the identities they name, and the reconciliation that
is not yet written are all recordkeeping. An audit happens when a party asks for
an account and examines what it gets, and nothing here performs that or stands
in for the party who does.

The distinction sets what success looks like. The value of this work is that it
makes an audit possible and gives one something to find. It is not that an audit
has been performed, and no artifact produced here should be described as though
it were the answer to one.

A note on vocabulary. "Record" is restricted elsewhere in this document to mean
a stored representation, as against an entry. **"Recordkeeping" is exempt from
that restriction.** It is a compound naming an activity, and the activity
includes making entries as much as storing them.

### What makes an event auditable

An auditable event is one that has a **persistent effect on system state**.

This is narrower than an activity log:

- Attempts are not recorded as such.
- Successes and failures are not recorded symmetrically.
- Most failures produce no persistent state change and are therefore not
  auditable events.
- Some failures do produce persistent state change. Those are auditable.

### Recordkeeping as an integrity property

The event is the persistent state change itself. An entry in the journal
is a reflection and recording of that event, not a part of it. The journal
observes events external to itself, and additions to the journal are therefore
not themselves events.

The integrity invariant follows: every persistent state change has a
corresponding journal entry, and every journal entry corresponds to a
persistent state change. A state change without its entry, or an entry without
its state change, is a violated invariant rather than a missing log line.

Nothing available enforces that invariant. Reconciliation is the means by which
violations of it are detected.

### The ideal, and the reality

The ideal is exactly one journal entry per persistent state change.

That ideal is not achieved in reality. The design must reflect that rather than
assume coherence. Divergence between the journal and the world is a normal
condition to be detected and resolved, not an error case excluded by
construction.

### Reconciliation

Reconciliation is used here in the sense it carries in accounting and double
entry bookkeeping: the actual state of the world is verified to be the same as
that which the journal describes.

- In accounting, reconciliation runs on a cycle, typically monthly.
- Here, reconciliation does not need to wait for a cycle boundary. It can begin
  immediately.
- Given a transaction manager, with operations exposed through resource
  managers, reconciliation could be complete upon commit of a multiparty
  transaction. The approach must not presume a transaction manager, so
  coherence is achieved after the fact rather than atomically. Reconciliation
  is the mechanism that achieves it.

### Reconciliation belongs to recordkeeping

Reconciliation is a recordkeeping activity, and treating it as the part of this
system that performs the auditing is a mistake worth naming.

A recordkeeper reconciles to find its own errors before anyone else does. In
accounting this is ordinary diligence, and a reconciliation is classed as an
internal control rather than as evidence.

An auditor does something else, and the profession has separate words for it. It
**reperforms** the reconciliation, independently executing the same steps to see
whether it arrives at the same answer, which tests both the control and the
balance. Separately it seeks **confirmation** from a third party, and the
governing rule is that the response must reach the auditor directly rather than
through the party being audited.

The distinction is not pedantic. A reconciliation we perform on our own records
is evidence offered, not evidence obtained, and accepting such a reconciliation
without testing what underlies it is explicitly inadequate as evidence in an
audit. We can build something that makes an auditor's work possible and cheap.
We cannot build something that discharges it.

### Reconciliation is out of scope for this phase

**No reconciliation is being built at this phase of the proof of concept.** The
journal is written; nothing yet reads it back and compares it against the world.

Two consequences follow, and they are stated here so that neither is mistaken
for an oversight.

Divergence is not detectable. Since detection is what makes a divergence
visible, the classes described under Derived below are properties of the design
rather than of anything running. Whether the journal and the world agree is at
this phase an unanswered question, not an answered one.

Divergence is therefore not a defect in code review. Phantoms and orphans are
expected while the entities an entry refers to are still being built, and they
are excluded from review of this work. They return as review material when
there is enough of a system for their absence to mean something.

This is a statement about what is being built now, not a retreat from
reconciliation. The positions above stand.

### Relationship to the existing mechanism

The mechanism currently called `audit_logs`, with its supporting packages in
`coderd/audit` and `enterprise/audit`, serves a different purpose from the
approach in this document. It records what a user did, attributed to a request,
carrying a field level diff and a status code. It is deletable by retention
policy, droppable by filter, and disabled entirely unless licensed. Those are
reasonable properties for what it does, and disqualifying properties for an
integrity ledger.

**The two are independent systems running side by side.** Neither supersedes the
other. This approach does not extend the existing mechanism and does not require
its use. The purposes are different enough that a single mechanism would have to
be simultaneously deletable and permanent, droppable and complete, licensed and
invariant.

**Recommendation: the existing mechanism should be renamed.** Its current name
claims the word "audit" for something that is not audit in the sense used here.
The confusion is not hypothetical: without a rename the two systems read as one
system in two states of completeness, and a reader encountering both will assume
the newer is meant to replace the older. No replacement name is proposed here.

### The two history tables are a different case

`user_status_changes` and `user_deleted` are already in the spirit of this
approach, though they were built ad hoc for one entity. **They should eventually
be incorporated into it.** What they do today may not match the eventual design
in detail, but the new system should be able to do everything they currently do,
consistently and on principle rather than case by case.

## Derived

Reasoning built on the positions above. Offered for challenge.

### Coverage becomes structurally checkable

If every persistent state change is to produce a journal entry, then "is our
audit complete?" becomes "does every write path to this state produce an
entry?" That is answerable by inspection of structure rather than by
recollection.

This argues for placing the obligation to make an entry at the lowest layer
that cannot be bypassed, so that a new write path cannot silently omit it. That
reduces the opportunities for the invariant to be violated. It does not enforce
the invariant, which remains a matter for reconciliation to detect.

### The unbypassability and actor tension

The layer that cannot be bypassed is also the layer that cannot see the actor.
A database trigger fires regardless of caller, but has no access to the
application's notion of who is acting. An entry written by application code
knows the actor, but can be omitted by a new caller.

| Placement                     | Unbypassable | Transactional | Knows actor |
|-------------------------------|--------------|---------------|-------------|
| Trigger on the state change   | yes          | yes           | no          |
| Store layer, same transaction | no           | yes           | yes         |
| Request handler               | no           | no            | yes         |

Known mitigations exist for the trigger case, such as carrying the actor in
transaction local state, but they trade the simplicity that made the trigger
attractive.

### Classes of divergence

Three classes, exhaustive:

- **Phantom.** An entry exists, the world does not. Detectable by walking our
  own entries and probing for each.
- **Orphan.** The world exists, no entry does. Detection requires enumerating
  the world. It cannot be found by consulting the journal, since the defining
  property is the absence of an entry.
- **Drift.** Both exist and disagree about state. Detectable by walking entries
  and comparing.

The asymmetry is the important part: phantoms and drift are tractable from the
journal alone, and orphans are not. Reconciliation that only walks entries is
structurally blind to one of the three classes.

### Ordering of entry and effect

Absent a distributed transaction, the entry and the effect cannot be made
simultaneous, so one must precede the other, and the choice determines which
class of divergence a failure produces:

- Entering before acting produces phantoms on failure.
- Acting before entering produces orphans on failure.

A phantom fails loudly when something tries to use it. An orphan is silent.
Where an unrecorded real effect is the more serious outcome, entering intent
first is preferable. That premise is domain dependent and should be checked
rather than assumed.

### Correspondence keys

An entry stating only that something happened cannot be reconciled. An entry
naming what was affected, and carrying an identity by which that thing can be
observed externally, can be.

This is the difference between entries that can eventually be verified and
entries that can only be believed. It is a property of the entries, so it can
be established before any reconciliation exists.

### Direction of observation

Accounting style reconciliation requires the ability to enumerate the world,
which is an outbound direction of observation. A design in which components
announce themselves and prove their identity is inbound, and is sufficient for
phantom and drift detection but not for orphan detection.

### Detection is not resolution

Reconciliation in accounting does not stop at identifying a discrepancy; it
produces adjusting entries. An audit approach therefore needs a resolution
policy as well as a detection mechanism, and the two are separable decisions.

### A restricted grant of agency

An entity may not write entries about itself. That looks at first like removing
the agent's duty to account, and it is not. The duty stands. What is narrowed is
the grant.

The authority delegated is not authority to act by any available means. It is
authority to act **through the services provided**. The agent discharges its
duty to account by confining its activity to those services, because the
recording they make is the account. The agent is not excused from reporting. It
is required to make itself reportable, and the only way to do that is to act
where recording happens.

That has a consequence worth stating. An act performed outside those services is
not merely an unrecorded act. It is an act outside the authority granted, which
is a different and worse failure, and one the doctrines under apparent authority
below bear on directly.

This is the same shape as sandboxing at the network level. A sandbox does not
ask a process to report its own traffic honestly. It arranges that traffic can
leave only through something that records it. The arrangement here is social and
legal rather than technical, and the reason is identical: an account produced by
the party being accounted for is worth less than one produced by a service that
party was obliged to use.

### Separating the party who acts from the party who records

Accounting has a control for this and calls it **segregation of duties**: the
party who records a transaction is not the party who authorized it or who holds
the asset. Banking has a close relative in **dual control**, where an action
requires two people who cannot be the same person.

The rule that an entity may not write entries about itself, stated in
`coderd/entity/DIRECTORY.md`, is a weak form of the same idea. It separates the
actor from the author of the entry, and no more. It does not separate
authorization from custody, and it does not require two parties to concur before
an act takes effect.

**The product cannot support the stronger form yet.** Dual control needs a
notion of two parties who must both assent, which nothing in this system
expresses. This is recorded as a direction rather than a proposal, for a later
stage of maturity.

### Apparent authority and ratification

If an AI agent stands in a genuine relation of principal and agent, then two
doctrines from the law of agency have analogues here. Both disturb the same
assumption: that attribution is a fixed property of an event, settled at the
moment the event occurs.

**Apparent authority.** The agent acts outside the scope actually granted, but
the principal's own conduct led a third party reasonably to believe the scope was
wider, and the principal is bound accordingly. Whether the principal is bound
then turns on a third party's reasonable belief, which is not a fact the system
holds at the time of the act. A journal that captures only the authority as
granted cannot express this case.

**Ratification.** The principal adopts an act that was unauthorized when taken,
and thereby becomes bound by it. The status of the act changes after the fact.
An entry describing the act was correct when it was made and is incomplete
afterward, which is the situation adjusting entries exist to handle.

Together these suggest that the journal may need to distinguish authority as it
was understood when an act occurred from authority as later determined, and that
a later entry may qualify an earlier one without contradicting it.

The language here is not settled. This section is recorded to preserve the
observation, not to fix terminology.

## Findings

Verifiable facts about the existing codebase, recorded for reference.

### The existing request recording mechanism

`coderd/audit` has three properties that constrain where it can be used:

- **Request scoped by construction.** `audit.InitRequest` returns a closure
  intended to be deferred, and that closure reads HTTP status from the
  `tracing.StatusWriter`, so it can only run after a handler returns. An actor
  with no originating request has nothing to attach to.
- **Non transactional and best effort.** The closure runs with
  `context.Background()`, deliberately detached from the request. On export
  failure it logs the error and returns. The state change has already
  committed, so entries can be lost silently.
- **The actor is the API key holder.** The user ID comes from
  `httpmw.APIKeyOptional`, falling back to an explicitly set `Request.UserID`.
  If neither is present, nothing is written. There is no notion of a delegated
  actor.

### The existing transactional mechanism

`record_user_status_change()` is a database trigger. It commits or rolls back
with the change it describes and cannot be bypassed by a new write path. It
writes to `user_status_changes` and `user_deleted`. It captures the subject of
the change and not the actor, since a trigger cannot see one.

The existence of these purpose built history tables alongside `audit_logs` is
itself a finding: they exist because `audit_logs` could not answer questions
about reconstructed past state.

### Existing reconcilers do not observe the world

Three components in the codebase are structured as reconcilers:
the prebuilds `StoreReconciler` (`enterprise/coderd/prebuilds/reconcile.go`,
with `SnapshotState`, `CalculateActions`, and a reconciliation lock), the
`autobuild` lifecycle executor, and the dormancy job.

All three reconcile desired configuration against database state. None
observes external reality. For prebuilds, the world is mediated entirely
through `provisioner_jobs`, so the reconciler consults recorded jobs rather
than infrastructure.

### The intent record pattern

`provisioner_jobs` treats a row as intent. Reality is reconciled against it,
and terminal state is written only when reality confirms. This is why a
workspace's `deleted` flag is set when the delete job completes rather than
when deletion is requested.

### The nearest external correspondence identifier

`workspace_agents.auth_instance_id`, indexed as `(auth_instance_id, deleted)`,
is used for cloud instance identity. The direction is inbound: the instance
announces itself and proves its identity. Nothing enumerates instances and
compares.

### The two directions of testing already have names

Auditing practice names the two directions separately, and they correspond to
the divergence classes above.

**Vouching** goes from a record to the evidence behind it, and tests occurrence:
whether what was recorded happened. It finds what is called a phantom here.

**Tracing** goes from the evidence to the records, and tests completeness:
whether what happened was recorded. It finds an orphan.

The profession states the same asymmetry this approach arrives at. Vouching
cannot show whether everything was recorded, which is exactly why tracing exists
as a separate procedure rather than a byproduct of the first. Walking our own
entries and probing outward is vouching. Enumerating the world and comparing
inward is tracing, and it is the one that needs an ability we do not have.

### Cost of registering with the existing machinery

Adding an audited resource to the existing mechanism touches eight places: the
migration, the query files, the `Auditable` union in `coderd/audit/diff.go`,
both `AuditActionMap` and `AuditableResources` in `enterprise/audit/table.go`,
the RBAC resource in `coderd/rbac/policy/policy.go`, `dbauthz` methods, and
the SDK types with their handlers. Audit documentation is then regenerated by
`scripts/auditdocgen`.

One trap: `resource_type` is a Postgres enum, and all migrations run inside a
single transaction, so adding a value and then using it in the same batch
fails with `unsafe use of new value`. This surfaces only on deployments with
existing data. See `.claude/docs/DATABASE.md`.

## Open

- **Strength of coherence required.** Whether the goal is that every mutation
  produces an entry, or the stronger property that for any past instant the
  state and its responsible parties can be reconstructed.
- **Placement of the entry.** Trigger, store layer, or handler, given the
  unbypassability and actor tension above.
- **Attribution of a delegated act.** Settled: an entry records the actor
  behind the action, and not both a principal and an agent. See the attribution
  position in `poc_audit/entity_model.md`, which establishes that delegation is
  authorized in advance and recorded separately, so it does not need to appear
  in each entry. Retained here as a pointer because the question was raised
  here.
- **Resolution policy.** On detecting divergence: note the discrepancy only,
  correct automatically, or produce adjusting entries for human disposition.
  Whether a correction is itself an auditable event.
- **Reconciliation cadence and trigger.** Eager per event, periodic sweep, or
  both, and what backstops the eager path when it fails.
- **Ordering guarantees.** Whether journal entries require a monotonic
  sequence, or whether timestamps suffice. Timestamps are not a safe sort key
  under concurrency.
- **Orphan detection.** What capability would allow enumerating the world, and
  who owns providing it.
- **Integration with existing mechanisms.** Whether, when, and in which
  direction.
