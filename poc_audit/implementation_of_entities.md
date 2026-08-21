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

**Stub**, and carrying proof of concept cheats.

One journal shared with every entity rather than one of its own, a single
timestamp where two are required, a row identifier where an entry identifier
and a line are required, no lines at all, and a ledger with no state column.

- `coderd/entity/aiagent.go`, `coderd/entity/journal.go`, and
  `coderd/entity/read.go`
- `coderd/database/migrations/000571_entity_journal.up.sql`
- `coderd/database/migrations/000573_entity_ai_agents.up.sql`

### Where lifecycle events are observable in existing code

Six points in code already reached by the events this work wants to record.
Each is in coderd, inside a request handler or a transaction, so none needs new
plumbing to reach. None of them journals anything today; each writes a row
directly.

| Event                    | Function                        | File                                              |
|--------------------------|---------------------------------|---------------------------------------------------|
| AI agent created         | `Create`                        | `coderd/aiagentidentity/aiagentidentity.go`       |
| AI agent revoked         | `revokeWorkspaceOrigin`         | `coderd/aiagentidentity/workspace.go`             |
| AI agent revoked         | `revokeAIAgentIdentity`         | `coderd/provisionerdserver/provisionerdserver.go` |
| Sandbox created          | `postWorkspaceAgentAISandbox`   | `coderd/aisandboxes.go`                           |
| Sandbox deleted          | `deleteWorkspaceAgentAISandbox` | `coderd/aisandboxes.go`                           |
| Session opened or closed | `postAISandboxSession`          | `coderd/aisandboxaudit.go`                        |

**The activity session already begins where it should.** `agent/confine`'s
supervisor builds the namespace and its redirect rules, then forks the child,
and stamps the start in the fork's start callback. So the moment recorded is the
one where the capacities exist and the thing able to use them has just begun,
which is what an activity session is meant to mark.

**An end is reported, and cannot be relied on.** The supervisor re-posts the
same session with an end time once the child returns, a convention the agent SDK
documents. That post is best effort and only happens if the child started and
the supervisor survived to send it, so a killed workspace or a crashed
supervisor leaves the session open with nothing to notice. The prompt path is
real and lapse is what covers its absence, which is the same arrangement the
credential sweep has.

**Jon's session is not this work's session.** `ai_sandbox_sessions` names four
parties, a reporter agent, a confined agent, an AI agent, and a sponsor user,
where the convention here allows two; and it is bounded by one child process
execution where an AI agent's session is bounded by the lifespans of its
participants. The two may be reconciled later. Treating them as one thing under
two names would be the easiest mistake available here.

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
