# Prometheus

Coder has support for Prometheus metrics using the dedicated [Go client library](github.com/prometheus/client_golang). The library exposes various [metrics types](https://prometheus.io/docs/concepts/metric_types/), such as gauges, histograms, and timers, that give insight into the live Coder deployment.

Feel free to browse through the [Getting started](https://prometheus.io/docs/prometheus/latest/getting_started/) guide, if you don't have an installation of the Prometheus server.

## Enable Prometheus metrics

Coder server exports metrics via the HTTP endpoint, which can be enabled using either the environment variable `CODER_PROMETHEUS_ENABLE` or the flag` --prometheus-enable`.

Use either the environment variable `CODER_PROMETHEUS_ADDRESS` or the flag ` --prometheus-address <network-interface>:<port>` to select a custom endpoint.

Once the `code server --prometheus-enable` is started, you can preview the metrics endpoint: <!-- markdown-link-check-disable -->http://localhost:2112/<!-- markdown-link-check-enable --> (default endpoint).

```
# HELP coderd_api_active_users_duration_hour The number of users that have been active within the last hour.
# TYPE coderd_api_active_users_duration_hour gauge
coderd_api_active_users_duration_hour 0
# HELP coderd_api_concurrent_requests The number of concurrent API requests
# TYPE coderd_api_concurrent_requests gauge
coderd_api_concurrent_requests 2
# HELP coderd_api_concurrent_websockets The total number of concurrent API websockets
# TYPE coderd_api_concurrent_websockets gauge
coderd_api_concurrent_websockets 1
# HELP coderd_api_request_latencies_ms Latency distribution of requests in milliseconds
# TYPE coderd_api_request_latencies_ms histogram
coderd_api_request_latencies_ms_bucket{method="GET",path="",le="1"} 10
coderd_api_request_latencies_ms_bucket{method="GET",path="",le="5"} 13
coderd_api_request_latencies_ms_bucket{method="GET",path="",le="10"} 14
coderd_api_request_latencies_ms_bucket{method="GET",path="",le="25"} 15
...
```

## Available collectors

### Coderd

[Coderd](../about/architecture.md#coderd) is the service responsible for managing workspaces, provisioners, and users. Coder resources are controlled using the authorized HTTP API - Coderd API.

The Prometheus collector tracks and exposes activity statistics for [platform users](https://github.com/coder/coder/blob/main/coderd/prometheusmetrics/prometheusmetrics.go#L15-L54) and [workspace](https://github.com/coder/coder/blob/main/coderd/prometheusmetrics/prometheusmetrics.go#L57-L108).

It also exposes [operational metrics](https://github.com/coder/coder/blob/main/coderd/httpmw/prometheus.go#L21-L61) for HTTP requests and WebSocket connections, including a total number of calls, HTTP status, active WebSockets, request duration, etc.

### Provisionerd

[Provisionerd](../about/architecture.md#provisionerd) is the execution context for infrastructure providers. The runner exposes [statistics for executed jobs](https://github.com/coder/coder/blob/main/provisionerd/provisionerd.go#L133-L154) - a number of jobs currently running, and execution timings.

### Go runtime, process stats

[Common collectors](https://github.com/coder/coder/blob/main/cli/server.go#L555-L556) monitor the Go runtime - memory usage, garbage collection, active threads, goroutines, etc. Additionally, on Linux and on Windows, they collect CPU stats, memory, file descriptors, and process uptime.
