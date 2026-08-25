# Overloading the Users Table

**The thesis.** `users` was converted from a primitive type into a sum type, a
discriminated union, and the conversion was never named as one. Eric,
2026-08-25. Everything below is that one problem: how it happened, what it has
cost, and what it will cost to undo.

**Who this is for.** Somebody planning work on `main`. **The argument does not
depend on the experiment during which it was written**, which is the point: this
needs doing whether or not that work proceeds. The measurements were taken while
it was in flight, and where a fact comes from the experiment rather than from
`main` it says so.

**What a sum type needs, and what `users` got.** A discriminant: `users` grew
three, one per variant, rather than one with three cases. A common interface, the
operations valid whatever the variant is: none was ever defined, so every call
site decides for itself whether it is handling the union or one member of it. And
acknowledgement: the type has no name, no documentation, and no stated invariant,
which is why each variant's arrival looked like a local change.

Three consequences follow, and each explains a symptom below.

**The placeholders are a struct pretending to be a union.** A variant that must
fill `email`, `hashed_password` and `login_type` with stand-ins it has no use for
is a member of a union stored in the shape of the widest member.

**The predicates are the missing interface, discovered one call site at a time.**
Every `is_system = false` filter is somebody hand-writing "this operation is
valid only on the human variant", locally, without knowing who else needs the
same line. An interface would have said it once.

**And the same defect exists one level up, where it has already been named.**
`api_keys.user_id` became a sum type when a holder could be an AI agent, and
`database.HolderID` with its deliberately ugly `AsUserIDUnchecked` is the first
acknowledgement of that anywhere in this codebase: a distinct type, and every
site that assumes a variant marked as assuming. **So the tactic for `users` is
already demonstrated in tree.** The count below is what that acknowledgement
exposed when it was applied to holders.

**Related documents.** `production_findings.md` carries a register of discrete
findings from the same period, including the work that must be done before a
credential table is converted to a ledger.

## How the overload arose: a workaround for a foreign key

Researched from `27414788f7` on 2026-08-25, the state of `main` the experiment
branched from, deliberately not from its tip.

**The overload was never a decision about identity.** It was a workaround for a
foreign key, and the commit that made it says so plainly. `4c33846f6d`,
2025-03-25, "chore: add prebuilds system user (#16916)":

> Our data model requires that all workspaces have an owner (a `users`
> relation), and prebuilds is a feature that will spin up workspaces to be
> claimed later by actual users, and thus needs to own the workspaces in the
> interim.

`workspaces.owner_id` references `users(id)`. Prebuilds had to own workspaces.
Therefore prebuilds had to be a users row. **The identity table was chosen
because a foreign key pointed at it**, and nothing about prebuilds being a person
entered the reasoning. The change is labelled `chore:`.

**Three discriminators arrived one at a time, and none generalised the last.**
`is_system` on 2025-03-25 for prebuilds; `is_service_account` on 2026-03-11 for
service accounts, eleven and a half months later; and `kind` for AI agents, which
was added and withdrawn again during a later experiment and is present in neither
state of `main`. **So `main` has two**, and had two throughout.

**The second discriminator paid for the first's placeholder strategy.** Service
accounts must have an empty email, which required dropping and recreating two
unique indexes to exclude empty emails. The migration says why: "This is the less
invasive alternative to making email nullable, which would require a big
refactor."

**The placeholders are identical every time, and that repetition is the
evidence.** Each non-person row fills `email` with `''` or a fabricated address,
`hashed_password` with `'none'` or empty bytes, `login_type` with `'none'`,
`name` with `''`, `rbac_roles` with `'{}'`, and `status` with `'active'`, which
means nothing for a thing that cannot log in. **Three actors, three
discriminators, one placeholder set.** A row mostly composed of stand-ins is a
diagnosis on its own.

**The first migration shipped an unresolved question inside itself**, in a
comment above the statement that puts the prebuilds user in an organization:
`-- TODO: do we *want* to use the default org here? how do we handle
multi-org?` That is the shape of the whole thing: the workaround worked, the
question it raised was written down, and nothing carried the question forward.

### A history of the discriminators

Measured from `27414788f7` on 2026-08-25. Dates are author dates. "Predicate"
means a query or constraint that consults the field.

**`is_system`, a boolean.**

| Date       | Commit       | What                                                                     | Feature it served                                   |
|------------|--------------|--------------------------------------------------------------------------|-----------------------------------------------------|
| 2025-03-25 | `4c33846f6d` | **field added**, migration 000308, plus the prebuilds row inline         | prebuilds owning pre-created workspaces             |
| 2025-03-25 | `4c33846f6d` | predicates in `groupmembers.sql`, `organizationmembers.sql`, `users.sql` | same commit as the field                            |
| 2026-01-12 | `cc2efe9e1f` | predicate in `roles.sql`                                                 | organization-member as a per-org system custom role |
| 2026-03-04 | `cfcb81fb0f` | predicate in `insights.sql`                                              | a user status chart accommodating DST               |
| 2026-03-16 | `cabb611fd9` | predicate in `aiseats.sql`                                               | database CRUD for AI seat usage                     |
| 2026-03-26 | `3fb7c6264f` | predicate in `aiseatstate.sql`                                           | an AI add-on column in the users table UI           |
| 2026-05-05 | `1b2a1af097` | predicate in `user_secrets.sql`                                          | reporting secrets adoption in telemetry             |

**`is_service_account`, a boolean.**

| Date       | Commit       | What                                                                                         | Feature it served                                    |
|------------|--------------|----------------------------------------------------------------------------------------------|------------------------------------------------------|
| 2026-03-11 | `e5c19d0af4` | **field added**, migration 000433, with two CHECK constraints and two unique indexes rebuilt | service accounts, admin managed and unable to log in |
| 2026-03-11 | `e5c19d0af4` | predicate in `users.sql`                                                                     | same commit as the field                             |
| 2026-03-17 | `91ec0f1484` | predicate in `workspaces.sql`                                                                | a service accounts workspace sharing mode            |
| 2026-03-24 | `81188b9ac9` | predicates in `groupmembers.sql`, `organizationmembers.sql`                                  | filtering by service account                         |

**`kind`, an enum, and a second encoding of the same discriminant.** Added during
an experiment for a third variant, an AI agent, with one new value and eighteen
predicates across six files, and withdrawn again in the same experiment. It
exists at neither end of that work.

**While it existed the table carried two encodings of one discriminant at
once**, and that is the most telling thing in this history.

**Booleans are an open encoding.** Each new variant adds a flag. There is no
exhaustiveness anywhere, so no reader and no compiler can enumerate the variants;
and the flags are independent, so the encoding admits combinations the type has
no meaning for.

**An enum is a closed encoding.** One value per variant, mutually exclusive by
construction, and exhaustive, so a `switch` can be checked.

The enum was the better encoding and it is the one that went. `main` is left with
the open one.

**And the openness is not hypothetical.** Nothing prevents a row from being both
`is_system` and `is_service_account`; no constraint separates them. Worse, the
one constraint that touches both couples them by accident:
`users_email_not_empty` requires an empty email exactly when
`is_service_account` is true, so **a system user created without an email would be
forced to claim it is also a service account.** Nobody chose that. It is what
happens when two independent flags stand in for one discriminant.

**The general lesson, and it is the one worth carrying off this page.** Sum types
are hard to get right in the tools ordinary software is built with. A relational
table has no native union, so the discriminant is a convention; a language
without exhaustiveness checking on that convention cannot help either. **Nothing
here required a mistake. It required only that each variant be added by somebody
solving a different problem**, which is what happened three times.

### Three things the history shows

**Predicates accrete, and the accretion is the cost.** Three arrived with
`is_system`; five more arrived over the following fourteen months. The author who
added the field covered the surface as it stood, and every later author had to
rediscover the obligation.

**None of those five features was about non-persons.** A chart fixing a daylight
saving bug, AI seat CRUD, a UI column, secrets telemetry, a per-organization
role. **Nobody was working on identity when they had to learn that some rows in
`users` are not people.** That is the mechanism by which the debt spreads: it is
paid by people who did not incur it and were not looking for it.

**The overload made an unrelated cleanup impossible, and this is the sharpest
evidence in the set.** On 2026-07-28, `0e104f38e0` deprecated
`login_type = 'none'` and converted existing users to password login. It could
not convert all of them:

```sql
UPDATE users
SET login_type = 'password'
WHERE login_type = 'none'
  AND is_service_account = false
  AND is_system = false;
```

`login_type = 'none'` was the platform's original marker for "cannot log in", and
retiring it required carving out both discriminators, because a CHECK constraint
requires a service account to keep it. **A change with no connection to
non-person actors had to consult both flags to avoid corrupting them.** That is
what an overloaded table does to work that has nothing to do with it, and it is
the concrete answer to what the overload costs.

### The exclusions were not added reactively, and that matters

It is natural to assume the predicates accumulated because each was forgotten
until something broke. **Measured, that is not what happened.** Three of them, in
`groupmembers.sql`, `organizationmembers.sql` and `users.sql`, were added **in
the same commit as the flag itself**, `4c33846f6d`. Only `aiseats.sql` came
later, on 2026-03-16, and that query did not exist in 2025.

So the prebuilds author did generalise across the surface as it then stood. They
found the places a non-person would leak and fixed them together.

**That makes the diagnosis sharper rather than softer.** The defect is not that
people forgot. It is that **a discriminator is a fact every future query must
remember to consult, and nothing makes it remembered.** The surface grew; the
obligation did not travel with it. An author in 2026 writing a new aggregate over
`users` has no way to learn that some rows are not people, short of knowing the
history.

That is also why the fix is not more filters. A filter is another thing to
remember. **Removing the rows is what makes the obligation unnecessary**, and the
experiment that added a third variant demonstrated it by removing that variant's
rows and then deleting every predicate that had been written for it.

### The constraints are the discriminant's fingerprint

**What a discriminated union lacks in the schema turns up in the constraint
system instead**, and the two CHECK constraints on `users` are where. Both were
introduced with `is_service_account`, and both encode a fact about variants
rather than about columns: a non-person has no email, and a non-person cannot log
in with a password.

**Their history follows the discriminant's.** When a third variant arrived, both
were amended to name the enum as well, so each constraint had to know about every
encoding. When the enum was withdrawn, both were dropped as a side effect of
dropping the column they had been amended to mention, and had to be restored
without that clause. **A constraint that must be edited every time a variant is
added is the interface that was never declared**, written one predicate at a time
in the only place the schema allows.

Before the third variant and after it they read identically:

```text
users_email_not_empty            CHECK ((is_service_account = true) = (email = ''))
users_service_account_login_type CHECK ((is_service_account = false) OR (login_type = 'none'))
```

So the exemption a third variant needed has been removed again, and what is left
is what the service account change wrote in March.

## What it costs now: a holder read as a user in a hundred and eighty places

**The same defect exists one level up, on `api_keys`, and can be counted there.**
The column that names a key's holder was a plain `uuid`, so every read of it
produced a value that could mean the party holding the credential or the user the
request is authorized as. A distinct type with one deliberately ugly accessor,
`AsUserIDUnchecked()`, makes each such read state that it is assuming rather than
establishing.

**That type is an instrument, not a fix**, and it is what makes the following
number exist. The sites it marks are ordinary code that predates it.

`database.HolderID.AsUserIDUnchecked()` marks each site where a holder is read
as though it were a user without establishing that it is one. Measured
2026-08-25, excluding `_test.go` and the file defining the method: **184 at
`7a19c05df1`**, the commit that introduced the marking, and **180 at
`d44016d4e3`**.

**The delta is 4 and the work done is 5, and the difference is the finding.**
Four call sites were removed, all in `ValidateAPIKey` in
`coderd/httpmw/apikey.go`, all by commit `c84be7070d`. A fifth decision was made
in `7ca3f77b38`, where `aibridgedserver` stopped deciding by `user.Kind` and
began asking `key.AIAgentID()`; that removed no call, because the user path below
still fetches by holder id. **So counting these sites measures what remains and
is blind to what was done.** Anyone using the delta as a progress metric will
undercount. Commit `c84be7070d` says as much in its own message: "counting sites
measures the edits and not the questions, and the questions are what the
schedule will turn on".

### The diagnosis

**The fault is that two different facts shared one column, one type, and one
name.** `api_keys.user_id` was a `uuid`, and every read of it produced a value
that could mean either *the party holding this credential* or *the user this
request is authorized as*. For a human holder those are the same value, so no
read had to say which it meant, and none did. The overload was therefore
invisible at every individual site while being present at all of them.

**Introducing `database.HolderID` did not fix any site. It made the sites
countable.** A distinct type with one deliberately ugly accessor,
`AsUserIDUnchecked()`, forces each read to state that it is assuming rather than
establishing. That is why the count exists at all, and why it appeared at
`7a19c05df1` rather than being measurable before.

**A call is not a defect.** It is an unanswered question. Some answers are
"nothing here can be reached by a non-user, rename the variable"; some are a
branch; and at least one, per commit `c84be7070d`, expands into further
questions the entity model has not reached.

### Concentration: why 180 is not the size of a punch list

**Thirty nine other non-test files carry the pattern and not one of them changed
across the entire branch.** Verified by diffing per-file counts between
`7a19c05df1` and `d44016d4e3`: exactly one file's count moved. Every decision
made during the experiment was made in one function.

That is not because the other sites are easy. It is because **authentication is
the only place that was forced to answer.** It is where a credential becomes a
subject, so it cannot defer the question. Everywhere else the question is
deferred precisely because it can be: the code is correct for a human holder,
and no non-human holder reaches it yet.

Three consequences for anyone estimating from the count.

**The sites are of unequal kind, not merely unequal size.** Of the 180, some are
reachable by a non-user holder today and need a decision; some are reachable
only by a person and want a rename; and some need a design decision that does
not exist. Which of the three each site is has not been measured, and that
classification is worth more than the count.

**The count can grow when a site is touched.** Commit `c84be7070d` records that
one site turned into six more and a design question, and its own note draws the
conclusion: "counting sites measures the edits and not the questions, and the
questions are what the schedule will turn on".

**The completed work is invisible in the count**, per the delta of four against
five decisions above. So the number moves down slower than work happens, and up
when work is done properly.

## Three recorded counts of those sites do not reproduce

Recorded so that the next count is comparable rather than merely similar.

`rewrite_rbac.md` records 185; commit `c84be7070d` records 186 becoming 183. A
plain recount at the same code states gives 184 and 184 becoming 180. The
pattern is consistent rather than a typo, one to two above a text count, twice,
in prose by the same author. The plausible cause is a semantic find references
count, which dedupes multi argument call groups differently from a text match.

**What is worth keeping is the counting rule, not the number.** Non test `.go`
files, excluding `coderd/database/modelmethods.go` where the method is defined.
The figure itself is of less use than the rule, and a recount is only meaningful
against a stated rule.
