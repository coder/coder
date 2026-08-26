# Findings for Production Work

Verifiable facts about this codebase that bear on work after the proof of
concept, with what motivates fixing each. Nothing here is scheduled.

**Security findings live in `security_findings.md` and keep their P numbers.**
That document is cited by number from code comments, from a migration's column
comment, and from `work_breakdown.md`, so it was not renamed to absorb this
material. Findings here are numbered F to keep the two sets apart. Where a
finding is arguably both, it goes to whichever document a reader would look in
first, and says so.

**This is scoping information for work planning, and it is meant to be
disposable.** Eric, 2026-08-25. Each entry exists so that somebody sizing work
knows the item is there and what motivates it. **If an entry does not survive
that work, it has done its job**, and the document getting thinner is the
document succeeding.

The analysis of the `users` overload was extracted to
`overloading_users.md` on 2026-08-25, being a different kind of writing: it has a
thesis and gets revised, where these entries accrete and stay put.

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

## Preparatory work for converting ordinary tables to ledgers

**Marked as its own section because it is found by purpose, not by severity.**
Everything here is work to do on `main` **before** a table of credentials is
converted to a journal and a ledger. Doing it after is negative value: the
conversion pays to model something that should not exist, and then the model has
to be undone.

**This is a sequencing claim, not a priority claim.** Each item may score low on
both motivation axes taken alone. What makes them urgent is order: they are cheap
before the conversion and expensive after it.

### P-A. A credential is issued only where its authenticator will be presented

**The position.** A credential is a means of exercising authority. An
authenticator that nobody ever presents exercises nothing, so what such a thing
actually supplies is a **name**: an identifier to attribute by. Issuing a
credential in order to obtain a name is the defect, and it is invisible because
the result works.

**The instance that produced the position.** An AI agent's chat profile key is
minted at two sites and **its authenticator is discarded at both**:
`coderd/x/chatd/chatd.go:1385` throws away both return values of `MintKey`, and
`coderd/x/chatd/synthetickey.go:86` keeps only `newKey.ID`. The key id is then
used to attribute AI Gateway traffic. Nothing authenticates with it, ever. So it
is a name wearing a credential's shape, and `api_keys` carries a stored digest of
a secret that was handed to nobody.

**Why this must be settled before conversion, not after.** Journaling it would
create a credential in the ledger that authenticates nobody, with issuance,
rotation and expiry entries recording the lifecycle of something with no use. The
expiry machinery already built for it is the worked example: see
`credential_expiration.working_state.md`, where the chat key's twenty four hour
expiry turns out to bound inactivity rather than life, and the extension path
that maintains it can move last use backwards. **All of that effort is spent on a
credential that is never presented.**

**The check to run before converting any credential table.** For each kind of
credential the table holds, find where its authenticator is presented. Where the
answer is nowhere, it is an identifier and should be converted into one rather
than into a credential. The presented set is the set worth journaling.

**How to run that check here.** Every issuance site returns the authenticator.
Find the sites that discard it. `MintKey` and `RotateKey` in
`coderd/aiagentidentity` are the AI agent doors; the general one is
`entity.IssueCredential`, whose `IssuedCredential.Authenticator` is documented as
the only time the value can be had.

### P-B. Items recorded elsewhere that belong to this section

**The renames.** Of the sites reading a holder as a user, most are unreachable by
a non-user holder and want a checked accessor rather than a decision. Doing those
before the conversion removes the largest source of noise from it. See `overloading_users.md`.

**Purging non-person rows from `users`.** The order and the restricting foreign
keys are recorded in the users overload material, which also carries the trigger
that blocks a delete. That is advance work by the same argument: the conversion
should not have to carry rows that do not belong to it.

**The `is_system` filters.** The sibling of the `kind` filters this branch
removed, and the one that survives untouched. They are the same debt for a
different overloaded kind, and the same argument applies to enumerating them
first.

## Findings

**F1, F1a and F2 have moved** to `overloading_users.md`, which holds the `users`
overload as a single analysis: how it arose, its discriminator history, the count
of sites that read a holder as a user, and the counting rule behind that count.
The numbers are not reused.

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

### F9. Two live defects, found while classifying the holder sites

> **Standing, not scheduled.** Eric, 2026-08-25: not to be worked on now. Neither
> affects a live demonstration, since both are notifications that go unsent
> rather than anything a viewer would see. Worth cleaning up eventually and not
> urgent.
>
> **The decision to make before fixing them is a design choice, not a repair.**
> Either an agent initiated action notifies the owner and names the agent as
> initiator, which is what the code did while the row existed, or it skips the
> notification because an agent has nobody to notify. The second is what the
> credential validation path chose for last seen, and it named a destination for
> the equivalent fact.

**Created here, by the users row removal, and both verified in code.** Each is
the same shape: a site fetches the holder's users row, requires it to exist, and
now finds nothing for an AI agent. Before migration 000592 the row existed, the
fetch succeeded, and the notification was sent naming the agent. **Now the
notification is silently dropped and a warning is logged.**

**These sites were already known, and what changed is their behaviour.**
`rewrite_rbac.md`, under "Twenty one places ask whether the holder is a
particular user", triages two of them as degrading noisily: "doing a lookup that
fails and logging it, and in one case sending a notification that would otherwise
have been suppressed." Almost certainly the same two, described at an earlier
stage. **The suppression guard not firing was the old symptom; the lookup failing
is the new one**, and it arrived when the row did not. So this is not a discovery
so much as a known site crossing into a worse state, which is the more useful way
to read it: the triage predicted where the pressure would fall.

**`coderd/workspaces.go:1554`, `putWorkspaceDormant`.** The guard
`apiKey.HolderID.AsUserIDUnchecked() != newWorkspace.OwnerID` is always true for
an agent, since an agent id can never equal a user id, so the branch is entered.
`GetUserByID` on the agent's id then fails, and the notification is gated behind
`initiatorErr == nil`. **Reachable by a presented key**: the route needs an
update on the one workspace the workspace-agent profile names, and that profile
carries `workspace:update`.

**A second, smaller defect sits in the same block.** The warning logs
`slog.Error(err)`, the outer error, rather than `initiatorErr`. So the log line
that reports this failure carries the wrong error, probably nil.

**`coderd/workspacebuilds.go:730`, `notifyWorkspaceUpdated`,** fed by the holder
read at line 654. Same fetch, same failure, and here the function returns
outright, so the notification to every admin is dropped. Its reachability by a
presented key is not established, the branch being a template version change,
and that turns on the same per-function pass still outstanding.

**Neither is a wrong answer, which is why nothing caught them.** They are
notifications not sent, and no test asserts that one was.

### F9a. Baseline measurements taken at the same point

Recorded so a later count is a comparison rather than a fresh guess. All measured
on 2026-08-25 at `d44016d4e3`.

**The `is_system` predicates, which are the sibling of the removed `kind`
filters.** Twenty three occurrences across eight query files, `users.sql` (7),
`groupmembers.sql` (5), `roles.sql` (4), `user_secrets.sql` (2),
`organizationmembers.sql` (2), and one each in `insights.sql`, `aiseats.sql` and
`aiseatstate.sql`, plus eleven reads in Go outside tests and generated code.
**For comparison, the `kind` filters removed by this work numbered eighteen
across six files.** So the discriminator that remains is the larger of the two,
and nothing here has touched it.

**How much of `api_keys` the ledger knows about.** Structurally rather than by row
count, since a proportion would be specific to one deployment's data. Six
non-test sites mint an `api_keys` row and **one goes through the ledger**:
`coderd/entity/credential.go`. The other five are `apikey.go` for a person's
session key and tokens, `provisionerdserver` for the workspace owner's session
token, two in `oauth2provider`, and `chatd/synthetickey.go` for the per user
gateway key. **The ledger's coverage is exactly one kind of credential**, the AI
agent's, and every other kind in that table is unrecorded.

### F9b. The eight past repairs, and what became of the figure

**The repairs are listed in `overloading_users.md` under "Reference: repairs
already made".** They belong there, being evidence for that argument rather than
scoping information.

**The list is a reconstruction, not a recovery.** The figure of eight came from
tickets or git history, found once and not written down, and the original
membership is gone. Seven `fix:` commits are defensible; counting the feature
commits that carried an exclusion takes it past eight, and counting only
unambiguous repairs leaves it under. **Eight is in range and the exact membership
is not recoverable.**

**The loss is an instance of the thing being described.** `rewrite_rbac.md` says
of this debt that "each instance was repaired locally" and "nothing ever
aggregated into a thing with a name". A list of those repairs was compiled once,
used once, and not written down.

**The figure is not in the corpus.** The nearest written number is "Eight guard
chat ownership", in `rewrite_rbac.md`'s triage of twenty one comparison sites,
which counts guards rather than fixes. Separately, the count of holder decisions
made by this work is five: four call sites removed plus one made without
removing a call.

### F9c. The per-function pass, completed

**Result: 27 of the 34 workspace surface sites are reachable by a presented AI
agent key, and 7 are not.** Against the 180 total, that means **27 need a
decision and about 153 are renames.**

**The test.** The six workspace scopes are low level, so each grants exactly one
`(resource, action)` pair on resource type `workspace` and nothing else, and the
allow list pins the object to one workspace. `httpmw.ExtractWorkspaceParam` is
the gate on most of these routes: it fetches the workspace through `dbauthz`,
requiring `workspace:read` on that object.

**Unreachable, seven sites in six functions.**

| Function                                    | Sites | Why                                                              |
|---------------------------------------------|-------|------------------------------------------------------------------|
| `postWorkspacesByOrganization`              | 1     | needs `workspace:create`, absent from the profile                |
| `postUserWorkspaces`                        | 1     | same                                                             |
| `patchWorkspaceACL`                         | 1     | needs `ActionShare`, absent from every workspace scope           |
| `templateVersionDynamicParametersWebsocket` | 1     | a template object, absent from the allow list                    |
| `tasksCreate`                               | 1     | `ExtractOrganizationMembersParam`, an organization member object |
| `taskGet`                                   | 1     | same                                                             |
| `workspaceByOwnerAndName`                   | 1     | mounted under a user param                                       |

**Reachable, twenty seven sites in thirteen functions.** `postWorkspaceBuilds`
internals (5), `workspaces` (4), `putWorkspaceDormant` (4),
`logTunnelConnection` (4), `tasksList` (2), and one each in `workspace`,
`watchWorkspace`, `putFavoriteWorkspace`, `deleteFavoriteWorkspace`,
`patchCancelWorkspaceBuild`, `workspaceApplicationAuth`, `tailnetRPCConn` and
`connLogInitRequest`.

**Reachable does not mean consequential, and the difference matters for
sizing.** Several of these are inert once reached: the two favourite handlers
compare the holder against the workspace owner, and an agent identifier can never
equal a user identifier, so the comparison always refuses. The `me` resolution in
`workspaces` and `tasksList` resolves to an identifier that owns nothing, so the
filter returns an empty set rather than a wrong one. **The sites that do
something are the ones that write**, and of those the two notification paths are
already recorded as F9.

**One is a genuine defect rather than an inert site.**
`workspaceApplicationAuth` mints a new API key with `HolderType` left unset,
which defaults to a user holder, over an identifier that has no users row. It is
reachable through `workspace:application_connect`, which the profile carries.

**Confidence.** `putWorkspaceDormant` is verified rather than inferred: the
notification defect in F9 is an observed agent reaching that handler. The rest
are inferred from the route, its middleware, and the scope set, without executing
them. `logTunnelConnection` and `connLogInitRequest` are helpers rather than
handlers, so they inherit the reachability of the connection paths that call
them.

### F10. The holder decisions already made, and which sites can be reached

**Both halves depend on `database.HolderID`, which exists only on this branch**,
so neither is a statement about `main` that `main` could check for itself. The
first half is a set of answers somebody will need again; the second bounds how
much of the count is live work. See `overloading_users.md` for the count itself
and for why the marked type is an instrument rather than a fix.

**Eric, 2026-08-25: the four decisions may be worth redoing inside the proof of
concept**, which is the other reason they sit here rather than in the analysis.

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
with a named destination, not an oversight.

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
policy and the workspace designation boundary did not engage. That defect is recorded
separately as a security finding; deleting the branch is where it is answered
rather than moved.

**Site 4, baseline line 532: the subject build for a human holder.** The `else`
arm, passing the holder id to `UserRBACSubject`.

*The choice: keep the fetch, narrow its purpose, and say so.* Sites 3 and 4
collapse onto a single `key.UserID()` in the rewritten branch, which is why
commit `c84be7070d` reports three sites where four occurrences went. The user
path still calls `GetUserByID`, but **for existence alone**, with the reason
stated: a key naming a user who is not there should be answered as an invalid
credential rather than as a server error, which is what letting the roles fetch
fail would produce.

#### The classification, and why most of the 180 cannot be reached at all

Attempted 2026-08-25. **The reachability question collapses**, and the collapse
is verified in code rather than inferred from scopes.

**Chat profile tokens are never presented.** Both sites that mint one throw the
token away: `coderd/x/chatd/chatd.go:1385` discards both return values of
`MintKey`, and `coderd/x/chatd/synthetickey.go:86` keeps only `newKey.ID`. That
key exists to attribute AI Gateway traffic by key id, and nothing ever
authenticates with it.

**So the only AI agent keys that are ever presented come from
`WorkspaceAgentIdentityProfile`**, handed to the agent in workspace build
metadata, and from `SandboxIdentityProfile`, handed to a sandbox. The second is
the first: `profile.go:100` builds it by calling
`WorkspaceAgentIdentityProfile` and changing only the token name.

**That profile's allow list is one workspace**, `profile.go:83-85`, and its
scopes are the six workspace scopes and nothing else.

**An allow list denies every type it does not name.**
`object_is_included_in_scope_allow_list` in `coderd/rbac/policy.rego:366-388`
admits an object only through `{"*","*"}`, `{type,"*"}`, or a matching
`{type,id}`; otherwise the set of allowed ids for that object's type is empty
and the rule fails. Every `scope_allow` branch requires it.

**Therefore a presented AI agent key can authorize actions on one workspace
object and nothing else.** Any `AsUserIDUnchecked` site in a handler whose
authorization touches a non-workspace object is **unreachable by an AI agent
today**, which is C3: a rename rather than a decision.

**And the sites are all on the presented path.** They read
`httpmw.APIKey(r).HolderID` or an `apiKey` derived from it, so they are reached
only by a request carrying a key. In-process agent paths such as
`ChatToolSubject` build a subject without a key and never read a holder, so they
do not widen this.

**What that leaves.** The whole chat surface, 40 sites in `exp_chats.go` and
`exp_chats_acl.go`, is C3, along with the user, template, notification, external
auth, MCP, OAuth2 and audit surfaces. The candidates for C1, C2 and C4 are
confined to the workspace surface: **34 sites across 20 functions** in
`workspaces.go`, `workspacebuilds.go`, `workspaceagents.go`, `aitasks.go`,
`workspaceapps.go`, `workspaceapps/db.go` and `parameters.go`.

**The per-function pass was completed later the same day and is recorded as F9c.**
It resolves the upper bound of 34 to 27 reachable and 7 not.

**What that pass would establish, if it is ever wanted.** Whether each of those 20
functions authorizes only a workspace object has to be established one at a
time; the ones that plainly do not, such as `postWorkspacesByOrganization` and
`postUserWorkspaces`, need `workspace:create`, which the presented profile does
not carry. **That pass is done and is F9c: 27 of the 34 are reachable, so 27 of
the 180 need a decision and about 153 are renames.**

**A partial classification of those 34 exists and its premise is wrong in one
respect.** A subagent classified the workspace surface on 2026-08-25 and reached
4 needing a holder decision, 14 meaning the authorized user, 1 unreachable and
15 undecidable. **It was not told that chat profile tokens are never presented**,
and it argued reachability from both profiles, so any site it called reachable
only through the chat profile is in fact unreachable. Its counts need
re-deriving under the presented-key constraint before they are used.

**What survives that correction is its concrete work**, which is worth keeping:
the identification of `workspaceapps.go:111` as minting a new key with
`HolderType` left unset, so an agent reaching the subdomain app redirect would
mint a key claiming a user holder over an agent's id; the observation that
`workspacebuilds.go:451` is the one site already doing the right thing, resolving
on behalf of through a checked ledger lookup; and the note that
`workspacebuilds.go:816` reads unscoped `RBACRoles`, so resolving it naively to
the owner would bypass profile narrowing. None of those depends on the
reachability premise.
