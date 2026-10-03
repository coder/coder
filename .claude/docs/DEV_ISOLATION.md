# Development Isolation Guide for Agents

This guide documents the local resources that the existing harness uses. It is
for avoiding collisions across worktrees and cleaning up after failed runs. Do
not add new readiness or debug endpoints for these workflows.

## Default local ports

`scripts/develop/main.go` defines these base defaults:

| Resource                 | Base default | Override                                         |
|--------------------------|--------------|--------------------------------------------------|
| API server               | `3000`       | `--port`, `CODER_DEV_PORT`                       |
| Frontend dev server      | `8080`       | `--web-port`, `CODER_DEV_WEB_PORT`               |
| Workspace proxy          | `3010`       | `--proxy-port`, `CODER_DEV_PROXY_PORT`           |
| Coder Prometheus metrics | `2114`       | `--prometheus-port`, `CODER_DEV_PROMETHEUS_PORT` |
| Embedded Prometheus UI   | `9090`       | Fixed in `scripts/develop/main.go`               |
| Delve debugger           | `12345`      | Fixed when `--debug` is used                     |

By default, plain `./scripts/develop.sh` uses the base defaults exactly:
`3000`, `8080`, `3010`, and `2114` for Coder Prometheus metrics. Set
`--port-offset` or `CODER_DEV_PORT_OFFSET=true` to opt in to a deterministic
per-worktree offset for API, frontend, workspace proxy, and Coder Prometheus
metrics ports.

When enabled, the develop script hashes the project root with FNV-64a, maps it
into one of 50 buckets, multiplies by 20, and adds that value to each unset base
default. The same worktree path always gets the same effective ports. A flag or
environment variable overrides only that port. Other unset ports still receive
the opt-in offset. The workspace proxy is only started when `--use-proxy` is
set. The embedded Prometheus UI is only started when `--prometheus-server` or
`CODER_DEV_PROMETHEUS_SERVER` is set, Docker is available, and the host is
Linux. The Prometheus UI port `9090` and Delve port `12345` remain hardcoded.

## Other useful develop flags and environment variables

The develop script also supports these existing flags and environment
variables:

| Purpose                           | Flag                 | Environment variable         |
|-----------------------------------|----------------------|------------------------------|
| Per-worktree port offset          | `--port-offset`      | `CODER_DEV_PORT_OFFSET`      |
| Access URL                        | `--access-url`       | `CODER_DEV_ACCESS_URL`       |
| Admin password                    | `--password`         | `CODER_DEV_ADMIN_PASSWORD`   |
| Starter template                  | `--starter-template` | `CODER_DEV_STARTER_TEMPLATE` |
| Roll back missing migrations      | `--db-rollback`      | `CODER_DEV_DB_ROLLBACK`      |
| Reset the development database    | `--db-reset`         | `CODER_DEV_DB_RESET`         |
| Accept changed migration tracking | `--db-continue`      | `CODER_DEV_DB_CONTINUE`      |

Extra `coder server` flags can be passed after `--`. For example,
`./scripts/develop.sh -- --trace` passes `--trace` to the API server.

## Multi-worktree guidance

Each worktree gets its own `.coderv2` directory because `scripts/develop.sh`
sets the global config directory to `<project-root>/.coderv2`. This isolates
built-in Postgres data, local session data, and Prometheus container storage on
disk.

The configurable develop ports use canonical defaults unless you opt in with
`--port-offset` or `CODER_DEV_PORT_OFFSET=true`. Enable the offset when running
multiple worktrees in parallel and you want most concurrent runs to avoid manual
port selection. When the offset is enabled, the startup banner prints the
effective API, web, proxy, and Coder metrics ports with their offset status.

Use overrides when you need fixed ports or when two worktree paths hash to the
same offset. For example:

```sh
CODER_DEV_PORT=3100 \
CODER_DEV_WEB_PORT=8180 \
CODER_DEV_PROXY_PORT=3110 \
CODER_DEV_PROMETHEUS_PORT=2214 \
./scripts/develop.sh --use-proxy
```

If you also need the embedded Prometheus UI in more than one worktree, use only
one at a time. The UI port is fixed at `9090`, and the Docker container name is
fixed to `coder-prometheus`. Delve is fixed at `127.0.0.1:12345` when `--debug`
is used.

## Known collision risks

- Two worktree paths can hash to the same opt-in offset. If preflight reports a
  busy effective port, set the relevant `CODER_DEV_*` environment variables or
  flags for one worktree.
- The embedded Prometheus UI always uses port `9090`.
- The embedded Prometheus Docker container name is always `coder-prometheus`.
- The Delve debugger always listens on `127.0.0.1:12345` when `--debug` is
  used.
- The develop script only checks the proxy port when `--use-proxy` is set, so
  a stale process on the effective proxy port can go unnoticed until the proxy
  is enabled.
- External databases configured through `CODER_PG_CONNECTION_URL` are shared if
  multiple worktrees point at the same database.

## Readiness without new probes

Do not invent a new readiness probe. The develop script already waits for the
API server to answer `GET /healthz` for up to 60 seconds, then logs `server is
ready to accept connections`. After setup completes, it prints a banner with
`Coder is now running in development mode`, the effective port list, and the API
and Web UI URLs.

For agent-driven runs, treat the banner as the ready signal for browser work.
If the banner does not appear, inspect the preceding `api`, `site`, database
recovery, and port conflict logs.

## Cleanup

Use the least destructive cleanup that fixes the problem:

1. Stop `./scripts/develop.sh` with `Ctrl+C` so child processes receive the
   orchestrator shutdown signal.
2. If a child process remains, identify it with `lsof -iTCP:<port> -sTCP:LISTEN`
   or `ps`, then terminate only that stale process.
3. To reset the built-in development database for the current worktree, rerun
   with `./scripts/develop.sh --db-reset` or remove `.coderv2/postgres` after
   stopping the app.
4. To clear local Coder session and generated state for the current worktree,
   remove the specific files under `.coderv2` that are relevant to the failure.
5. To clean the embedded Prometheus container, stop the develop script first,
   then remove the `coder-prometheus` container if it remains.
6. To clean test databases, prefer the owning test harness cleanup. If tests
   were interrupted, inspect the local PostgreSQL instance used by the test
   suite before dropping any database.

For database migration mismatches, prefer the develop script's recovery flags
before deleting state. Use `--db-rollback` when a migration disappeared from the
current branch, `--db-continue` after you manually reconcile changed migration
tracking, and `--db-reset` only when data loss is acceptable.
