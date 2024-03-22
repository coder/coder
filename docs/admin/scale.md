We scale-test Coder with [a built-in utility](#scale-testing-utility) that can
be used in your environment for insights into how Coder scales with your
infrastructure. For scale-testing Kubernetes clusters we recommend to install
and use the dedicated Coder template,
[scaletest-runner](https://github.com/coder/coder/tree/main/scaletest/templates/scaletest-runner).

Learn more about [Coder’s architecture](../about/architecture.md) and our
[scale-testing methodology](architectures/index.md#scale-testing-methodology).

## Recent scale tests

> Note: the below information is for reference purposes only, and are not
> intended to be used as guidelines for infrastructure sizing. Review the
> [Reference Architectures](architectures/index.md) for hardware sizing
> recommendations.

| Environment      | Coder CPU | Coder RAM | Coder Replicas | Database          | Users | Concurrent builds | Concurrent connections (Terminal/SSH) | Coder Version | Last tested  |
| ---------------- | --------- | --------- | -------------- | ----------------- | ----- | ----------------- | ------------------------------------- | ------------- | ------------ |
| Kubernetes (GKE) | 3 cores   | 12 GB     | 1              | db-f1-micro       | 200   | 3                 | 200 simulated                         | `v0.24.1`     | Jun 26, 2023 |
| Kubernetes (GKE) | 4 cores   | 8 GB      | 1              | db-custom-1-3840  | 1500  | 20                | 1,500 simulated                       | `v0.24.1`     | Jun 27, 2023 |
| Kubernetes (GKE) | 2 cores   | 4 GB      | 1              | db-custom-1-3840  | 500   | 20                | 500 simulated                         | `v0.27.2`     | Jul 27, 2023 |
| Kubernetes (GKE) | 2 cores   | 8 GB      | 2              | db-custom-2-7680  | 1000  | 20                | 1000 simulated                        | `v2.2.1`      | Oct 9, 2023  |
| Kubernetes (GKE) | 4 cores   | 16 GB     | 2              | db-custom-8-30720 | 2000  | 50                | 2000 simulated                        | `v2.8.4`      | Feb 28, 2024 |

> Note: a simulated connection reads and writes random data at 40KB/s per
> connection.

## Scale testing utility

Since Coder's performance is highly dependent on the templates and workflows you
support, you may wish to use our internal scale testing utility against your own
environments.

> Note: This utility is experimental. It is not subject to any compatibility
> guarantees, and may cause interruptions for your users. To avoid potential
> outages and orphaned resources, we recommend running scale tests on a
> secondary "staging" environment or a dedicated
> [Kubernetes playground cluster](https://github.com/coder/coder/tree/main/scaletest/terraform).
> Run it against a production environment at your own risk.

### Create workspaces

The following command will provision a number of Coder workspaces using the
specified template and extra parameters.

```shell
coder exp scaletest create-workspaces \
		--retry 5 \
		--count "${SCALETEST_PARAM_NUM_WORKSPACES}" \
		--template "${SCALETEST_PARAM_TEMPLATE}" \
		--concurrency "${SCALETEST_PARAM_CREATE_CONCURRENCY}" \
		--timeout 5h \
		--job-timeout 5h \
		--no-cleanup \
		--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces.json"

# Run `coder exp scaletest create-workspaces --help` for all usage
```

The command does the following:

1. Create `${SCALETEST_PARAM_NUM_WORKSPACES}` workspaces concurrently
   (concurrency level: `${SCALETEST_PARAM_CREATE_CONCURRENCY}`) using the
   template `${SCALETEST_PARAM_TEMPLATE}`.
1. Leave workspaces running to use in next steps (`--no-cleanup` option).
1. Store provisioning results in JSON format.
1. If you don't want the creation process to be interrupted by any errors, use
   the `--retry 5` flag.

### Traffic Generation

Given an existing set of workspaces created previously with `create-workspaces`,
the following command will generate traffic similar to that of Coder's Web
Terminal against those workspaces.

```shell
# Produce load at about 1000MB/s (25MB/40ms).
coder exp scaletest workspace-traffic \
	--template "${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE}" \
	--bytes-per-tick $((1024 * 1024 * 25)) \
	--tick-interval 40ms \
	--timeout "$((delay))s" \
	--job-timeout "$((delay))s" \
	--scaletest-prometheus-address 0.0.0.0:21113 \
	--target-workspaces "0:100" \
	--trace=false \
  --output json:"${SCALETEST_RESULTS_DIR}/traffic-${type}-greedy-agent.json"
```

Traffic generation can be parametrized:

1. Send `bytes-per-tick` every `tick-interval`.
1. Enable tracing for performance debugging.
1. Target a range of workspaces with `--target-workspaces 0:100`.
1. For dashboard traffic: Target a range of users with `--target-users 0:100`.
1. Store provisioning results in JSON format.
1. Expose a dedicated Prometheus address (`--scaletest-prometheus-address`) for
   scaletest-specific metrics.

The `workspace-traffic` supports also other modes - SSH traffic, workspace app:

1. For SSH traffic: Use `--ssh` flag to generate SSH traffic instead of Web
   Terminal.
1. For workspace app traffic: Use `--app [wsdi|wsec|wsra]` flag to select app
   behavior. (modes: _WebSocket discard_, _WebSocket echo_, _WebSocket read_).

### Cleanup

The scaletest utility will attempt to clean up all workspaces it creates. If you
wish to clean up all workspaces, you can run the following command:

```shell
coder exp scaletest cleanup \
	--cleanup-job-timeout 2h \
	--cleanup-timeout 15min
```

This will delete all workspaces and users with the prefix `scaletest-`.

## Scale testing template

Consider using a dedicated
[scaletest-runner](https://github.com/coder/coder/tree/main/scaletest/templates/scaletest-runner)
template alongside the CLI utility for testing large-scale Kubernetes clusters.

The template deploys a main workspace with scripts used to orchestrate Coder,
creating workspaces, generating workspace traffic, or load-testing workspace
apps.

### Parameters

The _scaletest-runner_ offers the following configuration options:

- Workspace size selection: minimal/small/medium/large (_default_: minimal,
  which contains just enough resources for a Coder agent to run without
  additional workloads)
- Number of workspaces
- Wait duration between scenarios or staggered approach

The template exposes parameters to control the traffic dimensions for SSH
connections, workspace apps, and dashboard tests:

- Traffic duration of the load test scenario
- Traffic percentage of targeted workspaces
- Bytes per tick and tick interval
- _For workspace apps_: modes (echo, read random data, or write and discard)

Scale testing concurrency can be controlled with the following parameters:

- Enable parallel scenarios - interleave different traffic patterns (SSH,
  workspace apps, dashboard traffic, etc.)
- Workspace creation concurrency level (_default_: 10)
- Job concurrency level - generate workspace traffic using multiple jobs
  (_default_: 0)
- Cleanup concurrency level

### Kubernetes cluster

It is recommended to learn how to operate the _scaletest-runner_ before running
it against the staging cluster (or production at your own risk). Coder provides
different
[workspace configurations](https://github.com/coder/coder/tree/main/scaletest/templates)
that operators can deploy depending on the traffic projections.

There are a few cluster options available:

| Workspace size | vCPU | Memory | Persisted storage | Details                                               |
| -------------- | ---- | ------ | ----------------- | ----------------------------------------------------- |
| minimal        | 1    | 2 Gi   | None              |                                                       |
| small          | 1    | 1 Gi   | None              |                                                       |
| medium         | 2    | 2 Gi   | None              | Medium-sized cluster offers the greedy agent variant. |
| large          | 4    | 4 Gi   | None              |                                                       |

Note: Review the selected cluster template and edit the node affinity to match
your setup.

#### Greedy agent

The greedy agent variant is a template modification that makes the Coder agent
transmit large metadata (size: 4K) while reporting stats. The transmission of
large chunks puts extra overhead on coderd instances and agents when handling
and storing the data.

Use this template variant to verify limits of the cluster performance.

### Observability

During scale tests, operators can monitor progress using a Grafana dashboard.
Coder offers a comprehensive overview
[dashboard](https://github.com/coder/coder/blob/main/scaletest/scaletest_dashboard.json)
that can seamlessly integrate into the internal Grafana deployment.

This dashboard provides insights into various aspects, including:

- Utilization of resources within the Coder control plane (CPU, memory, pods)
- Database performance metrics (CPU, memory, I/O, connections, queries)
- Coderd API performance (requests, latency, error rate)
- Resource consumption within Coder workspaces (CPU, memory, network usage)
- Internal metrics related to provisioner jobs

Note: Database metrics are disabled by default and can be enabled by setting the
environment variable `CODER_PROMETHEUS_COLLECT_DB_METRICS` to `true`.

It is highly recommended to deploy a solution for centralized log collection and
aggregation. The presence of error logs may indicate an underscaled deployment
of Coder, necessitating action from operators.

## Autoscaling

We generally do not recommend using an autoscaler that modifies the number of
coderd replicas. In particular, scale down events can cause interruptions for a
large number of users.

Coderd is different from a simple request-response HTTP service in that it
services long-lived connections whenever it proxies HTTP applications like IDEs
or terminals that rely on websockets, or when it relays tunneled connections to
workspaces. Loss of a coderd replica will drop these long-lived connections and
interrupt users. For example, if you have 4 coderd replicas behind a load
balancer, and an autoscaler decides to reduce it to 3, roughly 25% of the
connections will drop. An even larger proportion of users could be affected if
they use applications that use more than one websocket.

The severity of the interruption varies by application. Coder's web terminal,
for example, will reconnect to the same session and continue. So, this should
not be interpreted as saying coderd replicas should never be taken down for any
reason.

We recommend you plan to run enough coderd replicas to comfortably meet your
weekly high-water-mark load, and monitor coderd peak CPU & memory utilization
over the long term, reevaluating periodically. When scaling down (or performing
upgrades), schedule these outside normal working hours to minimize user
interruptions.

### A note for Kubernetes users

When running on Kubernetes on cloud infrastructure (i.e. not bare metal), many
operators choose to employ a _cluster_ autoscaler that adds and removes
Kubernetes _nodes_ according to load. Coder can coexist with such cluster
autoscalers, but we recommend you take steps to prevent the autoscaler from
evicting coderd pods, as an eviction will cause the same interruptions as
described above. For example, if you are using the
[Kubernetes cluster autoscaler](https://kubernetes.io/docs/reference/labels-annotations-taints/#cluster-autoscaler-kubernetes-io-safe-to-evict),
you may wish to set `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` as
an annotation on the coderd deployment.

## Troubleshooting

If a load test fails or if you are experiencing performance issues during
day-to-day use, you can leverage Coder's [Prometheus metrics](./prometheus.md)
to identify bottlenecks during scale tests. Additionally, you can use your
existing cloud monitoring stack to measure load, view server logs, etc.
