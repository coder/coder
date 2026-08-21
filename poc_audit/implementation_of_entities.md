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

**Stub**, with the schema built.

The tables exist to the current patterns. The code does not use them yet: it
stores a plaintext password in a table that predates the patterns and is to be
removed, and verifies by comparing every candidate for a holder.

- `coderd/database/migrations/000577_credential_lifecycle.up.sql`, the schema.
- `coderd/entity/credential.go` and
  `coderd/database/migrations/000575_valid_credentials.up.sql`, the stub.

### AI agent

**Stub**, and carrying proof of concept cheats.

One journal shared with every entity rather than one of its own, a single
timestamp where two are required, a row identifier where an entry identifier
and a line are required, no lines at all, and a ledger with no state column.

- `coderd/entity/aiagent.go`, `coderd/entity/journal.go`, and
  `coderd/entity/read.go`
- `coderd/database/migrations/000571_entity_journal.up.sql`
- `coderd/database/migrations/000573_entity_ai_agents.up.sql`

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
