# AI Governance integration (notes)

This page is an addendum to the
[Claude Code self-hosted runners on Coder](./index.md) recipe. It captures
how the runner could plug into Coder's [AI Gateway](../ai-gateway/index.md)
(formerly "AI Bridge") and the
[AI Governance Add-on](../ai-governance.md), what works today, what
doesn't, and what would have to ship to close the gap.

> [!NOTE]
> This page is intentionally not listed in the docs manifest. It's a
> design note shared by link, not a published recipe. The integration
> it describes is not usable today against the
> [System identity](./system-identity.md) recipe; the structural reason
> is in the [Per-human attribution gap](#per-human-attribution-gap)
> section below.

## TL;DR

Routing the per-session child `claude` process through AI Gateway is a
small wrapper-script change that "just works" against the
[User identity](./user-identity.md) recipe (when it ships), and is
structurally broken against [System identity](./system-identity.md).
The blocker is that AI Gateway rejects API tokens owned by Coder
system users; the System identity runner workspace is owned by
`prebuilds@system`, so the only token it has access to is a
system-user token AI Gateway will not honor. Either user identity has
to ship, or AI Gateway has to grow a delegated-attribution mechanism
that doesn't exist today.

The wrapper-script change itself is well understood and exactly
mirrors the existing
[`coder/claude-code` registry module](https://github.com/coder/registry/tree/main/registry/coder/modules/claude-code).
AI Gateway records to its own dedicated tables
(`aibridge_interceptions`, `aibridge_sessions`, etc.), distinct from
Coder's regular audit log; the regular audit log knows nothing about
model calls.

## How AI Gateway routing would work

The per-session wrapper at `$HOME/.claude/wrapper.sh` (written by the
[`coder-labs/claude-self-hosted-runner`](https://github.com/coder/registry/tree/claude-self-hosted-runner/registry/coder-labs/modules/claude-self-hosted-runner)
module) is the natural injection point. The wrapper runs once per
session, immediately before the child `claude` process starts, so any
env vars exported from it apply to the model traffic but not to the
runner's own polling traffic against `api.anthropic.com`.

```bash
#!/bin/bash
# Route this session's child claude through Coder AI Gateway.
export ANTHROPIC_BASE_URL="${CODER_ACCESS_URL}/api/v2/aibridge/anthropic"
export ANTHROPIC_AUTH_TOKEN="${CODER_AIBRIDGE_TOKEN}"
exec /opt/claude/claude "$@" --permission-mode bypassPermissions
```

The two env vars are the AI Gateway client-preset contract documented
in [Claude Code as a client](../ai-gateway/clients/claude-code.md);
the BYOK variant uses
`ANTHROPIC_CUSTOM_HEADERS="X-Coder-AI-Governance-Token: ..."` plus a
provider key on `ANTHROPIC_API_KEY` instead, and the
`X-Coder-AI-Governance-Token` header constant is defined in
`coderd/aibridge/aibridge.go`.

The `coder/claude-code` registry module already wires both values from
standard Coder Terraform data sources
(`data.coder_workspace.me.access_url` and
`data.coder_workspace_owner.me.session_token`), so the integration
shape is unchanged from "developer SSH's in and runs `claude` locally."

## What runner traffic AI Gateway sees vs. doesn't

The self-hosted runner has two distinct outbound paths:

| Source                  | Destination                                  | Through AI Gateway? |
|-------------------------|----------------------------------------------|---------------------|
| Runner process          | `api.anthropic.com` (pool register, polling) | No                  |
| Child `claude` (session)| Anthropic model API (`/v1/messages`)         | Yes, via wrapper    |

The runner uses the pool secret over the public Anthropic endpoint
for pool registration and session polling. Pool polling is not a
model call, so AI Gateway has nothing to intercept and no policy to
apply. Only the per-session child `claude` model calls are eligible.

The optional [`aibridgeproxyd`](../ai-gateway/setup.md) MITM proxy
rewrites traffic to known Anthropic / OpenAI / GitHub Copilot hosts
into AI Gateway calls; its supported route set
(`coderd/aibridge`-side) does not include the pool registration
endpoints, so even with `HTTP_PROXY` set the runner's own traffic
passes through unmodified.

## Per-human attribution gap

This is the central reason AI Gateway is not usable with
[System identity](./system-identity.md) today.

AI Gateway authenticates each request by looking up the API token in
the `Authorization` header and recording the token owner as the
**initiator** of the interception. The lookup lives in
`enterprise/aibridgedserver/aibridgedserver.go`. Three properties of
that lookup matter here:

1. **System users are rejected outright.** Tokens owned by a Coder
   user with `is_system = true` (introduced in migration
   `000308_system_user.up.sql`) hit an `ErrSystemUser` error path and
   the request is denied. `prebuilds@system`, which owns every
   System identity workspace until self-eviction, is a system user.
2. **There is no on-behalf-of header.** The initiator is always the
   token owner. There is no `X-Coder-Act-As-User`,
   `X-Coder-AI-Governance-Delegated-User`, or equivalent that lets a
   system-owned token attribute interceptions to a human.
3. **`ANTHROPIC_AUTH_TOKEN` carries the only identity AI Gateway
   sees.** Anything inside the request body (model name, message
   contents) is recorded against the token owner, not parsed for
   identity hints.

Concrete consequences for System identity:

- Setting `ANTHROPIC_AUTH_TOKEN = data.coder_workspace_owner.me.session_token`
  in the runner template means the workspace sends
  `prebuilds@system`'s token to AI Gateway, which is rejected with
  HTTP 403 on every model call.
- The recipe surfaces nothing else AI Gateway can use as an
  identity. The per-workspace scoped self-evict token is scoped to
  `workspace:delete` and won't authenticate against AI Gateway at all.
- The Anthropic session JWT (`CLAUDE_CODE_SESSION_ACCESS_TOKEN`)
  carries the human's identity (`act.sub` / `act.email`), but Coder
  has no path to convert that into a usable AI Gateway token from
  inside a workspace without granting the runner admin-level Coder
  API access. That would expand the System identity blast radius
  past where the rest of the recipe is comfortable.

Three paths could close this gap; none of them exist today:

1. **Wait for [User identity](./user-identity.md).** When the routing
   layer claims a prebuild on behalf of the human, the workspace
   owner becomes the human and
   `data.coder_workspace_owner.me.session_token` is the human's
   token. AI Gateway then "just works" with no further change. This
   is the cleanest answer and the one we'd recommend a customer
   wait for.
2. **AI Gateway accepts a delegate header from system users.** A new
   header (e.g. `X-Coder-AI-Governance-Delegated-User: <user-id>`)
   honored only when the bearer token is owned by a system user and
   has an appropriate scope. The interception's `initiator_id` would
   become the delegated user. This is a small, contained product
   change (a handful of lines plus a scope) but it is real product
   work; no draft today.
3. **Anthropic JWT subject lookup.** A coderd endpoint that resolves
   `act.sub` / `act.email` from the Anthropic JWT to a Coder user,
   then issues a short-lived token attributable to that user. Larger
   surface; not drafted either.

If you need AI Gateway coverage of Claude Code self-hosted runner
traffic on the **System identity** recipe today, the honest answer
is that it isn't available. Stage C in the
[implementation notes](./plan.md) needs to be re-scoped accordingly.

## Audit log: two parallel surfaces

AI Gateway does **not** write to Coder's regular audit log
(`enterprise/audit/table.go` has no aibridge entries). Instead it
writes to a dedicated set of tables introduced across migrations
`000374`, `000385`, and `000428`:

- `aibridge_interceptions` — one row per request through AI Gateway.
- `aibridge_token_usages` — token counts per interception.
- `aibridge_user_prompts` — user-side prompt content.
- `aibridge_tool_usages` — tool calls inside the session.
- `aibridge_model_thoughts` — thinking blocks.
- `aibridge_sessions` — session-level grouping.

These are exposed via `/api/v2/aibridge/sessions` and
`/api/v2/aibridge/interceptions`, surfaced in the `Sessions` UI
under AI Gateway, and optionally emitted as structured JSON log
lines (`docs/ai-coder/ai-gateway/audit.md`,
`docs/ai-coder/ai-gateway/setup.md`).

A reader auditing "who did what with AI in this workspace" needs to
look in two places: AI Gateway records for the model calls, and the
Coder audit log for the workspace builds and identity changes. The
join key is the `initiator_id` on AI Gateway records, which matches
a Coder user UUID.

## AI Governance add-on entitlements

The add-on gates two features (`codersdk/deployment.go`):

- `aibridge` — boolean, controls whether the `/api/v2/aibridge/*`
  endpoints respond at all. Enforced via
  `RequireFeatureMW(codersdk.FeatureAIBridge)` on the routes.
- `ai_governance_user_limit` — seat counter for users who actively
  consume AI Gateway interceptions
  (`enterprise/coderd/license/license.go`).

There's a transitional soft warning in the license code for
deployments with AI Bridge enabled via Premium but no explicit
add-on license; a `TODO` note says the soft warning goes away once
AI Bridge is enforced as an add-on. Read: assume any future
self-hosted-runner deployment that wants AI Gateway coverage will
need the AI Governance Add-on license.

## "AI Gateway" vs. "AI Bridge" vs. `aibridge`

These are all the same component. The user-facing rename is
documented in [the AI Gateway overview](../ai-gateway/index.md):

> AI Gateway was previously known as "AI Bridge". Some configuration
> options, environment variables, and API paths still use the old
> name and will be updated in a future release.

The Go package is `aibridge`, the in-process service is
`aibridged`, the DRPC server is `aibridgedserver`, the optional
MITM proxy is `aibridgeproxyd`. The HTTP route is
`/api/v2/aibridge/*`. None of this is changing in the near term.

## What to do today, what to wait for

For a team running [System identity](./system-identity.md) today and
asking about AI Governance coverage:

- **Don't add `ANTHROPIC_BASE_URL` to the wrapper today.** It produces
  silent HTTP 403s on every model call, which is a worse experience
  than not offering the toggle at all.
- **The Coder audit log is the closest signal available.** It
  attributes builds to the bot. The per-human breadcrumb is the
  Anthropic session URL trailer that Claude Code already appends to
  every commit; tooling can read it back to recover the human.
- **AI Gateway coverage waits for either User identity or the
  delegate-header product change.** When either lands, the wrapper
  one-liner above plus the standard
  [`coder/claude-code` env-var pattern](https://registry.coder.com/modules/coder/claude-code)
  brings the runner's children under AI Gateway with no further
  change to the recipe.

This page will be removed or rewritten as a real recipe once one of
those paths ships.

## References

- [Claude Code self-hosted runners (system identity)](./system-identity.md)
- [User identity](./user-identity.md)
- [Implementation notes](./plan.md)
- [AI Gateway overview](../ai-gateway/index.md)
- [Claude Code as an AI Gateway client](../ai-gateway/clients/claude-code.md)
- [AI Gateway audit](../ai-gateway/audit.md)
- [AI Governance](../ai-governance.md)
