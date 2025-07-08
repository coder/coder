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

### Test Package Naming

- **Test packages**: Use `package_test` naming (e.g., `identityprovider_test`) for black-box testing

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

```
coderd/
├── oauth2.go                    # Implementation
├── oauth2_test.go              # Main tests
├── oauth2_test_helpers.go      # Test utilities
└── oauth2_validation.go        # Validation logic
```

### Test Categories

1. **Unit Tests**: Test individual functions in isolation
2. **Integration Tests**: Test API endpoints with database
3. **End-to-End Tests**: Full workflow testing
4. **Race Tests**: Concurrent access testing

## Test Commands

### Running Tests

| Command | Purpose |
|---------|---------|
| `make test` | Run all Go tests |
| `make test RUN=TestFunctionName` | Run specific test |
| `go test -v ./path/to/package -run TestFunctionName` | Run test with verbose output |
| `make test-postgres` | Run tests with Postgres database |
| `make test-race` | Run tests with Go race detector |
| `make test-e2e` | Run end-to-end tests |

### Frontend Testing

| Command | Purpose |
|---------|---------|
| `pnpm test` | Run frontend tests |
| `pnpm check` | Run code checks |

## Common Testing Issues

### Database-Related

1. **SQL type errors** - Use `sql.Null*` types for nullable fields
2. **Race conditions in tests** - Use unique identifiers instead of hardcoded names

### OAuth2 Testing

1. **PKCE tests failing** - Verify both authorization code storage and token exchange handle PKCE fields
2. **Resource indicator validation failing** - Ensure database stores and retrieves resource parameters correctly

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
   - Run comprehensive test suites before considering complete

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
- Run `./scaletest/scaletest.sh` for performance testing

### Benchmarking

```go
func BenchmarkFunction(b *testing.B) {
    for i := 0; i < b.N; i++ {
        // Function call to benchmark
        _ = functionUnderTest(input)
    }
}
```

Run benchmarks with:
```bash
go test -bench=. -benchmem ./package/path
```
