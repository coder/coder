# Journals and Logs

Recorded 2026-08-13. This document distinguishes a journal from a log, and says
why the difference means the audit approach cannot be served by any of the logs
this codebase already has.

It is the framing for `poc_audit/audit_approach.md`, which states the approach
itself. Where that document says what to build, this one says why something new
has to be built at all.

**It exists because of a name.** A reader meeting `audit_logs` first will take
it for the audit system, and will then ask, reasonably, why a second thing is
being built. The answer needs the distinction this document draws, and needs it
before anything else about the design can be discussed. That question is
answered under "Why not extend `audit_logs`?" below, and everything before it is
what that answer rests on.

Two readers are in view. A colleague who has to be persuaded, and who is
entitled to check rather than believe. And a later session picking this work up
without the conversation that produced it, for whom a position recorded without
its reasons is worth little. Both are served the same way: every factual claim
is attributed to something that can be looked at, in this codebase under
Findings, and in the history of the words under Sources.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built
on top of them, which is offered for challenge rather than settled.
**Findings** records verifiable facts about the existing codebase. **Open**
records questions not yet answered.

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
facts entered in order, and usage went further still: a log records **activity**.
Attempting something, succeeding, failing, being asked, declining to act. Things
that changed nothing are as much at home there as things that changed
everything.

What generalized was the discipline, not the subject matter. A modern log is
still **sampled**, now by level, by sampling rate, or by whether the code path
bothered to write at all. It is still **kept for as long as it is useful**, with
retention chosen for cost rather than for obligation. A missing line is still a
**nuisance rather than a defect**.

Everything about a log is arranged for whoever reads it later. Levels and
filters exist so that the interesting lines can be found among the dull ones.
Retention is set to a period somebody chose, weighing storage against how far
back anyone is likely to look. Both are sensible, and both are possible only
because nothing depends on the log being complete.

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
  fact. Telemetry and forensic investigation draw on either, and the particulars
  differ without the purpose differing.
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

### The distinctions

A log is a record of what was observed, kept so that something can be estimated.
A journal is a record of what was done, kept so that someone can be answered.

The rest follows from that. Not every property of a journal is worth stating
here, only those where following the purpose leads somewhere a log does not go.

**Authority.** A journal is authoritative. It is the record, and where it and
some other account of the same events disagree, the other account is the one in
question. A log is best effort: useful when it is there, unremarkable when it is
not, and never by itself the thing that settles a question.

**Evidentiary standing.** A journal has to satisfy a party that did not produce
it, because that is what an audit is: someone skeptical asking whether the
account is faithful. A record that may be sampled, filtered, or trimmed cannot
do that work, since any absence in it has an innocent explanation available and
therefore proves nothing. Authority settles which account prevails between
parties who already accept the records. Evidence is what persuades a party who
accepts nothing yet.

**Completeness.** A log may be sampled, because an estimate tolerates gaps. A
journal may not, because an account with gaps is not an account.

**Unbypassability.** A log tolerates being bypassed. Code paths that write no
line are ordinary and nobody calls them defects. A journal cannot: a write path
that produces no entry leaves the account incomplete, and an incomplete account
is not an account. This is what turns completeness from an aspiration into a
constraint on the whole system, because it is a claim about every path that
changes state rather than a claim about the journal.

**Permanence.** A log may be discarded once the estimate it supported is no
longer interesting. A journal may not, because the obligation outlives the
moment.

**Reconcilability.** A journal can be reconciled against the world. A log cannot
be, not reliably. Reconciling compares a record of what was done against what is
there, and a record permitted to omit things cannot support the comparison: a
discrepancy might be a real divergence, or might be a line nobody wrote. Against
a journal the same discrepancy has one explanation left, which is why it can be
escalated rather than shrugged at.

The rest are worth listing rather than arguing. Each follows from the six above.

| Property       | A log                                  | A journal                                       |
|----------------|----------------------------------------|-------------------------------------------------|
| Unit of record | An activity                            | A persistent state change                       |
| Attempts       | Recorded, and often the point          | Absent, because nothing changed                 |
| Failures       | Recorded alongside successes           | Recorded only where state changed anyway        |
| Actor          | Whoever asked, where anyone did        | Whoever acted, of whatever kind                 |
| Ordering       | By time, which ties                    | By distinct identifiers, so sequence is settled |
| Filtering      | At write time, by level or by sampling | None available; a filter is a gap               |
| Mutability     | Trimmed on a retention schedule        | Append only                                     |
| When written   | Out of band, after the fact            | With the state change it accounts for           |

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

### Why not extend `audit_logs`?

**`audit_logs` is a log. This work needs a journal. One record cannot serve both
well, because the properties that make a log useful are the same ones that
disqualify a journal.**

That is the answer. The question is a fair one and the burden is on the new
thing, since two mechanisms covering adjacent ground is a cost paid by everyone
who later has to work out which of them answers a question.

The question is usually asked about `audit_logs`, and only its name makes it the
one that gets asked about. There are at least four logs in this codebase, listed
under Findings, and none of them behaves like a journal. Take the name away and
the question has two forms: **why not use one of the logs that already exist**,
and **why not add another log**. Both have the same answer, the one above. A
fifth log would be one more record with a log's properties, and the properties
are the problem.

The name is doing work the contents do not support. Were that table called
something like `user_http_logs`, after what it actually holds, the question
would probably not arise in this shape at all. The recommendation to rename it
stands in `poc_audit/audit_approach.md`.

The rest of this section is why each of the three claims holds.

#### `audit_logs` is a log

This is not a complaint about it. It records activity rather than state changes.
It can be filtered before anything is stored, trimmed by age, and replaced by an
implementation that writes nothing at all. Its required columns describe an HTTP
request made by a person, down to an icon for displaying the result.

Every one of those is right for what it does. A mechanism answering "who did
what through the API, and when" should be cheap to write, cheap to keep, and
easy to leave out of a hot path. See Findings below for each of these.

#### This work needs a journal

The distinctions above say what that requires: authority, completeness,
unbypassability, permanence, reconcilability. The demanding one is evidentiary
standing. The account has to satisfy a party who produced none of it and who
accepts nothing yet, and a record with permitted gaps cannot do that, because
every absence in it has an innocent explanation available.

#### One record cannot serve both well

The obligations contradict each other pairwise. Deletable against permanent.
Droppable against complete. Bypassable against unbypassable. Shaped for a
request against shaped for a state change. A single record would have to honour
both halves of each pair at once.

The strongest form of the question is whether one could be derived from the
other, so that only one need be kept. It cannot, in either direction. A journal
cannot be recovered from a log, because a log may omit and a gap cannot be
filled afterwards, and because a state change with no request behind it leaves
no line to recover from. A log cannot be recovered from a journal, because the
journal deliberately holds no attempts, no reads, and no failures that changed
nothing. Each is missing precisely what the other exists for.

Whatever "extend" is taken to mean, it meets that wall. Writing journal entries
into the table means filling required request columns with placeholders chosen to
satisfy the schema, and then watching retention remove the rows anyway. Changing
the table means making those columns nullable and breaking the contract every
existing reader relies on, teaching retention an exemption so that completeness
comes to depend on a boolean the purge must respect forever, taking away the
filter's ability to drop a record, and making writes transactional with the state
change they account for. At the end of that there are two tables sharing one
name. Replacing it outright is the honest reading and much the largest, and it
would take away a feature people use for reasons unconnected to this work.
Migrating into it is not a fourth option: there is nothing to migrate into that
would hold what is needed.

Nor is extending the cheaper path. Registering one new audited resource already
touches eight places and a documentation generator, and none of that goes away if
the properties have to change as well.

#### What this is not saying

The existing table is not defective, and nothing here should be read as arguing
that it be removed or rebuilt. That an unused value already sits in its enum
shows the enum was never the obstacle. The properties are.

Both records contribute to an audit trail, which is a collection assembled from
whatever sources exist rather than a single table. Neither supersedes the other.

The last argument is about risk rather than design. Building beside the existing
table is reversible: if the two should later merge, they can. Changing what the
existing table guarantees is not reversible in the same way, and a mistake there
breaks something already in use by people who never asked for any of this.

### Why not add a value to `audit_logs.resource_type`?

This is the cheapest form of the previous question: leave everything as it is,
add `ai_agent` to the `resource_type` enum, and let the existing machinery
record what happens to AI agents.

The answer is the same, and this codebase has already run the experiment.

Connections used to be recorded that way. The `audit_action` enum still carries
`connect`, `disconnect`, `open`, and `close`, marked in a comment as deprecated
and no longer used, "these events are now tracked in the connection_logs table".
Migration `000349` moved them out into a table shaped for what they are, with
its own retention setting and its own writer.

So the enum was never where the difficulty lay. A value in it costs nothing; the
coverage is the work, and what the coverage buys is a record with a log's
properties. When the properties did not fit before, the answer here was a new
table, not a new enum value.

### Is this not duplication?

Only at a coarse enough granularity.

The two records sit at different levels of the stack. `audit_logs` records that
a request was made and answered, which is what lets someone trace **how** an
action came about: which user asked, from where, and what the API said back. The
journal records the action itself, that an AI agent came into existence and
which party is answerable for it.

Creating an AI agent through the API would produce a row in each, and the two
would stand in a one to one relationship. That still does not make them one
record. A request that caused a thing is not the thing, and a correspondence
between two records is not an identity between them. They read as the same event
only when the granularity of vision is large enough to lose the difference.

Nor is the correspondence general. A state change made by a background loop
produces an entry and no request at all, so on that side there is nothing to be
duplicated. And where both do exist, neither can be folded into the other, for
the reason given above: neither is derivable from the other.

### Why not use triggers, as the user history tables do?

A trigger is subordinate by construction. It fires because something else
happened, and that something else has already been written. Whatever the trigger
records is therefore not the original entry. The row change that fired it was.

That inverts the relation this design rests on. Arranged correctly the journal
is the origin and current state is derived from it. Trigger-written, the state
comes first and the entry is derived from the state, which puts the entry in the
ledger's position rather than the journal's. What results is a change-log
belonging to a table, and a table's change-log cannot be an account of the
world, because it knows only what that table knows.

The subordination is narrower still: a trigger fires on a database action
specifically, an insert, an update, or a delete, and on nothing else. A change
made outside the database, a process started or an external call completed,
cannot be recorded this way at all. The reach of a trigger-written journal is
exactly the set of changes already representable as rows, which is smaller than
the set an account has to cover.

The familiar objection, that a trigger cannot see the actor, turns out to be a
consequence of the same thing rather than a separate complaint. A trigger was
not present at the act. It sees `NEW` and `OLD`, the residue the act left in a
table, and not who performed it. Mitigations exist, such as carrying the actor
in transaction local state, and they trade away the simplicity that made a
trigger attractive to begin with. The tradeoff is set out in
`poc_audit/audit_approach.md` under the unbypassability and actor tension.

None of which makes a trigger the wrong tool. It makes this direction the wrong
one. Fired **on** the insertion of an entry, to drive whatever should follow
from it, a trigger is doing something the vocabulary already has a word for.
That is posting. It is also the one position where the unbypassability a trigger
offers is worth having, since what it guards there is the derivation rather than
the origin.

`user_status_changes` and `user_deleted` are the local example. Both are written
by `record_user_status_change`, a trigger on the `users` table, which is why
they are a change-log of that table rather than a journal of events, and why the
actor is missing from them.

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

## Findings

Verifiable facts about this codebase, recorded so that the positions above can
be checked rather than taken on trust.

### There are at least four logs already

`audit_logs` is not the only one, and it is singled out above only because of
its name. Four tables in this schema are logs, each with its own retention
setting in `coderd/database/dbpurge/dbpurge.go`, and each deleted by age:

- `audit_logs`, requests made through the API.
- `connection_logs`, connections to workspaces.
- `boundary_logs`, network events captured against a session and numbered by a
  sequence within it.
- `workspace_agent_logs`, output from the agent and the scripts it runs.

`provisioner_job_logs` is a fifth, tied to the lifetime of its job rather than
to a retention setting.

None of them has the properties of a journal, and two go further than deletion
by age.

`workspace_agent_logs` is declared `CREATE UNLOGGED TABLE`. Postgres does not
write an unlogged table to its own write ahead log, does not replicate it, and
**truncates it after a crash**. That is a deliberate trade of durability for
write speed, entirely reasonable for script output, and it means a record that
may simply be gone after a restart.

`connection_logs` rows are updated in place. `BatchUpsertConnectionLogs` does
`ON CONFLICT (connection_id, workspace_id, agent_name) DO UPDATE`, revising
`connect_time` and `disconnect_time` as later events arrive, because a row
stands for a connection rather than for an event. Its merge key includes
`connection_id`, which the schema comment describes as originating from the
agent and "not guaranteed to be unique", so two connections can be folded into
one row. Both are acceptable in a log and disqualifying in an account.

Two of the four also have writer implementations that discard everything, in
`coderd/audit/audit.go` and `coderd/connectionlog/connectionlog.go`.

### The shape of `audit_logs`

Fifteen columns, thirteen of them `NOT NULL`. Only `ip` and `user_agent` are
nullable. Among the required ones are `user_id`, `organization_id`,
`request_id`, and `status_code`, each of which presumes an HTTP request made by
a person, and `resource_icon`, which is a presentation detail.

`id` is a uuid, and ordering comes from an index on `"time" DESC`. Two rows
written in one transaction share a time and have no defined order between them.

`resource_type` is a Postgres enum of 36 values. It already contains
`workspace_agent`, which has no mention in `coderd/audit/diff.go` or
`enterprise/audit/table.go`.

See the `audit_logs` table in `coderd/database/dump.sql`.

### What removes rows from `audit_logs`

`DeleteOldAuditLogs`, in `coderd/database/queries/auditlogs.sql`, deletes every
row older than a cutoff. Nothing exempts a row: no column marks one as spared
and no branch of the query skips one. The cutoff comes from the deployment's
audit log retention setting, and `coderd/database/dbpurge/dbpurge.go` runs the
purge on a ten minute ticker.

The deprecated connection actions are excluded from that query and removed by
their own, with their own maximum age.

### What writes to `audit_logs`

`InsertAuditLog` has two callers. The production one is
`enterprise/audit/backends/postgres.go`, whose `Export` runs after a `Filter`
has returned a decision. `FilterDecisionDrop` is documented as meaning the
record "should not be stored or exported anywhere". The default filter stores
and exports everything, but the ability to drop is part of the design rather
than an oversight.

The `Auditor` interface also has a no-op implementation whose `Export` returns
without writing anything.

The other caller, in `coderd/audit.go`, is a handler that generates fake entries
for development.

### What registering a new resource in `audit_logs` costs

Eight places, plus a documentation generator. The enumeration is in
`poc_audit/audit_approach.md` under the cost of registering with the existing
machinery, and is not repeated here.
