# flake-bot v2

flake-bot automatically triages and fixes flaky CI tests on `main`.

Every PR must pass the `ci` workflow before it can merge, so `main` is green at
merge time. When `ci` later fails on `main`, the same code that just passed is
failing, which means the failure is (almost always) a **test flake**. flake-bot
turns each such failure into tracked, actionable work.

## What it does

When `ci` completes with `failure` on `main`, `.github/workflows/flake-bot.yaml`:

1. Generates a token for the **flake-bot GitHub App** (its bot identity).
2. Checks out the failing commit and collects the failed job logs into a
   context file.
3. Runs a Claude agent (`anthropics/claude-code-action`) with the instructions
   in [`system-prompt.md`](./system-prompt.md). The agent:
   - identifies the specific failing test and root cause;
   - suggests an owner from `git blame` / `git log` (suggestion only, never
     assigned);
   - **deduplicates against Linear** (team `ENG`): comments on the existing
     `flake:` issue if one exists, otherwise creates one (label `flake`, High
     priority) via the **Linear MCP server**;
   - attempts a **minimal** fix and opens a **draft PR** (labels `flake`,
     `testing`) from a deterministic branch `flake-bot/<linear-id>`;
   - **avoids duplicate PRs**: pushes to its own existing PR for the same
     flake, or just adds context to a human's existing PR;
   - links the PR back to the Linear issue.

It **no-ops gracefully** when its secrets are unavailable (e.g. forks).

## Required configuration

### Secrets

| Secret | Purpose |
| --- | --- |
| `ANTHROPIC_API_KEY` | Auth for `claude-code-action`. |
| `LINEAR_ACCESS_KEY` | Linear API key; Bearer auth for the Linear MCP server (already used by `linear-release.yaml`). |
| `FLAKE_BOT_APP_ID` | flake-bot GitHub App ID. |
| `FLAKE_BOT_APP_PRIVATE_KEY` | flake-bot GitHub App private key (PEM). |

### GitHub App (`flake-bot`)

Create an App in the `coder` org, install it on `coder/coder`, and set the two
`FLAKE_BOT_APP_*` secrets. Required repository permissions:

- **Contents**: Read and write (push fix branches)
- **Pull requests**: Read and write (open/update draft PRs, comment)
- **Issues**: Read and write (PR labels/comments)
- **Actions**: Read (download failed CI logs)
- **Metadata**: Read

PRs and comments are then authored by `flake-bot[bot]`, which is what makes the
"is this PR already mine?" dedup reliable.

## Linear integration

flake-bot talks to Linear through the official hosted **Linear MCP server**
(`https://mcp.linear.app/mcp`, Streamable HTTP), wired into the agent with an
inline `claude_args` `--mcp-config`. It authenticates with `LINEAR_ACCESS_KEY`
through an `Authorization: Bearer` header; the server accepts a Linear API key
or OAuth token this way, so it works headlessly in CI with no interactive
OAuth flow.

The agent is allow-listed to a minimal, issue-focused set of tools and cannot
use any other Linear capability:

- `mcp__linear__list_issues`, `mcp__linear__get_issue`
- `mcp__linear__save_issue` (create/update, including state changes)
- `mcp__linear__list_comments`, `mcp__linear__save_comment`
- `mcp__linear__list_teams`, `mcp__linear__list_issue_labels`,
  `mcp__linear__list_issue_statuses`

## Testing it

`workflow_run` only fires from the workflow on the **default branch**, so this
must be on `main` before it auto-triggers. To exercise it before/after merge,
use the manual trigger with a known failed `ci` run on `main`:

```sh
gh workflow run flake-bot.yaml -f run_id=<failed-ci-run-id-or-url>
```

Linear access goes through the hosted Linear MCP server (see "Linear
integration" above); there is no local helper to run.

## Co-authorship on merge

flake-bot authors its fix commits as `flake-bot[bot]`, and the PR body includes
a `Co-authored-by:` trailer for the suggested engineer. When a maintainer
squash-merges, the merge commit is co-attributed to flake-bot and the reviewer
who merges it. Issue closure is driven by the linked PR merging.

## Scope and limitations (MVP)

- **Team routing is fixed to `ENG`.** Real flakes are owned by many teams
  (PLAT, DEVEX, CODAGT, ...). Routing via `CODEOWNERS` is a planned follow-up;
  for now the suggested-owner line in the issue body carries that signal.
- **Flake classification is intentionally trivial** ("failure on main =
  flake"). The agent still declines to force a code fix for obvious
  infrastructure/external failures and records them in Linear instead.
- **The Linear MCP tool allow-list is pinned by name.** Claude Code does not
  allow wildcards in `--allowedTools`, so each `mcp__linear__*` tool is listed
  explicitly in `flake-bot.yaml`. If Linear renames a tool or flake-bot needs
  a new one, update that allow-list.
- The agent will not open a low-confidence fix PR; a clean triage with no PR is
  a valid outcome.

## Files

- `../workflows/flake-bot.yaml`: trigger, context collection, agent run, and
  Linear MCP server wiring.
- `system-prompt.md`: the agent's full instructions.
