# Journals and Logs

Recorded 2026-08-13. This document distinguishes a journal from a log, and says
why the difference means the audit approach cannot be served by the mechanism
this codebase already calls `audit_logs`.

It is the framing for `poc_audit/audit_approach.md`, which states the approach
itself. Where that document says what to build, this one says why something new
has to be built at all.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built
on top of them, which is offered for challenge rather than settled.
**Findings** records verifiable facts about the existing codebase. **Open**
records questions not yet answered.

**This document is a draft.** The comparison with the existing mechanism, the
evidence for it, and the answers to the obvious objections are not written yet.
What remains, and what has been deliberately left out, is tracked in
`poc_audit/journal_vs_log.working_state.md`.

## Established

### Two words from two practices

Both words were borrowed into computing from older record keeping, and both
brought their purposes with them. The purposes are different, the words are now
used as though they were interchangeable, and that is what this document exists
to correct.

### A log records measurements

A log was a piece of wood. Attached to a knotted line and thrown from the stern,
it stayed roughly where it was dropped while the ship moved away, and the length
of line paid out during one turn of a sandglass gave the ship's speed. The knots
on that line are why speed at sea is still measured in knots.

The reading was chalked onto a log-board, a plank that was wiped and reused, and
about once a day the readings were copied into the permanent log book. Only much
later did "log" come to mean a record at all: a ship's progress from 1842, and
anything at all entered in order by 1913.

Three properties of that practice survive into what we now call a log:

- Its content is **measurements**, not events. A reading is an observation of the
  world, not something that happened to it.
- It is **sampled**. One reading per turn of the glass, and nothing between them.
- Its purpose is **estimation**. Speed and heading were integrated to estimate
  position, which is dead reckoning, and a dead reckoned position drifts. It was
  corrected by an external observation of the sun or a star. A series of
  measurements never established where the ship was. It only ever estimated.

### The word outgrew the measurements

"Log" did not stay with soundings and speeds. By 1913 it named any record of
facts entered in order, and usage went further still: a log records activity of
any kind, including attempts, requests, and things that changed nothing.

What generalized was the discipline, not the subject matter. A modern log is
still **sampled**, now by level, by sampling rate, or by whether the code path
bothered to write at all. It is still **kept for as long as it is useful**, with
retention chosen for cost rather than for obligation. A missing line is still a
**nuisance rather than a defect**.

This matters for the comparison ahead. The table named `audit_logs` records
requests, not measurements, and someone may reasonably object that a framing
built on ships has nothing to say about it. The answer is that the framing was
never about measurement. It is about what a record is for and what a gap in it
means, and on both counts `audit_logs` is a log in the old sense exactly. (The
name is half right: it is definitely a log, and definitely not an audit. An
audit is something a party does, and nothing there does it.)

### A journal records transactions

Journal means daily. Through Anglo-French from Late Latin `diurnalis`, it enters
English in the mid fourteenth century as the name for a book of the daily church
offices. By the late fifteenth century it names a book of daily accounts, a
sense classical Latin already had for `diurnus`.

In double entry bookkeeping a journal is a **book of original entry**. It sits at
the very beginning of the recordkeeping process, and nothing precedes it.
Transactions are written there first, in the order they occur, and are afterwards
posted to the ledger. The journal is where an event is recorded for the first
time; everything else is derived from it.

Three properties, opposite the three above:

- Its content is **transactions**. Things done, not things observed.
- Entries are **original and primary**. Other views, including any statement of
  current state, are derived by reading the journal.
- Its purpose is **to account**. A journal exists because someone is owed an
  answer to what was done, which is the duty an agent owes a principal.

### Why they are so easily confused

The two are not confused carelessly. They resemble each other in nearly every
respect that is visible.

- Both are chronological. Entries are added in the order events occur, and in
  normal operation are not rewritten afterwards.
- Both are written as a side effect. Nobody's purpose is to produce the record.
  It accompanies work done for another reason.
- Both grow without bound, so both raise the same operational questions of
  storage, indexing, and querying by time.
- Both are read backwards, by someone reconstructing what happened after the
  fact.
- Both call their elements entries.
- Both were books. A ship's log book and a merchant's journal were each a bound,
  dated record of a working day, kept by an authority and written up at
  intervals. The words drifted toward each other because the artifacts really
  were alike.
- Both are usually built the same way, as rows appended to a table or lines
  appended to a file. The mechanism does not distinguish them.
- Most of all, one entry from each looks like one entry from the other. The core
  columns line up almost exactly: a time, something identifying who or what was
  involved, and a name for what occurred.

So the difference cannot be seen in the shape of an entry. It can be seen in
what a missing entry means. In a log, a gap is a nuisance: the estimate is
poorer, the investigation is harder. In a journal, a gap is a failure of the
property the journal exists to provide, because an account that omits a
transaction is not an account of anything.

That is the test to apply when it is unclear which kind of record something is.
Ask what follows from losing one entry. If the answer is that somebody would
like it back, it is a log. If the answer is that the record can no longer be
relied upon, it is a journal.

### The distinction

A log is a record of what was observed, kept so that something can be estimated.
A journal is a record of what was done, kept so that someone can be answered.

Everything else follows from that. A log may be sampled, because an estimate
tolerates gaps. A journal may not, because an account with gaps is not an
account. A log may be discarded once the estimate it supported is no longer
interesting. A journal may not, because the obligation outlives the moment.

### Two hazards in the word "journal"

**A journal is not a ledger.** A journal is chronological and a ledger is
organized by account. What we are building records events in the order they
happened, so it is a journal. Anyone reaching for "ledger" is reaching for the
derived view, which we do not have.

**A journal here is not a write ahead log.** The resemblance is real rather than
false: a write ahead log is also a chronological record of events, and the
datastore it feeds stands where a ledger stands. What differs is that a
datastore is built from its journal by construction, so the two cannot disagree.
The journal described here and the world it accounts for can disagree, and
noticing when they do is the reason it exists.

### Sources for the etymology

The dates and the practices above are checkable, and are recorded here so that
nobody has to take them on trust.

- Log, and the 1842 and 1913 datings: <https://www.etymonline.com/word/log>
- The chip log, the log-line, and the knot as a unit:
  <https://en.wikipedia.org/wiki/Chip_log>
- The log-board and its daily transfer into the log book:
  <https://en.wikipedia.org/wiki/Logbook_(nautical)>
- Journal, `diurnalis`, the mid fourteenth century church sense, and the late
  fifteenth century accounting sense: <https://www.etymonline.com/word/journal>
- Day-book, for contrast, being a separate and later English compound:
  <https://www.etymonline.com/word/day-book>
