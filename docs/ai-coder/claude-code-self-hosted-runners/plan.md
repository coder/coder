# Plan: Claude Code self-hosted runners on Coder

This page captures the staged plan for documenting how Coder workspaces can
serve as Claude Code self-hosted runners. It lives in `docs/` so it is easy
to read and review alongside the user-facing pages.

The constraint for every stage on this page is **zero Coder product
changes.** Each item is either documentation, a template change, or a
configuration change a customer can make today.

## Goals

- Give platform teams a clear path from "I have an Anthropic pool" to
  "developers can route Claude Code sessions to Coder workspaces."
- Stay strictly within existing Coder capabilities so we can ship this as a
  docs-only update.
- Make it obvious which Anthropic features (wrapper scripts, lifecycle
  hooks, multi-account pools) translate to which Coder primitives so we
  don't accidentally pull product work into a docs project.
- Be honest about the rough edges so future product work has a clear scope.

## Non-goals (for this docs effort)

- Building any new Coder UI, API, or module for self-hosted runners.
- Wrapping the runner binary in a Coder-distributed package.
- Anthropic-side changes (pool management, JWT contents, scheduling).

If we want any of those, that is separate product work that should be
proposed in a Linear issue, not in this docs branch.

## Stage 1 — Basic flow (this PR)

Pages:

- [Overview](./index.md)
- [Setup](./setup.md)
- This plan

What it covers:

- One workspace per developer.
- Pool secret delivered via a sensitive Coder parameter or pre-injected
  env var.
- Runner baked into the workspace image; Git identity set via
  `/etc/gitconfig`.
- `coder_script` starts the runner on workspace start; workspace stop is
  the restart boundary.
- No wrapper script, no lifecycle hooks, no AI Gateway involvement.

Acceptance criteria:

- A reader who has never seen self-hosted runners can stand up a working
  Coder template that registers with an Anthropic pool, using only the
  setup page and the runner build from Anthropic.
- The reader does not have to make any Coder product changes.
- The pages explain how this maps to Anthropic's "one runner, one user at
  a time" model without overpromising shared-pool behavior we are not
  implementing yet.

## Stage 2 — Per-creator credentials via wrapper script (docs only)

Anthropic's runner exposes `CLAUDE_CODE_SESSION_ACCESS_TOKEN`, a JWT whose
`act` claim carries the session creator's email and IdP subject. Operators
are expected to decode that JWT in a wrapper script (passed via
`--exec-path`) and provision creator-scoped credentials before exec'ing
into the real binary.

This stage adds a docs page that:

- Explains the JWT and links to the
  [JWKS endpoint](https://api.anthropic.com/v1/code/.well-known/jwks.json).
- Provides a wrapper-script template that decodes the JWT, exchanges it
  for short-lived credentials from your IdP (the PDF example uses AWS
  STS; we should also document a Vault example), and execs the bundled
  `claude` binary.
- Calls out the warning from the Anthropic PDF that JWT claim shapes are
  subject to change during the EAP, and recommends pinning a runner build
  before relying on specific claims.

This is a no-op from Coder's perspective; the wrapper just runs inside the
workspace.

Open questions:

- How does this interact with the workspace's own identity? If the
  workspace is already authenticated to AWS or Vault via instance metadata
  or workload identity, we should document precedence so the wrapper does
  not silently use the workspace identity instead of the creator's.
- Should we recommend that the workspace template inject the wrapper
  script path via `--exec-path` from a `coder_script` argument, or via the
  `--hooks-dir` `command` hook? Both work; pick one as the recommended
  pattern.

## Stage 3 — Custom checkout via lifecycle hooks (docs only)

The runner supports `--hooks-dir <path>` and looks for executable scripts
with well-known names. The `checkout` hook is the most useful for Coder
deployments because it lets us:

- Clone from an internal Git replica (`ghes-replica.internal`) instead of
  the verbatim source URL.
- Use a local bare mirror with `--reference-if-able` to skip re-fetching
  shared objects, which is a big win when many sessions land on the same
  workspace.
- Materialize non-git sources (Perforce, S3 tarball) by setting
  `CLAUDE_RUNNER_SKIP_GIT_VERIFY=1`.

Docs page should include:

- A reference `checkout` hook that mirrors the PDF example but uses a
  `/var/cache/git-mirrors` location appropriate for a Coder workspace
  (likely on a persistent volume so it survives workspace restart).
- Guidance on which workspace volumes should persist vs which should be
  ephemeral. The runner *expects* a fresh filesystem on restart, but a
  read-only bare mirror is fine to persist.

## Stage 4 — Route the child through AI Gateway (docs only)

The runner process itself must talk to `api.anthropic.com` for pool
registration and polling, but the child `claude` process makes its own
outbound LLM calls. Those calls can be routed through Coder's
[AI Gateway](../ai-gateway/index.md) by setting `ANTHROPIC_BASE_URL` and
the appropriate auth headers in the wrapper script.

This is interesting because it gives platform admins audit and policy
coverage over the *model traffic* the runner generates, without changing
how Anthropic dispatches the session.

Docs page should cover:

- Which env vars to set in the wrapper (`ANTHROPIC_BASE_URL`,
  `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_CUSTOM_HEADERS`) and where they map
  in the existing [AI Gateway Claude Code](../ai-gateway/clients/claude-code.md)
  doc.
- How AI Governance Add-on entitlements interact with the runner. If the
  add-on is not enabled, this stage does not apply.
- A clear note that this is opt-in. The runner's own outbound traffic to
  `api.anthropic.com` is unaffected.

## Stage 5 — Pin permissions and tool allowlists in the image (docs only)

The runner gives each session its own `CLAUDE_CONFIG_DIR` seeded from
`~/.claude/` in the image. That means a template admin can ship a
`settings.json` with deny rules (`Bash(rm -rf:*)`, etc.) and skills,
commands, and CLAUDE.md content as a baseline for every session served by
the workspace.

Docs page should cover:

- The `settings.json` shape from the Anthropic PDF.
- How to ship project-scoped overrides via `<repo>/.claude/settings.json`.
- How `--permission-mode auto`, `--allowed-tools`, and `--disallowed-tools`
  interact, with copy-pasteable wrapper examples.
- The opt-in commit-nudge `Stop` hook the PDF mentions.

This is purely an image and settings exercise; no product work.

## Stage 6 — Fleet pools and runner-only workspaces (docs only, with caveats)

The basic flow is "one developer = one workspace = one runner." That's
fine for a small team but does not match Anthropic's intended fleet model
where many runners across many machines share one pool and lock to
whichever user shows up first.

Two options without product changes:

1. **Runner-only template.** Publish a separate template the platform team
   uses to provision N runner workspaces under a service account. Each
   workspace runs one runner. Anthropic locks each runner to a user on
   first session; the workspace is treated as ephemeral and recycled
   (`coder stop` + `coder start` or template delete-on-stop) when its
   user's queue drains. This works today but is awkward because Coder
   workspaces are user-owned, not pool-owned.

2. **Hybrid.** Developers run their own workspace runner (Stage 1) and the
   platform team runs a pool of service-account workspaces as overflow.
   The runner's `--exit-if-unused-min` flag plus Coder's autostop schedule
   gives a rough equivalent of scale-to-zero.

What we should *not* claim:

- That Coder has built-in autoscaling for self-hosted runners. It does
  not.
- That a single Coder workspace can serve multiple Anthropic users
  simultaneously. The runner locks to one user at a time by design.

Open product questions that come out of this stage (not in scope for the
docs PR but worth tracking):

- Should Coder grow a first-class "headless workspace pool" primitive that
  matches the runner's lifecycle (lock to first user, drain, exit, restart
  on a fresh filesystem)? That would let us implement the Anthropic
  webhook-driven scaling signal directly.
- Should the Coder template registry ship a `claude-code-self-hosted-runner`
  module that wraps the `coder_script` + image bake pattern from this
  doc? That would be a registry change, not a Coder core change.

## Stage 7 — Webhook-driven autoscaling (out of scope)

The PDF describes a runner-needed webhook (or fallback CLI poll mode) that
fires whenever a session is queued and no runner can take it. Anthropic is
still finalizing the contract.

This is the natural place where Coder product work would start. None of it
is in scope for this docs effort, but a follow-up could investigate:

- A small webhook receiver that translates `runner-needed` events into
  `coder create` calls against a runner-only template (a Coder API
  consumer, not a Coder core change).
- A Coder Agent skill that orchestrates the same thing from the control
  plane.

We should call these out in the docs as "Coming later, not yet supported"
so readers do not assume they exist.

## Sequencing and review

| Stage | Pages                       | Reviewers                   | Notes                                                                 |
|-------|-----------------------------|-----------------------------|-----------------------------------------------------------------------|
| 1     | overview, setup, this plan  | docs, AI team, platform-eng | Ship as a single PR. Don't gate on Stages 2+.                         |
| 2     | wrapper scripts page        | security, IdP owners        | Needs IdP examples beyond AWS STS.                                    |
| 3     | lifecycle hooks page        | infra, source-control       | Pair with a Coder template that demonstrates the cache volume layout. |
| 4     | AI Gateway integration page | AI Gateway maintainers      | Behind AI Governance Add-on entitlement.                              |
| 5     | permissions and skills page | security, AI team           | Mostly cribs from the PDF + existing `~/.claude` content.             |
| 6     | fleet pools page            | platform-eng, AI team       | Write with explicit "not autoscaled" disclaimer.                      |
| 7     | (intentionally empty)       | n/a                         | Tracked as product work, not docs.                                    |

## Risks and open issues

- **EAP churn.** The runner build, JWT claim shape, scaling signals, and
  flag set are all flagged in the PDF as subject to change during the EAP.
  We should ship Stage 1 with a clear EAP banner and pin the documented
  `BYOC_VERSION` to whatever Anthropic gave us at write time.
- **Two-source ownership.** Anthropic owns the runner binary, the pool, and
  the session control plane. Coder owns the workspace and the
  observability around it. A reader who hits a problem will need to know
  which logs to read first; the troubleshooting section in `setup.md` is
  the first attempt at that and will need iteration.
- **Multi-repo sessions.** The PDF mentions that multi-repo sessions spawn
  from a parent directory with `--add-dir` per repo. The basic flow does
  not exercise this. We should add a short note once we have tested it.
- **Persistence.** Anthropic expects each runner restart to give a fresh
  filesystem. Coder workspaces typically persist `$HOME`. We default to
  "keep `$HOME` persistent, treat workspace stop/start as the restart
  boundary," but a stricter deployment may want an `emptyDir` checkout
  path. Document both.

## What changes if we do allow product work later

This section is a parking lot, not a commitment. It lists things that
*would* require a Coder change so we can refer back to it during planning
without losing context.

- A first-class **runner pool resource** in Coder that knows how to spawn
  a workspace per `runner-needed` webhook from Anthropic, locked to a
  given user, with a TTL tied to drain status. This is the most useful
  product addition; it would close the fleet-pool gap from Stage 6 and
  unlock Stage 7.
- An AI Gateway client preset for "self-hosted runner child process" so
  template admins do not have to wire `ANTHROPIC_BASE_URL` themselves.
- An audit log integration that surfaces session pickup events from the
  runner alongside Coder's existing audit log, so admins have a single
  pane of glass.

None of those are needed for the basic flow this docs effort delivers.
