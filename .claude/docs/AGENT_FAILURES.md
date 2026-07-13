# Agent Failure Catalog

Use this catalog for repeatable failures. Diagnose the root cause, preserve the
smallest reproducer and useful artifacts, then make one focused fix.

Use this entry format for additions:

```markdown
## Symptom: <short description>

- Likely cause:
- Reproduce:
- Diagnose:
- Owner:
- Prevent:
```

## Common Failure Triage

1. Capture the exact command, error, environment, and smallest reproduction.
2. Read the complete compiler, test, validation, or generator output.
3. Inspect the relevant diff and recent changes.
4. Form one hypothesis. Do not stack speculative fixes.
5. Run the reproducer after each change.
6. Preserve CI summaries, traces, screenshots, and compact artifacts before
   retrying or cleaning the workspace.
7. Run the relevant broader validation only after the focused reproducer passes.
8. If the cause is unknown, report what is unknown and what evidence is needed.

Common quick checks:

- Missing final newline: configure the editor and inspect the file ending.
- Type mismatch around nullable database fields: compare the generated type and
  follow [Database Development Patterns](DATABASE.md).
- Black-box package lint failure: follow the package naming rules in
  [Testing Patterns and Best Practices](TESTING.md).
- Protocol error or public-endpoint authorization failure: follow
  [OAuth2 Development Guide](OAUTH2.md).
- Port already in use: inspect listeners and follow
  [Development Isolation Guide for Agents](DEV_ISOLATION.md).
- Missing API documentation: follow the endpoint checklist in
  [Development Workflows and Guidelines](WORKFLOWS.md).

## Symptom: Stale generated database code

- Likely cause: SQL or schema inputs changed without completing the generation
  workflow.
- Reproduce: Change a query or migration, then build with stale generated files.
- Diagnose: Compare SQL changes with generated Go and schema diffs.
- Owner: [Database Development Patterns](DATABASE.md).
- Prevent: Complete the owner guide's generation and audit sequence, and commit
  the inputs and outputs together.

## Symptom: Missing audit-table action

- Likely cause: An auditable field lacks a classification.
- Reproduce: Add the field and run the database generation workflow.
- Diagnose: Compare the reported field with `enterprise/audit/table.go`.
- Owner: [Database Development Patterns](DATABASE.md).
- Prevent: Classify every new field as tracked, ignored, or secret during the
  database workflow.

## Symptom: Database work inside `InTx` uses the outer store

- Likely cause: The closure calls an outer store directly or through a helper.
- Reproduce: Exercise the transaction under concurrent load.
- Diagnose: Trace every database call in the closure and inspect pool waits or
  `idle in transaction` symptoms.
- Owner: [Database Development Patterns](DATABASE.md).
- Prevent: Pass the transaction handle into helpers and fetch unrelated reads
  before opening the transaction.

## Symptom: Flaky timing-dependent test

- Likely cause: The test waits for elapsed wall time instead of a deterministic
  event.
- Reproduce: Run the test repeatedly, under load, and with the race detector.
- Diagnose: Inspect clocks, tickers, goroutines, channels, polling, and cleanup.
- Owner: [Testing Patterns and Best Practices](TESTING.md).
- Prevent: Use a controlled clock or explicit bounded synchronization signal.

## Symptom: Concurrent tests collide

- Likely cause: Tests reuse a constrained name, environment variable, global
  state, or fixed port.
- Reproduce: Run the package repeatedly with concurrency and the race detector.
- Diagnose: Compare identifiers and shared resources across failing tests.
- Owner: [Testing Patterns and Best Practices](TESTING.md).
- Prevent: Generate unique values, isolate resources, or document why the test
  must run serially.

## Symptom: Go test failure lacks diagnostics

- Likely cause: The report kept only the final job status.
- Reproduce: Discard the job summary and compact failure artifact.
- Diagnose: Recover the inline failure table, per-test details, and
  `go-test-failures-*.ndjson` artifact.
- Owner: `.github/workflows/ci.yaml`, `scripts/gotestsummary`, and
  [Testing Patterns and Best Practices](TESTING.md).
- Prevent: Preserve the focused command output and compact CI artifact before
  retrying.

## Symptom: Playwright failure lacks artifacts

- Likely cause: Screenshots, trace, video, console output, or report path were
  discarded.
- Reproduce: Run `pnpm playwright:test` from `site`, fail a test, then clean its
  output.
- Diagnose: Inspect `site/e2e/playwright.config.ts`, `site/e2e/README.md`, and
  the terminal report path.
- Owner: [Frontend Development Guidelines](../../site/AGENTS.md) and
  `site/e2e/README.md`.
- Prevent: Preserve the report path and browser artifacts before retrying.

## Symptom: Port collision across worktrees

- Likely cause: Worktrees use the same default local ports.
- Reproduce: Start development servers in two worktrees without isolation.
- Diagnose: Inspect the startup error and listeners with
  `lsof -iTCP:<port> -sTCP:LISTEN`.
- Owner: [Development Isolation Guide for Agents](DEV_ISOLATION.md).
- Prevent: Enable the per-worktree offset or assign unique port overrides.

## Symptom: OAuth2 endpoint returns a generic error

- Likely cause: The handler used the normal API error path.
- Reproduce: Trigger a protocol-defined client or request failure.
- Diagnose: Compare the response status, code, fields, and disclosure behavior
  with the applicable RFC.
- Owner: [OAuth2 Development Guide](OAUTH2.md).
- Prevent: Use the protocol error writer and test every defined error path.

## Symptom: Public endpoint cannot access required OAuth2 state

- Likely cause: The request has no user context and the database call uses the
  request context directly.
- Reproduce: Call the endpoint without an authenticated user.
- Diagnose: Trace the authorization context passed to the database operation.
- Owner: [OAuth2 Development Guide](OAUTH2.md).
- Prevent: Use the restricted system context defined by the owner guide.

## Symptom: New API endpoint is absent from generated documentation

- Likely cause: The handler lacks swagger annotations or an unstable route was
  not marked intentionally.
- Reproduce: Add the route and inspect generated API documentation.
- Diagnose: Compare the handler annotations with a nearby documented endpoint.
- Owner: [Development Workflows and Guidelines](WORKFLOWS.md) and
  [Docs Content Guidelines](../../docs/.style/content-guidelines.md).
- Prevent: Add annotations with the handler, or mark an experimental route to be
  skipped by public documentation generation.
