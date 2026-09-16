# pprof explorer MCP App example

An MCP server that captures Go `net/http/pprof` profiles from one fixed
target, keeps them as in-memory snapshots, and exposes them both as tools the
model can call and as an MCP App view (MCP Apps extension, protocol version
2026-01-26) for the Coder Agents side panel. Human and agent explore the same
snapshots by `profile_id`, and every row in the view has an Explain button
that hands the agent exactly what the user is looking at.

It pairs with `examples/mcp-apps/pprof-lab`, a Go service whose scenarios
produce leaks, allocation churn, CPU burn, and lock contention on demand, but
it works against any Go process that serves `/debug/pprof/`.

## Run

```sh
cd examples/mcp-apps/pprof-explorer
pnpm install
PPROF_TARGET=http://127.0.0.1:6060 pnpm start
```

The server listens on `http://127.0.0.1:3334/mcp` with stateless Streamable
HTTP: every request gets a fresh MCP server instance while the snapshot store
is shared at module scope.

| Variable        | Default                 | Purpose                                                        |
|-----------------|-------------------------|----------------------------------------------------------------|
| `PORT`          | `3334`                  | Listen port                                                    |
| `HOST`          | `127.0.0.1`             | Listen address (`0.0.0.0` when coderd runs in another container) |
| `PPROF_TARGET`  | `http://127.0.0.1:6060` | The only URL the server ever fetches; must be http or https with a host |
| `ALLOWED_HOSTS` | (empty)                 | Extra comma-separated hostnames accepted in the `Host` header, needed when a wildcard bind is reached by container or LAN address |

Requests whose `Host` header names anything other than `localhost`,
`127.0.0.1`, `[::1]`, the bound `HOST`, or an `ALLOWED_HOSTS` entry are
answered with 403 (DNS rebinding protection from `@modelcontextprotocol/node`).

`pnpm check` runs `tsc --noEmit`. `pnpm test` runs the Node test runner over
`src/*.test.ts` and `src/*.test.mjs` through `tsx`; the server tests stand up
a local HTTP fixture that serves encoded profiles, so no Go toolchain is
needed.

## Tools

Every tool carries `_meta.ui.resourceUri = "ui://pprof/explorer"` and returns
a compact text summary for the model plus `structuredContent` with a `view`
discriminator for the panel. Row-returning tools cap `limit` at 200 and trim
rows further until the serialized result fits in 56 KiB, setting
`truncated: true`.

| Tool               | Purpose                                                                                     |
|--------------------|---------------------------------------------------------------------------------------------|
| `capture_profile`  | Fetch `heap`, `allocs`, `goroutine`, `goroutineleak`, `profile` (CPU, `seconds` 1..30), `block`, or `mutex` and store it as `p1`, `p2`, ... Heap and allocs send `gc=1` unless `gc: false`. |
| `list_profiles`    | Snapshots with kind, capture time, label, sample types, and totals.                         |
| `top`              | Functions ranked by `flat` or `cum` for one `sample_type`, optional `focus` regex, `limit`. |
| `callers_callees`  | One function's flat/cum with weighted caller and callee edges (`pprof -peek` style).        |
| `diff_profiles`    | Signed per-function deltas between two snapshots of the same kind, sorted by absolute flat delta. |
| `goroutine_groups` | Goroutines grouped by identical stack and label set, with pprof labels per group; goroutine snapshots only. |
| `drop_profile`     | Removes a snapshot. App-only (`visibility: ["app"]`), used by the view's trash icon.        |

Workflow the tool descriptions teach the model: capture first, inspect by
`profile_id`, use `diff_profiles` for leaks (capture, reproduce, capture,
diff), and `callers_callees` to see who reaches a hot function. `block` and
`mutex` profiles are cumulative since process start, so two captures and a
diff isolate a time window.

Aggregation mirrors pprof's graph construction: locations are walked root
first, inlined frames inside a location are expanded (index 0 is the innermost
callee and receives the flat value), cum is added once per function per sample
so recursion is not double counted, and caller/callee edges are deduplicated
per sample. Sample types are read from the profile (`defaultSampleType`, else
the last declared type), never hardcoded.

## Register in Coder

1. Start Coder with the `chat-mcp-apps` experiment and a wildcard access URL:

   ```sh
   CODER_EXPERIMENTS=chat-mcp-apps CODER_WILDCARD_ACCESS_URL='*.localhost' ./scripts/develop.sh
   ```

2. In the dashboard go to Deployment > MCP servers and add a server with
   transport `streamable_http` and URL `http://localhost:3334/mcp`. The URL
   must be reachable from coderd, not from your browser.

## Try it

Run `pprof-lab`, then in a chat: "capture a heap profile". The explorer opens
in the side panel with the snapshot selected. Start the `heap-growth`
scenario in the lab UI, click the heap button in the panel to capture again,
set the first snapshot as base, and switch to Diff. Press Explain on the top
delta row: the agent receives the row's data and the snapshot ids, calls
`callers_callees` and `diff_profiles` itself, and explains what is holding the
memory.

## Trust model

- The target is fixed at startup. No tool or view input can change it,
  redirects are not followed, profile bodies are capped at 16 MiB both on the
  wire and after gunzip (`maxOutputLength`), and the optional `/api/status`
  read is capped at 8 KiB and summarized into at most 400 characters. Its
  text is labelled as untrusted in tool output.
- Profile strings are controlled by the profiled process. Aggregation keys use
  the raw string table so distinct functions stay distinct; every emitted
  string is normalized in one place (`src/sanitize.ts`): control characters
  and newlines are removed, function names are capped at 512 characters,
  file paths, labels, and other strings at 160. File paths are reduced to
  `dir/file.go`. Error bodies from the target are capped at 1024 characters.
- `focus` patterns follow a bounded backtracking policy rather than a proof:
  at most 128 characters and 3 quantifiers, no quantifier on a group
  (`(ab)+`), no lookaround, no backreferences. Rejections suggest a plain
  substring or a simple pattern. Accepted patterns run against strings
  already capped at 512 characters.
- The snapshot store is process-wide and unauthenticated: every chat attached
  to the server shares ids, contents, and LRU eviction (32 snapshots).
- CPU captures are serialized in-process because `net/http/pprof` rejects
  overlapping CPU profiles; the target's own error text is returned when a
  capture fails.
- The server binds to loopback by default and rejects unexpected `Host`
  headers. Like `net/http/pprof` itself, it should not be exposed outside the
  workspace.
