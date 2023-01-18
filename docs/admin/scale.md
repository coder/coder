We scale-test Coder with the [same utility](#scaletest-utility) that can be used in your environment for insights into how Coder scales with your infrastructure.

## General concepts

Coder runs workspace operations in a queue. The number of concurrent builds will be limited to the number of provisioner daemons across all coderd replicas.

- **coderd**: Coder’s primary service. Learn more about [Coder’s architecture](../about/architecture.md)
- **coderd replicas**: Replicas (often via Kubernetes) for high availability, this is an [enterprise feature](../enterprise.md)
- **concurrent workspace builds**: Workspace operations (e.g. create/stop/delete/apply) across all users
- **concurrent connections**: Any connection to a workspace (e.g. SSH, web terminal, `coder_app`)
- **provisioner daemons**: Coder runs one workspace build per provisioner daemon. One coderd replica can host many daemons
- **scaletest**: Our scale-testing utility, built into the `coder` command line.

```text
2 coderd replicas * 30 provisioner daemons = 60 max concurrent workspace builds
```

## Infrastructure recommendations

### Concurrent workspace builds

Workspace builds are CPU-intensive, as it relies on Terraform. Various [Terraform providers](https://registry.terraform.io/browse/providers) have different resource requirements. When tested with our [kubernetes](https://github.com/coder/coder/tree/main/examples/templates/kubernetes) template, `coderd` will consume roughly 8 cores per 30 concurrent workspace builds. For effective provisioning, our helm chart prefers to schedule [one coderd replica per-node](https://github.com/coder/coder/blob/main/helm/values.yaml#L110-L121).

To support 120 concurrent workspace builds, for example:

- Create a cluster/nodepool with 4 nodes, 8-core each (AWS: `t3.2xlarge` GCP: `e2-highcpu-8`)
- Run coderd with 4 replicas, 30 provisioner daemons each. (`CODER_PROVISIONER_DAEMONS=30`)
- Ensure Coder's [PostgreSQL server](./configure.md#postgresql-database) can use up to 1.5 cores

## Recent scale tests

| Environment        | Users | Concurrent builds | Concurrent connections (Terminal/SSH) | Coder Version | Last tested  |
| ------------------ | ----- | ----------------- | ------------------------------------- | ------------- | ------------ |
| Kubernetes (GKE)   | 1200  | 120               | 10,000                                | `v0.14.2`     | Jan 10, 2022 |
| Docker (Single VM) | 500   | 50                | 10,000                                | `v0.13.4`     | Dec 20, 2022 |

## Scale testing utility

Since Coder's performance is highly dependent on the templates and workflows you support, we recommend using our scale testing utility against your own environments.

The following command will run our scale test against your own Coder deployment. You can also specify a template name and any parameter values.

```sh
coder scaletest create-workspaces \
    --count 1000 \
    --template "kubernetes" \
    --concurrency 0 \
    --cleanup-concurrency 0 \
    --parameter "home_disk_size=10" \
    --run-command "sleep 2 && echo hello"

# Run `coder scaletest create-workspaces --help` for all usage
```

> To avoid potential outages and orphaned resources, we recommend running scale tests on a secondary "staging" environment.

The test does the following:

1. create `1000` workspaces
1. establish SSH connection to each workspace
1. run `sleep 3 && echo hello` on each workspace via the web terminal
1. close connections, attempt to delete all workspaces
1. return results (e.g. `998 succeeded, 2 failed to connect`)

Concurrency is configurable. `concurrency 0` means the scaletest test will attempt to create & connect to all workspaces immediately.

## Troubleshooting

If a load test fails or if you are experiencing performance issues during day-to-day use, you can leverage Coder's [prometheus metrics](./prometheus.md) to identify bottlenecks during scale tests. Additionally, you can use your existing cloud monitoring stack to measure load, view server logs, etc.
