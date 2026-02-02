# Testing Strategy Improvements

This document outlines recommendations for improving the Coder testing
infrastructure based on analysis of current patterns, pain points, and
industry best practices.

## Executive Summary

The Coder codebase has a robust testing foundation with 653+ test files, strong
parallel test support (5400+ uses of `t.Parallel()`), and sophisticated
database testing utilities. However, there are several areas where improvements
could reduce flakiness, speed up CI, and improve developer experience.

## Current State Analysis

### Strengths

1. **Comprehensive Coverage**: Tests exist for all major components
2. **Parallel Execution**: Wide adoption of `t.Parallel()`
3. **Rich Test Utilities**: Well-designed `testutil` and `coderdtest` packages
4. **Database Testing**: Sophisticated `dbtestutil` with embedded Postgres
5. **Quartz Integration**: Proper mock clock usage for timing-sensitive tests
6. **CI Infrastructure**: Multi-platform (Linux, macOS, Windows), race
   detection, multiple Postgres versions

### Pain Points Identified

1. **Flaky Tests**: ~30 tests explicitly marked as flaky with `t.Skip`
2. **`time.Sleep` Usage**: 49 occurrences in test files - potential timing
   issues
3. **Platform-Specific Issues**: Many tests skip on Windows/macOS due to
   flakiness
4. **Long CI Times**: Full test suite takes 20-25 minutes
5. **Test Isolation**: Some tests have implicit dependencies on system state

## Recommendations

### 1. Eliminate `time.Sleep` in Tests

**Priority: High**

**Problem**: 49 uses of `time.Sleep` in test files create timing-dependent
tests that are prone to flakiness, especially under CI load.

**Solution**: Replace all `time.Sleep` calls with proper synchronization
mechanisms:

```go
// BAD: time-dependent, flaky under load
time.Sleep(100 * time.Millisecond)
assert.Equal(t, expected, getValue())

// GOOD: event-driven, reliable
require.Eventually(t, func() bool {
    return getValue() == expected
}, testutil.WaitShort, testutil.IntervalFast)
```

For timing-controlled behavior, use the Quartz library:

```go
// Use mock clocks for time-dependent behavior
mClock := quartz.NewMock(t)
sut := NewSystemUnderTest(mClock)

// Advance time deterministically
mClock.Advance(5 * time.Minute)
```

**Files to prioritize**:

- `coderd/templates_test.go` (6 occurrences)
- `coderd/activitybump_test.go` (3 occurrences)
- `cli/ssh_test.go` (3 occurrences)

### 2. Improve Flaky Test Tracking

**Priority: High**

**Problem**: Flaky tests are tracked inconsistently - some use `t.Skip` with
GitHub issues, others have no tracking.

**Solution**: Implement a structured flaky test tracking system:

```go
// Create a centralized flaky test registry
package testutil

type FlakyTestInfo struct {
    Issue       string    // GitHub issue URL
    Platform    string    // "windows", "macos", "linux", or "all"
    Reason      string    // Brief description
    AddedDate   time.Time
    LastChecked time.Time
}

// SkipFlaky skips the test with proper tracking and logging
func SkipFlaky(t testing.TB, info FlakyTestInfo) {
    t.Helper()
    if !MatchesPlatform(info.Platform) {
        return
    }
    t.Skipf("Flaky test skipped: %s (Issue: %s, Added: %s)",
        info.Reason, info.Issue, info.AddedDate.Format("2006-01-02"))
}
```

**Benefits**:

- Centralized visibility into flaky tests
- Automatic tracking of how long tests have been flaky
- Easy querying for flaky test reports
- Consistent skip messaging

### 3. Test Categorization and Selective Running

**Priority: Medium**

**Problem**: All tests run together, making CI slow and local development
cumbersome.

**Solution**: Implement test categories using build tags:

```go
//go:build integration
// +build integration

package integration_test

func TestIntegration(t *testing.T) {
    // Integration tests here
}
```

**Suggested categories**:

| Category      | Tag            | Description                         |
| ------------- | -------------- | ----------------------------------- |
| Unit          | (default)      | Fast, isolated unit tests           |
| Integration   | `integration`  | Tests requiring external services   |
| Database      | `database`     | PostgreSQL-dependent tests          |
| E2E           | `e2e`          | Full end-to-end tests               |
| Slow          | `slow`         | Tests taking >30s                   |
| Platform      | `linux_only`   | Platform-specific tests             |

**Makefile updates**:

```makefile
test-unit:
    go test -v ./... -tags=!integration,!database,!e2e,!slow

test-integration:
    go test -v ./... -tags=integration

test-quick:
    go test -v ./... -short -tags=!slow,!e2e
```

### 4. Enhanced Database Test Utilities

**Priority: Medium**

**Problem**: Database tests are slow and sometimes leak connections or have
race conditions.

**Improvements**:

#### a. Connection Pool Management

```go
// Add connection tracking to dbtestutil
type TrackedDB struct {
    database.Store
    t          testing.TB
    openConns  atomic.Int32
    maxConns   int32
    connMu     sync.Mutex
}

func (d *TrackedDB) assertNoLeakedConnections() {
    d.t.Helper()
    open := d.openConns.Load()
    if open > 0 {
        d.t.Errorf("leaked %d database connections", open)
    }
}
```

#### b. Transaction-Based Test Isolation

```go
// Use savepoints for faster test isolation
func WithTransaction(t testing.TB, db database.Store, fn func(tx database.Store)) {
    ctx := testutil.Context(t, testutil.WaitLong)
    tx, err := db.Begin(ctx)
    require.NoError(t, err)
    defer tx.Rollback(ctx) // Always rollback

    fn(tx)
    // No commit - test isolation via rollback
}
```

#### c. Parallel Database Test Improvements

```go
// Pre-create test databases for faster parallel execution
type DatabasePool struct {
    mu        sync.Mutex
    available chan *TestDatabase
    created   int
}

func (p *DatabasePool) Acquire(t testing.TB) *TestDatabase {
    select {
    case db := <-p.available:
        t.Cleanup(func() { p.Release(db) })
        return db
    default:
        return p.createNew(t)
    }
}
```

### 5. Improved Test Context Management

**Priority: Medium**

**Problem**: Inconsistent timeout handling across tests, leading to either
timeouts that are too short (flaky) or too long (slow feedback).

**Solution**: Standardize context creation with better defaults:

```go
package testutil

// ContextWithCancel returns a context that is canceled when the test
// completes. Unlike Context(), this doesn't have a deadline.
func ContextWithCancel(t testing.TB) context.Context {
    ctx, cancel := context.WithCancel(context.Background())
    t.Cleanup(cancel)
    return ctx
}

// ContextCI returns a context with an appropriate timeout for CI
// environments (longer) vs local development (shorter).
func ContextCI(t testing.TB) context.Context {
    timeout := WaitLong
    if InCI() {
        timeout = WaitSuperLong // More time in CI due to load
    }
    return Context(t, timeout)
}
```

### 6. Test Output Improvements

**Priority: Low**

**Problem**: Test failures can be hard to diagnose, especially in CI.

**Solution**: Enhanced failure diagnostics:

```go
package testutil

// DumpOnFailure registers a cleanup function that dumps diagnostic
// information when the test fails.
func DumpOnFailure(t testing.TB, name string, dumpFn func() string) {
    t.Cleanup(func() {
        if t.Failed() {
            t.Logf("=== %s diagnostic dump ===\n%s", name, dumpFn())
        }
    })
}

// Usage:
func TestSomething(t *testing.T) {
    server := startServer(t)
    testutil.DumpOnFailure(t, "server logs", server.GetLogs)
    testutil.DumpOnFailure(t, "database state", func() string {
        return queryDatabaseState(t)
    })
}
```

### 7. Parallel Test Safety Linting

**Priority: Medium**

**Problem**: Race conditions in tests due to shared mutable state.

**Solution**: Add linting rules and documentation:

```go
// scripts/rules.go - add semgrep/staticcheck rules

// Rule: Detect closure capture of loop variables in parallel tests
// Rule: Detect use of shared variables without synchronization
// Rule: Ensure t.Parallel() is called at the start of each subtest
```

Add a pre-commit hook or CI check:

```bash
# Check for parallel test safety
go vet -vettool=$(which paralleltestctx) ./...
```

### 8. Test Fixture Improvements

**Priority: Low**

**Problem**: Test fixtures are scattered and sometimes duplicated.

**Solution**: Centralize and version test fixtures:

```
testdata/
├── fixtures/
│   ├── v1/
│   │   ├── workspace.json
│   │   └── template.json
│   └── v2/
│       └── workspace.json
├── golden/
│   └── *.golden
└── README.md
```

```go
package testutil

// LoadFixture loads a versioned test fixture
func LoadFixture[T any](t testing.TB, name string) T {
    t.Helper()
    path := filepath.Join("testdata", "fixtures", fixtureVersion, name)
    data, err := os.ReadFile(path)
    require.NoError(t, err)

    var result T
    require.NoError(t, json.Unmarshal(data, &result))
    return result
}
```

### 9. CI Pipeline Optimizations

**Priority: Medium**

**Improvements**:

#### a. Smart Test Selection

```yaml
# .github/workflows/ci.yaml
- name: Determine affected tests
  run: |
    # Use go list with -deps to find affected packages
    changed_files=$(git diff --name-only origin/main...)
    affected_packages=$(./scripts/affected-packages.sh $changed_files)
    echo "TEST_PACKAGES=$affected_packages" >> $GITHUB_ENV
```

#### b. Test Result Caching

```yaml
- name: Cache test results
  uses: actions/cache@v4
  with:
    path: ~/.cache/go-build
    key: test-${{ runner.os }}-${{ hashFiles('**/*.go') }}
    restore-keys: |
      test-${{ runner.os }}-
```

#### c. Parallel Job Splitting

```yaml
strategy:
  matrix:
    shard: [1, 2, 3, 4]
steps:
  - run: |
      gotestsum --packages="./..." -- \
        -run="$(./scripts/shard-tests.sh ${{ matrix.shard }} 4)"
```

### 10. Property-Based Testing

**Priority: Low**

**Opportunity**: Add property-based testing for complex logic:

```go
import "pgregory.net/rapid"

func TestRBACProperties(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Generate random but valid RBAC configurations
        role := rapid.Custom(genRole).Draw(t, "role")
        resource := rapid.Custom(genResource).Draw(t, "resource")
        action := rapid.Custom(genAction).Draw(t, "action")

        // Property: authorization decisions must be deterministic
        result1 := authorizer.Authorize(role, resource, action)
        result2 := authorizer.Authorize(role, resource, action)
        require.Equal(t, result1, result2)
    })
}
```

## Implementation Roadmap

### Phase 1: Quick Wins (1-2 weeks)

1. Audit and document all flaky tests with GitHub issues
2. Replace 10 highest-impact `time.Sleep` occurrences
3. Add `testutil.SkipFlaky` helper function

### Phase 2: Infrastructure (2-4 weeks)

1. Implement test categorization with build tags
2. Add smart test selection to CI
3. Improve database connection tracking

### Phase 3: Long-term (ongoing)

1. Migrate all `time.Sleep` to proper synchronization
2. Add property-based tests for critical paths
3. Implement test sharding for faster CI

## Metrics to Track

| Metric                        | Current | Target  |
| ----------------------------- | ------- | ------- |
| CI Duration (full suite)      | ~25 min | <15 min |
| Flaky test rate               | ~5%     | <1%     |
| `time.Sleep` in tests         | 49      | 0       |
| Tests skipped due to flakiness| ~30     | <5      |
| Test coverage                 | ?%      | >80%    |

## References

- [Quartz README](https://github.com/coder/quartz/blob/main/README.md) -
  Timing in tests
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
  - Testing patterns
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test) -
  Official Go testing guide
