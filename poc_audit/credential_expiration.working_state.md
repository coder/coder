# Working state for credential expiration

Recorded 2026-08-25.

**Working material, not design.** The credential machine's treatment of expiry
was raised early and deliberately left unsettled, and this file holds what has
accumulated against the day it is taken up. Nothing here is decided except where
it says so and names who decided it.

It exists because two things arrived on the same day. The chat gateway's
handling of expiry was measured and is worse than had been assumed, and a
reframing of what an expiry *is* arrived that is better than what the corpus
currently holds. The first is why an interim cheat was granted. The second is
why a passage in `entity_model.md` is now marked in dispute.

## The worker with an alarm clock

Eric, 2026-08-25:

> If there were a worker with an alarm clock, they'd wake up at the alarm and
> revoke the key. The actor is the issuer of the credential, acting in a delayed
> manner to undo what they'd done before.

The story supplies three things the treatment of expiry was missing.

**It says where the expiry lives.** With the worker, as the alarm setting, and
not with the credential. That is what "a fact about how the credential is
managed" means once it is made concrete rather than gestured at.

**It says why an extension is not an event.** Resetting your alarm before it
rings is not something that happens to the credential. Nothing about the
credential changed; the manager changed their own plans. So an extension having
no journal entry stops being an exemption granted to a difficult case and
becomes what the model predicts.

**It names the actor.** The issuer, acting at a distance to undo what they did
before. Expiry becomes a revocation the issuer scheduled at the moment of
issuance rather than a thing that happens by itself.

## What the story disputes

`entity_model.md` currently reads, of `expire`:

> `expire` arises when the clock passes the credential's expiry. Nobody decides
> it and nobody notices it: it follows from the expiry the record already holds,
> so it is entailed, and the entry carries no actor.

Under the alarm clock, **somebody did decide it**, at issuance, and **somebody
does notice it**, on waking. Both clauses of that passage are contradicted, and
with them the classification: `expire` would be commanded, not entailed, and it
would carry the issuer as its actor.

**Eric, 2026-08-25: the earlier analysis is wrong, this is the right actor, and
expiration can be modeled as a subtype of revocation or as parallel to it.** The
passage is marked in dispute pending that work. Which of subtype and parallel is
not settled.

## What the reframing buys

**It retires the last place the credential machine wanted a system actor.**
Classing the machine's non commanded transitions as entailed was what let the
fixed system identity be dropped, but expiry was the awkward one: a sweep still
has to be somebody when it writes an entry. This answers that without inventing
a party, and without a custodian.

**The actor is already recorded.** The issue entry names who issued the
credential. A later expiry attributing itself to that party is reading a fact
the journal already holds rather than asserting a new one.

**It collapses a transition rather than adding one.** Three entailed transitions
were justified by their differing in what they follow from. If expiry is a
scheduled revocation, the machine has two entailed transitions and a commanded
one that fires late, which is a smaller claim.

## What the reframing costs, and what stays open

**Who writes the entry when the alarm rings.** The issuer is the actor, but the
issuer is not present; a sweep is. Whether the sweep writes on the issuer's
behalf, and what that relationship is called, is the question this pushes into.
It resembles the custodian problem rather than escaping it.

**Subtype or parallel.** If expiry is a kind of revocation, the machine may want
one transition with a distinguishing parameter rather than two transitions. That
turns on whether anything reads them together, which is not known.

**Where `expires_at` belongs.** If the expiry is the manager's alarm setting
rather than a property of the credential, the column may belong somewhere other
than the credential ledger. Holding it on the credential is what makes it look
like part of the credential, which is the reading in dispute.

**Whether discharge absorbs part of this.** A chat ending is an ending of the
thing the credential is accessory to. Some of what the expiry currently
approximates is discharge wearing a clock.

## The interim treatment, and why it costs nothing

**Expiry is provisionally treated as a fact about management, so it moves
without a journal entry.** Eric, 2026-08-25, withdrew for this item only the
requirement that the solution use the journal.

This is consistent rather than merely tolerated, and the reason is narrow and
checkable: **the ledger holds no expiry to be falsified.** Every ledger row is
inserted with a null `expires_at`, both folds write other variables, and no
statement anywhere updates it. So an extension in the mirror contradicts nothing
the ledger says, and the transition it performs changes no state.

The column exists and carries a long comment describing what it would mean. That
comment describes an intention. **Nothing has ever written it.**

## Findings about the chat gateway

Verifiable facts about the codebase as of 2026-08-25, established by reading it.
The mechanism is `ensureChatGatewayKeyID` in `coderd/x/chatd/synthetickey.go`,
which runs on every generation.

**Extension is unbounded.** No cap on total lifetime and no count of extensions.
Any generation within the window pushes expiry to twenty four hours out.

**So the expiry bounds inactivity rather than life. Derived.** What was read is
that extension is unbounded and triggered by use; the idle timeout is inferred
from those and is not stated anywhere in the code.

**An expired credential can be returned to validity.** `GetAPIKeyByName` matches
on holder and token name and does not filter on expiry, so a chat waking after
its key expired finds the row, fails the margin test, and falls into the
extension arm. The row survives until `dbpurge` collects it at expiry plus the
API key retention, defaulting to seven days. **`invalid` is terminal in the
model, and this path leaves it.** Recorded as P11 in `security_findings.md`.

**The extension can roll `last_used` backwards.** It calls `UpdateAPIKeyByID`,
which writes `last_used`, `expires_at` and `ip_address` together, and the caller
feeds back the values it read a moment earlier. The per user synthetic key path
in the same file does the same thing.

**Archiving a chat does not end its credential or its agent.** Archive is the
user visible end, firing `ChatWatchEventKindDeleted`, and it blocks further
generation. Nothing in the chat path retires the agent or ends the key. The only
path that does is the orphan sweep in `dbpurge`, whose criterion is that the
chat row no longer exists, which archiving does not satisfy.

**The policy it shadows is off by default.** Auto archive answers the same
question with an administrator visible setting. `DefaultChatAutoArchiveDays` is
zero and `archiveOnce` returns immediately at zero or less.

**Two lifetimes are declared separately and asserted to match by comment.**
`chatAgentKeyLifetime` in chatd and `keyLifetime` in `aiagentidentity`.

## What the rewrite has to supply first

**An ending for chat AI agents.** The obvious simplification, deleting extension
and letting a long lived credential end when its holder does, does not work
today: nothing retires a chat agent except hard deletion of the chat by
retention. With retention off a chat agent is immortal and so would its
credential be.

**So the insane mechanism is load bearing.** The twenty four hour timeout is
currently the only thing bounding a chat agent credential at all, which is why
the interim answer is to document and leave it rather than to remove it.
