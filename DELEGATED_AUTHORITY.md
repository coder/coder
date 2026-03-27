# Delegated authority for Coder Agents in human workspaces

**Status:** draft sketch

## Summary

This document sketches an authority model for Coder Agents that allows
automations to run in human workspaces without inheriting ambient human-linked
authority.

The core proposal is a hybrid model:

1. **Base automation principal**: every automation runs as a service account or
   workflow principal for baseline Coder RBAC and template-provided
   credentials.
2. **Delegated session**: human-linked authority is additive, explicit,
   temporary, and brokered.

This separates two questions that are conflated today:

1. **May this automation execute in workspace `W`?**
2. **May this automation use provider `P` as human `U` for action `A` on
   resource `R` until time `T`?**

The design goal is not to forbid automation in human workspaces. The design
goal is to ensure that execution in a human workspace does not automatically
imply access to the human's Coder-managed external identity.

## Problem

Today, authority is partially derived from ownership fields rather than from a
first-class automation principal and explicit grants.

In the current implementation:

- `postChats` creates chats with `OwnerID = apiKey.UserID`.
- initial workspace attachment is gated by `policy.ActionSSH` on the selected
  workspace.
- some control-plane chat tools reconstruct the chat owner's RBAC subject and
  run as that owner.
- once attached, workspace file/edit/execute tools use the workspace agent
  connection directly.
- chat-side git auth resolves external auth from `chat.OwnerID`.
- workspace-side external auth resolves external auth from `workspace.OwnerID`.
- `workspaceAgentsExternalAuth` can return raw OAuth token material when the
  workspace agent uses the default `all` API key scope.

This creates the wrong mental model: "the automation runs as Mike".

That model is too broad for safe automation in human workspaces because it
conflates:

- the human who created the automation,
- the principal the automation executes as,
- the workspace in which code runs, and
- the human whose external identity is being borrowed.

## Goals

- Allow automations to run in human workspaces.
- Remove ambient human-linked authority from chat ownership and workspace
  ownership.
- Preserve service-account-backed workflows and template-provided credentials.
- Make borrowed human authority explicit, time-bounded, auditable, and
  revocable.
- Unify chat-side and workspace-side external authority resolution behind one
  model.
- Support delegated authority for Git, external auth, and MCP-backed tools.
- Keep authorization legible: operators should be able to answer who granted
  what, to whom, for which run, and until when.

## Non-goals

- Solving exfiltration of arbitrary local secrets already present in a human
  workspace, such as plaintext files, SSH keys, local git credentials, or
  environment variables.
- Redesigning org-scoping for model configs, provider configs, or MCP configs.
  However, org-scoping all chats is a prerequisite for this design and must be
  completed before delegated authority enforcement begins.
- Designing a complete provider-specific capability language for every external
  system. The initial vocabulary is intentionally coarse and covers only
  `provider_kind`, `access_level`, and `resource_scope`.
- Defining a new break-glass admin impersonation flow. The existing owner role
  already serves this purpose.
- Auto-archiving inactive chats. Chats have an archived state, not an expired
  state. Automatically archiving idle chats (and thereby ending their delegated
  sessions) may be useful but is a separate feature.
- Auditing or constraining template-author token injection. Template authors
  can inject raw tokens via environment variables or commands. This is an
  explicit infrastructure decision by a trusted principal, not an implicit
  authority leak. Auditing template content is an operational concern (GitOps
  review, template approval workflows) outside the scope of the delegation
  layer.

## Key decisions in this draft

### 1. Automation does not run as the human creator

An automation should not implicitly run as the human who created the chat,
workflow, or task. It should run as a **base automation principal**.

For the initial design, that principal should be a service account or workflow
identity.

### 2. Execution authority and credential authority are separate

Two independent grants exist:

1. **Workspace execution grant**: the automation principal may execute in a
   specific workspace.
2. **Delegated credential grant**: the automation may use a specific human's
   external identity for a constrained set of actions.

Neither grant implies the other.

### 3. Borrowed human authority requires the credential owner's consent

Only the **credential owner** may delegate use of their human-linked external
identity.

An admin role should not automatically imply authority to delegate another
human's GitHub, GitLab, or other external identity.

### 4. Human workspaces require workspace-side consent

Running an automation inside a human workspace also requires approval from the
workspace side.

In the simplest case, the workspace owner is the approver. If the workspace is
jointly administered through Coder permissions, the effective approver is
whoever is already allowed to grant workspace execution for that workspace.

### 5. Delegated sessions activate only when both sides agree

If workspace owner and credential owner are different humans, both approvals are
required:

- workspace-side approval for execution in that workspace, and
- credential-owner approval for borrowed human authority.

If they are the same human, one approval can satisfy both roles.

### 6. Workspace automation uses a dedicated permission, not SSH

Automation attachment should be gated by a new `workspace.automate` permission
rather than reusing `policy.ActionSSH`. SSH represents interactive human shell
access; overloading it for programmatic automation conflates two distinct trust
decisions and would require a harder migration later.

### 7. Provider capability policies are structured and tiered

Delegated session grants use a structured capability vocabulary so that policies
can be mechanically intersected. Three policy tiers exist: deployment, org, and
user. Each tier can only narrow what the tier above allows. Absent policy at any
tier is a no-op (allow-all if no tier above constrains it). The effective grant
is always the intersection of all applicable tiers.

### 8. Delegated credential approval is scoped to the root chat

Delegated credential approval is neither per-message nor permanent. It is
scoped to the **root chat** — the top-level chat session that initiated the
automation. When the root chat is explicitly ended or revoked, all delegated
sessions bound to it expire.

A root chat is considered active as long as it has not been explicitly ended or
revoked. Navigating away from the UI does not end the session — an in-progress
conversation turn continues running. Auto-archiving inactive chats is out of
scope for this design (see non-goals).

Subagents and descendant chats spawned from the root chat inherit its delegated
sessions. They do not require re-approval, and they cannot outlive the root
chat's session. Unrelated chats never inherit a delegation, even if they share
the same automation principal or workspace.

This follows the Deny / Allow / Allow-for-Session pattern common in permission
prompts, where "session" maps to the root chat lifecycle.

Delegated authority requests (allow/deny prompts from the agent) are blocking:
if the agent requests a new delegated session or an escalation of an existing
one, no further execution of that conversation turn proceeds until the
credential owner responds. This prevents an automation from racing ahead with
partial authority while a consent decision is pending. Blocking prompts do not
time out — an unanswered prompt remains pending indefinitely.

### 9. Org-scoping all chats is a prerequisite

Every chat must have a first-class `organization_id`. Delegated sessions,
provider capability policies, and the tiered policy model all require an
unambiguous org context. This design cannot be enforced consistently without it.

### 10. Automation agents are bound to their automation principal, not the workspace owner

Each automation principal gets a **dedicated agent instance** within the
workspace, bound to the automation principal's identity rather than the
workspace owner's. Template authors declare this in Terraform:

```hcl
resource "coder_agent" "bot" {
  user_id = data.coder_user.bot.id
}
```

This makes identity separation a natural property of the agent connection
rather than a per-endpoint check:

- `coder external-auth access-token` called from an automation agent should
  not return raw human tokens without an active delegated session. The
  endpoint should check for delegation authorization before returning
  credentials. An automation agent has no linked external credentials by
  default — it accesses external providers only through delegated sessions.
  Workspace-interior credential flows (e.g., `GIT_ASKPASS`) that receive
  tokens after this check operate within the workspace trust domain (see key
  decision 11).
- A human SSHing into the same workspace still connects through their own agent
  with their own identity. No special-case logic needed.
- Multiple agents per workspace are already supported. The human owner's agent
  and one or more automation agents coexist.

This does not reach inside the workspace to control what processes do with
local secrets (which remains a non-goal). It controls what identity the agent
presents to the control plane at the agent-to-API boundary.

### 11. No raw token disclosure through the delegation layer

The control plane must never disclose raw human OAuth tokens to the upstream
LLM provider or expose them in the agent-to-coderd API boundary where they
could enter an LLM context window. Delegated sessions broker actions on
behalf of the credential owner — the LLM never sees the token.

**Boundary clarification: workspace-side git credentials.** Git operations
are shell commands executed inside the workspace. The existing `GIT_ASKPASS`
flow (agent binary → coderd API → OAuth token → stdout → git) operates
entirely within the workspace's trust domain, the same domain that already
contains SSH keys, dotfiles, local credentials, and anything else the
workspace owner has configured. The delegation layer mediates *whether*
automation can trigger git operations (via the delegated session and provider
policy), not *how* git authenticates once a permitted operation executes
inside the workspace. A control-plane git proxy was evaluated and rejected:
it would require TLS MITM with CA distribution into every workspace, only
cover HTTPS remotes (not SSH), make coderd a data-path bottleneck for every
clone, and provide no meaningful security improvement over the existing flow
given that the workspace interior is already a shared trust domain.

The "no raw token" guarantee therefore applies at the interface between
coderd and the upstream LLM — preventing token material from entering model
context — not at the boundary between the workspace agent and git. If an
automation agent needs a raw token for a non-git use case, it must be
provided explicitly by the template author (e.g., via environment variable
or a command the agent runs), not resolved implicitly from a human's linked
external identity.

### 12. Revocation does not cancel in-flight operations

When a delegated session is revoked, operations already in progress are
allowed to complete. Subsequent new operations fail immediately.

The control plane checks session validity at the start of each brokered
operation (the "check on entry" point). An operation that has already passed
this check and is mid-execution — e.g., a `git push` being proxied — runs
to completion. The next operation the agent attempts will see the revoked
session and fail.

This is simpler than attempting to interrupt in-flight external calls, which
would require cancellation coordination with external providers and could
leave resources in an inconsistent state. The practical exposure window is
small: brokered operations are short-lived individual API calls, not
long-running streams.

### 13. Cross-org delegation is forbidden

A delegated session requires a single unambiguous org context. The credential
owner must be a member of the workspace's org, and the provider and template
policies evaluated are those of that org.

Cross-org delegation — a workspace in org A borrowing a credential whose
external auth link was configured in org B — is forbidden. The existing
workspace infrastructure is strictly org-scoped: workspace CRUD, listing,
agent connections, and RBAC role grants all require the acting principal to
be a member of the workspace's org. The only cross-org escape hatch in the
current codebase is `authenticated`-level app/port sharing, which is an
explicit, narrow exception for browser-accessible workspace apps — not a
general authority path.

Introducing cross-org delegation would require inventing a new cross-org
authority mechanism that conflicts with the strict org-scoping everywhere
else. The three-tier policy intersection (deployment → org → user) assumes
a single org context for tier 2; ambiguity about which org's policies apply
would make the effective capability set undefined.

If a future use case requires cross-org credential sharing, it should be
designed as an explicit cross-org trust relationship with its own approval
and audit model, not grafted onto the delegation layer.

## Design principles

- **Human-linked authority is brokered, never disclosed to the LLM.** The
  delegation layer brokers actions on behalf of the credential owner. Raw human
  tokens are never exposed at the coderd-to-LLM-provider boundary or allowed
  to enter an LLM context window. Workspace-interior credential flows (e.g.,
  `GIT_ASKPASS`) operate within the workspace's existing trust domain and are
  gated by delegation policy, not proxied.
- **Execution consent and credential consent are separate sovereign decisions.**
- **The default automation identity is not a human identity.**
- **Delegated authority is additive to the base principal, but policies
  constraining it are subtractive.** A delegation *adds* capabilities that the
  base automation principal does not have on its own (the human's external
  identity). Within that delegation, the tiered policy model *narrows* what is
  permitted: each tier can only restrict, never widen. The effective capability
  is the intersection of all tiers. The result should be least-privilege,
  revocable, and time-bounded.
- **Admin authority over Coder resources does not automatically become authority
  over a human's linked external identity.**
- **The same delegation model should govern both chat-side and workspace-side
  external authority resolution.**

## Current-state summary

The design is motivated by the current split between execution and external
identity resolution.

### Workspace attachment and execution

- Chat creation can attach a workspace only if the caller is authorized for
  `policy.ActionSSH` on that workspace.
- Workspace lifecycle tools such as `list_templates`, `read_template`,
  `create_workspace`, and `start_workspace` reconstruct the owner's RBAC subject
  and run with owner-scoped Coder permissions.
- Once a chat is attached to a workspace, file/process/computer-use tools talk
  to the workspace agent connection and do not re-authorize every operation as a
  separate user action.

This means execution inside the workspace is largely capability-based once the
attachment succeeds.

### External authority resolution

Two current code paths derive external authority from human ownership:

- chat-side Git flows resolve external auth from `chat.OwnerID`.
- workspace-agent external auth resolves external auth from `workspace.OwnerID`.

The second path is especially risky because the workspace agent endpoint can
return raw token material when the workspace agent runs with the default `all`
API key scope.

## Threat model

This proposal aims to prevent the following classes of failure:

- A human creator accidentally giving an automation all of their linked external
  authority just by creating a chat.
- An automation in Alice's workspace using Mike's external identity without
  Mike's explicit consent.
- Mike using his own external identity inside Alice's workspace without Alice's
  approval of that workspace context.
- A site or org admin role being treated as implicit authority to borrow any
  human's external identity.
- Chat-side and workspace-side integrations making inconsistent authority
  decisions because they resolve credentials through different ownership fields.

This proposal does **not** prevent arbitrary code in a workspace from reading
secrets already present in that workspace.

## Proposed model

## Actors

| Actor | Meaning |
| --- | --- |
| Automation principal | Service account or workflow identity used as the base actor for a run. Bound to a dedicated workspace agent instance. |
| Workspace owner | Human or service account that owns the workspace in which automation runs. |
| Credential owner | Human whose linked external identity may be borrowed. |
| Grantor | Principal that approves a grant. For human-linked authority, this is the credential owner. |
| Run owner / creator | Human who initiated the run. This is metadata, not the ambient authority source. |
| Template author | Defines workspace topology in Terraform, including which agents exist and which principals they are bound to. |

## Resources

| Resource | Purpose |
| --- | --- |
| Automation run | A concrete execution context, such as a chat, task, or future workflow run. |
| Workspace execution grant | Permission for an automation principal to execute in a workspace. |
| Delegated session | A concrete, revocable authority envelope tied to a run and provider. |
| Delegated session grant | A scoped permission within a delegated session, such as GitHub comment access on one repository. |

## Conceptual data model

This is intentionally conceptual and not yet a migration plan.

### Automation run

The system needs a first-class way to answer: "what principal is this run
executing as?"

Possible shape:

- `id`
- `kind` (`chat`, `task`, `workflow_run`, ...)
- `root_chat_id` (the top-level chat that defines the session boundary)
- `parent_run_id` (if this is a subagent or descendant run)
- `automation_principal_id`
- `creator_id`
- `workspace_id`
- `organization_id` or derivable org context
- `created_at`

This may map onto existing `chats` and `tasks`, but the important changes are
the explicit `automation_principal_id` and the `root_chat_id` that ties a run
to its session boundary.

### Delegated session

Possible shape:

- `id`
- `root_chat_id` (session boundary; all runs under this root chat share the
  session)
- `workspace_id`
- `automation_principal_id`
- `credential_owner_id`
- `provider_kind` (`github`, `gitlab`, `external_auth`, `mcp`, ...)
- `state` (`pending`, `active`, `expired`, `revoked`, `denied`)
- `granted_at`
- `expires_at`
- `revoked_at`
- `created_by`
- `reason`

The session is bound to `root_chat_id`, not to an individual `run_id`.
Subagent runs under the same root chat inherit active sessions without
re-approval.

### Delegated session grant

Possible shape:

- `id`
- `delegated_session_id`
- `provider_id` or server/config identifier
- `provider_kind` (`github`, `gitlab`, `mcp`, `external_auth`)
- `access_level` (`read`, `write`, `admin`)
- `resource_scope` (provider-specific, e.g., `org/repo`, `group/project`,
  `server/tool`)
- `constraints` (optional provider-specific extensions: branch, tool allowlist,
  rate limit, etc.)

## Approval model

### Approval rule

A delegated session is active only when all required approvals exist.

#### Workspace-side approval

Approves the statement:

> automation principal `P` may execute in workspace `W`.

This should be grantable by the workspace owner or another principal already
allowed to manage automation access for that workspace.

#### Credential-side approval

Approves the statement:

> automation principal `P`, in run `R`, may use provider `X` as human `U` for
> actions `A` on resources `S` until `T`.

This should be grantable only by the credential owner.

### Time scale

The two approvals should live at different time scales.

- **Workspace execution approval** may be relatively durable. It answers
  whether an automation principal may run in a given workspace at all.
- **Delegated credential approval** is scoped to the **root chat session**.
  It activates when the root chat begins (and the credential owner approves),
  and expires when the root chat is explicitly ended or revoked. Navigating
  away from the UI does not end the session. Subagents and descendant chats
  spawned within the same root chat inherit the approval without re-prompting.
  Unrelated root chats require their own approval.

This follows the Deny / Allow / Allow-for-Session pattern, where "session"
maps to the root chat lifecycle. It keeps human workspace automation usable
without making borrowed external identity ambient or requiring per-message
consent.

Delegated authority requests are blocking — the agent's conversation turn is
suspended until the credential owner responds to an allow/deny prompt. Prompts
do not time out.

### Examples

#### Example A: Alice automates inside her own workspace using her own GitHub

Alice is both workspace owner and credential owner.

One approval flow can cover both dimensions:

- allow the automation principal to run in Alice's workspace
- allow that run to use Alice's GitHub for a narrow set of actions

#### Example B: Mike creates an automation in Alice's workspace using Mike's GitHub

Two approvals are required:

- Alice approves execution in her workspace
- Mike approves use of Mike's GitHub identity

Mike being an admin does not remove Alice's approval requirement, and Alice's
workspace ownership does not let her delegate Mike's GitHub identity.

#### Example C: service-account workflow in its own workspace

No human delegation is required if the workflow relies only on:

- service-account RBAC, and
- template-provided or service-account-provided credentials

This is the existing "dedicated automation workspace" pattern and should remain
first-class.

## Provider capability policy

### Capability vocabulary

Delegated session grants need a structured vocabulary so that policies at
different tiers can be intersected mechanically. An opaque-string model would
prevent the control plane from computing effective permissions.

Minimal first-cut vocabulary per provider kind:

| Field | Type | Examples |
| --- | --- | --- |
| `provider_kind` | enum | `github`, `gitlab`, `mcp`, `external_auth` |
| `access_level` | enum | `read`, `write`, `admin` |
| `resource_scope` | provider-specific string set | `org/repo`, `group/project`, `server/tool` |

This vocabulary is intentionally coarse. Provider-specific integrations may
interpret finer-grained constraints stored alongside these fields, but the
control plane only needs to intersect at this level.

### Policy tiers

Three tiers of policy can constrain what a delegated session grant is allowed
to express:

| Tier | Set by | Purpose |
| --- | --- | --- |
| Deployment | Deployment admin | Floor/ceiling that overrides everything |
| Organization | Org admin | Narrows within deployment bounds |
| User | Credential owner | Narrows within org bounds |

Each tier can only restrict what the tier above allows. No tier can widen
permissions beyond what a higher tier permits.

### Resolution rules

The effective capability set for a delegated session grant is the intersection
of all applicable tiers:

1. Start with **allow all**.
2. If a deployment policy exists, intersect with it.
3. If an org policy exists, intersect with the result.
4. If a user policy exists, intersect with the result.

Absent policy at any tier is a no-op — it does not restrict. The only way
something gets restricted is by an explicit policy existing at some tier.

Intersection semantics per field:

- **`access_level`**: effective level is the minimum of all tiers that specify
  one. (`admin` > `write` > `read`.)
- **`resource_scope`**: effective scope is the set intersection of all tiers
  that specify one. A tier that specifies `orgA/*` intersected with a tier
  that specifies `orgA/repo1, orgA/repo2` yields `orgA/repo1, orgA/repo2`.
- **Absent field at a tier**: defer to the tier above (or allow-all if no tier
  above specifies it).
- **Explicit empty set**: deny all for that field.

### Relationship to Coder RBAC

This policy hierarchy is similar in shape to Coder's existing RBAC model —
collect applicable policies, intersect, cache — but the domain is different.
RBAC governs actions on Coder control-plane resources. Provider capability
policies govern what an automation can do with a human's external identity on
an external system.

The evaluation pattern (layered policy collection, intersection, caching) could
reuse RBAC infrastructure, but the policy storage should be separate since the
resources being constrained are not Coder objects.

### Editor and effective-permission visibility

Admins and users need an editor for managing policies at their respective
tiers. The editor should show **effective permissions** — the computed
intersection — not just the policy the current tier defines. This lets a user
see what they actually end up with after deployment and org policies are
applied.

## Enforcement model

### 1. Base execution identity

All new automation runs should execute as an explicit automation principal.

The run metadata should retain:

- who created the run,
- who approved delegated authority,
- which workspace it targets,
- which provider grants are active.

But the ambient actor used for authorization should be the automation principal,
not the human creator.

### 2. Workspace execution checks

Automation attachment should be gated by a new `workspace.automate` permission
rather than reusing `policy.ActionSSH`.

`ActionSSH` represents interactive human shell access. Overloading it for
long-running programmatic execution conflates two distinct trust decisions and
would require a harder migration later. Introducing `workspace.automate` from
the start keeps the permission model self-documenting and lets Coder
distinguish:

- a human being allowed to shell into a workspace, from
- an automation principal being allowed to execute tool calls there.

Existing `ActionSSH` checks remain unchanged for interactive SSH access.

### 3. External authority checks

Because automation agents are bound to their automation principal (not the
workspace owner), credential resolution naturally keys off the agent's identity.
An automation agent has no linked external credentials by default.

Both chat-side and workspace-side external authority should resolve through the
same delegation layer:

- `resolveChatGitAccessToken` — chat-side Git credential resolution
- `workspaceAgentsExternalAuth` — workspace-side external auth resolution

For automation agents, both paths should answer the same question:

> is there an active delegated session that authorizes this run to use this
> provider as this credential owner for this action on this resource?

If not, the request should fail. No fallback to `chat.OwnerID` or
`workspace.OwnerID`.

For automation agents, `workspaceAgentsExternalAuth` must not return raw human
tokens (see enforcement section 4). It should either broker the action through
the control plane or return a short-lived, scoped credential minted by the
control plane.

A human's own agent in the same workspace continues to resolve credentials
from the human's identity as today. No special-case logic is needed — the
agent's bound principal determines the resolution path.

### 4. Brokered credentials

The control plane brokers external actions on behalf of the credential owner.
Raw human OAuth tokens are never disclosed at the coderd-to-LLM-provider
boundary (see key decision 11).

- The automation agent calls the control plane (e.g., chat-side Git action,
  external auth endpoint).
- The control plane verifies an active delegated session exists. This is the
  "check on entry" point — session validity is evaluated here, not
  continuously during execution.
- For chat-side actions (coderd calls the external provider directly), the
  control plane executes the action and returns the result, not the token.
- For workspace-side credential flows (`GIT_ASKPASS`, `coder external-auth
  access-token`), the control plane returns credentials to the workspace
  agent after the delegation check. These credentials operate within the
  workspace's trust domain (see key decision 11).
- If the session was revoked after the check but before
  completion, the in-flight operation is allowed to finish (see key
  decision 12).

If a template author needs to provide raw tokens to automation code, they do so
explicitly through environment variables or commands — a deliberate
template-level decision outside the delegation model.

### 5. Auditability

Every delegated authority decision should be attributable.

At minimum, audit records should answer:

- which automation principal acted,
- which human created the run,
- which human approved workspace execution,
- which human approved credential delegation,
- which provider and resource were accessed,
- when the grant expired or was revoked.

## Interaction with existing features

### Service accounts

Service accounts already exist and should remain the base building block for
non-human automation identity.

This design builds on them rather than replacing them.

### Tasks

Tasks already demonstrate a useful pattern:

- a first-class run/resource,
- a workspace attached to that run,
- template-provided credentials for dedicated automation workspaces.

That pattern is still valid. It is just insufficient for borrowed human
authority inside human workspaces.

### MCP

This document does not redesign MCP configuration scoping. It does require MCP
calls that act with human-linked authority to resolve through the same
delegation engine as Git and external auth.

The grant model should be able to express at least:

- allowed MCP server(s)
- allowed tool(s)
- expiration and revocation

### Org scoping

Org-scoping all chats is a prerequisite for this design. Delegated sessions,
provider capability policies, and automation runs all require an unambiguous org
context. Deriving org context indirectly (e.g., from a workspace's org) is
fragile and creates edge cases when a chat is not yet attached to a workspace.

All chats must have a first-class `organization_id` before delegated authority
can be enforced consistently.

Cross-org delegation is forbidden (see key decision 13). All participants in
a delegated session — the automation principal, workspace, credential owner,
and the provider/template policies — must share a single org context. This
matches the existing strict org-scoping of workspace infrastructure (CRUD,
listing, agent connections, RBAC role grants).

## Migration sketch

### Phase 0: document and instrument current authority paths

- document current owner-based authority resolution
- add tracing and audit events around chat-side and workspace-side external auth
  resolution

### Phase 1: org-scope all chats

- ensure every chat has a first-class `organization_id`
- this is a prerequisite for all subsequent phases

### Phase 2: introduce explicit automation principal and workspace.automate

- new runs record an automation principal
- creator identity becomes metadata rather than the default ambient actor
- introduce `workspace.automate` permission for automation attachment
- existing `ActionSSH` checks remain for interactive SSH access only
- support binding workspace agents to automation principals in Terraform
  (`coder_agent` with explicit `user_id` referencing a bot account)
- automation agents resolve credentials from their bound principal, not
  `workspace.OwnerID`

### Phase 3: introduce delegated session records

- create pending and active delegated sessions
- add revocation and expiry
- thread run/workspace/provider context through resolution paths

### Phase 4: move chat-side Git authority to delegation

- stop resolving Git authority from `chat.OwnerID`
- require delegated-session resolution for chat-side provider access

### Phase 5: move workspace-side external auth to delegation

- stop resolving external auth from `workspace.OwnerID` for automation agents
- require delegated-session resolution for workspace-agent mediated access
- `workspaceAgentsExternalAuth` for automation agents checks for an active
  delegated session before returning credentials
- workspace-side git credential flows (`GIT_ASKPASS`) continue to operate
  within the workspace trust domain — delegation gates *whether* the
  automation may trigger git operations, not the credential delivery mechanism
- use `root_chat_id` as the session correlation key

### Phase 6: remove legacy raw token paths

- remove any remaining code paths that resolve human tokens for automation
  agents without a delegated session check
- confirm all automation external auth flows require delegation-layer
  authorization
- workspace-interior credential flows (git, SSH) remain unchanged — they
  operate within the workspace trust domain
- raw tokens for automation agents only available via explicit template-level
  injection (env var, command)

### Phase 7: remove ActionSSH fallback

- remove any remaining `ActionSSH` fallback paths for automation attachment
- confirm all automation flows use `workspace.automate` exclusively

## Open questions

### Agent `user_id` binding complexity

Key decision 10 proposes `user_id = data.coder_user.bot.id` on `coder_agent`,
but agents today have no `owner_id`/`user_id` column. Identity is always
derived from `workspace.OwnerID` at auth time. Adding user binding requires a
DB migration, protobuf schema change, Terraform provider change (separate
repo), and reworking the RBAC AllowIDList.

The RBAC AllowIDList problem needs design attention: today the agent scope is
restricted to the workspace owner's resources. If a bot-user-agent is scoped to
the bot user's resources, it cannot access the workspace (owned by a human).
Either the AllowIDList must explicitly permit cross-user workspace access, or an
alternative identity mechanism is needed that doesn't use the RBAC subject
impersonation pattern.

### `no_user_data` scope as interim mechanism

The `api_key_scope = "no_user_data"` setting already blocks
`workspaceAgentsExternalAuth` via an `ActionReadPersonal` RBAC check. This is
the closest existing mechanism to what the design proposes and proves the RBAC
plumbing works for blocking token access at the agent level.

The migration sketch should consider whether Phase 2 can leverage
`no_user_data` as an interim step — setting automation agents to this scope
before the full delegation layer is built — rather than requiring the complete
agent `user_id` binding from the start.

### Session boundary for non-chat automation

Key decision 8 ties delegation to `root_chat_id`, but the conceptual data model
lists `kind` values of `chat`, `task`, and `workflow_run`. Tasks today are a
completely separate system with no chat hierarchy — they have their own table
with `organization_id`, `template_version_id`, and `template_parameters` but no
`parent_chat_id` or `root_chat_id`.

If a task or workflow run needs delegated credentials, what defines its session
boundary? The design needs a more general concept like "root run" rather than
"root chat," or it must explicitly state that delegation is chat-only for the
initial design.

### Subagent depth and transitive delegation inheritance

Chat subagents are currently limited to max depth 1. The doc says subagents
inherit delegated sessions but doesn't address whether the depth limit should be
relaxed, or what happens if it is. If depth > 1 is allowed in the future,
transitive delegation inheritance across a chain of subagents could create
accountability gaps — the audit trail of "who approved what" becomes harder to
follow through multiple levels of indirection.

### Workspace execution grant lifecycle

The doc says workspace execution approval "may be relatively durable" but
doesn't define what manages its lifecycle. Is it a persistent DB record, a role
assignment, or an RBAC policy? Who can revoke it — only the workspace owner, or
also org admins? Does it survive workspace rebuilds? If a workspace is
transferred to a new owner, do existing execution grants persist? This contrasts
with the delegated session, which has a clear lifecycle with explicit states.

### Credential-owner notification and delivery

When the credential owner is a different person from the chat initiator, how do
they receive the allow/deny prompt? The blocking-no-timeout decision (key
decision 8) means the automation is suspended indefinitely until the credential
owner responds. The self-service case (same person) is straightforward — the
prompt appears in their own chat. The cross-person case needs a notification
channel, and the blocking-no-timeout decision makes this channel
reliability-critical.

### Tiered policy default posture

The resolution rules start with "allow all" and treat absent policy at any tier
as a no-op. This means a fresh deployment with no policies configured allows all
delegation by default. An org admin who hasn't configured policies gets
unrestricted delegation. This is the opposite of a secure-by-default posture.

The doc should decide whether this is intentional. A middle ground: absent
deployment policy = allow-all (backward compat), absent org policy = deny-all
(new orgs must explicitly opt in to delegation).

### Template author abuse via bot-agent binding

The threat model doesn't cover a template author injecting a `coder_agent`
bound to a bot account that has been pre-granted delegated sessions, effectively
creating a persistent backdoor for credential harvesting. The non-goals section
says template author token injection is out of scope, but the bot-agent binding
mechanism (key decision 10) creates a new template-author attack surface
distinct from raw token injection. A malicious template could bind an agent to a
shared bot account that accumulates delegated sessions across many workspaces.

### Split-brain window during migration phases 4–5

Phase 4 moves chat-side Git auth to delegation. Phase 5 moves workspace-side
external auth to delegation. Between these phases, the same automation could
resolve credentials through two different authority models depending on whether
the request came from the chat side or the workspace side. The doc's goal of
unifying chat-side and workspace-side resolution is temporarily violated. The
migration sketch should specify whether the two phases can be collapsed or
acknowledge the split-brain window and its implications.

### Transition behavior for pre-existing chats

When delegation enforcement begins (Phase 2), what happens to chats created
before that phase that are still active? They have `OwnerID` but no
`automation_principal_id`. Do they continue running under the old model, get
force-migrated to an automation principal, or get terminated? The early-access
status reduces the blast radius, but the doc should still specify the transition
behavior explicitly.

## Future work

### Short-lived scoped tokens for burst traffic

Every brokered external action round-trips through the control plane. The
existing human flow (`workspaceAgentsExternalAuth`) already has the same
structural cost: agent → control plane → DB read → lazy refresh (via
`oauth2.TokenSource.Token()`, which makes no remote call if the token is not
expired) → validate → return. The per-request overhead of brokering is
therefore modest.

For burst-traffic scenarios (rapid clone, fetch, push sequences), issuing a
short-lived, narrowly scoped token that the agent holds transiently could
reduce per-operation round-trips further. This optimization would benefit both
human agents and automation agents equally and is not blocking for the initial
design.

### Fine-grained provider capability language

The initial capability vocabulary (`provider_kind`, `access_level`,
`resource_scope`) is intentionally coarse. Provider-specific integrations may
need richer constraint expressions over time — branch restrictions, tool
allowlists, rate limits, or action-level scoping. The structured vocabulary
should be extensible without breaking the intersection semantics.

## Alternatives considered

### Run the automation as the human creator

Rejected.

This makes human-linked authority ambient and poorly bounded.

### Disallow automations in human workspaces by default

Rejected as a product direction.

Execution in human workspaces is a valid goal. The design problem is authority,
not the mere fact of execution.

### Use only service accounts and dedicated workspaces

Rejected as insufficient.

This pattern works well for fully non-human automation, but it does not solve
borrowed human authority in human workspaces.

### Let only the workspace owner approve everything

Rejected.

The workspace owner should not be able to delegate another human's linked
external identity.

### Let only the credential owner approve everything

Rejected.

A credential owner should not be able to inject their external identity into
another human's workspace without workspace-side consent.

## Success criteria

A successful design should make the following true:

- an automation can run in a human workspace without automatically inheriting
  the workspace owner's linked external identity
- an automation can use a human's external identity only through an explicit,
  auditable, revocable delegated session
- raw human tokens are never disclosed at the coderd-to-LLM-provider boundary;
  workspace-interior credential flows operate within the workspace trust domain
  and are gated by delegation policy
- automation agents are bound to their automation principal at the agent level,
  not the workspace-owner level
- chat-side and workspace-side provider access are governed by the same
  authority model
- service-account-backed automation workflows continue to work without human
  delegation when they do not need it
- operators can explain every privileged action in terms of principal, grant,
  resource, and expiration
