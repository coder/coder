# Rewriting RBAC

Started 2026-08-21. This document holds what is known about rewriting the
authorization and credential machinery so that an entity other than a user can
hold a credential and act.

It is deliberately thin. Unlike the audit approach, the corpus here is not being
settled before the work begins: the rewrite happens once, at proof of concept
scale, to find out whether it can be done at all. A production rewrite would
want this document well ahead of the code. This one runs alongside it, recording
the specific before any attempt is made to generalise.

A section near the end collects **benefits and costs of the rewrite** as they
are noticed, for later use in deciding whether to do this outside a proof of
concept.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built on
top of them, offered for challenge rather than settled. **Findings** records
verifiable facts about the existing codebase. **Open** records questions not yet
answered.

The shape of the actor type column is settled in `implementation_patterns.md`
under "An actor type column on a core table is text with a CHECK".

## Established

### There is no path to auditable credentials without this rewrite

Credentials cannot be made auditable while a credential is a row keyed on a user
and destroyed rather than retired. Both are properties of the existing schema
rather than of the code over it, so neither can be fixed from outside.

Credentials in general are not in proof of concept scope. The thesis is about
the path rather than the demonstration: **as the system stands there is no solid
way forward to auditable credentials that does not address this**, so building
further on the present arrangement accumulates work that a later correction
would undo.

### A cascade delete cannot be journaled

A row removed by a foreign key cascade is removed by the database. No
application code runs, so nothing writes an entry, and the credential ends with
no record that it ended.

**A trigger does not rescue this.** It could be made to write a row shaped like
an entry, but not an entry, because the actor is not available to it. Who acted
is an application fact with no representation inside the database, and it is cut
off at the point the cascade fires. An entry whose actor cannot be resolved
breaks the journal's link to the party responsible, which is the one thing an
entry may not do. `journal_vs_log.md` declines triggers for the user history
tables on related grounds.

The consequence is that a cascade has to be **removed** rather than worked
around. An ending that only the storage engine knows about cannot be brought
into the account afterwards.

### Authorization is an institutional fact, and RBAC is capability to act

They look similar because both concern what an agent may do, and they are not
the same thing. An institutional fact holds because it was constituted and
recorded. A capability is what the machinery will permit at the moment of
asking.

**The grant gates whether RBAC is relevant at all.** An unauthorized agent
should have no RBAC subject built for it. The question the grant answers comes
before the question RBAC answers, so an absent grant is not a denial by RBAC but
a reason never to reach it.

In a more complete system the grant would bound the subject: what an RBAC
subject built for an AI agent is capable of would be limited by the authority
its grant conferred. This is why a universal grant suffices for the proof of
concept without sufficing in general. Universal means the gate is open, and
nothing yet narrows what passes through it.

### The rewrite is a proof of concept, and goes first because it is riskiest

What is being tested is whether the rewrite is possible at all. A negative
result is a real finding: that full auditability is not practically achievable
in this architecture. It is only worth having if it arrives before work is built
on the assumption that auditability is achievable.

This follows Humphrey in *A Discipline for Software Engineering*. Tasks that
gate completion are ordered by risk of failure, riskiest first, so that low risk
effort is not spent on work a later failure would waste. Journaling credentials
was wanted independently, and its value is blunted if the core change proves
impossible.

### Making an assumption visible to the compiler is a measurement

Where a wrong assumption is spread through code and fails silently, a change
that forces every site holding it to say so is worth more than the change
itself. **Its product is a count.** Until it is made, nobody can say how large
the assumption is, only that it is somewhere.

The instance here is giving an API key's holder a type distinct from the type it
had. The holder was a `uuid.UUID` read throughout as "the user making this
request", and nothing distinguished a site that meant the holder from a site
that meant the user. A defined type refuses to be read as either without saying
which, which turned an assumption nobody had counted into a list of a hundred
and eighty four places, each one a decision that is now owed rather than
unnoticed.

This does not settle any of those decisions and is not meant to. It converts a
question of unknown size into a question of known size, which is what makes an
estimate possible at all.

### An API key's holder is a pair, and reading it as a user is a marked cheat

The holder of an API key is a `(type, id)` pair. The identifier column is a type
of its own rather than a bare UUID, so no site can read it as a user without
saying that is what it is doing. The narrowing is called `AsUserIDUnchecked`,
and the name is the point.

**Every call to it is a decision that has not been made.** There are a hundred
and eighty five outside tests. They preserve the behaviour a human holder
already had, which is correct for a human and wrong for anything else, and they
are left that way deliberately for the proof of concept rather than repaired.
Grepping the name gives the outstanding work; removing a call means deciding
what that site meant.

**The cheat is safe because the failure is silent and the guards are honest.**
A site handing a non-user holder to something expecting a user gets an empty
record rather than an error, and a site asking whether the holder is a
particular user gets a correct no. Neither produces an unsafe answer. What they
produce is a wrong one, which "These sites are found by someone noticing a wrong
number" describes.

**Two things are not cheatable and were not cheated.** Authentication resolves
the holder before anything else runs, and refuses the request outright if it
cannot; and the trigger refusing keys to deleted users had to learn that a
holder which is not a user cannot be a deleted one.

### Scope for the proof of concept is user and ai_agent

System actors already recorded as users stay there. Going from one identity
table to two is where nearly all the work is; going from two to three afterwards
is small.

## Derived

### A ledger of rows it does not own is a description, not a record

If a ledger records a credential while some other row is what actually
authenticates, the two can diverge, and the other row wins, because it is what
the system consults. That inverts the authority a journal is supposed to have:
`journal_vs_log.md` holds that a journal is authoritative and that a competing
account of the same events is the one in question. Here the competing account
would be reality and the ledger a shadow of it.

So a credential ledger over `api_keys` rows it does not own describes those
credentials rather than recording them, and a description may be wrong without
anything being wrong with the thing described.

**This wording wants revisiting.** The reasoning is not thought to be wrong, but
it is stated confusingly enough that it was moved out of Established rather than
repaired in place.

### The change has three rings

The work is not one change but three, of increasing difficulty, and only the
first is available with the entities already built.

The innermost ring is the identity code itself: creation, both revocation paths,
and wiring the lapse transitions so that retiring an agent ends its
authorizations and invalidates its credentials. It needs the AI agent,
authorization and credential entities and nothing else.

The second ring is schema. Five columns hold an AI agent identifier and three
constraints reference the table it is keyed on, two of them on core tables
rather than AI specific ones. Mechanical, but not confined to the identity code.

The third ring is the credential and the subject built from it. This is where
the users table is load bearing, because a subject is built from a user's roles
and a credential is a row keyed on a user. It is the largest of the three and
the one whose failure would be the finding.

### An actor table would be a union pretending to be a table

The obvious remedy for a single kind reference is to give the actors a table of
their own and point at that. It does not work. Such a table needs a column
saying which kind of actor each row is, which reproduces exactly the situation
the users table is already in: a union of kinds presented as one relation,
discriminated by a column.

The pair is not a workaround for the absence of that table. It is the honest
representation of a union, which is what an actor is. Materialising the union
would be the same mistake under a better name.

### What the pair costs

Two costs, neither a reason to reconsider.

**Display side queries gain a branch.** Anything wanting a holder's name rather
than its identifier has to switch on the type. Most callers want only the
identifier and are unaffected. The remedy is a function that generates a name
from the pair rather than a stored name, which also avoids a denormalized value
that can go stale.

**The type set has no closure the schema enforces**, unless a constraint is
added deliberately. A value naming no table yields a credential whose holder can
never be resolved, so the reconciliation that catches it matters more on a core
table than it does on a journal of ours.

### The waypoint that was rejected

There was a smaller path: keep a users row per agent purely as an anchor for its
credential, and make our ledger the authority for validity and authorization.
That would unblock the innermost ring immediately, add no new actor kind, and
leave the credential storage untouched.

It was rejected on Humphrey's grounds rather than on technical ones. It defers
the riskiest ring while spending effort that the failure of that ring would
waste, and it would ship a record that is honest about agents and authorizations
while remaining a shadow about credentials.

### These sites are found by someone noticing a wrong number

A site that treats a holder as a user does not announce itself. It does not
crash and it does not return an error: it returns an answer computed from a
record that is absent, which is usually an empty one. So the failure is silent,
and saying only that understates it.

**The observed mechanism of discovery is a person noticing that a figure is
wrong.** A percentile counting subjects that could never have contributed to it,
a list of people containing something that is not one, a seat consumed by a
machine. Each was corrected once seen, and each correction was local to where it
was seen.

The consequence for estimating is that any figure arrived at by finding the
sites through testing will be too low, because the behaviour under test is
plausible. This is the case that "Making an assumption visible to the compiler
is a measurement" generalises from.

### The overloading is debt, and it has never been named as such

Nothing found in this trace was a bad decision. Excluding an internal subject
from a percentile is right. Keeping a machine out of a list of people is right.
Refusing it a paid seat is right. Each repair was correct where it was made.

**What is missing is the principle that would have predicted them.** Every one
was derived from an incident rather than from a rule, so there has never been a
way to ask where else the same thing holds. The comment saying the handling
covers one system user because only one exists is that absence stated plainly by
someone who could see it locally and had nothing to generalise from.

Two properties keep the debt from accumulating visibly.

**It does not fail**, so it produces no incidents in the ordinary sense, only
answers that are wrong in ways somebody eventually queries. A cost that never
announces itself is not weighed against anything.

**Each instance was repaired locally**, a filter in one query, a sweep in
another, a bespoke expiry somewhere else. Nothing ever aggregated into a thing
with a name, and an unnamed thing cannot be scheduled, estimated, or argued
about.

That is what distinguishes this from ordinary shortcuts taken knowingly. Debt
usually gets recorded and deferred. This was never recorded, because at each
point there was nothing visible to record beyond the one query in front of
someone.

### The subject is a smaller problem than the credential

Tracing the prebuilds user moves weight between two of the three rings.

**The subject ring shrinks.** A compiled in subject for a non-person already
exists and works, so building one for an AI agent is a pattern the codebase
has rather than one it lacks.

**The credential ring grows.** The transfer case has already happened to a
system user, has already produced two layers of workaround, and the second layer
leans on parsing an identifier out of a name because the schema has nowhere to
record what a credential was issued for. That gap is closed by construction in a
credential ledger, and the argument for closing it does not depend on
auditability at all.

## Findings

### RBAC decides by roles, scope and policy

Orientation, because the rest of this section assumes it. Every database call
passes through an authorization layer that asks whether a subject may perform an
action on an object. A subject carries an identifier, roles, groups and a scope.
Roles grant permissions. **A scope only ever narrows them**, being a ceiling
rather than a grant. The decision itself is evaluated by a policy written in
Rego.

Two words in this area carry more than one sense, which makes the code hard to
read until they are separated. **A user is an account; `member` is a role that
an account holds**, one of five built in site roles, and the baseline nearly
everyone has. Both words are also names of permission *scopes* inside a role
definition: "user" for objects the subject owns across the deployment, "member"
for objects it owns within one organization. A third sense of "user" is the
resource type, meaning user records as something to be acted upon.

The role called `member` grants almost nothing across the deployment. Nearly all
of its permissions apply only to objects the subject owns.

### An AI agent never authorizes as itself

The RBAC subject is built from the owning human:
`UserRBACSubject(ctx, db, identity.OwnerUser.ID, key.ScopeSet())`. `Subject.ID`
is that human and carries that human's roles. The agent travels separately in
`Subject.AIAgentID`, set by `Subject.AsAIAgent`, which keeps "the sponsoring
human's ID, roles, and scope: only the acting identity and type change".

**Reach is built by subtraction, in three layers.** The agent inherits the
owner's roles. The API key's profile narrows that to given scopes and resource
ids, a ceiling that can only subtract. The policy narrows again by designation,
requiring that a workspace record the acting identity before most workspace
actions are permitted.

There is no role held by an agent, no per-agent role assignment, and no relation
in the roles machinery between an agent and the human it acts for. "Sponsor" is
narration rather than a term of art: it names the human in
`ai_agents.owner_user_id`, and RBAC has no sponsorship relation for a grant to
attach to or replace.

### Capability and attribution travel on separate channels

Two things flow with a request and they are not the same. The RBAC subject
carries **capability**: whose roles apply, narrowed by scope and by policy. A
separate value in the request context carries **attribution**: which AI agent is
acting and which human it acts for.

The second channel touches no authorization decision. Five places read it, for
designating a workspace at creation, for chats, for attributing a sandbox
session, for building a subject for chat tools, and for the existing activity
trace. **It is the closer analogue of the institutional fact**, existing to
record who acted rather than to decide what may be done.

That the two already exist separately is convenient. A gate on the grant belongs
where the capability channel is constructed, and the attribution channel can
keep flowing whether or not a subject is built.

### Three places build a subject for an AI agent, and they differ

The workspace agent path and the chat tool path both set the acting identity on
the subject. The path that authenticates an agent's own API key sets the subject
type and the display name but not the acting identity, storing the agent
elsewhere for the attribution channel instead.

That difference has a consequence, recorded under Open because it is a question
for the author rather than a defect to assert. It also means **a gate on the
grant has three sites to cover, not one**, and that they are unalike enough that
a single helper may not serve all three.

### The policy compares identities as opaque strings

The designation rule is string equality between the acting identity on the
subject and the designation on the object. It has no opinion about which
identity space either came from. **Changing what an AI agent's identifier is
therefore requires no policy change**, provided both sides change together.

Two fail closed touches are worth preserving through any rewrite. The policy
enumerates the exempt actions rather than the protected ones, so a workspace
action added later is protected by default. And either marker, the subject type
or a non-empty acting identity, classifies a subject as an agent, so a half
populated subject fails closed rather than passing as human.

### An AI agent has no identifier of its own

`ai_agents` is keyed on `user_id`, which references `users(id)`. The users row
identifier is the agent identifier everywhere it appears. Three foreign keys
reference it, two of them on core tables rather than on AI specific ones, and
two further columns hold the same value without foreign keys so that the record
survives identity cleanup.

The code that consumes those columns never reads `users`. It receives an opaque
identifier and stores it. **The dependency is structural rather than semantic**,
which is what makes repointing it mechanical.

#### What holds that identifier

Three foreign keys reference the AI agent table, one on a sandbox row and two on
core tables, being the workspace and the workspace agent. Two further columns on
the sandbox session and its network events hold the same value without a foreign
key, deliberately, so that the record outlives identity cleanup. One column on
the existing audit trail holds it as well.

Every one of those is a uuid compared by equality. **None of the code consuming
them reads the users table**, which is what makes repointing them mechanical
rather than semantic.

### Credentials are destroyed rather than retired

Both revocation paths delete the API key row. Nothing records that the
credential ended, because the row that would carry the record is the row being
removed.

A second and latent source of the same problem is the foreign key from
`api_keys` to `users`, which cascades on delete. User deletion appears to be
soft, so the cascade rarely fires, but a lifecycle whose terminal event is
performed by the storage engine cannot be journaled at all.

### What depends on api_keys.user_id being a user

Less than expected, which is a finding in its own right. **No query joins the
API key table to the users table.** The endpoint that lists tokens with their
owner's name already tolerates a missing user and falls through to an empty
name. The RBAC object for a key names its holder as owner, but that already
fails to match an acting subject, because the subject is the human while the key
names the agent, so user scoped permissions on an agent's own key never applied.

**One cascade is deliberately relied upon and is not affected.** Two places in
the OAuth2 provider delete an API key in order to cascade to a token. That
cascade is keyed on the key's own identifier rather than on its holder, so
changing the holder column does not disturb it. This was the coupling most
likely to be fatal and it is not one.

The remaining cost is breadth rather than depth: roughly two hundred sites pair
an API key with a user identifier across the server, enterprise, CLI and SDK
packages, excluding tests and mocks. One column renamed, one added, and a long
tail of call sites.

### One kind of credential is used in three unrelated ways

Every AI agent key is the same kind of thing, an API key row with a token login
and a lifetime, differing only in scopes, allow list and token name. How they
reach a holder differs completely.

One is delivered into a workspace by the build. One is returned over HTTP to a
workspace agent. **The third is never delivered at all**: the token is
discarded and only the row's identifier is kept, passed to the AI gateway so
that traffic is attributed per chat rather than per user.

That third one is not a credential in the sense this corpus uses. Nothing
presents it and no verifier checks it. It is a label, and whether the credential
entity should cover it is a decision rather than an oversight.

The token name is the handle by which a credential is found and revoked, and
rotation is delete then mint because minting does not replace by name. The chat
key is instead **extended in place near expiry**, because an in flight request
may already have handed its identifier to the gateway: an identifier that has
escaped cannot be rotated safely.

#### What the gateway sees

The traffic record written by the AI gateway names its initiator with a user
identifier. For a chat agent that identifier is **the agent's own**, because the
key delegated to the gateway belongs to the agent rather than to the human. So
attribution to the agent is present in that record.

What is absent is any marker saying the initiator is an agent at all.
Distinguishing one from a human requires joining the users table on its kind
discriminator, or joining the AI agent table. An identifier alone does not say
what kind of thing it names, which is the same lack the pair exists to remedy.

### api_keys cannot express a credential that does not expire

`api_keys.expires_at` is `NOT NULL`, so every row asserts an end. That was
sound while every key came from a login: the key expired when the session did,
and no key existed for which "never" was the answer. A credential held by an AI
agent has no session to outlive, and the intent is that it ends by revocation
when its holder ends.

The column has no way to say so. A mirror of a credential the ledger gives no
expiry has to write a date, and the date it writes is a stand-in chosen to be
recognisable rather than a fact anything recorded. The ledger's own column is
nullable, and an absent expiry there stands where a row would have been absent
had expirations been kept in a table of their own.

This is small and it is the shape of the larger problem. A column that cannot
represent the absent case forces every writer to invent a value, and a reader
cannot afterwards tell an invented value from a stated one.

### api_keys admits two kinds of holder where the ledger admits any

The holder pair on `api_keys` carries a `CHECK` constraint naming `user` and
`ai_agent`, which is the proof of concept scope. The credential ledger names no
kinds at all: a credential authenticates whatever holds it, and the entity types
are the journal's business rather than the ledger's.

So the two disagree about what may hold a credential, and the narrower one is
the one authentication reads. A credential for a `workspace_agent` can be
recorded and cannot be mirrored, which means it can be recorded and cannot be
used. Issuance refuses it rather than letting the constraint report it, because
a caller that never named `api_keys` should not be told about its constraints.

The disagreement is a consequence of the mirror rather than of either table.
It disappears when the ledger is what authentication reads.

### Twenty one places ask whether the holder is a particular user

Comparisons rather than lookups, and they behave differently from the rest
because a comparison cannot return an empty record. Triaged:

- **Ten refuse an actor that may have a legitimate claim.** Eight guard chat
  ownership and two guard workspace ownership. All fail closed, so the outcome
  is safe and the agent is simply refused.
- **One refuses something no agent has business doing**, reading a login type.
- **Four guard against something impossible for a non-user**: suspending
  yourself, changing your own roles, changing your own chat sharing role, and
  supplying an old password when changing your own. The guard does not fire, and
  not firing is correct, because the holder genuinely is not that user.
- **Four decorate output**, marking which row is the caller. The mark is absent.
- **Two degrade noisily**, doing a lookup that fails and logging it, and in one
  case sending a notification that would otherwise have been suppressed.

**None of them fails open.** Each asks whether the holder is one specific user,
and for any other kind of actor the honest answer is no, so every guard is
answered correctly by the question it actually asks.

The consequence is that the first group is not a work item but **a list to check
a demonstration against**. If a scenario has an agent send a chat message or
favourite a workspace, those gates fire; if it stays on credential and
authorization lifecycle, none is reached.

### An existing security control describes the grant, negatively

The chat message endpoint refuses anyone but the owner, and says why in a
comment written without AI agents in view:

> processing forwards the *owner's* credentials (OIDC tokens, provider API
> keys) to external services. Allowing a non-owner to trigger processing would
> leak the owner's tokens to MCP servers the caller controls.

**That is the delegation problem stated as a prohibition.** An AI agent acting
for its owner is precisely a non-owner triggering processing with the owner's
credentials, which is the case the comment forbids. It is forbidden entirely
because there was no way to say that a particular non-owner had been authorized
by the owner to do precisely this, and no record that would settle it after the
fact.

So the grant is not an idea imported into this codebase from outside. **The
question it answers is already here**, asked by someone who could only answer it
by refusing the whole class. Seven more owner-only gates in the same file take
the same shape, and two on workspaces.

This also bounds what the grant has to do to be useful. It does not need to
express arbitrary permission: it needs to distinguish an authorized delegate
from an arbitrary caller, which is the distinction the prohibition could not
make.

### A non-user actor already has an RBAC subject of its own

The prebuilds system user is a seeded `users` row with no roles on it, and its
authority does not come from that row. A compiled in subject in `dbauthz` gives
it a hand written role covering templates, workspaces, prebuilt workspaces and
several reads, and the reconciler injects that subject directly.

**So RBAC does not need a users row in order to authorize a non-person.** It
needs one in order to authenticate. The subject is a structure the code builds,
and nothing in the policy asks where its roles came from.

The same identity is therefore capable in two different degrees. Injected, it
holds the orchestrator role. Arriving as a session token over HTTP, its subject
is built from the users row instead, which yields nothing but the implied
`member` that every user receives. Nothing reconciles the two.

### The transfer problem has already occurred, and cost two retrofits

A prebuilt workspace is created owned by the system user and later claimed,
which changes the workspace's owner. The session token minted under the old
owner remains valid for a workspace that owner no longer holds. This is the
transfer transition of the AI agent machine, arriving early and in another
guise.

Two mitigations exist and neither was part of the original design. The claim
path deletes the old token and mints a replacement. A periodic query then
expires whatever the claim path missed, which is the eager and swept pair this
corpus describes under reconciliation.

**The sweep recovers the workspace from the token's name by slicing characters
out of it**, because no column relates a credential to the resource it was
issued for. The same sweep also collects keys with no token name at all,
recorded in its own comment as probably created by logging in as the system
user, which the column comment says cannot happen.

### The exclusions of system users were added reactively

`is_system = false` appears as a filter in queries for group membership,
organization membership, insights, user secrets, AI seats and roles, and a
separate rule refuses the prebuilds actor permission to create API keys. Beside
one of them sits the admission that the handling covers a single system user
because only one exists.

**An absent filter is therefore not evidence that a site tolerates a non-person
actor.** These were found one at a time, and the set of places that would need
one has never been enumerated.

#### None of those exclusions prevents a failure

Read for what they do rather than for where they sit, the filters divide into
three purposes and none is defensive. Two correct a statistic, keeping an
internal subject from diluting a distribution or a count. Two hide non-people
from a listing of people, and are written so a caller may ask for them anyway.
One withholds a paid seat.

The system user broke none of those queries. It produced answers that were
**wrong rather than absent**, and a query for a per-person record it has no rows
in returns empty rather than failing: the appearance settings query builds its
result from `COALESCE(MAX(...), '')` over a configuration table, so a holder
with nothing there receives blank values and no error.

The seat query is the one worth copying. It requires `kind = 'human'` rather
than excluding the kinds known at the time, so an actor kind added later is
excluded without anyone remembering to exclude it.

## Benefits and costs of the rewrite

Collected as they appear rather than argued here. **This section exists to be
harvested**: doing this change in production needs a case made to people who
will not have followed the work, and both sides of it are easier to notice while
working than to reconstruct afterwards.

It is an account of cost against benefit, **not a case for and a case against**.
Those are different things. A pro and con argument admits anything that can be
said, weighted by how well it is said; an account admits only what is actually
spent and what is actually got, and can be checked. The second is what a
decision to spend real effort deserves.

**Costs are of three kinds and the account is wrong if they are mixed.**

- **Extra cost** is work that exists only because of this change. It is the only
  kind that is a true additional charge.
- **Brought forward cost** is work that has to happen regardless, arriving
  sooner. Its true cost is the time value of doing it now rather than later,
  which is usually small, plus the risk of doing it with less knowledge than
  waiting would have supplied. Recording it as though it were extra overstates
  the price of the change, sometimes by a great deal.
- **A tradeoff** is something genuinely worse afterwards, which no amount of
  effort removes. These deserve the most attention, being the only costs that do
  not end.
- **An unknown cost** is one whose kind or size has not been established. It is
  marked as unknown rather than guessed, because a guess entered into an account
  is indistinguishable from a measurement once it is written down.

Every cost entry says which kind it is. A brought forward entry says what
already requires the work. **An unknown entry says what research would settle
it**, since those are the tasks that have to be done before the final argument
can be made.

Its standing is **Derived**. Each entry rests on something recorded elsewhere in
this document or in the code, and is offered for challenge. An entry that cannot
name its evidence does not belong here.

### A credential's ending becomes recordable at all

Credentials are destroyed rather than retired: both revocation paths delete the
row, and a trigger deletes a user's keys on soft delete. A lifecycle whose
terminal event removes the record of itself cannot be journaled, so no amount of
work outside the table produces an auditable credential.

Lands with anyone who has been asked when a credential stopped being valid and
found the answer is not recorded anywhere.

### A class of bug disappears rather than being swept

The prebuilds system user already hits the case where a resource changes owner
and a credential issued under the old owner stays valid. Two mitigations exist,
neither part of the original design, and the second recovers a workspace
identifier by slicing characters out of a token name because nothing records
what a credential was issued for.

Relating a credential to its holder and its subject by construction removes the
need for both the eager deletion and the sweep, and removes the string parsing
with them. This is not a new capability, it is deleted code.

### The cost of the next actor kind stops rising

`users` carries four kinds of actor under three discriminators, each added by a
different mechanism as it arrived. Every kind so far has cost a column or an
enum value plus the discovery of wherever it needed excluding.

A typed holder makes the next kind a value rather than a schema change, which
matters because the set has grown three times recently and is about to grow
again when system actors leave `users`.

Lands with whoever is asked to estimate the next one.

### An unmeasured assumption becomes a number

Before this work nobody could say how much of the codebase assumes a credential
belongs to a person, only that the assumption was somewhere. Making it visible
to the compiler produced a count of a hundred and eighty five.

This is worth stating separately from the rewrite itself, because the count is
available whether or not the rewrite proceeds, and no estimate of the work was
possible without it.

### A blanket prohibition can become a precise one

The chat endpoint refuses every non-owner because processing forwards the
owner's credentials outward, and there was no way to say that a particular
non-owner had been authorized to do exactly that. Seven more gates in the same
file and two on workspaces take the same shape.

A recorded grant makes the legitimate case expressible, which converts a
prohibition covering a whole class into a decision about one delegate. The
demand for this already exists in the codebase, written as a refusal.

### Wrong answers stop being found by accident

The filters excluding system users correct statistics and hide non-people from
listings. None prevents a failure, so each was added when somebody noticed a
figure was wrong, and the set of places needing one has never been enumerated.

The seat query shows the alternative: requiring `kind = 'human'` excludes an
actor kind that does not exist yet. A typed holder makes that formulation the
natural one rather than the exception.

### Reliance on delete cascades goes away

Independently of auditing, a foreign key cascade is a poor mechanism for
revoking a credential: it runs inside the database with no actor in scope, it
cannot be observed, and it cannot be made conditional. Removing the holder's
foreign key removes the reliance, and the credential's ending becomes something
code does rather than something the storage engine does silently.

### Credential use becomes visible, including the uses that failed

`api_keys.last_used` records when a key was last accepted, and nothing records
when one was presented and refused. The two differ exactly where it matters: a
credential being offered after it stopped being valid is the signature of a
leaked or stale secret, and today it leaves no trace beyond whatever the request
log happens to keep.

Separating **presentation** from **use** makes that difference recordable, and
recording each presentation as an entry rather than only its latest time makes
the particulars available: which credential, offered where, refused why.

Lands with security, and it is the kind of improvement that is hard to retrofit
onto a column and nearly free once presentation is an event with a record.

### Capability becomes checkable against authorization

An API key's scopes and allow list are **capability**: what the machinery will
permit the holder to do. Authorization is a different level, an institutional
fact about what the holder has been permitted to do. Neither reduces to the
other, and requiring that all capability granting pass through authorization
would be a category error.

What the rewrite adds is the second level for actors that presently have only
the first. An AI agent's reach is bounded by its key's scopes and by the roles
it borrows, and nothing on record says what it was authorized to, so there is
nothing for those bounds to be compared against.

While the only grant is universal, little comes of the comparison beyond whether
the grant is valid and whether a credential exists for it. **The value is in
what it becomes.** Once authorization is elaborated, the reconciliation is that
an
actor's capabilities do not exceed their authorization, which is a question
nothing in the system can presently ask.

### Issuance can move to the journal before authentication moves

Issuance and verification are separable, and separating them is what lets the
rewrite be staged rather than landed whole.

An api key issued through the journal is a row in the ledger and a row in
`api_keys`, written in one transaction. The authentication path is untouched: it
reads `api_keys`, splits a token, compares a digest, and knows nothing of any
ledger. So the first half of the change can ship, and be exercised against the
unchanged system, before the half that carries the risk.

What that buys is a measurement. The claim that a credential's whole life can be
journaled is otherwise argued from a schema; with issuance moved it is argued
from a running system that authenticates tokens the journal minted.

The cost it defers is real and appears below as its own item: while the mirror
covers issuance alone, the two records diverge on every other path, and nothing
detects it. **A ledger complete about beginnings and silent about endings is
not yet authoritative**, and reading it as though it were is the mistake this
staging makes available.

### A retry loop for name collisions disappears

Creating an AI agent identity today runs a loop, up to five attempts, generating
a username of the form `ai-ws-` plus eight hex characters and retrying on unique
violation. `coderd/aiagentidentity/aiagentidentity.go` carries the loop, the
attempt count, the unique violation test, and a terminal error for exhausting
them.

**None of that is about AI agents.** It is there because the name is a `users`
row's username, which carries a unique index and a case insensitive one beside
it. An AI agent's name is used for logging and display: `rbac.Subject`'s own
comment says the friendly name "is entirely optional and is used for logging and
debugging".

Take the name off `users` and the requirement goes with it. A name computed from
the agent's identity needs no uniqueness check, no retry, and no failure mode
for running out of attempts, and the thirty or so lines implementing all three
go away with the constraint that forced them.

This is the smallest benefit recorded here and it is worth recording because of
its shape. Nobody would defend that loop on its merits. It exists because a name
had to be unique, the name had to be unique because it was a username, and it
was a username because an AI agent had to be a user.

### Cost: last use has to move off the credential row

**Extra, and smaller than it first appeared.** This entry is amended: it
originally concluded that last use belongs in neither a journal nor a ledger,
which the entity model has since answered.

`last_used` is written on every authenticated request, which makes `api_keys` a
hot write table. A ledger row changes when something is posted to it, and
`posting_reference` exists to detect concurrent posting, so a credential ledger
cannot simply absorb a column rewritten per request. That much stands, and it
constrains how the two credential stores can be joined.

Where last use goes is now settled rather than open. It is a variable, with a
journal recording assignments and a ledger holding the value last assigned. The
cost is moving it and amending the code that reads it, which is mechanical.

**It does not follow that the write volume rises.** The existing behaviour
already writes at most once per key per hour, and that throttle is expressible
as the predicate selecting which presentations the journal records. At the
default the cost is what it is today. A customer ordering that everything be
recorded raises it, and that is their decision to make.

### Cost: deciding the hundred and eighty five sites

**Unknown, and it is the largest line in the account.** Part is extra and part
is brought forward, and nothing establishes the proportion.

Each site reads a holder as though it were a user. For a human holder that is
correct and always will be, so a site touched only by humans is extra work
created by this change.

But `users` already contains four kinds of actor, and the sites are already
wrong for the three that are not human. The filters excluding system users from
statistics and listings are that wrongness being paid down one instance at a
time, reactively, by whoever noticed. **For any site an existing non-human
already reaches, this is brought forward rather than extra**: a latent defect
becoming visible and payable, rather than a new bill.

**What would settle it:** determine, for each site, whether an actor already in
`users` that is not a human reaches it. Those are brought forward, being a
latent defect becoming payable. The remainder is extra. Until that is done, a
hundred and eighty five is an upper bound on the extra cost and not an estimate
of it, and quoting it as one would overstate the change badly.

### Cost: the churn of renaming a column on a core table

**Extra.** A hundred and nine files for one column. It is mechanical, the
compiler finds every site, and a rename cannot be half done, but it is a large
diff to review and it touches packages whose owners have no interest in this
work.

The mitigation is that the cost is paid once and does not recur, and that a
mechanical diff is cheap to review per line even when there are many lines.

### Cost: replacing triggered deletion with a recorded ending

**Brought forward.** Credentials are presently destroyed by a trigger and by a
cascade. Both have to be replaced by something that records the ending, because
a credential lifecycle cannot be journaled otherwise, and that is required by
the audit work whether or not the holder ever stops being a user.

**So the charge is not the work, it is doing the work sooner.** For a piece of
work that is certainly required, the cost of advancing it is the time value of
the effort over the interval by which it moves, which is small unless the
interval is long. Against that sits a second component worth naming: work done
earlier is done with less knowledge, and may be done worse than it would have
been later.

Here that second component is small too, because the shape of the replacement is
already settled and does not depend on anything the rewrite would teach us.
**This is the clearest instance of a cost that would be overstated by being
counted as extra.**

### Cost: rewriting authentication to resolve a holder of either kind

**Unknown, and probably the largest single piece of work.** Nothing has been
attempted here. Authentication currently looks the holder up in `users` before
anything else runs, and refuses the request if it is not there. Making it
resolve either kind touches the path every request in the product takes.

The prebuilds system user shows the destination exists, since a compiled in
subject for a non-person already works, but it never authenticates: its subject
is injected server side. Nothing yet demonstrates the authenticating case.

**What would settle it:** build the smallest possible version, one holder, one
credential, one request, and see what it costs. That is a day's work and it
converts the largest unknown in the account into a measurement.

### Cost: consulting a grant on every authenticated request

**Unknown, and it is a running cost rather than a one off.** If an absent grant
means no subject is built, the grant has to be read during authentication, which
is a query added to the hottest path in the deployment.

Nothing here has measured it, and the shape that would make it cheap, a cache, a
join onto work already done, or a column carried on the credential itself, has
not been chosen.

**What would settle it:** decide where the grant is read, then measure the added
latency against a realistic request rate. Until then this is the only cost in
the account that recurs per request rather than per engineer.

### Cost: migrating a large api_keys table

**Unknown, and an operational cost rather than an engineering one.** Renaming a
column and adding a constrained one is cheap in a development database. On a
deployment with a large `api_keys` table the lock behaviour, the duration, and
whether it can be done without a maintenance window are all unestablished.

**What would settle it:** the row counts of the largest deployments, and a
rehearsal against a copy of one. The technique for avoiding a long lock is
known, adding the constraint as not valid and validating separately, so the
question is whether it is needed rather than whether it exists.

### Cost: referential integrity moves from a constraint to a reconciliation

**A tradeoff, and the one that does not end.** No foreign key can point at a
union of identity tables, so nothing in the schema prevents a credential naming
a holder that does not exist. The remedy is a reconciliation, which finds such
rows after the fact rather than refusing them.

This corpus already accepts that trade elsewhere and has a standard place to put
such checks, so it is a known quantity rather than a novelty. It is still a real
loss: a constraint is free and always correct, and a reconciliation costs
something to run and is only as current as its last run.

### Cost: a destructive down migration

**Extra, and a genuine downside.** Restoring the foreign key requires every
holder to be a user, so reversing the change must remove any credential held by
something else. A migration that deletes rows to go backwards is a migration
that cannot be reversed casually.

In production this is the kind of thing that turns a rollback into an incident,
and it deserves more care than a proof of concept branch gives it. **How much
more is unknown**, and depends on a rollback policy this work has not consulted.

### Cost: a branch wherever a holder is displayed

**A tradeoff, and a small one.** Showing a holder's name means switching on its
type, where before there was one table to join. A function that resolves a pair
to a name confines the branch to one place, and that function is wanted anyway
in preference to storing a name that can go stale.

## Open

### Whether an AI agent's own API key can reach a designated workspace

The workspace agent path and the chat tool path both populate the acting
identity on the subject. The path that authenticates an agent's own API key sets
the subject type but not the acting identity.

Read against the policy, such a subject is classified as an agent, has an empty
acting identity, and is therefore refused every workspace action outside the
exempt set. Yet the profile minted for that path grants scopes for actions the
policy would refuse. Either those scopes are unreachable by this path, or the
path is not the one used for those actions, or the acting identity is missing by
oversight. **This is a question for the code's author rather than a defect to be
asserted.**

### How revoking a grant reaches subjects that already exist

If an absent grant means no subject is built, a grant withdrawn after subjects
exist has to reach them, rather than only stopping the next one. Nothing in the
current code does this and no mechanism has been considered.

### How this is broken into work packages

The rewrite is too large for one, so it wants a sequence, and that sequence has
not been planned. What exists instead is a partial ordering: the three rings in
"The change has three rings", the risk ordering that puts the credential and
subject work first, and the observation that repointing the sandbox and session
clusters is mechanical once the identifier changes.

The work packages already written cover journals over entities this work owns.
This covers code it does not own and gates all of them, so it is not obvious
that the existing shape, with its acceptance test and its list of cheats per
package, transfers unchanged.

### Where fundamental changes to the codebase are documented

This document exists because there was nowhere to put material about changing
the code beneath the audit work. Whether that is a gap worth closing properly,
or whether one document per rewrite is enough, is unsettled.
