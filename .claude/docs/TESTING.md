# Testing Patterns and Best Practices

## Testing Best Practices

### Avoiding Race Conditions

1. **Unique Test Identifiers**:
   - Never use hardcoded names in concurrent tests
   - Use `time.Now().UnixNano()` or similar for unique identifiers
   - Example: `fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())`

2. **Database Constraint Awareness**:
   - Understand unique constraints that can cause test conflicts
   - Generate unique values for all constrained fields
   - Test name isolation prevents cross-test interference

### Testing Patterns

- Use table-driven tests for comprehensive coverage
- Mock external dependencies
- Test both positive and negative cases
- Use `testutil.WaitLong` for timeouts in tests

### Timing Issues

NEVER use `time.Sleep` to mitigate timing issues. If an issue seems like
it should use `time.Sleep`, read through https://github.com/coder/quartz
and specifically the README to better understand how to handle timing
issues.

Do not wait on real time either: when a ticker, heartbeat, or deadline is
under test, inject `quartz.NewMock(t)` and advance it instead of
constructing the code under test with `quartz.NewReal()`.

### Fixture Cost

`coderdtest.New` clones a Postgres database and starts an in-process NATS
server (about 150ms before the test does any work) and `dbtestutil.NewDB`
clones a database (about 50ms). Multiplied across a table, fixtures
dominate package wall time.

- Build the server or database once per `Test` func and share it across
  subtests that only read from it or create uniquely named resources. Give
  a subtest its own fixture only when it changes deployment values,
  licenses, feature flags, or rows other subtests read.
- Use `dbtestutil.NewDB` only when SQL behavior is under test. For many
  parallel query-only leaves, bind each leaf to a rolled-back transaction
  on one shared database instead of cloning a database per leaf.
- Measure the per-leaf floor before restructuring: `EXPLAIN (ANALYZE)` for
  a slow query, phase timers for a slow fixture. Past root causes were a
  query that Postgres JIT-compiled on every call and a 5s ticker before the
  first DERP map fetch, not the fixtures themselves.

### Test Package Naming

- **Black-box tests**: Default to a `package foo_test` test file (e.g.,
  `oauth2provider_test`). This is what the `testpackage` linter enforces.
- **White-box / internal tests**: When a test needs to touch unexported
  symbols, put it in a file named `*_internal_test.go` with `package foo`.
  The `testpackage` linter's `skip-regexp` already exempts that filename
  suffix, so no `//nolint:testpackage` directive is needed.
- **Do not add `//nolint:testpackage`.** If a test needs internal access,
  rename the file to `*_internal_test.go` instead. A directive plus a
  justification comment is strictly worse than the established naming
  convention, and the repo standardizes on the latter.

## RFC Protocol Testing

### Compliance Test Coverage

1. **Test all RFC-defined error codes and responses**
2. **Validate proper HTTP status codes for different scenarios**
3. **Test protocol-specific edge cases** (URI formats, token formats, etc.)

### Security Boundary Testing

1. **Test client isolation and privilege separation**
2. **Verify information disclosure protections**
3. **Test token security and proper invalidation**

## Test Organization

### Test File Structure

```text
coderd/
├── oauth2.go                    # Route handlers that delegate to oauth2provider
├── oauth2_test.go               # API-level tests
└── oauth2provider/
    ├── tokens.go                # Implementation
    ├── tokens_test.go           # Black-box tests (package oauth2provider_test)
    └── tokens_internal_test.go  # White-box tests (package oauth2provider)
```

### Test Categories

1. **Unit Tests**: Test individual functions in isolation
2. **Integration Tests**: Test API endpoints with database
3. **End-to-End Tests**: Full workflow testing
4. **Race Tests**: Concurrent access testing

## Test Commands

### Running Tests

| Command                                              | Purpose                         |
|------------------------------------------------------|---------------------------------|
| `make test`                                          | Run all Go tests                |
| `make test RUN=TestFunctionName`                     | Run specific test               |
| `go test -v ./path/to/package -run TestFunctionName` | Run test with verbose output    |
| `make test-race`                                     | Run tests with Go race detector |
| `make test-e2e`                                      | Run end-to-end tests            |

### Frontend Testing

| Command      | Purpose            |
|--------------|--------------------|
| `pnpm test`  | Run frontend tests |
| `pnpm check` | Run code checks    |

## Common Testing Issues

### Database-Related

1. **SQL type errors** - Use `sql.Null*` types for nullable fields
2. **Race conditions in tests** - Use unique identifiers instead of hardcoded names

### OAuth2 Testing

1. **PKCE tests failing** - Verify both authorization code storage and token exchange handle PKCE fields
2. **Resource indicator validation failing** - Ensure database stores and retrieves resource parameters correctly

### OAuth2 Test Scripts

- Full suite: `./scripts/oauth2/test-mcp-oauth2.sh`
- Manual testing: `./scripts/oauth2/test-manual-flow.sh`

### General Issues

1. **Missing newlines** - Ensure files end with newline character
2. **Package naming errors** - Use `package_test` naming for test files
3. **Log message formatting errors** - Use lowercase, descriptive messages without special characters

## Systematic Testing Approach

### Multi-Issue Problem Solving

When facing multiple failing tests or complex integration issues:

1. **Identify Root Causes**:
   - Run failing tests individually to isolate issues
   - Use LSP tools to trace through call chains
   - Check both compilation and runtime errors

2. **Fix in Logical Order**:
   - Address compilation issues first (imports, syntax)
   - Fix authorization and RBAC issues next
   - Resolve business logic and validation issues
   - Handle edge cases and race conditions last

3. **Verification Strategy**:
   - Test each fix individually before moving to next issue
   - Use `make lint` and `make gen` after database changes
   - Verify RFC compliance with actual specifications
   - Before handoff, run the affected packages' tests and the broader checks the changed area requires

## Test Data Management

### Unique Test Data

```go
// Good: Unique identifiers prevent conflicts
clientName := fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())

// Bad: Hardcoded names cause race conditions
clientName := "test-client"
```

### Test Cleanup

```go
func TestSomething(t *testing.T) {
    // Setup
    client := coderdtest.New(t, nil)

    // Test code here

    // Cleanup happens automatically via t.Cleanup() in coderdtest
}
```

## Test Utilities

### Common Test Patterns

```go
// Table-driven tests
tests := []struct {
    name     string
    input    InputType
    expected OutputType
    wantErr  bool
}{
    {
        name:     "valid input",
        input:    validInput,
        expected: expectedOutput,
        wantErr:  false,
    },
    // ... more test cases
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()

        result, err := functionUnderTest(tt.input)
        if tt.wantErr {
            require.Error(t, err)
            return
        }
        require.NoError(t, err)
        require.Equal(t, tt.expected, result)
    })
}
```

### Test Assertions

```go
// Use testify/require for assertions
require.NoError(t, err)
require.Equal(t, expected, actual)
require.NotNil(t, result)
require.True(t, condition)
```

## Performance Testing

### Load Testing

- Use `scaletest/` directory for load testing scenarios
- Run load scenarios with `coder exp scaletest` (see `cli/exp_scaletest.go`);
  the runner template lives in `scaletest/templates/scaletest-runner/`

### Benchmarking

```go
func BenchmarkFunction(b *testing.B) {
    for b.Loop() {
        // Function call to benchmark
        _ = functionUnderTest(input)
    }
}
```

Run benchmarks with:

```bash
go test -bench=. -benchmem ./package/path
```

### Suite Cost

- `make test-timings TEST_PACKAGES=./package/path/` writes per-test elapsed
  times to `test-timings.tsv`, slowest first. A parent's elapsed time
  includes its sequential children and queue waits, so attribute cost to
  leaves.
- To find production code kept alive only by tests, run
  `go run golang.org/x/tools/cmd/deadcode@latest -tags testsmallbatch -test ./...`
  and the same command without `-test` on `./cmd/... ./enterprise/cmd/...`.
  Symbols in both outputs are dead everywhere; symbols only in the second
  are reachable from tests alone. The `unused` linter cannot see this class
  because tests count as usage.
