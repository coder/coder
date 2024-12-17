# Scale Coder

December 20, 2024

---

This best practice guide helps you prepare a low-scale Coder deployment so that
it can be scaled up to a high-scale deployment as use grows, and keep it
operating smoothly with a high number of active users and workspaces.

## Observability

Observability is one of the most important aspects to a scalable Coder
deployment.

Identify potential bottlenecks before they negatively affect the end-user
experience. It will also allow you to empirically verify that modifications you
make to your deployment to increase capacity have their intended effects.

- Capture log output from Coder Server instances and external provisioner
  daemons and store them in a searchable log store.

  - For example: Loki, CloudWatch Logs, etc.

  - Retain logs for a minimum of thirty days, ideally ninety days. This allows
    you to look back to see when anomalous behaviors began.

- Metrics:

  - Capture infrastructure metrics like CPU, memory, open files, and network I/O
    for all Coder Server, external provisioner daemon, workspace proxy, and
    PostgreSQL instances.

  - Capture metrics from Coder Server and external provisioner daemons via
    Prometheus.

    - On Coder Server

      - Enable Prometheus metrics:

        ```yaml
        CODER_PROMETHEUS_ENABLE=true
        ```

      - Enable database metrics:

        ```yaml
        CODER_PROMETHEUS_COLLECT_DB_METRICS=true
        ```

      - Configure agent stats to avoid large cardinality:

        ```yaml
        CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY=agent_name
        ```

        - To disable Agent stats:

          ```yaml
          CODER_PROMETHEUS_COLLECT_AGENT_STATS=false
          ```

  - Retain metric time series for at least six months. This allows you to see
    performance trends relative to user growth.

  - Integrate metrics with an observability dashboard, for example, Grafana.

### Key metrics

**CPU and Memory Utilization**

- Monitor the utilization as a fraction of the available resources on the
  instance. Its utilization will vary with use throughout the day and over the
  course of the week. Monitor the trends, paying special attention to the daily
  and weekly peak utilization. Use long-term trends to plan infrastructure
  upgrades.

**Tail latency of Coder Server API requests**

- Use the `coderd_api_request_latencies_seconds` metric.
- High tail latency can indicate Coder Server or the PostgreSQL database is
  being starved for resources.

**Tail latency of database queries**

- Use the `coderd_db_query_latencies_seconds` metric.
- High tail latency can indicate the PostgreSQL database is low in resources.

Configure alerting based on these metrics to ensure you surface problems before
end users notice them.

## Coder Server

### Locality

If increased availability of the Coder API is a concern, deploy at least three
instances. Spread the instances across nodes (e.g. via anti-affinity rules in
Kubernetes), and/or in different availability zones of the same geographic
region.

Do not deploy in different geographic regions. Coder Servers need to be able to
communicate with one another directly with low latency, under 10ms. Note that
this is for the availability of the Coder API – workspaces will not be fault
tolerant unless they are explicitly built that way at the template level.

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
[validated architectures](../../admin/infrastructure/validated-architectures.md)
give specific sizing recommendations for various user scales. These are a useful
starting point, but very few deployments will remain stable at a predetermined
user level over the long term, so monitoring and adjusting of resources is
recommended.

We don't recommend that you autoscale the Coder Servers. Instead, scale the
deployment for peak weekly usage.

Although Coder Server persists no internal state, it operates as a proxy for end
users to their workspaces in two capacities:

1. As an HTTP proxy when they access workspace applications in their browser via
   the Coder Dashboard

1. As a DERP proxy when establishing tunneled connections via CLI tools
   (`coder ssh`, `coder port-forward`, etc.) and desktop IDEs.

Stopping a Coder Server instance will (momentarily) disconnect any users
currently connecting through that instance. Adding a new instance is not
disruptive, but removing instances and upgrades should be performed during a
maintenance window to minimize disruption.

## Provisioner daemons

### Locality

We recommend you disable provisioner daemons within your Coder Server:

```yaml
CODER_PROVISIONER_DAEMONS=0
```

Run one or more
[provisioner daemon deployments external to Coder Server](../../admin/provisioners.md).
This allows you to scale them independently of the Coder Server.

We recommend deploying provisioner daemons within the same cluster as the
workspaces they will provision or are hosted in.

- This gives them a low-latency connection to the APIs they will use to
  provision workspaces and can speed builds.

- It allows provisioner daemons to use in-cluster mechanisms (for example
  Kubernetes service account tokens, AWS IAM Roles, etc.) to authenticate with
  the infrastructure APIs.

- If you deploy workspaces in multiple clusters, run multiple provisioner daemon
  deployments and use template tags to select the correct set of provisioner
  daemons.

- Provisioner daemons need to be able to connect to Coder Server, but this need
  not be a low-latency connection.

Provisioner daemons make no direct connections to the PostgreSQL database, so
there's no need for locality to the Postgres database.

### Scaling

Each provisioner daemon instance can handle a single workspace build job at a
time. Therefore, the number of provisioner daemon instances within a tagged
deployment equals the maximum number of simultaneous builds your Coder
deployment can handle.

If users experience unacceptably long queues for workspace builds to start,
consider increasing the number of provisioner daemon instances in the affected
cluster.

You may wish to automatically scale the number of provisioner daemon instances
throughout the day to meet demand. If you stop instances with `SIGHUP`, they
will complete their current build job and exit. `SIGINT` will cancel the current
job, which will result in a failed build. Ensure your autoscaler waits long
enough for your build jobs to complete before forcibly killing the provisioner
daemon process.

If deploying in Kubernetes, we recommend a single provisioner daemon per pod. On
a virtual machine (VM), you can deploy multiple provisioner daemons, ensuring
each has a unique `CODER_CACHE_DIRECTORY` value.

Coder's
[validated architectures](../../admin/infrastructure/validated-architectures.md)
give specific sizing recommendations for various user scales. Since the
complexity of builds varies significantly depending on the workspace template,
consider this a starting point. Monitor queue times and build times to adjust
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
[validated architectures](../../admin/infrastructure/validated-architectures.md)
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

Workspace proxy load is determined by the amount of traffic they proxy. We
recommend you monitor CPU, memory, and network I/O utilization to decide when to
resize the number of proxy instances.

We do not recommend autoscaling the workspace proxies because many applications
use long-lived connections such as websockets, which would be disrupted by
stopping the proxy. We recommend you scale for peak demand and scale down or
upgrade during a maintenance window.

## Workspaces

Workspaces represent the vast majority of resources in most Coder deployments.
Because they are defined by templates, there is no one-size-fits-all advice for
scaling.

### Hard and soft cluster limits

All Infrastructure as a Service (IaaS) clusters have limits to what can be
simultaneously provisioned. These could be hard limits, based on the physical
size of the cluster, especially in the case of a private cloud, or soft limits,
based on configured limits in your public cloud account.

It is important to be aware of these limits and monitor Coder workspace resource
utilization against the limits, so that a new influx of users doesn't encounter
failed builds. Monitoring these is outside the scope of Coder, but we recommend
that you set up dashboards and alerts for each kind of limited resource.

As you approach soft limits, you might be able to justify an increase to keep
growing.

As you approach hard limits, you will need to consider deploying to additional
cluster(s).

### Workspaces per node

Many development workloads are "spiky" in their CPU and memory requirements, for
example, peaking during build/test and then ebbing while editing code. This
leads to an opportunity to efficiently use compute resources by packing multiple
workspaces onto a single node. This can lead to better experience (more CPU and
memory available during brief bursts) and lower cost.

However, it needs to be considered against several trade-offs.

- There are residual probabilities of "noisy neighbor" problems negatively
  affecting end users. The probabilities increase with the amount of
  oversubscription of CPU and memory resources.

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
