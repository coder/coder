# Findings for Production Work

Verifiable facts about this codebase that bear on work after the proof of
concept, with what motivates fixing each. Nothing here is scheduled.

**Security findings live in `security_findings.md` and keep their P numbers.**
That document is cited by number from code comments, from a migration's column
comment, and from `work_breakdown.md`, so it was not renamed to absorb this
material. Findings here are numbered F to keep the two sets apart. Where a
finding is arguably both, it goes to whichever document a reader would look in
first, and says so.

**This document is provisional.** It was opened on 2026-08-25 to stop perishable
measurements from being lost, ahead of a corpus pass that will decide whether
two findings documents is right. See the corpus queue.

## How to read the motivation on each finding

Eric, 2026-08-25, on what production work needs to be told. Two axes, scored
independently, because they combine rather than substitute.

**Axis A: would this need dealing with regardless of anything the proof of
concept did?** A defect on its own merits, arguable without reference to this
work.

**Axis B: is this something that must change for the proof of concept's goals to
be reachable?** Not necessarily a problem in itself.

Four combinations, and they are not the same case.

| A            | B    | What follows                                                                                                                                                                                         |
|--------------|------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| high         | low  | Schedule on its own merits. The proof of concept is irrelevant to the argument.                                                                                                                      |
| low          | high | The case for it *is* the case for the proof of concept, and should be argued that way rather than dressed as a repair.                                                                               |
| real but low | high | **The dangerous cell.** Ignorable in isolation, load bearing with this work. Invisible on a defect list and invisible in the case for the work, because its severity exists only in the combination. |
| high         | high | Strongest position. The two motivations reinforce, the same fix serving both.                                                                                                                        |

**A third disposition, which is a proposal rather than Eric's words.** Some
findings are debt this work *created*. They score nothing on either axis: axis B
is work needed to make progress, and these are the cost of progress already
made. They are not prioritised against other work but are conditions on
shipping, payable if the direction is adopted and evaporating if it is not.
Marked **created here**.

## Findings

### F1. An API key's holder is read as a user in a hundred and eighty places

**A high, B high.** Eric places the conflation squarely on axis A. It is also
what blocks a non-user actor from holding a credential, so the proof of concept
cannot proceed around it. The same fix serves both, which is the strongest
position any finding here holds.

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

#### The diagnosis

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

#### The four decisions, and what each one chose

Recorded in full because `main` will have to make them again and this branch
never merges. Baseline citations are `coderd/httpmw/apikey.go` at
`7a19c05df1`, all four inside `ValidateAPIKey`. Head citations are by marker
rather than line, since lines move.

**Site 1, baseline line 280: the provider token refresh.** It fetched the
holder's OIDC or GitHub login link, in order to refresh a provider token, inside
a block already gated on `key.LoginType` being `LoginTypeGithub` or
`LoginTypeOIDC`.

*The choice: make an existing truth explicit rather than repair a live fault.*
An AI agent's key is minted with `LoginTypeToken`, so the block could never run
for one. The site was not reachable and was not broken. The replacement adds
`userID, holderIsUser := key.UserID()` and conjoins `holderIsUser` to the
existing condition, so the code now states the assumption that the login type
was silently carrying. **A maintainer should know this one is a clarification,
not a bug fix**, and should not expect a behaviour change from it.

**Site 2, baseline line 461: the last seen update.** It wrote `last_seen_at` on
the holder's users row after a successful validation.

*The choice: skip, rather than write elsewhere or fail.* The replacement wraps
the update in `if userID, isUser := key.UserID(); isUser`. The reasoning is
recorded at the site: last seen belongs to a user, an AI agent has no users row
to carry one, and **when an agent wants the equivalent that is the credential use
model's business rather than this column's**. So this is a deliberate omission
with a named destination, not an oversight. Note that the destination does not
work yet: see F4.

**Site 3, baseline line 480: the kind check.** It fetched the users row with
`GetUserByID` specifically so that `user.Kind` could be read, to tell an AI agent
from a human. Everything downstream branched on that.

*The choice: ask the credential, not the row.* The replacement leads with
`if agentID, ok := key.AIAgentID(); ok`, so **what holds a credential became a
property of the credential** rather than something discovered by fetching a row
and inspecting it. This is the substantive decision of the four and the one that
made the users row removable at all.

*It also closed a security finding rather than moving it.* The baseline's agent
branch built the subject by assigning `actor.Type` and `actor.FriendlyName` onto
the value `UserRBACSubject` returned. That value carries a cached rego AST and
`regoValue()` short-circuits on the cache, so neither assignment reached the
policy and the workspace designation boundary did not engage. That is **P9** in
`security_findings.md`. Deleting the branch is where P9 is answered.

**Site 4, baseline line 532: the subject build for a human holder.** The `else`
arm, passing the holder id to `UserRBACSubject`.

*The choice: keep the fetch, narrow its purpose, and say so.* Sites 3 and 4
collapse onto a single `key.UserID()` in the rewritten branch, which is why
commit `c84be7070d` reports three sites where four occurrences went. The user
path still calls `GetUserByID`, but **for existence alone**, with the reason
stated: a key naming a user who is not there should be answered as an invalid
credential rather than as a server error, which is what letting the roles fetch
fail would produce.

#### Concentration: why 180 is not the size of a punch list

**Thirty nine other non-test files carry the pattern and not one of them changed
across the entire branch.** Verified by diffing per-file counts between
`7a19c05df1` and `d44016d4e3`: exactly one file's count moved. Every decision
made on this branch was made in one function.

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

### F2. Three recorded counts of F1's sites do not reproduce

**A low, B low.** Measurement hygiene rather than a defect, recorded so the next
count is comparable.

`rewrite_rbac.md` records 185; commit `c84be7070d` records 186 becoming 183. A
plain recount at the same code states gives 184 and 184 becoming 180. The
pattern is consistent rather than a typo, one to two above a text count, twice,
in prose by the same author. The plausible cause is a semantic find references
count, which dedupes multi argument call groups differently from a text match.

**What is worth keeping is the counting rule, not the number.** Non test `.go`
files, excluding `coderd/database/modelmethods.go` where the method is defined.
Whether to amend the corpus figure is Eric's, and is in the corpus queue.

### F3. The credential ledger is not on the authentication path

**A low, B high.** There is no independent case for changing this. Absent the
proof of concept, `api_keys` authenticating a request is not a defect, it is how
authentication works. The case is the proof of concept's case and should be
argued as one.

`coderd/httpmw/apikey.go` reads `api_keys` at line 611 and validates against
`api_keys.hashed_secret` at line 634. **`entity.VerifyCredential` is complete and
has no production caller.** Non test callers of the mirror's writes, and how many
reach the ledger: `InsertAPIKey` 6 of which 1, `DeleteAPIKeyByID` 11 of which 3,
`UpdateAPIKeyByID` 4 of which 0.

**"One step" describes shape, not cost.** One call site decides it, and moving
verification is the step with the most exposure in the design: on every request,
no read replicas, and a naive move puts a write on the read path. Anyone reading
"one step" as "nearly done" is claiming more than the evidence carries.

### F4. Last use is written by a read-modify-write that can move it backwards

**A real but low, B high. This is the dangerous cell.** On its own a timestamp
occasionally moves backwards and nobody notices. With this work, last use is one
of the two facts the credential use model exists to record, and the path that
corrupts it is the path that would have to become authoritative.

`UpdateAPIKeyByID` writes `last_used`, `expires_at` and `ip_address` together,
and callers feed back values they read earlier. `coderd/x/chatd/synthetickey.go`
does this on both its extension paths; `coderd/httpmw/apikey.go:447` writes last
use on the request path. A concurrent update between read and write is silently
lost.

**Meanwhile the credential use journal is never written in production.**
`PostCredentialPresentation` is reached only from `recordPresentation`, only from
`VerifyCredential`, which nothing calls outside tests. Both its events exist and
the table is empty by construction.

### F5. Six test failures, all of which arose on this branch

**Created here, and not what I first recorded.** I classified these as
pre-existing defects on axis A. They are not pre-existing: **all six pass at the
merge-base** `27414788f7`, verified by extracting that tree with `git archive`
and running them, 73 subtests all passing at real durations including the exact
subtests that fail at head.

Attribution by bisect, in a local clone so the working tree was untouched.

| Test                         | First bad commit         | Author    | Cause                       |
|------------------------------|--------------------------|-----------|-----------------------------|
| `TestDeleteUser`             | `61ef96e994`             | Jon Ayers | RBAC designation attributes |
| `TestTemplateVersion`        | `61ef96e994`             | Jon Ayers | same                        |
| `TestDebugCollectProfile`    | `61ef96e994`             | Jon Ayers | same                        |
| `TestWorkspace`              | `61ef96e994`             | Jon Ayers | same                        |
| `TestWorkspaceAgent_Startup` | `8494ca1beb`             | ours      | agent API version bump      |
| `TestPostChats`              | see F6, not `c13b5e1e38` | ours      | WP12 milestones 2 and 3     |

**Four come from one commit of Jon's**, `61ef96e994` "feat(coderd/rbac): add AI
agent designation attributes to Object and Subject", which touches only
`coderd/rbac/authz.go` and `coderd/rbac/object.go`. Verified directly: all four
pass at its parent and fail at it. They fail at
`coderd/coderdtest/authorize.go:255` with "assertion missing", the recorder
expecting an authorization call it did not see. The mechanism is consistent with
`rbac.Object` gaining fields the harness's expected objects do not carry, but I
did not confirm that.

**One was a pinned version in a test helper, and is fixed.**
`TestWorkspaceAgent_Startup/OK` asserts that the recorded API version equals
`proto.CurrentVersion`, which `8494ca1beb` moved to 2.11, while the helper
`postStartup` called `ConnectRPC210`. **Production was never affected**: the
agent calls `ConnectRPC211WithRole` and `agentsdk` requests 2.11 correctly.
Pointing the helper at `ConnectRPC211` fixed it. A pinned version in a helper
silently stops testing the version the agent actually connects with, which is
why the assertion is written against `CurrentVersion` in the first place.

### F6. One test broke twice, and both breaks are WP12 asserting itself

**Created here. No production defect, and the `UpdateUserStatus` call is the
test's own.** Chased on 2026-08-25 at Eric's instruction.

`TestPostChats/AIAgentToolSubject` in `coderd/exp_chats_test.go` tests a control
it states plainly: "In-process platform callbacks must re-check agent and owner
liveness because chat workers are detached from the HTTP gate." Its mechanism
was to suspend the AI agent's **users row** and require that `ChatToolSubject`
then refuses.

**Break one, at `7ca3f77b38`, WP12 milestone 2.** Line 480, `require.Error`,
gets nil: suspending the users row no longer causes a refusal, because milestone
2 stopped reading that row's status and reads the ledger's state instead. **This
is the window recorded under WP12 milestone 2, observed rather than reasoned
about.** The control did not disappear; it moved to a fact the test does not
touch.

**Break two, at `d44016d4e3`, milestone 3.** Line 478, `require.NoError` on
`db.UpdateUserStatus(agent.ID, Suspended)`, gets `sql: no rows in result set`:
there is no row left to suspend. So the setup fails before the assertion that
was already failing.

**The fix is a test change and the intent survives intact**: retire the agent
through `entity.RetireAIAgent` and assert the refusal. That is the same
conversion three other tests got during milestone 3. This one escaped because it
was already red, so it produced neither a compile error nor a new failure.

**A correction to the attribution above.** My bisect named `c13b5e1e38` as the
first bad commit for this test. **That attribution is wrong.** At that commit the
test fails inside `dbtestutil` with `create database`, an environment failure,
which satisfied the bisect's exit-status predicate without being the regression
under study. **Bisecting on exit status over a range that is not otherwise green
attributes confidently and wrongly**, and the attributions to `61ef96e994` and
`8494ca1beb` are trustworthy only because I confirmed those by testing the
parent commit directly.

### F6a. The method finding, which is the part that transfers

I classified every failure in F5 and F6 as "not ours" by stashing local changes
and confirming the tests already failed. **That predicate is too coarse.**
Pass-or-fail cannot distinguish a pre-existing failure from a pre-existing
failure plus a new one, and here it hid exactly that twice over: a control
regression at milestone 2 and a setup break at milestone 3, in one test that read
as "already broken" throughout.

**Where a baseline is already red, compare the failure message, not the exit
status.** The same applies to `git bisect run`, whose predicate is exit status.

Every "verified pre-existing" claim made that way is worth less than it sounds,
including the ones made earlier the same day.

### F7. Other debt this work created

**Created here.** Conditions on shipping rather than items to prioritise.

**`TestAISandboxLifecycleCreateUnboundParent` fails deterministically**, arising
on this branch, with `Egress policy is delivered to the supervising agent, never
to the confined agent`. It does not exist at the merge-base, so it is this
branch's throughout. Eric, 2026-08-25: it needs fixing, and mainly not by him.

**No migration fixtures exist for any credential or authorization table**, so
`TestMigrateUpWithFixtures` fails. The gap dates to WP2 and WP4. It listed eight
such tables before migration 000591 and nine after.

**The recorded proof of concept cheats** are enumerated per package under PoC
cheats in `work_breakdown.md` and are not repeated here.

### F8. Storybook tests fail, and it is nobody here's doing

**A low, and not ours.** `make pre-push` fails on `test-storybook`: three test
files of 444, an xterm "get dimensions" error and a teardown timeout, in
`IconField`, `CoderAgentsPageView`, `AgentChatPageView`, `Tool` and `DebugPanel`
stories. The marker push touched zero files under `site/`. Recorded only so that
a failing pre-push is not mistaken for a consequence of this work.
