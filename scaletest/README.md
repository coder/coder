# Scale Testing

This folder contains CLI commands, Terraform code, and scripts to aid in performing load tests of Coder.
At a high level, it performs the following steps:

- Using the Terraform code in `./terraform`, stands up a preconfigured Google Cloud environment
  consisting of a VPC, GKE Cluster, and CloudSQL instance.
  > **Note: You must have an existing Google Cloud project available.**
- Creates a dedicated namespace for Coder and installs Coder using the Helm chart in this namespace.
- Configures the Coder deployment with random credentials and a predefined Kubernetes template.
  > **Note:** These credentials are stored in `${PROJECT_ROOT}/scaletest/.coderv2/coder.env`.
- Creates a number of workspaces and waits for them to all start successfully. These workspaces
  are ephemeral and do not contain any persistent resources.
- Waits for 10 minutes to allow things to settle and establish a baseline.
- Generates web terminal traffic to all workspaces for 30 minutes.
- Directly after traffic generation, captures goroutine and heap snapshots of the Coder deployment.
- Tears down all resources (unless `--skip-cleanup` is specified).

## Usage

The main entrypoint is the `scaletest.sh` script.

```console
$ scaletest.sh --help
Usage: scaletest.sh --name <name> --project <project> --num-workspaces <num-workspaces> --scenario <scenario> [--dry-run] [--skip-cleanup]
```

### Required arguments

- `--name`: Name for the loadtest. This is added as a prefix to resources created by Terraform (e.g. `joe-big-loadtest`).
- `--project`: Google Cloud project in which to create the resources (example: `my-loadtest-project`).
- `--num-workspaces`: Number of workspaces to create (example: `10`).
- `--scenario`: Deployment scenario to use (example: `small`). See `terraform/scenario-*.tfvars`.

> **Note:** In order to capture Prometheus metrics, you must define the environment variables
> `SCALETEST_PROMETHEUS_REMOTE_WRITE_USER` and `SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD`.

### Optional arguments

- `--dry-run`: Do not perform any action and instead print what would be executed.
- `--skip-cleanup`: Do not perform any cleanup. You will be responsible for deleting any resources this creates.

### Environment Variables

All of the above arguments may be specified as environment variables. Consult the script for details.

### Prometheus Metrics

To capture Prometheus metrics from the loadtest, two environment variables are required:

- `SCALETEST_PROMETHEUS_REMOTE_WRITE_USER`
- `SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD`

### Enterprise License

To add an Enterprise license, set the `SCALETEST_CODER_LICENSE` environment variable to the JWT string

## Scenarios

A scenario defines a number of variables that override the default Terraform variables.
A number of existing scenarios are provided in `scaletest/terraform/scenario-*.tfvars`.

For example, `scenario-small.tfvars` includes the following variable definitions:

```hcl
nodepool_machine_type_coder      = "t2d-standard-2"
nodepool_machine_type_workspaces = "t2d-standard-2"
coder_cpu                        = "1000m" # Leaving 1 CPU for system workloads
coder_mem                        = "4Gi"   # Leaving 4GB for system workloads
```

To create your own scenario, simply add a new file `terraform/scenario-$SCENARIO_NAME.tfvars`.
In this file, override variables as required, consulting `vars.tf` as needed.
You can then use this scenario by specifying `--scenario $SCENARIO_NAME`.
For example, if your scenario file were named `scenario-big-whopper2x.tfvars`, you would specify
`--scenario=big-whopper2x`.

## Utility scripts

A number of utility scripts are provided in `lib`, and are used by `scaletest.sh`:

- `coder_shim.sh`: a convenience script to run the `coder` binary with a predefined config root.
  This is intended to allow running Coder CLI commands against the loadtest cluster without
  modifying a user's existing Coder CLI configuration.
- `coder_init.sh`: Performs first-time user setup of an existing Coder instance, generating
  a random password for the admin user. The admin user is named `admin@coder.com` by default.
  Credentials are written to `scaletest/.coderv2/coder.env`.
- `coder_workspacetraffic.sh`: Runs traffic generation against the loadtest cluster and creates
  a monitoring manifest for the traffic generation pod. This pod will restart automatically
  after the traffic generation has completed.

## Grafana Dashboard

A sample Grafana dashboard is provided in `scaletest_dashboard.json`. This dashboard is intended
to be imported into an existing Grafana instance. It provides a number of useful metrics:

- **Control Plane Resources**: CPU, memory, and network usage for the Coder deployment, as well as the number of pod restarts.
- **Database**: Rows inserted/updated/deleted/returned, active connections, and transactions per second. Fine-grained `sqlQuerier` metrics are provided for Coder's database as well, broken down my query method.
- **HTTP requests**: Number of HTTP requests per second, broken down by status code and path.
- **Workspace Resources**: CPU, memory, and network usage for all workspaces.
- **Workspace Agents**: Workspace agent network usage, connection latency, and number of active connections.
- **Workspace Traffic**: Statistics related to workspace traffic generation.
- **Internals**: Provisioner job timings, concurrency, workspace builds, and AuthZ duration.

A subset of these metrics may be useful for a production deployment, but some are only useful
for load testing.

> **Note:** in particular, `sqlQuerier` metrics produce a large number of time series and may cause
> increased charges in your metrics provider.
