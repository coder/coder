# User identity: per-developer attribution

User identity will let Coder workspaces host Claude Code self-hosted
runners on behalf of the **individual developer** who started the
session, not just a fleet-wide bot. The runner workspace becomes the
developer's, their git push credential is used, their commits are
authored by them, and Coder's audit log attributes runner activity to
them.

<img src="../../images/guides/claude-code-self-hosted-runners/user-identity-flow.svg" alt="When an Anthropic session is queued, a routing component maps the Anthropic user to their Coder user and claims a warm prebuild on their behalf. The workspace owner flips to the human, Coder external auth wires their git push token, and the runner is born locked to their Anthropic account. User identity is planned and not yet available." />

> [!IMPORTANT]
> User identity is not available yet.
>
> - The Anthropic runner protocol pieces it depends on (per-user
>   routing webhook, pre-locking a runner before its first session)
>   have not shipped. Anthropic flags both as pending in their EAP
>   guide; the wire format is not finalized and there is no
>   destination URL to configure in `claude.ai` or in the runner
>   today.
> - The Coder-side routing component that would receive that webhook,
>   map the Anthropic user to a Coder user, and claim a prebuild on
>   their behalf does not exist yet. There is no `coderd` endpoint, no
>   middleware, no Terraform block you can wire up today.
>
> Until both ship, the model documented for production use is
> [System identity](./system-identity.md). This page describes what
> the user-identity model will look like once both pieces land.

## What user identity gives you

Compared to [System identity](./system-identity.md), user identity
restores the per-developer audit trail across the whole stack:

| Concern                  | System identity                                          | User identity                                                              |
|--------------------------|----------------------------------------------------------|----------------------------------------------------------------------------|
| Coder workspace owner    | A bot service account                                    | The Coder user who matches the Claude Code session creator                 |
| Git push credential      | A fleet-wide bot PAT delivered as a Terraform variable   | The user's own git push credential via Coder external auth                 |
| Git author on commits    | The bot                                                  | The bot for the initial clone; the user's identity (via Coder external auth) for commits and pushes inside the session |
| Coder audit log          | Attributes to the bot service account                    | Attributes to the user, with the routing service account shown as on-behalf-of creator |
| Routing                  | Anthropic picks any free runner; the runner locks on first session | The runner is pre-bound to the matching user before sessions arrive        |
| Pool size / concurrency  | Fixed: at most `instances` concurrent Anthropic users, because every workspace is the same service account | Dynamic: one workspace per Anthropic user, spawned on demand; prebuilds are just a warm cache that hides cold-start time |
| Failure if the user is missing in Coder | Not possible to detect: the workspace runs as the bot regardless | Pre-flight rejects with a friendly error so onboarding can complete first   |

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
>   user, spawned on demand. Prebuilds become a warm cache for
>   cold-start latency, not the inventory itself.
> - **Pre-flight validation.** Sessions for unknown Anthropic users
>   reject up front instead of silently running as the bot.
> - **Per-runner audit context.** The routing service account shows up
>   as the on-behalf-of creator in the Coder audit log, so you can
>   still tie a workspace build back to a specific Anthropic session,
>   even if the workspace owner is the bot.
>
> You opt into per-human attribution separately by pointing the router
> at the matching Coder user rather than the bot. The two decisions
> (dynamic spawn vs. fixed pool, and human owner vs. bot owner) are
> independent.

## What stays the same

User identity is built on top of the System identity recipe. You keep:

- The same Coder template and image.
- The same prebuilt-workspace pool as inventory.
- The same self-eviction loop and metadata blocks.
- The same Anthropic pool secret and pool configuration.

So the System identity rollout you ship today is the foundation. When
user identity ships, you turn it on by adding a thin routing component
between Anthropic and Coder and switching the template's git-credential
plumbing from the bot PAT to Coder's external auth feature.

## Where this depends on Anthropic

Two pieces of the Anthropic runner protocol have not shipped:

- A way for Anthropic to **tell your infrastructure** that a specific
  user has a session waiting, so Coder can spawn a workspace on their
  behalf instead of waiting for a runner to be claimed. Webhook
  support is on Anthropic's roadmap (the EAP guide describes a
  `runner-needed` webhook plus a CLI poll fallback), but the wire
  format and auth contract are not finalized and there is no
  destination URL to configure today.
- A way to **pre-bind a runner to a specific user** at startup, so
  there is no race where a session lands on the wrong runner. The
  `--lock-immediate` flag exists in the runner today but is marked
  pending and intended for webhook-driven spawn.

Anthropic invites operator input on both contracts. Once they ship,
this page will be replaced with a copy-and-go recipe.

## Where this depends on Coder

Once Anthropic publishes the webhook contract, two implementation
paths get you to per-user workspaces:

- **A small middleware service in front of Coder.** Receives the
  Anthropic webhook, maps the session creator's identity to a Coder
  user, calls `POST /api/v2/users/{user}/workspaces` on the user's
  behalf with the runner pre-locked to that user, and reports back to
  Anthropic. A few hundred lines of Go or Node, authenticated to
  Coder as a dedicated service account with a scoped admin-like
  token. This is what we expect most early adopters to deploy because
  it can ship the day Anthropic's webhook ships, on top of Coder
  primitives that already exist.
- **A first-class integration inside `coderd`.** Coder's server
  natively consumes the Anthropic webhook, with the user-mapping
  rules, the on-behalf-of spawn, and the audit-log wiring built in.
  The webhook receiver collapses from "a service you write" to "a
  config block in the template." This is the better long-term shape
  for any team that doesn't want to operate a separate service, and
  it is the natural follow-on once we have learned from middleware
  deployments what the right defaults are.

The two paths are not mutually exclusive: middleware lets us iterate
on the integration shape before committing to anything in the
product, and the in-tree integration absorbs whatever middleware
teaches us once the patterns are clear.

## What to do today

If you need per-user attribution **today**, the closest thing System
identity offers is the commit trailer that Claude Code automatically
appends to every commit:

```text
Co-authored-by: Claude <noreply@anthropic.com>
Session: https://claude.ai/code/sessions/cse_...
```

That trailer points at the Anthropic session, which is attributed to
the human user. Your tooling (CODEOWNERS bots, dashboards, audit
reports) can read it to recover the per-human signal.

## Where to next

- [System identity](./system-identity.md): the recipe that ships today.
- [Implementation notes](./plan.md): the staged plan and the open
  questions for both Anthropic and Coder that gate user identity.
