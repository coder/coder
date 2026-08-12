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
- Sandboxes, which hold actors and so bear on the attribution of what those
  actors do, even though a sandbox does not act.

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

## Where the analogous code lives today

Nothing has moved here yet, and it is worth being exact about what has not, so
that the gap is visible.

User creation is `api.CreateUser` at `coderd/users.go:1950`, a method on the
coderd API struct sitting among HTTP handlers. It takes a `database.Store` as a
parameter rather than capturing one, which is the property that lets a caller
hand it a transaction.

User history is not application code at all. `user_status_changes` and
`user_deleted` are populated by the Postgres trigger
`record_user_status_change`. Application code never mentions those tables.

Two existing packages supply the shapes worth copying. `coderd/wsbuilder` owns
transactional creation spanning several tables and takes the store as a
parameter. `coderd/audit` shows how a journal writer is kept behind an
interface.

## Conventions for code here

**Take `database.Store` as a parameter.** Never capture it. A caller must be
able to pass a transaction handle so that several operations commit together.
This follows `api.CreateUser` and `wsbuilder.Build`, and it is what makes the
next convention possible.

**Journal writes go through their own function.** No lifecycle function writes
a journal entry inline. The entry and the state change it accounts for must be
able to commit in one transaction, and separating the write is what allows the
lifecycle function to compose them rather than duplicate them.

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

## Not yet decided

- Whether appending to the journal is authorized in `dbauthz` as its own
  action, and whether an entity may append entries concerning itself.
- How the lifecycle function behaves when called from inside an existing
  transaction, which depends on what this codebase's `InTx` does when a
  transaction is already open.
- Whether user journaling eventually moves here and supersedes the trigger.
  A trigger cannot see the actor that caused a change, so it cannot satisfy
  the attribution the audit approach requires. That argues for application
  code, but the trigger is the established precedent and displacing it is a
  decision rather than a cleanup.

## On this file

A `DIRECTORY.md` stating intended scope is not an existing convention in this
repository. It is here because the scope of this directory is a decision, and
decisions that live only in the arrangement of files get eroded by the next
person who adds a file. If the convention proves useful it should spread; if it
does not, this file should be deleted rather than left to go stale.
