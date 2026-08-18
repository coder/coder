# Working state for journal_vs_log.md

Recorded 2026-08-13, trimmed 2026-08-18.

The document this file supported is finished, and the punch list that tracked
its sections has been removed. It had done its work and held nothing worth
returning to.

**What remains is material developed alongside the document that has not been
recorded anywhere else.** Some of it is owed to another document and says so.
Some exists to stop a later session redoing work already done, or reintroducing
a formulation already rejected. Some is probably discardable. It has not been
sorted, and sorting it is a conversation still to have.

## Decisions taken

**The framing is etymological, and that is a choice about audience.** Starting
from ship's logs and church offices gives a human reader a reason to accept the
distinction before meeting any schema, and gives a later session the same reason
without the conversation. The alternative, opening with the property comparison,
was rejected as persuasive only to someone already convinced there is something
to compare.

**Reasoning already in the approach document was not repeated.** Divergence
classes, correspondence keys,
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
and reconcilability are stated there, rather than in the table of properties
originally imagined.

**Licensing is out of scope for the proof of concept.** Whether the existing
table is gated behind a licence has no bearing on anything being built, so every
point resting on it was dropped.

**The draft notice is gone from the document.** It would be a distraction once
the pass is finished, and the deferred worked example is recorded outside this
file rather than as a caveat inside the document. This also removes the
document's only pointer to this file; `poc_audit/AGENTS.md` still indexes both.

**The preamble carries what the document says about itself.** It names the
failure mode it prevents, names its two readers, and states the evidence
standard it holds itself to. A separate section for any of that would have been
odd, since none of it is about the subject.

**A worked example, the deliberate omissions, and the surplus evidence were all
dropped.** The example from our own code waits until after delivery, since the
code is not worth holding up yet. The omissions are implementation detail and
proof of concept scope, neither of which belongs in a document about two kinds
of record. The remaining evidence then had nothing left to support, each piece
having been gathered for an argument since answered another way or dropped.

**Triggers fail on ordering, not on attribution.** The answer was going to
argue that a trigger cannot see the actor. The deeper reason is that a
journal is the book of original entry and a trigger is subordinate to a prior
database write, so a trigger-written entry is derived from state rather than
being its origin. That inverts the journal and ledger relation, which is the
same inversion as the write ahead log hazard running the other way. The actor
problem then follows: a trigger was not present at the act and sees only the
residue.

**`audit_logs` is not special, and the first finding says so.** There are at
least four logs in this codebase, none behaving like a journal, and one of them
is an `UNLOGGED` table that Postgres truncates after a crash. That changes the
shape of the answer: the question only fastens on `audit_logs` because of its name, and
splits into why not an existing log and why not a new one, with the same answer
to both. The name would not invite the question if the table were called
something like `user_http_logs`.

**The main answer states its conclusion before its reasons.** The first draft
opened by
conceding that the question was fair and then walked three readings of "extend",
which buried the conclusion. It now leads with the three claims and gives the
concession one sentence afterwards. The technical detail sits under the third
claim, where it is a demonstration rather than the argument's structure.

**Findings became its own section, and the etymology did not move.** The
codebase facts are separated from the arguments they support, so a skeptical
reader can check them without first accepting the reasoning. The etymology stays
in place: its citations are there for the curious rather than to carry an
argument, and cutting the history out of the narrative would break the thing
that does the persuading. "Why they are so easily confused" also stays in
Established, since its purpose is to set out the similarities that make the
differences matter rather than to assert that anyone is confused.

**The main answer points at Findings rather than carrying its evidence.** It
was written
the other way first, with the schema and the retention query inline, and split
once the argument and the facts proved easier to check apart. The three readings
of "extend" read more tightly for it.

**Two properties were elevated, the rest tabulated.** Unbypassability and
evidentiary standing became paragraphs under The distinctions; the remaining
eight are a table at the end of it. The table is deliberately terse, since each
row follows from a paragraph above it.

**The table of properties illustrates, it does not persuade.** It shows where
the two kinds of record differ, and is not required to argue each difference or
to rest on definitions established elsewhere. Several candidates were dropped on
that basis, including a precise account of "event", which a reader does not need
in order to follow an illustration.

**"Audit journal" is retired, not merely deprecated.** The defect is the
attributive use of an action word, which turns the action into a kind of thing.
Accounting qualifies a journal by its contents and never by its purpose or its
audience, which is why `entity_journal` is well named and why purpose belongs in
a sentence rather than in a compound. Recorded in `audit_approach.md` under
Terminology.

**The similarities are stated before the distinctions, not after.** A reader who
has taken the two for one thing is owed an account of why that is reasonable
before being told it is wrong.

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
