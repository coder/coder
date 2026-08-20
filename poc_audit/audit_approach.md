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

**Ledger.** The derived view that entries are posted into. A journal is
organized by when its entries were made; a ledger reorganizes the same content
by the thing each entry concerns, and carries the result of those entries taken
together. So it holds what is currently true, and every word of it is
downstream of the journal.

Where what is being recorded has a lifecycle, the distinction takes a
particular and useful form: **with respect to a state machine, a journal
records transitions and a ledger records states.** That holds for every entity
in this work, each of which has one. It is a specialization and not the
definition, and should not be used to gloss the general case: a ledger
accumulating money carries a balance, which is the state of no machine.

A ledger here is a row for each entity holding its present state, updated in
place. A reader who has kept paper books should note the difference in form. A
paper ledger account is a growing list of postings, because paper cannot be
updated in place, and in the running balance form each line carries the balance
that resulted from it. That is the same fold over the same transitions, kept as
a sequence of snapshots rather than in one mutable cell. Nothing is lost by
keeping it the other way, since the journal already holds every transition.

**Journalizing, posting, and auditing.** Three actions, deliberately
distinguished from each other. Journalizing is making the original entry.
Posting is carrying entries into the derived view. Auditing is the examination
of what results. They are performed at different times and, in a mature
arrangement, by different parties. Collapsing all three into "a database write"
loses every distinction this document depends on, which is why the words are
kept.

**Recorder.** The party that makes entries in the journal. Accounting calls this
role the **bookkeeper**, and the word is worth knowing, because the practice
this approach borrows from is written in it. This document says "recorder"
instead. A reader who does not already think of a database as a set of books
will read "bookkeeper" as a person doing something else entirely, and the
substitution costs nothing.

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

**Institutional fact.** A fact that holds because the parties concerned treat it
as holding, rather than because of any physical arrangement. The term is
Searle's, from *The Construction of Social Reality* (1995). A grant of
authorization is one. The moment it is made, an agency relation holds between
two parties, and nothing anywhere else need have changed for that to be so.

An institutional fact is as real, for present purposes, as a material one. It
persists, it changes, and its changes are exactly what this document means by an
auditable event. What it lacks is an independent physical form to be checked
against, which has a consequence for reconciliation recorded below.

**Perfect.** Used in its legal sense: to complete the steps that make an
otherwise inchoate right effective, as in perfecting a security interest. Not
the ordinary sense of making something ideal.

A grant of authorization arises in a mind or in a system, and is **perfected**
by an utterance or a recording. Before perfection there is an intention (in a
mind) or a point of execution (in a program). There is actual state in both
cases, and in both it is transitory. After perfection it is unambiguous that a
grant exists. The distinction earns its keep because the entry is itself the
perfection. Recording the grant does not report a perfection that happened
somewhere else; the recording performs it. The origin remains outside the
journal and prior to it, so the ordering of origin and entry is undisturbed.

Recording a grant perfects it just as a speech act does. That is an analogy and
not a classification: a grant is not a kind of speech act. What the two share is
that making the utterance, or making the record, is what completes the thing
rather than merely reporting on it. Austin's word for an utterance of that kind
is **performative**, in *How to Do Things with Words* (1962).

The law has a formula that shows the act plainly. A power of attorney conveys
authority with the words "do hereby **make, constitute, and appoint**", and each
verb does a distinct piece of work: *make* creates the relationship,
*constitute* empowers, *appoint* designates the person. Bouvier's *Law
Dictionary* (1856) glosses "constitute" as "to empower, to authorize". The
instrument does not report that a grant happened somewhere else. Saying the
words in it is itself the granting.

The formula is cited for what it shows about the act, not because anything here
creates an attorney relationship. What a grant creates is broader, and the same
triplet serves other agency relationships equally well.

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

### Auditing is not forensic investigation

The two consult the same records, which is why they are easily conflated. The
difference is in what each is looking for.

**Forensics examines a known anomaly.** Something has gone wrong, or is
suspected to have, and the investigation works outward from it to establish what
happened.

**Auditing looks for anomalies not yet known.** Nothing need be wrong. The
examination is systematic rather than directed, and finding nothing is a result.

They are complementary rather than competing. An auditor who finds an anomaly
may open a forensic investigation in order to finish the audit. That is one
party taking on two roles, not the two activities collapsing into one.

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

An auditable event is one that has a **persistent effect on state**.

Persistent state is broader than the state of the technical system. It also
covers relationships between parties, which are institutional facts in the sense
defined above and are no less real for having no physical form. A grant of
authorization changes the state of the world in the only way that matters here:
an agency relation holds afterwards that did not hold before.

That such a grant is complete once perfected can be seen from what it does not
depend on. The grant stands whether or not anybody else has been told, whether
or not the agent knows, whether or not the agent has any means of acting, and
whether or not a credential has been issued. None of those is a
constituent of the grant. Each is a separate fact that may or may not follow,
and some of them are separately auditable events in their own right. The list is
not exhaustive, and particular contexts will add to it.

This is narrower than an activity log:

- Attempts are not recorded as such.
- Successes and failures are not recorded symmetrically.
- Most failures produce no persistent state change and are therefore not
  auditable events.
- Some failures do produce persistent state change. Those are auditable.

### Incipient authority and actual authority

The law of agency draws a line the section above steps over, and it is better
drawn deliberately than left to be discovered.

A principal's grant confers what can be called **incipient authority**. The
principal has done everything required of them, and nothing further is owed by
them for the grant to stand. **Actual authority** is the term of art, and the
Restatement (Third) of Agency puts it at a further remove: by section 3.01 it is
created by the principal's manifestation to the agent, as reasonably understood
by the agent. It therefore does not arise until the agent has been told and has
understood.

**What this system records is incipient authority.** The entry is made when the
grant is made, which is what makes the journal a book of original entry for it.

Actual authority could be recorded as well. It would take a second entry, made
once the agent had been told, which here is the moment the agent's credential is
returned to it. The two entries would stand one to one and could be reconciled
against each other.

**That distinction is deliberately not modelled.** The interval between the two
entries is a few statements wide, and the only failure it admits is a process
crash falling between them. What such a reconciliation would detect does not
repay what it would cost to model. This is a choice and not an oversight, and it
is recorded so that a reader who knows the law of agency does not take the
omission for ignorance of it.

### The question is one of origin

Every change is recorded somehow, so a technical manifestation is universal and
by itself distinguishes nothing. The question that separates one kind of event
from another is where it arises: **does the event arise as a relationship, or as
behaviour of a technical system?**

Both kinds are auditable, and both are recorded the same way. The distinction
earns its place by predicting what can be checked afterwards. An event arising
from technical behaviour leaves something behind that can be observed
independently of the journal. An event arising as a relationship does not,
because the relationship has no form other than the record of it.

Which of this system's entities arise which way is a question about entities
rather than about audit, and is answered in `poc_audit/entity_model.md`.

### Recordkeeping as an integrity property

The event is the persistent state change itself. An entry in the journal
is a reflection and recording of that event, not a part of it. The journal
observes events external to itself, and additions to the journal are therefore
not themselves events.

That holds for institutional facts as well, though the line there is a fine one.
What the journal records is the perfection of a grant and not its origin, and
the origin lies outside the journal in every case. Where the recording is itself
the perfecting act, the two coincide in a single action without the journal
becoming its own subject: there is one entry and one fact, not an entry about an
entry. A signed contract is the ordinary analogue. The signature is the act of
agreeing and the evidence that the agreement happened, one thing physically and
two things conceptually, and nobody is misled by it.

The rule worth stating as a rule is therefore the narrow one: **the journal does
not record its own operation.** Rotation, compaction, and the act of writing are
not auditable events. Without that restriction the definition regresses, since
every entry would need an entry of its own.

The integrity invariant follows: every persistent state change has a
corresponding journal entry, and every journal entry corresponds to a
persistent state change. A state change without its entry, or an entry without
its state change, is a violated invariant rather than a missing log line.

Nothing available enforces that invariant. Reconciliation is the means by which
violations of it are detected.

### One recorder

**There is exactly one recorder, and it is coderd.** Every entry is made by it,
whatever party the entry is about and whichever component the event originated
in.

This is a policy, not an observed property of the code, and it could be relaxed
later. The code is too immature for its conformance to be worth reporting as a
finding, but checking the policy as the code is written is worthwhile: a second
recorder appearing by accident is exactly the sort of thing that goes unnoticed
until it matters.

The reason for the policy is that the party who acts and the party who records
are separated by design, which is developed under Derived below. A single
recorder makes that separation checkable in one place rather than distributed
across every component that might have something to say.

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

### What reconciliation cannot reach

Reconciliation compares the journal against the world. Where an event arose as a
relationship, there is no world to compare it against, the relationship having
no existence apart from the record of it. Such entries are within the scope of
reconciliation and the reconciliation returns nothing. That is a property of
what is being reconciled, not a check left unimplemented.

It is also not a gap to be closed. An institutional fact whose record is its
only form cannot be confirmed against anything but another record of the same
kind, and confirming a record against itself establishes nothing.

What can be reconciled is the relationship against its technical consequences. A
grant of authorization and the issuance of a credential stand one to one but are
things of different kinds, so each is evidence about the other. A credential
with no grant behind it is a capability nobody authorized, which is precisely
the condition worth detecting. A grant with no credential is authority the
system has not equipped anyone to exercise, which is a different fault and
ordinarily a less alarming one.

### Events that arise together

Where several events necessarily arise at the same moment, recording them in one
transaction forecloses any divergence between them. Nothing is left to reconcile
afterwards, because no intermediate state is ever observable.

This is a technical means of avoiding a reconciliation problem rather than a
solution to one, and it is available only where the events genuinely co-occur.
Each remains a separate event and takes its own entry. Collapsing them into a
single entry would discard the distinctions the entries exist to record, and
would leave that entry impossible to interpret without knowing which of several
things it was standing for.

### Two dates on every entry

An entry carries two timestamps. Collapsing them into one loses a distinction
the journal exists to keep.

**`recording_date` is the time the entry was made.** It is set inside the
function that writes the entry. It is never passed in by a caller, never
overridden, and never backdated for any purpose whatever. A book of original
entry that misreports when it was written is worth less than one that admits
the gap.

**`effective_date` is the time the event occurred.** It is ordinarily an
explicit parameter to that same function, because only the caller is in a
position to know it.

Accounting proper would call the second a transaction date. The generalisation
is deliberate: not everything recorded here is a transaction, and naming the
column for the commonest case would invite the same error as naming a log for
the activity somebody hoped it would support.

Two traditions meet at this pair of columns, pointing in opposite directions.
Accounting dates a journal entry by when the transaction occurred and orders the
journal by that date, leaving the order of recording to the page rather than to
any column. Data integrity practice requires the reverse, a timestamp generated
by the system at the moment of the entry and alterable by nobody. They do not
truly conflict, since they describe different things: one describes the
transaction, the other the act of recording. A journal with one date column has
to choose between them. A journal with two does not.

**The two values are read from the clock at different moments and are therefore
distinct**, though a clock coarse relative to the interval between them may
render them equal. That is worth having rather than avoiding, since it reifies
the principle that the recording comes after the event. `effective_date` is
never later than `recording_date`.

Where a caller does not know when the event occurred, it reads the clock at the
earliest moment it can vouch for and uses that. The value is then an upper bound
rather than a measurement, and it errs in the safe direction: it never claims
the event happened earlier than can be shown.

What neither column can carry is how good the value in it is. A time known
exactly, a time known only to fall within a window, and a time taken at the
moment of detection because nothing better was available all sit in
`effective_date` looking alike. That qualification belongs in free text beside
the entry, which can hold all three and say which it is.

This generalises past time. **Reconciliation will never be as tight as
enforcement before the fact.** What it recovers is more approximate than what
was enforced, and the record should be able to say so rather than round it to
something crisper than the evidence supports.

The data integrity tradition mentioned above has a name: **ALCOA**, for
attributable, legible, contemporaneous, original, and accurate. It began as a
mnemonic for inspectors in regulated manufacturing and has been adopted well
beyond it since. Nothing built here falls under that regime, so ALCOA is taken
as a set of standards worth striving for rather than as a rule that binds.
Contemporaneous is the one bearing directly here: a record made at the time of
the activity rather than afterwards.

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
