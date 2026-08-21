# Implementation of Entities

Recorded 2026-08-21. This document records, for each entity, how much of it
exists in code and where that code is. `poc_audit/entity_model.md` says what the
entities are; this says what has been built.

Sections are separated by the standing of their contents. **Established**
records positions that have been decided. **Derived** records reasoning built
on top of them, which is offered for challenge rather than settled.
**Findings** records verifiable facts about the existing codebase.
**Open** records questions not yet answered.

## Established

### The entity corpus refers to implementation only through this file

**No document about entities cites code.** Where one needs to say something
about implementation it refers here, and **no more finely than an entity**: to
the entity, never to a function, a table, or a line.

The reason is that code moves and a citation into it rots. This file is a buffer
holding the citations that need maintaining, so that a heading somewhere else
does not silently come to point at nothing. The granularity rule protects the
same thing from the other side: an entity is a stable unit, where a function is
not.

### Code maturity has three levels and takes annotations

| Level    | Means                                                             |
|----------|-------------------------------------------------------------------|
| `absent` | No code.                                                          |
| `stub`   | Code exists and predates the patterns now settled.                |
| `built`  | Journal and ledger to current patterns, with the lifecycle wired. |

The levels are useful and are **not full descriptors**. In practice they are
qualified, because a level says how far something has been taken and not what is
irregular about it. Two annotations are in use.

**Carries proof of concept cheats.** Code may be built to the patterns and still
hold deliberate simplifications, or depend on something that does.
`entity.CreateAIAgent` is the standing example: it was written to the patterns
where it could be, and it calls AI agent code that is still transitory, so a
cheat lingers in it that is not its own.

**External misaligned code.** Substantial code exists that is not this work's
and is not built to these patterns. It is not a fourth level, because level says
maturity and this says provenance, and the two vary independently. The existing
`users` tables are the standing example.

## Findings

### Authorization

**Built.** The journal, the ledger, and the grant.

- `coderd/database/migrations/000576_authorization_lifecycle.up.sql`
- `coderd/database/queries/authorizationlifecycle.sql`
- `coderd/entity/authorization.go`

### Credential

**Built**, carrying proof of concept cheats.

Issuance and revocation write to the journal and post to the ledger. A password
is kept as an unsalted SHA-256 digest in hex, so nothing presentable is at rest.
The table that predated the patterns is dropped.

This closes **P7** in `poc_audit/security_findings.md`, which held that
credential events had no auditability. The cheats that remain are the null
credential type, whose always-validates path is real code, and the absence of
any expiry evaluation, which belongs to the work package that will write
expiries.

- `coderd/database/migrations/000577_credential_lifecycle.up.sql`
- `coderd/database/queries/credentiallifecycle.sql`
- `coderd/entity/credential.go`

### AI agent

**Built.**

Its own journal and ledger. Creation, finish, and kill are implemented, with
`transfer` present in the machine and performed by nothing. The ledger carries a
state, the entry precedes the posting it produces, and the pair is encapsulated
so a caller cannot write one without the other.

Creation records the **owner** as actor, a relaying `workspace_agent` commanding
nothing. The owner is kept as a pair, because ownership is not authorization and
an owner may be a system actor.

The stub that preceded this is gone: one journal shared by every entity, one
timestamp, a row identifier serving as an entry identifier, no lines, and no
state column.

- `coderd/database/migrations/000579_ai_agent_lifecycle.up.sql`
- `coderd/database/queries/aiagentlifecycle.sql`
- `coderd/entity/aiagent.go`

### User

**Absent**, external misaligned code.

Nothing here implements a user. The `users` table is long standing, is not built
to these patterns, and carries a system account filed among people.

### workspace_agent, workspace, sandbox

**Absent**, and for `workspace_agent` and workspace, external misaligned code.

No lifecycle of any of the three is journaled. A sandbox has no representation
at all.

### Session and coderd

**Absent.** Neither is journaled, and a session is described inline in a log
rather than identified.
