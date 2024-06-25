# Scale Testing

Scaling Coder involves planning and testing to ensure it can handle more load
without compromising service. This process encompasses infrastructure setup,
traffic projections, and aggressive testing to identify and mitigate potential
bottlenecks.

A dedicated Kubernetes cluster for Coder is recommended to configure, host and
manage Coder workloads. Kubernetes provides container orchestration
capabilities, allowing Coder to efficiently deploy, scale, and manage workspaces
across a distributed infrastructure. This ensures high availability, fault
tolerance, and scalability for Coder deployments. Coder is deployed on this
cluster using the
[Helm chart](../../install/kubernetes.md#install-coder-with-helm).

## Methodology

Our scale tests include the following stages:

1. Prepare environment: create expected users and provision workspaces.

2. SSH connections: establish user connections with agents, verifying their
   ability to echo back received content.

3. Web Terminal: verify the PTY connection used for communication with Web
   Terminal.

4. Workspace application traffic: assess the handling of user connections with
   specific workspace apps, confirming their capability to echo back received
   content effectively.

5. Dashboard evaluation: verify the responsiveness and stability of Coder
   dashboards under varying load conditions. This is achieved by simulating user
   interactions using instances of headless Chromium browsers.

6. Cleanup: delete workspaces and users created in step 1.

## Infrastructure and setup requirements

The scale tests runner can distribute the workload to overlap single scenarios
based on the workflow configuration:

|                      | T0  | T1  | T2  | T3  | T4  | T5  | T6  |
| -------------------- | --- | --- | --- | --- | --- | --- | --- |
| SSH connections      | X   | X   | X   | X   |     |     |     |
| Web Terminal (PTY)   |     | X   | X   | X   | X   |     |     |
| Workspace apps       |     |     | X   | X   | X   | X   |     |
| Dashboard (headless) |     |     |     | X   | X   | X   | X   |

This pattern closely reflects how our customers naturally use the system. SSH
connections are heavily utilized because they're the primary communication
channel for IDEs with VS Code and JetBrains plugins.

The basic setup of scale tests environment involves:

1. Scale tests runner (32 vCPU, 128 GB RAM)
2. Coder: 2 replicas (4 vCPU, 16 GB RAM)
3. Database: 1 instance (2 vCPU, 32 GB RAM)
4. Provisioner: 50 instances (0.5 vCPU, 512 MB RAM)

The test is deemed successful if users did not experience interruptions in their
workflows, `coderd` did not crash or require restarts, and no other internal
errors were observed.

## Traffic Projections

In our scale tests, we simulate activity from 2000 users, 2000 workspaces, and
2000 agents, with two items of workspace agent metadata being sent every 10
seconds. Here are the resulting metrics:

Coder:

- Median CPU usage for _coderd_: 3 vCPU, peaking at 3.7 vCPU while all tests are
  running concurrently.
- Median API request rate: 350 RPS during dashboard tests, 250 RPS during Web
  Terminal and workspace apps tests.
- 2000 agent API connections with latency: p90 at 60 ms, p95 at 220 ms.
- on average 2400 Web Socket connections during dashboard tests.

Provisionerd:

- Median CPU usage is 0.35 vCPU during workspace provisioning.

Database:

- Median CPU utilization is 80%, with a significant portion dedicated to writing
  workspace agent metadata.
- Memory utilization averages at 40%.
- `write_ops_count` between 6.7 and 8.4 operations per second.

## Available reference architectures

[Up to 1,000 users](../architectures/1k-users.md)

[Up to 2,000 users](../architectures/2k-users.md)

[Up to 3,000 users](../architectures/3k-users.md)

## Hardware recommendation

### Control plane: coderd

To ensure stability and reliability of the Coder control plane, it's essential
to focus on node sizing, resource limits, and the number of replicas. We
recommend referencing public cloud providers such as AWS, GCP, and Azure for
guidance on optimal configurations. A reasonable approach involves using scaling
formulas based on factors like CPU, memory, and the number of users.

While the minimum requirements specify 1 CPU core and 2 GB of memory per
`coderd` replica, it is recommended to allocate additional resources depending
on the workload size to ensure deployment stability.

#### CPU and memory usage

Enabling [agent stats collection](../../cli.md#--prometheus-collect-agent-stats)
(optional) may increase memory consumption.

Enabling direct connections between users and workspace agents (apps or SSH
traffic) can help prevent an increase in CPU usage. It is recommended to keep
[this option enabled](../../cli.md#--disable-direct-connections) unless there
are compelling reasons to disable it.

Inactive users do not consume Coder resources.

#### Scaling formula

When determining scaling requirements, consider the following factors:

- `1 vCPU x 2 GB memory` for every 250 users: A reasonable formula to determine
  resource allocation based on the number of users and their expected usage
  patterns.
- API latency/response time: Monitor API latency and response times to ensure
  optimal performance under varying loads.
- Average number of HTTP requests: Track the average number of HTTP requests to
  gauge system usage and identify potential bottlenecks. The number of proxied
  connections: For a very high number of proxied connections, more memory is
  required.

**HTTP API latency**

For a reliable Coder deployment dealing with medium to high loads, it's
important that API calls for workspace/template queries and workspace build
operations respond within 300 ms. However, API template insights calls, which
involve browsing workspace agent stats and user activity data, may require more
time. Moreover, Coder API exposes WebSocket long-lived connections for Web
Terminal (bidirectional), and Workspace events/logs (unidirectional).

If the Coder deployment expects traffic from developers spread across the globe,
be aware that customer-facing latency might be higher because of the distance
between users and the load balancer. Fortunately, the latency can be improved
with a deployment of Coder [workspace proxies](../workspace-proxies.md).

**Node Autoscaling**

We recommend disabling the autoscaling for `coderd` nodes. Autoscaling can cause
interruptions for user connections, see
[Autoscaling](scale-utility.md#autoscaling) for more details.

### Control plane: Workspace Proxies

When scaling [workspace proxies](../workspace-proxies.md), follow the same
guidelines as for `coderd` above:

- `1 vCPU x 2 GB memory` for every 250 users.
- Disable autoscaling.

### Control plane: provisionerd

Each external provisioner can run a single concurrent workspace build. For
example, running 10 provisioner containers will allow 10 users to start
workspaces at the same time.

By default, the Coder server runs 3 built-in provisioner daemons, but the
_Enterprise_ Coder release allows for running external provisioners to separate
the load caused by workspace provisioning on the `coderd` nodes.

#### Scaling formula

When determining scaling requirements, consider the following factors:

- `1 vCPU x 1 GB memory x 2 concurrent workspace build`: A formula to determine
  resource allocation based on the number of concurrent workspace builds, and
  standard complexity of a Terraform template. _Rule of thumb_: the more
  provisioners are free/available, the more concurrent workspace builds can be
  performed.

**Node Autoscaling**

Autoscaling provisioners is not an easy problem to solve unless it can be
predicted when a number of concurrent workspace builds increases.

We recommend disabling autoscaling and adjusting the number of provisioners to
developer needs based on the workspace build queuing time.

### Data plane: Workspaces

To determine workspace resource limits and keep the best developer experience
for workspace users, administrators must be aware of a few assumptions.

- Workspace pods run on the same Kubernetes cluster, but possibly in a different
  namespace or on a separate set of nodes.
- Workspace limits (per workspace user):
  - Evaluate the workspace utilization pattern. For instance, web application
    development does not require high CPU capacity at all times, but will spike
    during builds or testing.
  - Evaluate minimal limits for single workspace. Include in the calculation
    requirements for Coder agent running in an idle workspace - 0.1 vCPU and 256
    MB. For instance, developers can choose between 0.5-8 vCPUs, and 1-16 GB
    memory.

#### Scaling formula

When determining scaling requirements, consider the following factors:

- `1 vCPU x 2 GB memory x 1 workspace`: A formula to determine resource
  allocation based on the minimal requirements for an idle workspace with a
  running Coder agent and occasional CPU and memory bursts for building
  projects.

**Node Autoscaling**

Workspace nodes can be set to operate in autoscaling mode to mitigate the risk
of prolonged high resource utilization.

One approach is to scale up workspace nodes when total CPU usage or memory
consumption reaches 80%. Another option is to scale based on metrics such as the
number of workspaces or active users. It's important to note that as new users
onboard, the autoscaling configuration should account for ongoing workspaces.

Scaling down workspace nodes to zero is not recommended, as it will result in
longer wait times for workspace provisioning by users. However, this may be
necessary for workspaces with special resource requirements (e.g. GPUs) that
incur significant cost overheads.
