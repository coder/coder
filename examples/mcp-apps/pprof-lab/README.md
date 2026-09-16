# pprof lab

A small Go service that exposes `net/http/pprof` next to a control panel
whose buttons start and stop pathological workloads. Each workload is
built so its signature is obvious in one specific profile, which makes the
lab a safe target for learning pprof or for exercising a profile explorer
such as the `pprof-explorer` MCP App in this directory.

It is a standalone module with no dependencies beyond the standard
library. It is not part of the repository's root `go.mod` and is not
covered by `make lint` or `make test`.

## Run

```sh
cd examples/mcp-apps/pprof-lab
go run .
```

The server listens on `http://127.0.0.1:6060/` by default. Pass `-addr` to
change it, for example `go run . -addr :6060` to listen on every interface
inside a container. `Ctrl-C` or `SIGTERM` drains the HTTP server and stops
every scenario.

Checks:

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

## HTTP surface

| Route                              | Purpose                                                                     |
|------------------------------------|-----------------------------------------------------------------------------|
| `GET /`                            | Control panel                                                               |
| `GET /api/status`                  | Scenario states, `runtime.NumGoroutine`, `MemStats` subset, uptime, Go version |
| `POST /api/scenarios/{name}/start` | Body: JSON object of knobs. Restarts the scenario if it is already running |
| `POST /api/scenarios/{name}/stop`  | Stops the scenario and releases its goroutines and memory                  |
| `POST /api/reset`                  | Stops every scenario, then runs `debug.FreeOSMemory()` (which includes a full GC) |
| `GET /debug/pprof/`                | `net/http/pprof` index and the usual named profiles                        |

Mutating routes return `415` without `Content-Type: application/json`,
`403` when the browser sends `Sec-Fetch-Site: cross-site`, `404` for an
unknown scenario, and `400` for a knob that is missing from the scenario,
not an integer, or outside its range.

Starting a scenario that is already running stops the current run first
and starts a new one with the new knobs. Repeating a request with the same
knobs is therefore harmless; changing a knob takes effect immediately.

Every scenario goroutine runs under `pprof.Do` with the label
`scenario=<name>`, so goroutine and CPU samples can be filtered by
scenario.

## Scenarios

| Scenario           | Knobs (default / max)                              | Hot function                          | Look at                                              |
|--------------------|----------------------------------------------------|---------------------------------------|------------------------------------------------------|
| `goroutine-leak`   | `count` 500 / 20000                                | `(*goroutineLeak).parkedWorker`       | `goroutine`: N goroutines parked in `select`         |
| `heap-growth`      | `mib_per_second` 4 / 64, `cap_mib` 512 / 1024      | `(*heapGrowth).retainBlob`            | `heap` `inuse_space` and `inuse_objects`             |
| `alloc-churn`      | `workers` 4 / 32                                   | `(*allocChurn).allocateAndDrop`       | `allocs` `alloc_space`; GC time in the CPU profile   |
| `cpu-burn`         | `workers` GOMAXPROCS / 64                          | `(*cpuBurn).hotLoop`                  | `profile` (CPU)                                      |
| `mutex-contention` | `goroutines` 64 / 1024                             | `(*mutexContention).holdAndRelease`   | `mutex`: delay at the `Unlock` call                  |
| `block-wait`       | `goroutines` 64 / 1024                             | `(*blockWait).waitForWake`            | `block`: channel receive wait time                   |

`heap-growth` retains 1 KiB `Record` structs in a slice at the configured
rate and stops growing at `cap_mib`. The records stay allocated until the
scenario is stopped or the lab is reset, so there is time to capture the
profile. The hot functions are marked `//go:noinline` so they always appear
as their own frame.

## Profile mechanics worth knowing

- The `mutex` and `block` profiles are cumulative since process start.
  Stopping a scenario or calling `/api/reset` does not clear them. To
  isolate a window, capture a baseline before starting the scenario and
  diff against it (for example `go tool pprof -base`, or the explorer's
  `diff_profiles` tool).
- The mutex profile only has data because the lab calls
  `runtime.SetMutexProfileFraction(5)` at startup, and the block profile
  because of `runtime.SetBlockProfileRate(100000)` (100 microseconds). A
  mutex event is recorded at the `Unlock` that ends the contention, and a
  block event is recorded when the goroutine is unblocked. Permanently
  parked goroutines therefore never show up in the block profile, which is
  why `block-wait` cycles its waiters instead of parking them.
- Heap profile data is at most as fresh as the last completed GC. Request
  `/debug/pprof/heap?gc=1` to force a collection first, otherwise
  `inuse_space` can lag the live strip by a cycle or two.
- `runtime.MemProfileRate` defaults to one sample per 512 KiB allocated, so
  `inuse_objects` and `alloc_objects` are scaled estimates. They are
  accurate in aggregate for `heap-growth` (thousands of identical objects)
  but individual small counts are noisy.
- The CPU profile (`/debug/pprof/profile?seconds=N`) blocks for `N`
  seconds (default 30) and only one capture can run at a time; a second
  concurrent request gets a `500`. The server's `WriteTimeout` is 60s;
  `net/http/pprof` extends the write deadline by `seconds` and rejects a
  `profile` or `trace` request whose `seconds` is longer than that, so a
  single capture is capped at 60s.

## Security posture

The lab is intentionally self-harmful: it exists to leak goroutines, grow
the heap, and burn CPU on request. It has no authentication, the same
posture as `net/http/pprof` in a development process. Keep it inside the
workspace:

- The default bind address is loopback. Only bind to other interfaces when
  the port is reachable solely from inside the workspace or its container.
- Do not publish it as a public workspace app or forward it beyond the
  workspace.
- Every knob has a hard maximum and `/api/reset` returns the process to a
  clean baseline.
- Mutating routes require `Content-Type: application/json`, which a
  cross-origin page cannot send without a CORS preflight that this server
  never answers, and they reject requests that carry
  `Sec-Fetch-Site: cross-site`. Together these stop a page on another
  origin from firing simple POSTs at the lab through a port-forward URL.
- The `GET /debug/pprof/*` routes have no such guard, because
  `net/http/pprof` handlers are plain GETs. A cross-site page can still
  trigger them: `heap?gc=1` and `allocs?gc=1` force a garbage collection,
  and `profile?seconds=N` runs the CPU profiler for `N` seconds (capped at
  60 by the server's write timeout, and only one at a time). This is the
  standard exposure of `net/http/pprof` and another reason to keep the
  port inside the workspace.

## Pairing with pprof-explorer

`pprof-explorer` (in the sibling directory) is an MCP server plus MCP App
view that captures profiles from a single target URL and lets a human and
an agent explore the same snapshots. Point it at this lab with
`PPROF_TARGET=http://127.0.0.1:6060`, start a scenario here, and capture
the matching profile there. A typical loop:

1. `POST /api/reset`, then capture a baseline `heap` (or `mutex`, `block`).
2. Start a scenario from the control panel.
3. Capture the same profile kind again and diff it against the baseline.
4. Stop the scenario or reset, and confirm the diff collapses.
