# Scale Coder

This best practice guide helps you prepare a Coder deployment that you can
scale up to a high-scale deployment as use grows, and keep it operating smoothly with a
high number of active users and workspaces.

## Observability

Observability is one of the most important aspects to a scalable Coder deployment.
When you have visibility into performance and usage metrics, you can make informed
decisions about what changes you should make.

[Monitor your Coder deployment](../../admin/monitoring/index.md) with log output
and metrics to identify potential bottlenecks before they negatively affect the
end-user experience and measure the effects of modifications you make to your
deployment.

- Log output
  - Capture log output from from Coder Server instances and external provisioner daemons
  and store them in a searchable log store like Loki, CloudWatch logs, or other tools.
  - Retain logs for a minimum of thirty days, ideally ninety days.
  This allows you investigate when anomalous behaviors began.

- Metrics
  - Capture infrastructure metrics like CPU, memory, open files, and network I/O for all
  Coder Server, external provisioner daemon, workspace proxy, and PostgreSQL instances.
  - Capture Coder Server and External Provisioner daemons metrics
  [via Prometheus](#how-to-capture-coder-server-metrics-with-prometheus).

Retain metric time series for at least six months. This allows you to see
performance trends relative to user growth.

For a more comprehensive overview, integrate metrics with an observability
dashboard like [Grafana](../../admin/monitoring/index.md).

### Observability key metrics

Configure alerting based on these metrics to ensure you surface problems before
they affect the end-user experience.

- CPU and Memory Utilization
  - Monitor the utilization as a fraction of the available resources on the instance.

     Utilization will vary with use throughout the course of a day, week, and longer timelines.
     Monitor trends and pay special attention to the daily and weekly peak utilization.
     Use long-term trends to plan infrastructure upgrades.

- Tail latency of Coder Server API requests
  - High tail latency can indicate Coder Server or the PostgreSQL database is underprovisioned
  for the load.
  - Use the `coderd_api_request_latencies_seconds` metric.

- Tail latency of database queries
  - High tail latency can indicate the PostgreSQL database is low in resources.
  - Use the `coderd_db_query_latencies_seconds` metric.

### How to capture Coder server metrics with Prometheus

Edit your Helm `values.yaml` to capture metrics from Coder Server and external provisioner daemons with
[Prometheus](../../admin/integrations/prometheus.md):

1. Enable Prometheus metrics:

   ```yaml
   CODER_PROMETHEUS_ENABLE=true
   ```

1. Enable database metrics:

   ```yaml
   CODER_PROMETHEUS_COLLECT_DB_METRICS=true
   ```

1. For a high scale deployment, configure agent stats to avoid large cardinality or disable them:

   - Configure agent stats:

     ```yaml
     CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY=agent_name
     ```

   - Disable agent stats:

     ```yaml
     CODER_PROMETHEUS_COLLECT_AGENT_STATS=false
     ```

## Coder Server

### Locality

If increased availability of the Coder API is a concern, deploy at least three
instances of Coder Server. Spread the instances across nodes with anti-affinity rules in
Kubernetes or in different availability zones of the same geographic region.

Do not deploy in different geographic regions.

Coder Servers need to be able to communicate with one another directly with low
latency, under 10ms. Note that this is for the availability of the Coder API.
Workspaces are not fault tolerant unless they are explicitly built that way at
the template level.

Deploy Coder Server instances as geographically close to PostgreSQL as possible.
Low-latency communication (under 10ms) with Postgres is essential for Coder
Server's performance.

### Scaling

Coder Server can be scaled both vertically for bigger instances and horizontally
for more instances.

Aim to keep the number of Coder Server instances relatively small, preferably
under ten instances, and opt for vertical scale over horizontal scale after
meeting availability requirements.

Coder's
[validated architectures](../../admin/infrastructure/validated-architectures/index.md)
give specific sizing recommendations for various user scales. These are a useful
starting point, but very few deployments will remain stable at a predetermined
user level over the long term. We recommend monitoring and adjusting resources as needed.

We don't recommend that you autoscale the Coder Servers. Instead, scale the
deployment for peak weekly usage.

Although Coder Server persists no internal state, it operates as a proxy for end
users to their workspaces in two capacities:

1. As an HTTP proxy when they access workspace applications in their browser via
the Coder Dashboard.

1. As a DERP proxy when establishing tunneled connections with CLI tools like
`coder ssh`, `coder port-forward`, and others, and with desktop IDEs.

Stopping a Coder Server instance will (momentarily) disconnect any users
currently connecting through that instance. Adding a new instance is not
disruptive, but you should remove instances and perform upgrades during a
maintenance window to minimize disruption.

## Provisioner daemons

### Locality

We recommend that you run one or more
[provisioner daemon deployments external to Coder Server](../../admin/provisioners/index.md)
and disable provisioner daemons within your Coder Server.
This allows you to scale them independently of the Coder Server:

```yaml
CODER_PROVISIONER_DAEMONS=0
```

We recommend deploying provisioner daemons within the same cluster as the
workspaces they will provision or are hosted in.

- This gives them a low-latency connection to the APIs they will use to
  provision workspaces and can speed builds.

- It allows provisioner daemons to use in-cluster mechanisms (for example
  Kubernetes service account tokens, AWS IAM Roles, and others) to authenticate with
  the infrastructure APIs.

- If you deploy workspaces in multiple clusters, run multiple provisioner daemon
  deployments and use template tags to select the correct set of provisioner
  daemons.

- Provisioner daemons need to be able to connect to Coder Server, but this does not need
  to be a low-latency connection.

Provisioner daemons make no direct connections to the PostgreSQL database, so
there's no need for locality to the Postgres database.

### Scaling

Each provisioner daemon instance can handle a single workspace build job at a
time. Therefore, the maximum number of simultaneous builds your Coder deployment
can handle is equal to the number of provisioner daemon instances within a tagged
deployment.

If users experience unacceptably long queues for workspace builds to start,
consider increasing the number of provisioner daemon instances in the affected
cluster.

You might need to automatically scale the number of provisioner daemon instances
throughout the day to meet demand.

If you stop instances with `SIGHUP`, they will complete their current build job
and exit. `SIGINT` will cancel the current job, which will result in a failed build.
Ensure your autoscaler waits long enough for your build jobs to complete before
it kills the provisioner daemon process.

If you deploy in Kubernetes, we recommend a single provisioner daemon per pod.
On a virtual machine (VM), you can deploy multiple provisioner daemons, ensuring
each has a unique `CODER_CACHE_DIRECTORY` value.

Coder's
[validated architectures](../../admin/infrastructure/validated-architectures/index.md)
give specific sizing recommendations for various user scales. Since the
complexity of builds varies significantly depending on the workspace template,
consider this a starting point. Monitor queue times and build times and adjust
the number and size of your provisioner daemon instances.

## PostgreSQL

PostgreSQL is the primary persistence layer for all of Coder's deployment data.
We also use `LISTEN` and `NOTIFY` to coordinate between different instances of
Coder Server.

### Locality

Coder Server instances must have low-latency connections (under 10ms) to
PostgreSQL. If you use multiple PostgreSQL replicas in a clustered config, these
must also be low-latency with respect to one another.

### Scaling

Prefer scaling PostgreSQL vertically rather than horizontally for best
performance. Coder's
[validated architectures](../../admin/infrastructure/validated-architectures/index.md)
give specific sizing recommendations for various user scales.

## Workspace proxies

Workspace proxies proxy HTTP traffic from end users to workspaces for Coder apps
defined in the templates, and HTTP ports opened by the workspace. By default
they also include a DERP Proxy.

### Locality

We recommend each geographic cluster of workspaces have an associated deployment
of workspace proxies. This ensures that users always have a near-optimal proxy
path.

### Scaling

Workspace proxy load is determined by the amount of traffic they proxy.

Monitor CPU, memory, and network I/O utilization to decide when to resize
the number of proxy instances.

Scale for peak demand and scale down or upgrade during a maintenance window.

We do not recommend autoscaling the workspace proxies because many applications
use long-lived connections such as websockets, which would be disrupted by
stopping the proxy.

## Workspaces

Workspaces represent the vast majority of resources in most Coder deployments.
Because they are defined by templates, there is no one-size-fits-all advice for
scaling workspaces.

### Hard and soft cluster limits

All Infrastructure as a Service (IaaS) clusters have limits to what can be
simultaneously provisioned. These could be hard limits, based on the physical
size of the cluster, especially in the case of a private cloud, or soft limits,
based on configured limits in your public cloud account.

It is important to be aware of these limits and monitor Coder workspace resource
utilization against the limits, so that a new influx of users don't encounter
failed builds. Monitoring these is outside the scope of Coder, but we recommend
that you set up dashboards and alerts for each kind of limited resource.

As you approach soft limits, you can request limit increases to keep growing.

As you approach hard limits, consider deploying to additional cluster(s).

### Workspaces per node

Many development workloads are "spiky" in their CPU and memory requirements, for
example, they peak during build/test and then lower while editing code.
This leads to an opportunity to efficiently use compute resources by packing multiple
workspaces onto a single node. This can lead to better experience (more CPU and
memory available during brief bursts) and lower cost.

There are a number of things you should consider before you decide how many
workspaces you should allow per node:

- "Noisy neighbor" issues: Users share the node's CPU and memory resources and might
be susceptible to a user or process consuming shared resources.

- If the shared nodes are a provisioned resource, for example, Kubernetes nodes
  running on VMs in a public cloud, then it can sometimes be a challenge to
  effectively autoscale down.

  - For example, if half the workspaces are stopped overnight, and there are ten
    workspaces per node, it's unlikely that all ten workspaces on the node are
    among the stopped ones.

  - You can mitigate this by lowering the number of workspaces per node, or
    using autostop policies to stop more workspaces during off-peak hours.

- If you do overprovision workspaces onto nodes, keep them in a separate node
  pool and schedule Coder control plane (Coder Server, PostgreSQL, workspace
  proxies) components on a different node pool to avoid resource spikes
  affecting them.

Coder customers have had success with both:

- One workspace per AWS VM
- Lots of workspaces on Kubernetes nodes for efficiency

### Cost control

- Use quotas to discourage users from creating many workspaces they don't need
  simultaneously.

- Label workspace cloud resources by user, team, organization, or your own
  labelling conventions to track usage at different granularities.

- Use autostop requirements to bring off-peak utilization down.

## Networking

Set up your network so that most users can get direct, peer-to-peer connections
to their workspaces. This drastically reduces the load on Coder Server and
workspace proxy instances.

## Next steps

- [Scale Tests and Utilities](../../admin/infrastructure/scale-utility.md)
- [Scale Testing](../../admin/infrastructure/scale-testing.md)
