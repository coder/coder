# Go Development Guidelines

Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
except where this repository defines a rule below. Check the `go` directive in
`go.mod` before using a language or standard-library feature.

## Go LSP Navigation

Use Go LSP before text search for symbol-aware navigation:

- Definition: `mcp__go-language-server__definition symbolName`
- References: `mcp__go-language-server__references symbolName`
- Type and documentation: `mcp__go-language-server__hover filePath line column`
- Rename: `mcp__go-language-server__rename_symbol filePath line column newName`
- Diagnostics: `mcp__go-language-server__diagnostics filePath`

Use references before changing a public symbol or shared helper. Run diagnostics
on edited files before broader checks.

## Repository Style Deltas

- Use `cdr.dev/slog/v3`, not the standard library `log/slog`. Match nearby
  logging calls.
- Wrap errors with `%w` when callers need the cause. Match package conventions
  for message wording, sentinel errors, and error types.
- Default black-box tests to `package foo_test`. Use `package foo` only in
  `*_internal_test.go` when unexported access is necessary. See
  [Testing Patterns and Best Practices](TESTING.md).
- Prefer existing package patterns over introducing a new abstraction for one
  call site.
- Keep exported APIs narrow. Add interfaces at the consumer boundary when a
  concrete dependency needs substitution.
- Use `gofmt` through `make fmt`; do not hand-align Go code.

## Comments

- Write comments as sentences with punctuation.
- Keep comment lines at most 80 characters, including `//`.
- Explain non-obvious behavior, constraints, invariants, or tradeoffs. Do not
  narrate code that clear names already explain.
- Preserve concise comments that explain non-obvious behavior.
- Start exported declarations with the declared name unless a clearer godoc
  sentence requires another construction.
- Do not record implementation history, review feedback, or the previous
  behavior in code comments.

```go
// retryLimit bounds attempts so a disconnected provisioner cannot keep a job
// active indefinitely.
const retryLimit = 3
```

## Avoid Unrelated Edits

- Change only files and lines required by the task.
- Do not reword nearby comments, rename unrelated symbols, or reformat untouched
  code.
- Read existing tests before adding cases. Preserve what existing cases verify.
- Remove only imports, variables, or helpers made unused by your change.
- Review the final diff for whitespace-only and generated-file churn.

## Enforced Modern Go Idioms

Use these when allowed by the module's `go` version and consistent with nearby
code:

| Avoid                                   | Prefer                             |
|-----------------------------------------|------------------------------------|
| `interface{}`                           | `any`                              |
| Loop-variable shadow copies in Go 1.22+ | Per-iteration loop variables       |
| `sort.Slice` for typed slices           | `slices.Sort` or `slices.SortFunc` |
| Custom `min` and `max` helpers          | Built-in `min` and `max`           |
| `strings.SplitN` for a two-part split   | `strings.Cut`                      |
| Manual map clearing                     | `clear`                            |
| `net.IP` in new APIs                    | `net/netip.Addr`                   |
| New code using `math/rand`              | `math/rand/v2`                     |
| Custom multi-error aggregation          | `errors.Join`                      |
| `Unwrap() error` for multiple causes    | `Unwrap() []error`                 |
| Manual context cleanup in tests         | `t.Context()` when supported       |

Observe these contracts:

- Stop an `iter.Seq` immediately when `yield` returns false.
- Use `slices.SortFunc` for structs or multi-field ordering.
- Do not mix `math/rand` and `math/rand/v2` in one package.
- Remember that `cmp.Or` evaluates every argument.
- Use a non-blocking receive, not `len(timer.C)`, to inspect timer channels.
- Use `context.WithoutCancel` only when work must outlive request cancellation;
  it still carries context values.

## Exact Checks

```sh
make fmt
make lint
```

Use targeted `go test` commands from the testing guide while iterating. Use the
repository workflows for commit-time and push-time validation.
