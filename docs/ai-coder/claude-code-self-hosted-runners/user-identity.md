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
> - The Anthropic runner protocol piece it depends on (pre-locking a
>   runner to a specific user before its first session) has not shipped.
>   The `--lock-to-account` flag is still marked "pending" in the
>   byoc.14 guide.
> - The on-demand runner orchestrator (shipped in byoc.14) now provides
>   the user's account ID and email to the `spawn-runner` hook via
>   `CLAUDE_RUNNER_ACCOUNT_ID` and `CLAUDE_RUNNER_ACCOUNT_EMAIL`
>   environment variables. This is the integration point for mapping
>   an Anthropic user to a Coder user.
> - The Coder-side routing component that would receive that spawn hint,
>   map the Anthropic user to a Coder user, and claim a prebuild on
>   their behalf does not exist yet.
>
> Until both the Coder routing and `--lock-to-account` ship, the model
> to use is [System identity](./system-identity.md). This page describes
> what the user-identity model will look like once both pieces land.

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

One piece of the Anthropic runner protocol has not shipped:

- A way to **pre-bind a runner to a specific user** at startup, so
  there is no race where a session lands on the wrong runner. The
  `--lock-to-account` flag exists in the runner today but is marked
  pending.

The scaling signal that tells your infrastructure a specific user has
a session waiting **has shipped** in byoc.14: the on-demand runner
orchestrator polls Anthropic for pending spawn requests and invokes a
`spawn-runner` hook per session. The hook environment includes
`CLAUDE_RUNNER_ACCOUNT_ID` and `CLAUDE_RUNNER_ACCOUNT_EMAIL`, which
is the data the Coder routing component needs to map an Anthropic
user to a Coder user.

Once `--lock-to-account` graduates from pending, this page will be
replaced with a copy-and-go recipe.

## Where this depends on Coder

With the orchestrator's `spawn-runner` hook providing user identity,
**one of two** architectures gets you to per-user workspaces:

- **`spawn-runner` hook script.** A shell script in the orchestrator's
  hooks directory reads `CLAUDE_RUNNER_ACCOUNT_EMAIL`, looks up the
  matching Coder user via the API, and calls `coder create` on their
  behalf with the work order passed as a parameter. Fastest to ship:
  works today with the orchestrator from byoc.14, on top of Coder
  primitives that already exist.
- **Built into `coderd`.** Coder's server runs the orchestrator
  process internally. No separate host to run, no extra token to
  rotate. The integration is a config block in the template instead
  of a deployment you operate. Better long-term shape, but a larger
  product change.

We expect early adopters to start with a hook script and, once the
integration shape settles, fold it into `coderd`.

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
