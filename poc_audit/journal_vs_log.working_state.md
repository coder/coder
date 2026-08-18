# Working state for journal_vs_log.md

Recorded 2026-08-13. This file is scaffolding for
`poc_audit/journal_vs_log.md`, not part of the design. It exists so that the
material feeding that document is disposed of deliberately rather than
forgotten, and so that a later session can pick the work up without
reconstructing the conversation that produced it.

Delete this file when the document is finished and nothing here is open.

## How to read the punch list

The list below is the original inventory, in its original order, with its
original numbers. **Numbers are never reused and never renumbered**, including
when an item is dropped. New items take the next free number in their section.

Each item carries one of four states:

- **done.** In the document, and nothing further is owed.
- **part.** Some of it landed. The note says what is still owed.
- **open.** Not addressed yet.
- **drop.** Deliberately excluded. The note says why, so that the exclusion is
  not mistaken for an oversight and not silently reversed.

## Punch list

### 1. Purpose and audience

1. **open.** Two readers: a human collaborator to persuade, and a future session
   to bring to the same understanding.
2. **open.** Standing failure mode it prevents: anyone meeting `audit_logs`
   first will assume that is the audit system.
3. **part.** Reconstructible from itself, with claims carrying evidence. The
   etymology carries sources. Code claims are owed once section 6 is written.
4. **done.** Refers to `audit_approach.md` for the approach; its own subject is
   the distinction.
5. **done.** Follows the standing-marked section convention.

### 2. Vocabulary to fix before anything else

1. **done.** Journal in the accounting sense: a chronological record of events.
2. **done.** Not a journal in the systems sense, meaning a write ahead log. The
   note in the document stops at the misreading it prevents. The reasoning
   behind it is recorded under "Coupling, and where this belongs" below, and is
   owed to `audit_approach.md` rather than to this document.
3. **done.** A journal is not a ledger.
4. **drop.** Entry versus record. The distinction is defined in
   `audit_approach.md` under Terminology, and this document has no need of it.
   A single sentence remains available if the comparison section turns out to
   want one.
5. **done.** Log defined, in both senses. The historical one, and the modern
   one under "The word outgrew the measurements".
6. **drop.** "Audit journal" names a category, not one particular journal. Moot:
   the phrase is retired. `audit_approach.md` now records that "audit" is an
   action and is never used to modify another noun, so there is no compound left
   to explain.
7. **drop.** The word "audit" is currently claimed by `audit_logs`, and the
   rename recommendation stands. Landed in `audit_approach.md` instead: the
   Terminology entry states that "audit" never refers to that table, and the
   attributive rule explains how "audit log" came to name a log nobody audits.
   The rename recommendation continues to live under the relationship section
   there.

### 3. What a journal is for

1. **done.** Recordkeeping is an integrity property, not a feature. Summarized
   as "Authority" under The distinctions: a journal is the record, a log is best
   effort. The definitive text stays in `audit_approach.md`, where the section
   is now "Recordkeeping as an integrity property".
2. **drop.** The auditable event is the persistent state change; the entry is a
   reflection and recording of it. It was held open in case section 5 needed it.
   It does not: section 5 is illustrative rather than persuasive, so a precise
   understanding of "event" is not required to read it.
3. **done.** The obligation comes from delegated authority: the duty to account.
4. **done.** Double entry bookkeeping named. Resolved where the item arose, by
   removing the claim from `entity_model.md` rather than by repeating it here.
   Checking it found the causal claim contested: recent work dates the method to
   Florentine moneychanger-bankers and attributes the catalyst to cash shortage
   and defence against disputes, not to agents accounting to principals. What
   survives there does not depend on the history: the duty to account is an
   agency duty, so the account exists because authority was delegated. Double
   entry is still named in `journal_vs_log.md`, where the use is definitional
   and makes no claim about origins.
5. **drop.** The ideal is one entry per state change; reality diverges. Not a
   point about how the two kinds of record are used, which is this document's
   subject, and the full story is already in `audit_approach.md`.
6. **done.** Reconciliation in the accounting sense. Stated as a contrast under
   The distinctions: a log cannot be reconciled against reliably, because a
   discrepancy might be a divergence or might be a line nobody wrote. Against a
   journal one explanation is left, so it can be escalated.
7. **drop.** Three exhaustive divergence classes.
8. **drop.** Correspondence keys.
9. **drop.** Ordering of entry and effect.
10. **drop.** One transaction collapses the ordering problem.
11. **drop.** Detection is not resolution.
12. **drop.** Who reads each, and why. Not a distinction but a side effect of
    what each is made for, and the purposes are already stated. Reworded once
    before dropping, because the original made a process into a reader.

### 4. What a log is for

1. **done.** Telemetry and forensics. Not a property of logs at all: both are
   consulted for both purposes, so this belongs with the resemblances and is now
   a clause under Why they are so easily confused. The related point that
   auditing is not forensic investigation, one looking for unknown anomalies and
   the other working outward from a known one, is recorded in
   `audit_approach.md`.
2. **done.** Records **activity**, with attempting, succeeding, failing, being
   asked, and declining to act as exemplars. Symmetry needed no separate
   statement once the exemplars carry it.
3. **done.** Arranged for whoever reads it later: levels and filters to find
   the interesting lines, retention chosen by weighing storage against how far
   back anyone will look. Both possible only because nothing depends on the log
   being complete.
4. **done.** Deletable, droppable, and samplable, all covered by the modern
   sense and by the retention point. **Licensing is out of scope for the proof
   of concept**, so that clause is dropped rather than owed.
5. **done.** No completeness obligation, so gaps are not defects.

### 5. Properties where the two diverge

The core of this section is expected to be a table over these axes.

1. **done.** Completeness. Stated under The distinctions rather than in a
   table.
2. **done.** Permanence. Stated under The distinctions.
3. **drop.** Availability, meaning unconditional versus licence-gated.
   Licensing is out of scope for the proof of concept.
4. **open.** Filterability.
5. **open.** Unit of record: persistent state change versus request.
6. **open.** Attempts.
7. **open.** Failure symmetry.
8. **open.** Actor: any entity kind versus a user.
9. **open.** Ordering: distinct monotonic identifiers versus timestamps that tie.
10. **done.** Reconcilability. Stated under The distinctions.
11. **open.** Mutability.
12. **open.** Transactionality.
13. **open.** Unbypassability.
14. **open.** Evidentiary standing: a filtered record is not evidence.
15. **done.** Points of similarity, and why the two are so often confused. The
    resemblance is in the form of an entry; the difference shows in what a
    missing entry means. Added after the first draft was read. Numbered here
    because it is the complement of this section, though it appears earlier in
    the document.

### 6. Evidence in this codebase

1. **open.** `audit_logs` is request-shaped: `ip`, `user_agent`, `status_code`,
   `request_id`.
2. **open.** `user_id NOT NULL` with no actor type.
3. **open.** `resource_type` has 36 values, a quarter of them settings or events
   rather than entities.
4. **open.** `workspace_agent` is in that enum with zero coverage, which is P7.
5. **open.** `id` is a uuid; ordering comes from an index on `"time" DESC`, so
   same-transaction writes have no defined order.
6. **open.** `diff jsonb` holds field-level before and after.
7. **open.** Retention deletion exists as its own queries. Needs 10.2.
8. **open.** Registering one audited resource touches eight places plus
   `auditdocgen`.
9. **open.** `resource_type` is a Postgres enum, carrying the single-transaction
   migration trap.
10. **open.** Written from handlers through middleware, so non-HTTP paths bypass
    it. Needs 10.3.
11. **open.** `connection_logs` split out and four enum values were deprecated.
    Needs 10.4.
12. **open.** `user_status_changes` and `user_deleted` are journal-spirited but
    trigger-written, and a trigger cannot see the actor.
13. **open.** `provisioner_jobs` is the existing intent-record pattern.
14. **open.** `workspace_agents.auth_instance_id` is the nearest external
    correspondence identifier, and it is inbound only.

### 7. Counterarguments to answer

1. **open.** Why not extend `audit_logs`?
2. **open.** Why not just add enum values?
3. **open.** Is this not duplication?
4. **open.** Can one be derived from the other?
5. **open.** Why not triggers, as with the user history tables?
6. **open.** Why not a transaction manager?
7. **open.** Why two systems rather than a migration?

### 8. The concrete instance, so the abstract lands

1. **open.** One journal for every entity, with `(type, identifier)` pairs.
2. **open.** `BIGSERIAL`, because entries need distinct identifiers.
3. **open.** Written in the same transaction as the state change.
4. **open.** Append only: no update or delete query exists.
5. **open.** Guarded so that an entity cannot write entries about itself.
6. **open.** A closed type set.

### 9. Deliberate omissions, named as such

1. **open.** No before-and-after values, by scope decision.
2. **open.** No external correspondence identifier, so orphan detection is
   unreachable.
3. **open.** No reconciliation code at this phase.
4. **open.** No credential lifecycle entries, reproducing P7 knowingly.
5. **open.** No tamper evidence, such as hash chaining.

### 10. To verify before drafting the sections that need it

1. **drop.** The exact licensing gate for the existing mechanism. Nothing
   depends on it now that licensing is out of scope.
2. **open.** The retention deletion query names, and what schedules them.
3. **open.** Whether anything writes `audit_logs` outside the HTTP middleware
   path.
4. **open.** Whether `connection_logs` has the properties of a log or of a
   journal. It is the most recent split and may be a useful precedent either
   way.

## Decisions taken

**The framing is etymological, and that is a choice about audience.** Starting
from ship's logs and church offices gives a human reader a reason to accept the
distinction before meeting any schema, and gives a later session the same reason
without the conversation. The alternative, opening with the property comparison,
was rejected as persuasive only to someone already convinced there is something
to compare.

**Items 2.6 and 2.7 are held back rather than dropped.** Both concern the name
`audit_logs`, and land better beside the comparison than in the framing.

**Items 3.7 through 3.11 are dropped.** Divergence classes, correspondence keys,
the ordering of entry and effect, the effect of a single transaction, and
detection versus resolution are all stated in `audit_approach.md`. Restating
them here would create two homes for one position, and the two would drift. The
argument against dropping, that this document should be readable alone, was
weighed and rejected: it is readable alone as an account of the distinction,
which is its subject, and a reader who needs the mechanics of reconciliation is
already being sent to the approach document.

**"The distinction" became "The distinctions", and is where contrasts live.**
The document does not need every property of a journal, only those where the
purpose leads somewhere a log does not go. Authority, completeness, permanence,
and reconcilability are stated there. Items were closed against that section
rather than against the table originally imagined for section 5.

**Licensing is out of scope for the proof of concept.** Whether the existing
table is gated behind a licence has no bearing on anything being built, so 4.4's
clause, 5.3, and 10.1 are all disposed of on that basis.

**Section 5 is illustrative, not persuasive.** The table of properties shows
where the two kinds of record differ. It is not required to argue for each
difference or to rest on definitions established elsewhere. Several items were
dropped on that basis, including 3.2, since a reader does not need a precise
account of "event" to follow an illustration.

**"Audit journal" is retired, not merely deprecated.** The defect is the
attributive use of an action word, which turns the action into a kind of thing.
Accounting qualifies a journal by its contents and never by its purpose or its
audience, which is why `entity_journal` is well named and why purpose belongs in
a sentence rather than in a compound. Recorded in `audit_approach.md` under
Terminology.

**The similarities are stated before the distinction, not after.** A reader who
has confused the two is owed an account of why the confusion is reasonable
before being told it is wrong. Item 5.15.

## Coupling, and where this belongs

An earlier draft said a write ahead log was "nearly the opposite" of the journal
described here. That is wrong, and wrong in a way worth keeping, because
correcting it produced something more general than the sentence it replaced.

A write ahead log is the same pattern, not the opposite one. It records events in
the order they occur, and the datastore it feeds stands exactly where a ledger
stands: the derived view, built by posting the entries.

Discarding is incidental rather than essential. A write ahead log is not
discarded immediately, because there is a lag between the entry and the update
of the datastore. It becomes discardable afterwards only because nothing
ordinarily consults anything but the datastore. A temporal database effectively
never discards it, since answering what the data was at a past instant means the
entries are still doing work.

**What actually differs is coupling.** A datastore and its journal have no
daylight between them, by design and by construction, so they cannot disagree.
There is therefore no analogue of reconciliation, and nothing for one to
perform. A journal in the audit sense accounts for a world it does not
construct, so the two can disagree, and detecting that is the point.

**This generalizes, and the general statement is owed to
`poc_audit/audit_approach.md`, beside reconciliation.** Coupling determines
whether reconciliation is needed at all, with a write ahead log at one end of
the scale and an audit journal at the other. Our own design sits at both ends
depending on where the effect is. The `ai_agents` row and the entry accounting
for it are written in one transaction in one database, which is the tightly
coupled end: nothing to reconcile, which is why `CreateAIAgent` says that where
entry and effect are both rows in one transaction the ordering problem does not
arise. Reconciliation becomes necessary as effects move outside the database,
which is where sandboxes and AI agent processes will put them.

It is deliberately not in `journal_vs_log.md`. The document needs only to stop a
reader importing write ahead log assumptions. The argument above is a digression
at the top of a document whose subject is something else.

## Language settled, and language rejected

**Rejected: treating dead reckoning as an argument for reconciliation.** The
parallel is tempting, since a dead reckoned position drifts and is corrected
against the world. It is wrong. Taking a star sight replaces an estimate with a
better observation; reconciling an account compares a record of what was done
against what is there. The draft keeps only the part that carries, which is that
a series of measurements never established position.

**Kept: the paragraph answering the ships objection.** The modern-sense section
ends by conceding that `audit_logs` records requests rather than measurements,
and answering that the framing never rested on content. Without it the document
invites a one-line rebuttal that the analogy simply does not apply.

**Kept: "book of original entry".** It is the bookkeeping term of art, and it
says exactly what the design does, that current state is derived by folding the
journal rather than stored alongside it.

**Rejected: "nearly the opposite of the sense used here", of a write ahead log.**
It is the same pattern rather than the opposite one. See the section above.

**Avoided: "immutable" and "append only" in the framing.** Both are mechanism
words and both invite the write ahead log reading the document is trying to
dispel. The framing says permanent instead.

## Research validated

Both etymological claims were checked rather than asserted. This was not on the
original list; it arose from the request to validate them.

- Log is the wooden object first. Chip log, log-line, sandglass, count the knots.
  The knot as a unit of speed dates from the 1630s.
- There was an intermediate artifact, the log-board, wiped and reused, with
  readings transferred into the permanent log book about once a day.
- `log-book` is attested from the 1670s, `log` for a ship's progress from 1842,
  and the general sense of any ordered record of facts only by 1913. The
  generalization is recent.
- Journal is from Late Latin `diurnalis`, daily, and enters English in the mid
  fourteenth century as a book of the daily church offices.
- The accounting sense follows in the late fifteenth century in English, and
  classical Latin `diurnus` already carried a noun sense of account-book. The
  accounting use is a recovery rather than a drift.
- In double entry practice the journal is the book of original entry, posted
  afterwards to the ledger.

Sources are listed in the document itself, so that they travel with the claims.
