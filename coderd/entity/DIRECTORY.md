# coderd/entity

This directory owns the lifecycle of entities: their creation and destruction,
the identities and credentials issued to them, and the audit journals that
account for all of it.

It is new, and it is close to empty. What follows describes what the directory
is for, so that the intent does not have to be inferred from whatever happens
to be here first.

## What counts as an entity

**Anything whose lifecycle needs to be journaled is an entity.** That is the
whole test, and it is deliberately a test about obligation rather than about
structure.

An entity is therefore not merely a table with rows. Plenty of tables hold rows
nobody needs to account for. What makes something an entity is that its coming
into existence, its changes of state, and its ceasing to exist are events some
party is entitled to an account of.

In scope by that test:

- Users.
- AI agents.
- `workspace_agent` entities, including the sub-agents created for
  devcontainers.

A sandbox is not tracked separately. It holds an actor, and it is that actor,
the AI agent, whose lifecycle is journaled. Tracking the container as well as
what it contains would produce two accounts of one thing, and the account that
matters is the one naming the party whose acts are attributed.

Terminology for these is fixed in `poc_audit/entity_model.md` and is
authoritative over anything said here. In particular the bare word "agent"
means an agent in the relation of principal and agent, `workspace_agent` is
always written in full, and "AI agent" is always written in full.

## Scope

Three responsibilities, which belong together because they are the same
obligation seen from three sides.

**Lifecycle.** Creating an entity, changing its state, and ending it. The
function that does this is the single place that knows what a complete creation
consists of, so that no caller can perform half of one.

**Identity and credentials.** Minting the identifier an entity is known by, and
issuing, rotating, and revoking whatever credential it authenticates with. The
mandates governing this are in `poc_audit/security_findings.md`. The short form
is that the control plane is the sole issuer, that only a non reversible form of
a credential is stored, and that a credential is named for what it is rather
than for how it is presented or encoded.

**Audit journals.** Keeping the account. An entry is written for each
persistent state change, so that the state of the world can later be reconciled
against the journal. The approach is in `poc_audit/audit_approach.md`.

These are one directory rather than three because the account is not a
reporting feature bolted onto lifecycle management. Authority was delegated to
these entities, and the duty to account follows from that delegation. Splitting
the two would let either drift without the other noticing, which is the
condition the audit approach exists to detect.

## What this directory is not

**Not the mechanism named `audit_logs`.** That is a separate system that
records requests, and it is not what "audit" means here or in the design
documents. The two run side by side and neither is built on the other. Where
the existing mechanism is meant, it is named explicitly.

**Not `agent/`.** That directory is the `workspace_agent` binary that runs
inside a workspace. Lifecycle and credential decisions about `workspace_agent`
entities are made by the control plane and belong here; the code that runs in
the workspace does not.

**Not a replacement for `coderd/database/`.** Queries and generated access
stay there. This directory composes them into operations that mean something,
and owns the transaction those operations run in.

**Not a rewrite of how users are tracked.** Users are entities by the test
above, and the concerns here apply to them in principle. Rewriting the existing
mechanisms is nonetheless out of scope. User creation and the tables recording
user history stay where they are and keep working as they do. What remains
relevant after the proof of concept is the design question of whether one
account should eventually cover every entity, which is a question for that time
and not a licence to move code now.

## Shapes worth copying

Nothing has moved here yet. Two existing packages supply the structure.
`coderd/wsbuilder` owns transactional creation spanning several tables and
takes the store as a parameter. `coderd/audit` shows how a journal writer is
kept behind an interface.

## Conventions for code here

**Take `database.Store` as a parameter.** Never capture it. A caller must be
able to pass a transaction handle so that several operations commit together.
This follows `api.CreateUser` and `wsbuilder.Build`, and it is what makes the
next convention possible.

**Join the caller's transaction, or open one.** Lifecycle events frequently
have to be atomic with work that is not a lifecycle event, so the caller, not
the function, decides the boundary. Given a transaction, the function operates
within it and commits nothing itself. Given none, it opens its own, so that a
caller with nothing else to coordinate is not obliged to manage one.

**Journal writes go through their own function.** No lifecycle function writes
a journal entry inline. The entry and the state change it accounts for must be
able to commit in one transaction, and separating the write is what allows the
lifecycle function to compose them rather than duplicate them.

**An entity may not write entries about itself.** An account is rendered to a
party, by someone answerable to that party. An entity reporting on its own
conduct is not an account of it, and a journal that admitted such entries would
be a log of what its subjects chose to say. That is not what an audit journal
is: it is neither a raw log nor a record of activity, and its entries are
written by the party accounting for the entity rather than by the entity.

**The journal is entity-agnostic; creation is entity-specific.** One journal
serves every entity here. That follows the audit approach, which is stated
independently of any particular entity.

**An entry is a journal element; a record is stored representation.** The
distinction is fixed in `poc_audit/audit_approach.md` and is observed in code,
comments, and identifiers.

## Status

Nothing in this directory is implemented beyond what a proof of concept
requires, and the proof of concept lives on a branch that is not merging. Read
`poc_audit/AGENTS.md` before treating any of the design documents referenced
above as a description of current behavior. They describe an intent.

## On this file

A `DIRECTORY.md` stating intended scope is not an existing convention in this
repository. It is here because the scope of this directory is a decision, and
decisions that live only in the arrangement of files get eroded by the next
person who adds a file. If the convention proves useful it should spread; if it
does not, this file should be deleted rather than left to go stale.
