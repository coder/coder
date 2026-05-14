# AI Governance integration (notes)

This page is an addendum to the
[Cursor self-hosted workers on Coder](./index.md) recipe. It captures
how the worker could plug into Coder's [AI Gateway](../ai-gateway/index.md)
(formerly "AI Bridge") and the
[AI Governance Add-on](../ai-governance.md), what works today, what
doesn't, and what would have to ship to close the gap.

> [!NOTE]
> This page is intentionally not listed in the docs manifest. It's a
> design note shared by link, not a published recipe. The integration
> it describes is not usable today against the
> [System identity](./system-identity.md) recipe; the structural
> reasons are in
> [The Cursor architecture mismatch](#the-cursor-architecture-mismatch)
> and the
> [Per-human attribution gap](#per-human-attribution-gap) sections
> below.

## TL;DR

AI Gateway is **not applicable** to Cursor self-hosted workers today,
and the blockers are different from the Claude Code recipe.

The Claude blocker is identity: the wrapper script that already exists
in the recipe would route model traffic through AI Gateway in a
one-line change, but AI Gateway rejects tokens owned by Coder system
users and the workspace owner is `prebuilds@system`. Cursor has the
same identity issue, but a more fundamental one stacked in front:
**the worker process does not make Anthropic, OpenAI, or any other
model-provider API calls.** The agent loop runs in Cursor's cloud and
sends tool calls down to the worker over a long-lived outbound HTTPS
connection to `api2.cursor.sh`. AI Gateway would have no model traffic
to intercept even with the right token.

The honest answer is: today, AI Governance Add-on does not cover the
LLM calls a Cursor self-hosted worker session makes. The coverage
question moves to Cursor's session log and Coder's audit log, not AI
Gateway. The desktop Cursor client is separately not supported by AI
Gateway today (the OpenAI base-URL override is broken upstream); see
[AI Gateway client compatibility](../ai-gateway/clients/index.md) for
the canonical list.

## The Cursor architecture mismatch

AI Gateway intercepts traffic between a client (Claude Code, Codex,
Copilot, Cursor desktop, etc.) and a model provider
(`api.anthropic.com`, `api.openai.com`, the GitHub Copilot endpoint).
For a client to be intercepted, it has to make the model-provider call
itself, and you have to be able to redirect that call. Cursor's
self-hosted worker satisfies neither condition.

Per [Cursor's Self-Hosted Pool docs][cursor-self-hosted-pool], the
worker opens **one long-lived outbound HTTPS connection** to Cursor's
cloud and serves tool calls over that connection. The agent loop
(inference and planning) runs in Cursor's cloud; the worker only
executes the tool calls locally in your infrastructure.

[cursor-self-hosted-pool]: https://cursor.com/docs/cloud-agent/self-hosted-pool

The outbound hosts the worker actually opens are documented as:

- `api2.cursor.sh` and `api2direct.cursor.sh` for the agent session.
- `cloud-agent-artifacts.s3.us-east-1.amazonaws.com` for
  [artifact](https://cursor.com/docs/cloud-agent/self-hosted-pool.md#artifacts)
  uploads.

Notably absent from that list: `api.anthropic.com`, `api.openai.com`,
`generativelanguage.googleapis.com`, or any other model provider.
There is no model call leaving the worker for AI Gateway to intercept.

| Source                                   | Destination                                                               | Through AI Gateway? |
|------------------------------------------|---------------------------------------------------------------------------|---------------------|
| Worker process                           | `api2.cursor.sh` (session control plane)                                  | No                  |
| Worker process                           | `cloud-agent-artifacts.s3.us-east-1.amazonaws.com`                        | No                  |
| Worker (during tool calls)               | Whatever a session reads: internal Git, package registries, internal APIs | No                  |
| Cursor cloud (where the agent loop runs) | Anthropic / OpenAI / etc.                                                 | Not reachable       |

The model-provider call lives one network boundary further out, inside
Cursor's cloud. You cannot put Coder's AI Gateway between Cursor's
backend and Anthropic from inside a Coder workspace.

The optional [`aibridgeproxyd`](../ai-gateway/setup.md) MITM proxy is
also a dead end here. Its supported route set
(`coderd/aibridge`-side) rewrites known Anthropic / OpenAI / GitHub
Copilot hosts; it does not rewrite traffic to `api2.cursor.sh`, and
even if it did, the request body is Cursor's session protocol, not an
Anthropic or OpenAI request, so AI Gateway has no schema it knows how
to interpret.

This is structurally different from the
[Claude Code self-hosted runner recipe][claude-recipe], where the
session's child `claude` process makes Anthropic API calls directly
from inside the workspace. For Claude, AI Gateway's
[Claude Code client preset](../ai-gateway/clients/claude-code.md) is a
one-liner away from working; for Cursor self-hosted workers, the
client preset has nothing to point at.

[claude-recipe]: https://github.com/coder/coder/blob/main/docs/ai-coder/claude-code-self-hosted-runners/index.md

## Per-human attribution gap

Even setting the architecture mismatch aside, the
[System identity](./system-identity.md) recipe hits the same
identity-side blocker the Claude recipe documents.

AI Gateway authenticates each request by looking up the API token in
the `Authorization` header and records the token owner as the
**initiator** of the interception. The lookup lives in
`enterprise/aibridgedserver/aibridgedserver.go`. Three properties of
that lookup matter here:

1. **System users are rejected outright.** Tokens owned by a Coder
   user with `is_system = true` (introduced in migration
   `000308_system_user.up.sql`) hit an `ErrSystemUser` error path and
   the request is denied. `prebuilds@system`, which owns every System
   identity workspace until self-eviction, is a system user.
2. **There is no on-behalf-of header.** The initiator is always the
   token owner. There is no `X-Coder-Act-As-User`,
   `X-Coder-AI-Governance-Delegated-User`, or equivalent that lets a
   system-owned token attribute interceptions to a human.
3. **The `Authorization` token carries the only identity AI Gateway
   sees.** Anything inside the request body (model name, message
   contents) is recorded against the token owner, not parsed for
   identity hints.

If a future Cursor product change moved any model traffic into the
worker (a session-level wrapper hook that talked to Anthropic directly,
say), the System identity recipe's workspace would still be owned by
`prebuilds@system`, so the only token it has handy is a system-user
token AI Gateway will not honor. The token problem stacks on top of
the architecture problem.

Three paths could close the identity half of this gap; none of them
exist today:

1. **Wait for [User identity](./user-identity.md).** When the routing
   layer claims a prebuild on behalf of the human, the workspace
   owner becomes the human and the standard
   `data.coder_workspace_owner.me.session_token` is the human's
   token.
2. **AI Gateway accepts a delegate header from system users.** A new
   header (e.g. `X-Coder-AI-Governance-Delegated-User: <user-id>`)
   honored only when the bearer token is owned by a system user and
   has an appropriate scope. The interception's `initiator_id` would
   become the delegated user. Small, contained product change. No
   draft today.
3. **Cursor session-id subject lookup.** A coderd endpoint that
   resolves `activeBcId` or a Cursor session JWT subject to a Coder
   user, then issues a short-lived token attributable to that user.
   Larger surface; not drafted either, and dependent on Cursor
   exposing a verifiable user-identity claim per session, which today
   it doesn't.

None of these matter until the architecture-side blocker resolves,
because there is no model traffic to apply them to.

## What about Cursor desktop?

A common follow-up question is whether AI Gateway can at least cover
the developer's **desktop Cursor client** that triggers the Background
Agent session in the first place.

Today, no. [AI Gateway's client compatibility table](../ai-gateway/clients/index.md)
lists Cursor as not supported across all three providers (Anthropic,
OpenAI, Bedrock) and links to the upstream Cursor issue: the OpenAI
base URL override doesn't direct requests to the configured endpoint.
Until Cursor fixes that, AI Gateway cannot intercept either the
desktop client's model traffic *or* the in-cloud Background Agent's
model traffic, and the Coder + Cursor combination has no AI Gateway
coverage in either direction.

If desktop Cursor support lands first (via Cursor fixing the base URL
override), AI Gateway will cover developer-initiated chat from the
client. It will not cover the Background Agent sessions those clients
spawn, because those run in Cursor's cloud, not in the client.

## Audit log: where the signal lives instead

AI Gateway records to its own dedicated tables (`aibridge_interceptions`,
`aibridge_sessions`, `aibridge_token_usages`, etc., introduced across
migrations `000370` onward and surfaced via `/api/v2/aibridge/*` and
the Sessions UI). It does **not** write to Coder's regular audit log
(`enterprise/audit/table.go` has no aibridge entries).

For Cursor self-hosted workers, both of those surfaces are empty:

| Surface                                  | What's there for Cursor workers                                                                                                         |
|------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| AI Gateway (`/api/v2/aibridge/*`)        | Nothing. No model calls flow through it from a Cursor worker session.                                                                   |
| Coder audit log                          | Workspace builds attributed to `prebuilds@system` (System identity) or the human (User identity, once it ships). No per-session signal. |
| Cursor session log (Cursor-side)         | Full session attribution, model calls, tool calls, artifacts. Keyed by `activeBcId`.                                                    |
| Worker `cursor-agent.log` (in workspace) | Local trace of claim, release, tool calls. Not retained centrally.                                                                      |

A reader auditing "who used Cursor against repo X today" has to look in
Cursor's session log. Coder's audit log can confirm which workspace
backed the session (join key is `activeBcId` in the fleet API, which
the recipe surfaces as `coder_agent.metadata`), but it cannot answer
"what did the model say." Cursor's session log is the source of truth
for the LLM half.

This is a real coverage gap relative to a deployment that ships
Anthropic + Claude Code + AI Gateway today. A team that wants
"all AI activity in one audit pane of glass" cannot get there with
Cursor private workers regardless of which identity model they pick.

## AI Governance add-on entitlements

The add-on gates two features (`codersdk/deployment.go`):

- `aibridge` (boolean) controls whether the `/api/v2/aibridge/*`
  endpoints respond at all. Enforced via
  `RequireFeatureMW(codersdk.FeatureAIBridge)` on the routes.
- `ai_governance_user_limit` (integer) is the seat counter for users
  who actively consume AI Gateway interceptions
  (`enterprise/coderd/license/license.go`).

Neither entitlement matters for the Cursor self-hosted worker recipe
today. There is no flow that produces aibridge records, so no flow
that consumes the seat counter. If a future Cursor or Coder product
change brings worker session model calls under AI Gateway, both
entitlements would start to apply.

The transitional soft-warning Premium fallback that exists in the
license code today (a `TODO` to enforce AI Bridge as an explicit
add-on) does not change the conclusion: the recipe doesn't generate
billable AI Gateway events today, on either Premium or the add-on.

## "AI Gateway" vs. "AI Bridge" vs. `aibridge`

These are all the same component. The user-facing rename is documented
in [the AI Gateway overview](../ai-gateway/index.md):

> AI Gateway was previously known as "AI Bridge". Some configuration
> options, environment variables, and API paths still use the old
> name and will be updated in a future release.

The Go package is `aibridge`, the in-process service is `aibridged`,
the DRPC server is `aibridgedserver`, the optional MITM proxy is
`aibridgeproxyd`. The HTTP route is `/api/v2/aibridge/*`. None of this
is changing in the near term.

## What to do today, what to wait for

For a team running [System identity](./system-identity.md) today and
asking about AI Governance coverage:

- **Don't try to point worker traffic at AI Gateway.** There is no env
  var to set. The worker doesn't make model calls. The desktop client
  has its own upstream bug. Both ends of the conversation are out of
  AI Gateway's reach.
- **Coder's audit log + Cursor's session log is the full attribution
  story.** The join key is `activeBcId`, which the recipe already
  surfaces as workspace metadata. Tooling that wants a single
  attribution feed should ingest both surfaces and join them on
  session id.
- **AI Gateway coverage for Cursor worker model calls waits on
  Cursor.** Until Cursor exposes a way for the worker to mediate its
  own model calls (a per-session wrapper hook with provider routing
  control, similar to Claude Code's `--exec-path`), there is nothing
  to point AI Gateway at from a Coder workspace. The
  [implementation notes](./plan.md) track this as an open question
  for Cursor.
- **AI Gateway coverage for desktop Cursor waits on Cursor's
  upstream OpenAI base URL fix.** Track [the Cursor forum issue][cursor-base-url-issue].
  Once it lands, the standard "Configuring External and Desktop
  Clients" path in the [AI Gateway client docs](../ai-gateway/clients/index.md)
  applies to Cursor.

[cursor-base-url-issue]: https://forum.cursor.com/t/requests-are-sent-to-incorrect-endpoint-when-using-base-url-override/144894

This page will be removed or rewritten as a real recipe when either
of those Cursor-side changes lands.

## References

- [Cursor self-hosted workers (overview)](./index.md)
- [System identity](./system-identity.md)
- [User identity](./user-identity.md)
- [Implementation notes](./plan.md)
- [AI Gateway overview](../ai-gateway/index.md)
- [AI Gateway client compatibility](../ai-gateway/clients/index.md)
- [AI Gateway audit](../ai-gateway/audit.md)
- [AI Governance](../ai-governance.md)
- [Cursor Self-Hosted Pool docs](https://cursor.com/docs/cloud-agent/self-hosted-pool)
- [Cursor Security and network docs](https://cursor.com/docs/cloud-agent/security-network)
