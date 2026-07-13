# Testing Patterns and Best Practices

Use deterministic tests that remain isolated under parallel and race-detector
execution.

## Parallelism and Isolation

- Use `t.Parallel()` by default in new tests.
- Serial tests are acceptable only with a stated reason, such as shared global
  state, environment variables, or fixed ports.
- Use unique identifiers for constrained fields and externally visible names in
  concurrent tests. Include `t.Name()` and a generated suffix when practical.
- Never depend on package execution order or state left by another test.
- Register cleanup with `t.Cleanup`, or use helpers that do so.

```go
name := fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())
```

## Synchronization and Time

Never use `time.Sleep` as synchronization or to hide a timing failure. Wait for
a deterministic condition with a bounded timeout, use a channel or callback, or
control time with [quartz](https://github.com/coder/quartz). Prefer trapped
tickers, fake clocks, and explicit signals over wall-clock waiting.

Use `testutil.WaitShort`, `testutil.WaitMedium`, or `testutil.WaitLong` for
bounded waits that match the operation. A timeout is a failure bound, not a
substitute for a synchronization condition.

## Test Package Naming

- Default black-box tests to `package foo_test`.
- Put tests that require unexported symbols in `*_internal_test.go` with
  `package foo`.
- Do not add `//nolint:testpackage`; use the established internal-test filename.

## Test Structure

- Prefer table-driven tests when cases share setup and assertions.
- Cover success, validation, authorization, and error paths relevant to the
  changed behavior.
- Use `require` when the test cannot continue after a failed assertion. Use
  `assert` only when later assertions remain meaningful.
- Prefer real repository test helpers to bespoke mocks.
- Keep fixtures minimal and name subtests by behavior.

## Repository Test Utilities

- `coderdtest.New(t, options)` starts a test server and registers cleanup.
- `dbtestutil.NewDB(t)` provides a test database.
- `coderdenttest` provides Enterprise test setup.
- `testutil` provides bounded waits and common helpers.
- Read nearby tests before introducing a new helper or fixture pattern.

## Commands

| Command                                      | Purpose                                |
|----------------------------------------------|----------------------------------------|
| `make test`                                  | Run the Go test suite                  |
| `make test RUN=TestName`                     | Run matching Go tests                  |
| `go test -v ./path/to/package -run TestName` | Run one package test verbosely         |
| `make test-race`                             | Run the repository race-detector suite |
| `go test -race ./path/to/package`            | Race-test one package                  |
| `make test-e2e`                              | Run end-to-end tests                   |
| `pnpm test`                                  | Run frontend Vitest tests              |
| `pnpm check`                                 | Run frontend checks                    |

Use the race detector for concurrency changes, shared caches, background work,
and fixes for flaky tests. Do not treat a normal test pass as evidence that
concurrent access is safe.

## Failure Diagnosis

1. Reproduce the smallest failing test command.
2. Preserve the complete error, failing subtest name, and relevant artifacts.
3. Check package isolation, identifiers, synchronization, cleanup, and race
   output before changing production code.
4. Form one hypothesis, make one focused change, and rerun the reproducer.
5. Run the relevant broader suite after the targeted test passes.

For repeatable failure-report formats, see [Agent Failure Catalog](AGENT_FAILURES.md).
