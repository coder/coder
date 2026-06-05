# User identity: per-developer attribution

Coder workspaces can host Claude Code self-hosted runners on behalf of
the **individual developer** who started the session, not just a
fleet-wide bot. The runner workspace becomes the developer's, their git
push credential is used, their commits are authored by them, and Coder's
audit log attributes runner activity to them.

<img src="../../images/guides/claude-code-self-hosted-runners/user-identity-flow.svg" alt="The orchestrator's spawn-runner hook maps the Anthropic account email to a Coder user, creates the workspace on their behalf, and passes lock-to-account so the runner is born locked to that user. The workspace owner is the developer, Coder external auth wires their git push token, and there is no first-session race." />

> [!NOTE]
> User identity has been **prototyped and proven end-to-end** using the
> on-demand runner orchestrator from byoc.14 and a `spawn-runner` hook
> that maps Anthropic account emails to Coder users. The workspace is
> created on behalf of the matching Coder user, so it shows up under
> their name in the Coder UI.
>
> The `--lock-to-account` flag is confirmed working in byoc.14. The
> hook passes the Anthropic account ID to the runner via
> `SELF_HOSTED_RUNNER_LOCK_TO_ACCOUNT`, so the runner only accepts
> sessions from that specific user from startup. No first-session race.

## How it works

The on-demand runner orchestrator polls Anthropic for pending spawn
requests. For each request, it invokes a `spawn-runner` hook that:

1. Reads `CLAUDE_RUNNER_ACCOUNT_EMAIL` from the orchestrator environment.
2. Looks up the matching Coder user by email via the Coder REST API.
3. Creates a workspace **on behalf of that Coder user**, not the service
   account.
4. Passes the single-use work order JWT as an ephemeral parameter.

The workspace is owned by the developer. Coder external auth resolves
to their linked tokens, audit log entries attribute to them, and git
credentials can use their identity instead of a bot PAT.

A reference implementation of the hook and template is at
[`coder/coder-anthropic-integration-poc`](https://github.com/coder/coder-anthropic-integration-poc/tree/on-demand-user-identity)
on the `on-demand-user-identity` branch.

## What user identity gives you

Compared to [System identity](./system-identity.md), user identity
restores the per-developer audit trail across the whole stack:

| Concern                  | System identity                                          | User identity                                                              |
|--------------------------|----------------------------------------------------------|----------------------------------------------------------------------------|
| Coder workspace owner    | A bot service account                                    | The Coder user who matches the Claude Code session creator                 |
| Git push credential      | A fleet-wide bot PAT delivered as a Terraform variable   | The user's own git push credential via Coder external auth                 |
| Git author on commits    | The bot                                                  | The developer's identity (via Coder external auth)                         |
| Coder audit log          | Attributes to the bot service account                    | Attributes to the user, with the service account shown as on-behalf-of creator |
| Routing                  | Anthropic picks any free runner; the runner locks on first session | The orchestrator's hook creates a workspace per session, owned by the matching user |
| Pool size / concurrency  | Fixed: at most `instances` concurrent Anthropic users    | Dynamic: one workspace per session, spawned on demand                      |
| Unknown user handling    | Not possible to detect: the workspace runs as the bot    | Hook rejects with exit 1 so the session backs off; the user can be onboarded before the next retry |

The single biggest practical win is that **per-developer git push,
external-auth refresh, and Coder audit log all just work the way the
rest of Coder works**. You stop having to special-case Claude Code
sessions in your audit and policy story.

> [!TIP]
> If you stay on a bot identity for commits and pushes, the routing
> layer is still useful on its own. Pointing it at a single bot Coder
> user instead of the matching human gives you:
>
> - **Dynamic concurrency.** A workspace per concurrent Anthropic
>   user, spawned on demand.
> - **Pre-flight validation.** Sessions for unknown Anthropic users
>   reject up front instead of silently running as the bot.
> - **Per-runner audit context.** The service account shows up as the
>   on-behalf-of creator in the Coder audit log, so you can still tie
>   a workspace build back to a specific Anthropic session, even if
>   the workspace owner is the bot.
>
> You opt into per-human attribution separately by pointing the hook
> at the matching Coder user rather than the bot. The two decisions
> (dynamic spawn vs. fixed pool, and human owner vs. bot owner) are
> independent.

## Role of prebuilds

Prebuilds are **optional** in the on-demand model. They pre-warm
workspace images so the Docker pull and build happen ahead of time,
but the orchestrator creates each workspace on demand regardless.
Without prebuilds, the first session for a given template version pays
the image-build cost. With prebuilds, that cost is hidden.

Prebuilds are not the runner pool. The orchestrator is.

## `--lock-to-account`

The `--lock-to-account` flag is **confirmed working** in byoc.14
(Anthropic confirmed the "pending" label in the docs is stale and
being removed). Pass an email or `user_...` account ID:

```text
--lock-to-account <id>    Lock runner to a single account at registration
                          (webhook-driven on-demand spawn). Only that
                          account's sessions are assigned.
                          [env: SELF_HOSTED_RUNNER_LOCK_TO_ACCOUNT]
```

The on-demand template sets `SELF_HOSTED_RUNNER_LOCK_TO_ACCOUNT` from
the `lock_to_account` ephemeral parameter, which the `spawn-runner`
hook populates with `CLAUDE_RUNNER_ACCOUNT_ID`. The runner rejects
sessions from any other account at registration time, eliminating the
first-session lock race entirely.

## Where this depends on Coder

The `spawn-runner` hook creates workspaces via the Coder REST API
(`POST /api/v2/organizations/{org}/members/{user}/workspaces`) using a
service account token with the owner role. This works today with no
Coder product changes.

The hook uses the REST API directly because `coder create` does not
wire the `--ephemeral-parameter` flag yet. Once that ships, the hook
simplifies to a single `coder create` call.

A longer-term option is to build the orchestrator into `coderd` so
there is no separate process to run. The hook collapses from "a script
to maintain" to "a config block in the template."

## Security considerations

- `CLAUDE_RUNNER_ACCOUNT_EMAIL` is the privilege boundary. The hook
  trusts it to determine which Coder user owns the workspace. In
  production, verify the email against the work order JWT's signed
  claims (`ccr:spawn_account_email`) rather than the unsigned env var.
  The JWT can be verified against Anthropic's JWKS at
  `<api-base-url>/v1/code/.well-known/jwks.json`.
- The service account token used by the hook can create workspaces as
  any user. Treat it like the pool secret: kept out of runner
  workspaces, rotated on schedule, vaulted.
- Coder users must exist with emails matching their Anthropic accounts
  before they can use the pool. Typically this means SSO/OIDC with the
  same identity provider on both sides.

## Where to next

- [On-demand runners](./on-demand.md): the on-demand orchestrator
  recipe (system identity variant).
- [System identity](./system-identity.md): the fixed-fleet prebuild
  recipe, useful when you want zero cold-start latency and can size
  the pool statically.
- [Implementation notes](./plan.md): the staged plan and open
  questions tracked alongside this delivery.
