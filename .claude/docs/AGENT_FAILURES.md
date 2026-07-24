# Agent Failure Catalog

Use this catalog for repeatable agent failures. Keep each entry short,
actionable, and tied to existing docs or tools. Use the exact entry format
shown below when adding new failures.

```markdown
## Symptom: <short description>

- Likely cause:
- How to reproduce:
- How to diagnose:
- Existing docs or tools:
- Missing harness piece:
- Proposed prevention:
```

## Symptom: Stale generated DB code after SQL changes

- Likely cause: A query or migration changed without running `make gen`.
- How to reproduce: Modify `coderd/database/queries/*.sql` and run tests or
  builds without regenerating `coderd/database/queries.sql.go` and related
  generated files.
- How to diagnose: Check `git diff` for SQL changes without generated Go
  changes. Run `make gen` and inspect the resulting diff.
- Existing docs or tools: `AGENTS.md`, [Database Development Patterns](DATABASE.md),
  and the `make gen` target.
- Missing harness piece: No preflight doc checklist currently points agents at
  generated DB drift before they run unrelated checks.
- Proposed prevention: Always run `make gen` after database query or migration
  edits, then include the generated diff in the same commit.

## Symptom: Missing audit table updates

- Likely cause: A database schema change affects audited data but
  `enterprise/audit/table.go` was not updated.
- How to reproduce: Add or change a table that audit logging expects, run
  `make gen`, and observe audit-related generation or test failures.
- How to diagnose: Inspect the `make gen` failure, then compare the changed
  database tables with `enterprise/audit/table.go`.
- Existing docs or tools: `AGENTS.md`, [Database Development Patterns](DATABASE.md),
  and `make gen`.
- Missing harness piece: Agents need a failure catalog entry that connects
  generation failures to audit table maintenance.
- Proposed prevention: After database changes, run `make gen`, update
  `enterprise/audit/table.go` when generation reports audit drift, and rerun
  `make gen`.

## Symptom: Playwright failure without artifacts

- Likely cause: The failing run did not preserve screenshots, traces, videos,
  browser console output, or the Playwright report path.
- How to reproduce: Run a Playwright test from `site` with
  `pnpm playwright:test`, let it fail, and discard the generated output before
  reporting the failure.
- How to diagnose: Check `site/e2e/playwright.config.ts`, `site/e2e/README.md`,
  and the terminal output for the report or `test-results` location.
- Existing docs or tools: [Frontend Development Guidelines](../../site/AGENTS.md),
  `site/e2e/README.md`, and `pnpm playwright:test`.
- Missing harness piece: No central checklist tells agents which browser
  artifacts must be attached to a failure report.
- Proposed prevention: Capture the Playwright report path, screenshot, trace,
  video, browser console output, and command output before retrying or cleaning
  the workspace.

## Symptom: Go test failure without preserved diagnostics

- Likely cause: The failing CI job summary or compact failures artifact was
  discarded before reporting or retrying the failure.
- How to reproduce: Let a Go test job fail in CI, then report the failure using
  only the final job status instead of the job summary and artifacts.
- How to diagnose: Open the failed Go test job summary for the inline failure
  table and per-test details. Download `go-test-failures-*.ndjson` for deeper
  inspection of the compact failures-only records.
- Existing docs or tools: `.github/workflows/ci.yaml` Go test jobs and
  `scripts/gotestsummary`.
- Missing harness piece: Agents need a central reminder to preserve the small
  Go test diagnostics artifact instead of the old raw test log.
- Proposed prevention: Attach or summarize the inline job summary and preserve
  `go-test-failures-*.ndjson` when reporting CI Go test failures.

## Symptom: Port collision across worktrees

- Likely cause: Multiple worktrees use the same default develop ports.
- How to reproduce: Start `./scripts/develop.sh` in one worktree, then start it
  in another worktree without overriding ports.
- How to diagnose: Look for `port <n> is already in use` or conflict errors in
  the develop output. Check listeners with `lsof -iTCP:<port> -sTCP:LISTEN`.
- Existing docs or tools: [Development Isolation Guide for Agents](DEV_ISOLATION.md)
  and `scripts/develop/main.go`.
- Missing harness piece: There is no automatic per-worktree port allocator.
- Proposed prevention: Assign each worktree a unique `CODER_DEV_PORT`,
  `CODER_DEV_WEB_PORT`, `CODER_DEV_PROXY_PORT`, and
  `CODER_DEV_PROMETHEUS_PORT` before starting the app.

## Symptom: Test using `time.Sleep`

- Likely cause: A test waits for time to pass instead of synchronizing on a
  deterministic condition or using the quartz clock.
- How to reproduce: Add a test that depends on `time.Sleep`, then run it under
  load or with the race detector until it flakes.
- How to diagnose: Search the test diff for `time.Sleep`. Inspect whether the
  code under test can use `quartz` or another explicit synchronization point.
- Existing docs or tools: `AGENTS.md`, [Testing Patterns and Best Practices](TESTING.md),
  and the quartz README referenced from `AGENTS.md`.
- Missing harness piece: Agents need a failure entry that labels sleep-based
  waiting as a flake risk before review.
- Proposed prevention: Replace `time.Sleep` with a fake clock, trapped ticker,
  channel, poll with timeout, or another deterministic signal.

## Symptom: DB work inside `InTx` uses the outer store

- Likely cause: Code inside a transaction closure calls `api.Database`, `p.db`,
  or a helper that uses the outer store instead of the `tx` handle.
- How to reproduce: Add DB work inside `db.InTx(...)` that calls back into the
  outer store, then exercise it under concurrent load.
- How to diagnose: Inspect the closure and helper call graph for database calls
  that do not use the transaction handle. Look for pool waits, idle in
  transaction symptoms, or deadlocks under load.
- Existing docs or tools: `AGENTS.md`, [Database Development Patterns](DATABASE.md),
  and code review of `InTx` closures.
- Missing harness piece: No automated check currently proves every helper used
  inside `InTx` stays on the transaction handle.
- Proposed prevention: Fetch read-only inputs before opening the transaction,
  pass `tx` into helpers that need DB access, and avoid receiver helpers that
  hide outer-store usage.

## Symptom: New API endpoint missing swagger annotations

- Likely cause: A handler or route was added without matching swagger comments.
- How to reproduce: Add a stable HTTP endpoint and skip `@Summary`, `@Router`,
  or related annotations.
- How to diagnose: Compare the new handler with nearby handlers and inspect
  generated API docs for the route.
- Existing docs or tools: `AGENTS.md`, [Documentation Style Guide](DOCS_STYLE_GUIDE.md),
  and API generation checks.
- Missing harness piece: Agents need a doc reminder that endpoint work includes
  docs unless the route is intentionally experimental.
- Proposed prevention: Add swagger annotations in the same change as stable
  endpoints. For experimental or unstable API paths, add
  `// @x-apidocgen {"skip": true}` after `@Router`.
