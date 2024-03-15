We scale-test Coder with [a built-in utility](#scale-testing-utility) that can
be used in your environment for insights into how Coder scales with your
infrastructure.

## General concepts

Coder runs workspace operations in a queue. The number of concurrent builds will
be limited to the number of provisioner daemons across all coderd replicas.

- **coderd**: Coder’s primary service. Learn more about
  [Coder’s architecture](../about/architecture.md)
- **coderd replicas**: Replicas (often via Kubernetes) for high availability,
  this is an [enterprise feature](../enterprise.md)
- **concurrent workspace builds**: Workspace operations (e.g.
  create/stop/delete/apply) across all users
- **concurrent connections**: Any connection to a workspace (e.g. SSH, web
  terminal, `coder_app`)
- **provisioner daemons**: Coder runs one workspace build per provisioner
  daemon. One coderd replica can host many daemons
- **scaletest**: Our scale-testing utility, built into the `coder` command line.

```text
2 coderd replicas * 30 provisioner daemons = 60 max concurrent workspace builds
```

## Infrastructure recommendations

> Note: The below are guidelines for planning your infrastructure. Your mileage
> may vary depending on your templates, workflows, and users.

When planning your infrastructure, we recommend you consider the following:

1. CPU and memory requirements for `coderd`. We recommend allocating 1 CPU core
   and 2 GB RAM per `coderd` replica at minimum. See
   [Concurrent users](#concurrent-users) for more details.
1. CPU and memory requirements for
   [external provisioners](../admin/provisioners.md#running-external-provisioners),
   if required. We recommend allocating 1 CPU core and 1 GB RAM per 5 concurrent
   workspace builds to external provisioners. Note that this may vary depending
   on the template used. See
   [Concurrent workspace builds](#concurrent-workspace-builds) for more details.
   By default, `coderd` runs 3 integrated provisioners.
1. CPU and memory requirements for the database used by `coderd`. We recommend
   allocating an additional 1 CPU core to the database used by Coder for every
   1000 active users.
1. CPU and memory requirements for workspaces created by Coder. This will vary
   depending on users' needs. However, the Coder agent itself requires at
   minimum 0.1 CPU cores and 256 MB to run inside a workspace.

### Concurrent users

We recommend allocating 2 CPU cores and 4 GB RAM per `coderd` replica per 1000
active users. We also recommend allocating an additional 1 CPU core to the
database used by Coder for every 1000 active users. Inactive users do not
consume Coder resources, although workspaces configured to auto-start will
consume resources when they are built.

Users' primary mode of accessing Coder will also affect resource requirements.
If users will be accessing workspaces primarily via Coder's HTTP interface, we
recommend doubling the number of cores and RAM allocated per user. For example,
if you expect 1000 users accessing workspaces via the web, we recommend
allocating 4 CPU cores and 8 GB RAM.

Users accessing workspaces via SSH will consume fewer resources, as SSH
connections are not proxied through Coder.

### Concurrent workspace builds

Workspace builds are CPU-intensive, as it relies on Terraform. Various
[Terraform providers](https://registry.terraform.io/browse/providers) have
different resource requirements. When tested with our
[kubernetes](https://github.com/coder/coder/tree/main/examples/templates/kubernetes)
template, `coderd` will consume roughly 0.25 cores per concurrent workspace
build. For effective provisioning, our helm chart prefers to schedule
[one coderd replica per-node](https://github.com/coder/coder/blob/main/helm/coder/values.yaml#L188-L202).

We recommend:

- Running `coderd` on a dedicated set of nodes. This will prevent other
  workloads from interfering with workspace builds. You can use
  [node selectors](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector),
  or
  [taints and tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)
  to achieve this.
- Disabling autoscaling for `coderd` nodes. Autoscaling can cause interruptions
  for users, see [Autoscaling](#autoscaling) for more details.
- (Enterprise-only) Running external provisioners instead of Coder's built-in
  provisioners (`CODER_PROVISIONER_DAEMONS=0`) will separate the load caused by
  workspace provisioning on the `coderd` nodes. For more details, see
  [External provisioners](../admin/provisioners.md#running-external-provisioners).
- Alternatively, if increasing the number of integrated provisioner daemons in
  `coderd` (`CODER_PROVISIONER_DAEMONS>3`), allocate additional resources to
  `coderd` to compensate (approx. 0.25 cores and 256 MB per provisioner daemon).

For example, to support 120 concurrent workspace builds:

- Create a cluster/nodepool with 4 nodes, 8-core each (AWS: `t3.2xlarge` GCP:
  `e2-highcpu-8`)
- Run coderd with 4 replicas, 30 provisioner daemons each.
  (`CODER_PROVISIONER_DAEMONS=30`)
- Ensure Coder's [PostgreSQL server](./configure.md#postgresql-database) can use
  up to 2 cores and 4 GB RAM

## Recent scale tests

> Note: the below information is for reference purposes only, and are not
> intended to be used as guidelines for infrastructure sizing.

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

> Note: This utility is intended for internal use only. It is not subject to any
> compatibility guarantees, and may cause interruptions for your users. To avoid
> potential outages and orphaned resources, we recommend running scale tests on
> a secondary "staging" environment. Run it against a production environment at
> your own risk.

### Workspace Creation

The following command will run our scale test against your own Coder deployment.
You can also specify a template name and any parameter values.

```shell
coder exp scaletest create-workspaces \
    --count 1000 \
    --template "kubernetes" \
    --concurrency 0 \
    --cleanup-concurrency 0 \
    --parameter "home_disk_size=10" \
    --run-command "sleep 2 && echo hello"

# Run `coder exp scaletest create-workspaces --help` for all usage
```

The test does the following:

1. create `1000` workspaces
1. establish SSH connection to each workspace
1. run `sleep 3 && echo hello` on each workspace via the web terminal
1. close connections, attempt to delete all workspaces
1. return results (e.g. `998 succeeded, 2 failed to connect`)

Concurrency is configurable. `concurrency 0` means the scaletest test will
attempt to create & connect to all workspaces immediately.

If you wish to leave the workspaces running for a period of time, you can
specify `--no-cleanup` to skip the cleanup step. You are responsible for
deleting these resources later.

### Traffic Generation

Given an existing set of workspaces created previously with `create-workspaces`,
the following command will generate traffic similar to that of Coder's web
terminal against those workspaces.

```shell
coder exp scaletest workspace-traffic \
    --byes-per-tick 128 \
    --tick-interval 100ms \
    --concurrency 0
```

To generate SSH traffic, add the `--ssh` flag.

### Cleanup

The scaletest utility will attempt to clean up all workspaces it creates. If you
wish to clean up all workspaces, you can run the following command:

```shell
coder exp scaletest cleanup
```

This will delete all workspaces and users with the prefix `scaletest-`.

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
day-to-day use, you can leverage Coder's [prometheus metrics](./prometheus.md)
to identify bottlenecks during scale tests. Additionally, you can use your
existing cloud monitoring stack to measure load, view server logs, etc.
