# Modern Go (1.18–1.26)

Reference for writing idiomatic Go. Covers what changed, what it
replaced, and what to reach for. Respect the project's `go.mod` `go`
line: don't emit features from a version newer than what the module
declares. Check `go.mod` before writing code.

## How modern Go thinks differently

**Generics** (1.18): Design reusable code with type parameters instead
of `interface{}` casts, code generation, or the `sort.Interface`
pattern. Use `any` for unconstrained types, `comparable` for map keys
and equality, `cmp.Ordered` for sortable types. Type inference usually
makes explicit type arguments unnecessary (improved in 1.21).

**Per-iteration loop variables** (1.22): Each loop iteration gets its
own variable copy. Closures inside loops capture the correct value. The
`v := v` shadow trick is dead. Remove it when you see it.

**Iterators** (1.23): `iter.Seq[V]` and `iter.Seq2[K,V]` are the
standard iterator types. Containers expose `.All()` methods returning
these. Combined with `slices.Collect`, `slices.Sorted`, `maps.Keys`,
etc., they replace ad-hoc "loop and append" code with composable,
lazy pipelines. When a sequence is consumed only once, prefer an
iterator over materializing a slice.

**Error trees** (1.20–1.26): Errors compose as trees, not chains.
`errors.Join` aggregates multiple errors. `fmt.Errorf` accepts multiple
`%w` verbs. `errors.Is`/`As` traverse the full tree. Custom error
types that wrap multiple causes must implement `Unwrap() []error` (the
slice form), not `Unwrap() error`, or tree traversal won't find the
children. `errors.AsType[T]` (1.26) is the type-safe way to match
error types. Propagate cancellation reasons with
`context.WithCancelCause`.

**Structured logging** (1.21): `log/slog` is the standard structured
logger. This project uses `cdr.dev/slog/v3` instead, which has a
different API. Do not use `log/slog` directly.

## Replace these patterns

The left column reflects common patterns from pre-1.22 Go. Write the
right column instead. The "Since" column tells you the minimum `go`
directive version required in `go.mod`.

| Old pattern | Modern replacement | Since |
|---|---|---|
| `interface{}` | `any` | 1.18 |
| `v := v` inside loops | remove it | 1.22 |
| `for i := 0; i < n; i++` | `for i := range n` | 1.22 |
| `for i := 0; i < b.N; i++` (benchmarks) | `for b.Loop()` (correct timing, future-proof) | 1.24 |
| `sort.Slice(s, func(i,j int) bool{…})` | `slices.SortFunc(s, cmpFn)` | 1.21 |
| `wg.Add(1); go func(){ defer wg.Done(); … }()` | `wg.Go(func(){…})` | 1.25 |
| `func ptr[T any](v T) *T { return &v }` | `new(expr)` e.g. `new(time.Now())` | 1.26 |
| `var target *E; errors.As(err, &target)` | `t, ok := errors.AsType[*E](err)` | 1.26 |
| Custom multi-error type | `errors.Join(err1, err2, …)` | 1.20 |
| Single `%w` for multiple causes | `fmt.Errorf("…: %w, %w", e1, e2)` | 1.20 |
| `rand.Seed(time.Now().UnixNano())` | delete it (auto-seeded); prefer `math/rand/v2` | 1.20/1.22 |
| `sync.Once` + captured variable | `sync.OnceValue(func() T {…})` / `OnceValues` | 1.21 |
| Custom `min`/`max` helpers | `min(a, b)` / `max(a, b)` builtins (any ordered type) | 1.21 |
| `for k := range m { delete(m, k) }` | `clear(m)` (also zeroes slices) | 1.21 |
| Index+slice or `SplitN(s, sep, 2)` | `strings.Cut(s, sep)` / `bytes.Cut` | 1.18 |
| `TrimPrefix` + check if anything was trimmed | `strings.CutPrefix` / `CutSuffix` (returns ok bool) | 1.20 |
| `strings.Split` + loop when no slice is needed | `strings.SplitSeq` / `Lines` / `FieldsSeq` (iterator, no alloc) | 1.24 |
| `"2006-01-02"` / `"2006-01-02 15:04:05"` / `"15:04:05"` | `time.DateOnly` / `time.DateTime` / `time.TimeOnly` | 1.20 |
| Manual `Before`/`After`/`Equal` chains for comparison | `time.Time.Compare` (returns -1/0/+1; works with `slices.SortFunc`) | 1.20 |
| Loop collecting map keys into slice | `slices.Sorted(maps.Keys(m))` | 1.23 |
| `fmt.Sprintf` + append to `[]byte` | `fmt.Appendf(buf, …)` (also `Append`, `Appendln`) | 1.18 |
| `reflect.TypeOf((*T)(nil)).Elem()` | `reflect.TypeFor[T]()` | 1.22 |
| `*(*[4]byte)(slice)` unsafe cast | `[4]byte(slice)` direct conversion | 1.20 |
| `atomic.LoadInt64` / `StoreInt64` | `atomic.Int64` (also `Bool`, `Uint64`, `Pointer[T]`) | 1.19 |
| `crypto/rand.Read(buf)` + hex/base64 encode | `crypto/rand.Text()` (one call) | 1.24 |
| Checking `crypto/rand.Read` error | don't: return is always nil | 1.24 |
| `time.Sleep` in tests | `testing/synctest` (deterministic fake clock) | 1.24/1.25 |
| `json:",omitempty"` on zero-value structs like `time.Time{}` | `json:",omitzero"` (uses `IsZero()` method) | 1.24 |
| `strings.Title` | `golang.org/x/text/cases` | 1.18 |
| `net.IP` in new code | `net/netip.Addr` (immutable, comparable, lighter) | 1.18 |
| `tools.go` with blank imports | `tool` directive in `go.mod` | 1.24 |
| `runtime.SetFinalizer` | `runtime.AddCleanup` (multiple per object, no pointer cycles) | 1.24 |
| `httputil.ReverseProxy.Director` | `.Rewrite` hook + `ProxyRequest` (Director deprecated in 1.26) | 1.20 |
| `sql.NullString`, `sql.NullInt64`, etc. | `sql.Null[T]` | 1.22 |
| Manual `ctx, cancel := context.WithCancel(…)` + `t.Cleanup(cancel)` | `t.Context()` (auto-canceled when test ends) | 1.24 |
| `if d < 0 { d = -d }` on durations | `d.Abs()` (handles `math.MinInt64`) | 1.19 |
| Implement only `TextMarshaler` | also implement `TextAppender` for alloc-free marshaling | 1.24 |
| Custom `Unwrap() error` on multi-cause errors | `Unwrap() []error` (slice form; required for tree traversal) | 1.20 |

## New capabilities

These enable things that weren't practical before. Reach for them in the
described situations.

| What | Since | When to use it |
|---|---|---|
| `cmp.Or(a, b, c)` | 1.22 | Defaults/fallback chains: returns first non-zero value. Replaces verbose `if a != "" { return a }` cascades. |
| `context.WithoutCancel(ctx)` | 1.21 | Background work that must outlive the request (e.g. async cleanup after HTTP response). Derived context keeps parent's values but ignores cancellation. |
| `context.AfterFunc(ctx, fn)` | 1.21 | Register cleanup that fires on context cancellation without spawning a goroutine that blocks on `<-ctx.Done()`. |
| `context.WithCancelCause` / `Cause` | 1.20 | When callers need to know WHY a context was canceled, not just that it was. Retrieve cause with `context.Cause(ctx)`. |
| `context.WithDeadlineCause` / `WithTimeoutCause` | 1.21 | Attach a domain-specific error to deadline/timeout expiry (e.g. distinguish "DB query timed out" from "HTTP request timed out"). |
| `errors.ErrUnsupported` | 1.21 | Standard sentinel for "not supported." Use instead of per-package custom sentinels. Check with `errors.Is`. |
| `http.ResponseController` | 1.20 | Per-request flush, hijack, and deadline control without type-asserting `ResponseWriter` to `http.Flusher` or `http.Hijacker`. |
| Enhanced `ServeMux` routing | 1.22 | `"GET /items/{id}"` patterns in `http.ServeMux`. Access with `r.PathValue("id")`. Wildcards: `{name}`, catch-all: `{path...}`, exact: `{$}`. Eliminates many third-party router dependencies. |
| `os.Root` / `OpenRoot` | 1.24 | Confined directory access that prevents symlink escape. 1.25 adds `MkdirAll`, `ReadFile`, `WriteFile` for real use. |
| `os.CopyFS` | 1.23 | Copy an entire `fs.FS` to local filesystem in one call. |
| `os/signal.NotifyContext` with cause | 1.26 | Cancellation cause identifies which signal (SIGTERM vs SIGINT) triggered shutdown. |
| `io/fs.SkipAll` / `filepath.SkipAll` | 1.20 | Return from `WalkDir` callback to stop walking entirely. Cleaner than a sentinel error. |
| `GOMEMLIMIT` env / `debug.SetMemoryLimit` | 1.19 | Soft memory limit for GC. Use alongside or instead of `GOGC` in memory-constrained containers. |
| `net/url.JoinPath` | 1.19 | Join URL path segments correctly. Replaces error-prone string concatenation. |
| `go test -skip` | 1.20 | Skip tests matching a pattern. Useful when running a subset of a large test suite. |

## Key packages

### `slices` (1.21, iterators added 1.23)

Replaces `sort.Slice`, manual search loops, and manual contains checks.

Search: `Contains`, `ContainsFunc`, `Index`, `IndexFunc`,
`BinarySearch`, `BinarySearchFunc`.

Sort: `Sort`, `SortFunc`, `SortStableFunc`, `IsSorted`, `IsSortedFunc`,
`Min`, `MinFunc`, `Max`, `MaxFunc`.

Transform: `Clone`, `Compact`, `CompactFunc`, `Grow`, `Clip`,
`Concat` (1.22), `Repeat` (1.23), `Reverse`, `Insert`, `Delete`,
`Replace`.

Compare: `Equal`, `EqualFunc`, `Compare`.

Iterators (1.23): `All`, `Values`, `Backward`, `Collect`, `AppendSeq`,
`Sorted`, `SortedFunc`, `SortedStableFunc`, `Chunk`.

### `maps` (1.21, iterators added 1.23)

Core: `Clone`, `Copy`, `Equal`, `EqualFunc`, `DeleteFunc`.

Iterators (1.23): `All`, `Keys`, `Values`, `Insert`, `Collect`.

### `cmp` (1.21, `Or` added 1.22)

`Ordered` constraint for any ordered type. `Compare(a, b)` returns
-1/0/+1. `Less(a, b)` returns bool. `Or(vals...)` returns first
non-zero value.

### `iter` (1.23)

`Seq[V]` is `func(yield func(V) bool)`. `Seq2[K,V]` is
`func(yield func(K, V) bool)`. Return these from your container's
`.All()` methods. Consume with `for v := range seq` or pass to
`slices.Collect`, `slices.Sorted`, `maps.Collect`, etc.

### `math/rand/v2` (1.22)

Replaces `math/rand`. `IntN` not `Intn`. Generic `N[T]()` for any
integer type. Default source is `ChaCha8` (crypto-quality). No global
`Seed`. Use `rand.New(source)` for reproducible sequences.

### `log/slog` (1.21)

`slog.Info`, `slog.Warn`, `slog.Error`, `slog.Debug` with key-value
pairs. `slog.With(attrs...)` for logger with preset fields.
`slog.GroupAttrs` (1.25) for clean group creation. Implement
`slog.Handler` for custom backends.

**Note:** This project uses `cdr.dev/slog/v3`, not `log/slog`. The
API is different. Read existing code for usage patterns.

## Pitfalls

Things that are easy to get wrong, even when you know the modern API
exists. Check your output against these.

**Version misuse.** The replacement table has a "Since" column. If the
project's `go.mod` says `go 1.22`, you cannot use `wg.Go` (1.25),
`errors.AsType` (1.26), `new(expr)` (1.26), `b.Loop()` (1.24), or
`testing/synctest` (1.24). Fall back to the older pattern. Always
check before reaching for a replacement.

**`slices.Sort` vs `slices.SortFunc`.** `slices.Sort` requires
`cmp.Ordered` types (int, string, float64, etc.). For structs, custom
types, or multi-field sorting, use `slices.SortFunc` with a comparator
function. Using `slices.Sort` on a non-ordered type is a compile error.

**`for range n` still binds the index.** `for range n` discards the
index. If you need it, write `for i := range n`. Writing
`for range n` and then trying to use `i` inside the loop is a compile
error.

**Don't hand-roll iterators when the stdlib returns one.** Functions
like `maps.Keys`, `slices.Values`, `strings.SplitSeq`, and
`strings.Lines` already return `iter.Seq` or `iter.Seq2`. Don't
reimplement them. Compose with `slices.Collect`, `slices.Sorted`, etc.

**Don't mix `math/rand` and `math/rand/v2`.** They have different
function names (`Intn` vs `IntN`) and different default sources. Pick
one per package. Prefer v2 for new code. The v1 global source is
auto-seeded since 1.20, so delete `rand.Seed` calls either way.

**Iterator protocol.** When implementing `iter.Seq`, you must respect
the `yield` return value. If `yield` returns `false`, stop iteration
immediately and return. Ignoring it violates the contract and causes
panics when consumers break out of `for range` loops early.

**`errors.Join` with nil.** `errors.Join` skips nil arguments. This is
intentional and useful for aggregating optional errors, but don't
assume the result is always non-nil. `errors.Join(nil, nil)` returns
nil.

**`cmp.Or` evaluates all arguments.** Unlike a chain of `if`
statements, `cmp.Or(a(), b(), c())` calls all three functions. If any
have side effects or are expensive, use `if`/`else` instead.

**Timer channel semantics changed in 1.23.** Code that checks
`len(timer.C)` to see if a value is pending no longer works (channel
capacity is 0). Use a non-blocking `select` receive instead:
`select { case <-timer.C: default: }`.

**`context.WithoutCancel` still propagates values.** The derived
context inherits all values from the parent. If any middleware stores
request-scoped state (deadlines, trace IDs) via `context.WithValue`,
the background work sees it. This is usually desired but can be
surprising if the values hold references that should not outlive the
request.

## Behavioral changes that affect code

- **Timers** (1.23): unstopped `Timer`/`Ticker` are GC'd immediately.
  Channels are unbuffered: no stale values after `Reset`/`Stop`. You no
  longer need `defer t.Stop()` to prevent leaks.
- **Error tree traversal** (1.20): `errors.Is`/`As` follow
  `Unwrap() []error`, not just `Unwrap() error`. Multi-error types must
  expose the slice form for child errors to be found.
- **`math/rand` auto-seeded** (1.20): the global RNG is auto-seeded.
  `rand.Seed` is a no-op in 1.24+. Don't call it.
- **GODEBUG compat** (1.21): behavioral changes are gated by `go.mod`'s
  `go` line. Upgrading the version opts into new defaults.
- **Build tags** (1.18): `//go:build` is the only syntax. `// +build`
  is gone.
- **Tool install** (1.18): `go get` no longer builds. Use
  `go install pkg@version`.
- **Doc comments** (1.19): support `[links]`, lists, and headings.
- **`go test -skip`** (1.20): skip tests by name pattern from the
  command line.
- **`go fix ./...` modernizers** (1.26): auto-rewrites code to use
  newer idioms. Run after Go version upgrades.

## Transparent improvements (no code changes)

Swiss Tables maps, Green Tea GC, PGO, faster `io.ReadAll`,
stack-allocated slices, reduced cgo overhead, container-aware
GOMAXPROCS. Free on upgrade.