# Rewriting RBAC

Started 2026-08-21. This document holds what is known about rewriting the
authorization and credential machinery so that an entity other than a user can
hold a credential and act.

It is deliberately thin. Unlike the audit approach, the corpus here is not being
settled before the work begins: the rewrite happens once, at proof of concept
scale, to find out whether it can be done at all. A production rewrite would
want this document well ahead of the code. This one runs alongside it, recording
the specific before any attempt is made to generalise.

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

### Where fundamental changes to the codebase are documented

This document exists because there was nowhere to put material about changing
the code beneath the audit work. Whether that is a gap worth closing properly,
or whether one document per rewrite is enough, is unsettled.
