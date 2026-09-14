# Observability Guide for Agents

This guide maps the observability surfaces that already exist in local
Coder development. Do not add new endpoints for agent debugging. Prefer the
existing logs, tracing, Prometheus metrics, browser artifacts, and command
output described here.

## Start the app

Use `./scripts/develop.sh` for local development. See
[Development Workflows and Guidelines](WORKFLOWS.md) for the full workflow.
The script builds the dev orchestrator, starts the API server and frontend,
waits for the API server to answer `/healthz`, creates the first user if
needed, and prints a banner with the local URLs.

Useful defaults from `scripts/develop/main.go` are:

- API server: `http://localhost:3000`.
- Frontend dev server: `http://localhost:8080`.
- Workspace proxy, when `--use-proxy` is set: `http://localhost:3010`.
- Coder Prometheus metrics: `http://localhost:2114/`.
- Embedded Prometheus UI, when `--prometheus-server` is set and Docker is
  available on Linux: `http://localhost:9090`.

## Local logs

`./scripts/develop.sh` writes orchestrator and child process logs to the
terminal. The orchestrator uses `sloghuman`, and each child process is logged
under a named logger such as `api`, `site`, `proxy`, `ext-provisioner`, or
`prometheus`.

HTTP request logging is implemented in `coderd/httpmw/loggermw`. Request log
fields include `user_agent`, `host`, the effective trust-aware host,
`received_host`, the raw received Host header, `path`, `proto`,
`remote_addr`, `start`, `status_code`, `latency_ms`, route params, and
selected safe query params.
Responses with status codes of 500 or higher include the response body in the
request log. Successful `GET /api/v2` requests are skipped.

When investigating failures, keep the full terminal output from
`./scripts/develop.sh`. If you ran a command through Mux or another harness,
record the command, exit code, and artifact path for the captured output.

## Tracing

HTTP tracing lives in `coderd/tracing`. The middleware covers `/api`,
`/api/**`, workspace app routes, and external auth callback routes. When an
active trace span exists, responses include `X-Trace-ID`, `X-Span-ID`, and a
W3C `traceparent` header.

Tracing export is controlled by existing server flags and environment
variables, not by the develop orchestrator itself:

- `--trace` or `CODER_TRACE_ENABLE` enables application tracing.
- `--trace-logs` or `CODER_TRACE_LOGS` adds log events to traces.
- `--trace-honeycomb-api-key` or `CODER_TRACE_HONEYCOMB_API_KEY` enables the
  Honeycomb exporter.
- `--trace-datadog` or `CODER_TRACE_DATADOG` enables sending Go runtime
  traces to the local DataDog agent.

To pass server flags through the develop script, put them after `--`. For
example, use `./scripts/develop.sh -- --trace` when you already have an OTLP
backend configured through the standard OpenTelemetry environment variables.

## Prometheus metrics

`./scripts/develop.sh` enables Coder Prometheus metrics by default on
`0.0.0.0:2114`, served at `http://localhost:2114/`. The port is controlled by
`--prometheus-port` or `CODER_DEV_PROMETHEUS_PORT`. Set it to `0` to disable
metrics. The develop script passes these existing server flags when metrics are
enabled: `--prometheus-enable`, `--prometheus-address`,
`--prometheus-collect-agent-stats`, and `--prometheus-collect-db-metrics`.

If `--prometheus-server` or `CODER_DEV_PROMETHEUS_SERVER` is set, the develop
script attempts to start a Docker container named `coder-prometheus` on Linux.
The Prometheus UI listens on `http://localhost:9090`. If a previous container
is reused, confirm the scrape target because it may point at an older metrics
port.

Relevant metric implementations include:

- `coderd/httpmw/prometheus.go` for HTTP request counters, concurrency gauges,
  websocket gauges, and latency histograms.
- `coderd/prometheusmetrics/` for active users, workspaces, agents, build
  info, experiments, insights, and agent stats collectors.
- `coderd/database/dbmetrics/` for database query and transaction metrics.
- `docs/admin/integrations/prometheus.md` for the user-facing Prometheus
  integration guide and metric reference.

## Correlating a failed action

Use this sequence when a browser or API action fails:

1. Record the local clock time, browser action, URL, HTTP method, and response
   status from the browser network panel or test output.
2. If the response includes `X-Trace-ID` or `X-Span-ID`, copy both values. If
   not, copy the `traceparent` header if present.
3. Search the `./scripts/develop.sh` terminal output for the route, method,
   status code, response body, or timestamp. Match fields such as `path`,
   `status_code`, and `latency_ms`.
4. Check `http://localhost:2114/` for metrics that match the route or subsystem.
   Start with `coderd_api_requests_processed_total`,
   `coderd_api_request_latencies_seconds`, and database metrics under the
   `coderd_db_` prefix.
5. Attach the browser screenshot, trace, video, or command output artifact to
   the failure report when the harness produced one.

## If an API request fails

- Capture method, URL, status code, response body, and response headers.
- Check the API log line for matching `path`, `status_code`, and `latency_ms`.
- If the status is 500 or higher, include the logged response body.
- Check `coderd_api_requests_processed_total` and
  `coderd_api_request_latencies_seconds` for the matching route.
- If database work is involved, check `coderd_db_query_counts_total`,
  `coderd_db_query_latencies_seconds`, and transaction metrics.

## If the frontend hangs

- Confirm that the develop banner printed both the API and Web UI URLs.
- Check the `site` logger output for Vite errors and dependency failures.
- Use the browser network panel to separate frontend asset failures from API
  failures.
- If API calls are pending or failing, follow the API request checklist above.
- Capture browser console output and screenshots before retrying.

## If a workspace provision fails

- Capture the workspace build ID, template name, workspace name, user, and
  action that triggered the build.
- Search logs for `provisioner`, `workspace`, `build`, and the workspace build
  ID.
- Check whether `ext-provisioner` is running in the develop output.
- Review metrics for API request failures, database latency, and agent stats if
  the failure reaches agent startup.
- Preserve provisioner logs, template files, command output, and any browser
  artifacts from the failed flow.

## Failure report checklist

Include these details in every observability failure report:

- Absolute timestamp with timezone and the local command that was running.
- Git branch, commit SHA, and whether generated files were fresh.
- Browser action, API method, URL, route, status code, and response body.
- `X-Trace-ID`, `X-Span-ID`, or `traceparent` when present.
- Relevant log lines with nearby context.
- Prometheus metrics checked and the observed values or absence of values.
- Artifact paths for screenshots, traces, videos, logs, and command output.
- Any cleanup performed before reproducing the failure again.
