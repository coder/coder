# Reference Architectures

This document provides prescriptive solutions and reference architectures to
support successful deployments of up to 3000 users and outlines at a high-level
the methodology currently used to scale-test Coder.

## General concepts

This section outlines core concepts and terminology essential for understanding
Coder's architecture and deployment strategies.

### Administrator

An administrator is a user role within the Coder platform with elevated
privileges. Admins have access to administrative functions such as user
management, template definitions, insights, and deployment configuration.

### Coder

Coder, also known as _coderd_, is the main service recommended for deployment
with multiple replicas to ensure high availability. It provides an API for
managing workspaces and templates. Each _coderd_ replica has the capability to
host multiple [provisioners](#provisioner).

### User

A user is an individual who utilizes the Coder platform to develop, test, and
deploy applications using workspaces. Users can select available templates to
provision workspaces. They interact with Coder using the web interface, the CLI
tool, or directly calling API methods.

### Workspace

A workspace refers to an isolated development environment where users can write,
build, and run code. Workspaces are fully configurable and can be tailored to
specific project requirements, providing developers with a consistent and
efficient development environment. Workspaces can be autostarted and
autostopped, enabling efficient resource management.

Users can connect to workspaces using SSH or via workspace applications like
`code-server`, facilitating collaboration and remote access. Additionally,
workspaces can be parameterized, allowing users to customize settings and
configurations based on their unique needs. Workspaces are instantiated using
Coder templates and deployed on resources created by provisioners.

### Template

A template in Coder is a predefined configuration for creating workspaces.
Templates streamline the process of workspace creation by providing
pre-configured settings, tooling, and dependencies. They are built by template
administrators on top of Terraform, allowing for efficient management of
infrastructure resources. Additionally, templates can utilize Coder modules to
leverage existing features shared with other templates, enhancing flexibility
and consistency across deployments. Templates describe provisioning rules for
infrastructure resources offered by Terraform providers.

### Workspace Proxy

A workspace proxy serves as a relay connection option for developers connecting
to their workspace over SSH, a workspace app, or through port forwarding. It
helps reduce network latency for geo-distributed teams by minimizing the
distance network traffic needs to travel. Notably, workspace proxies do not
handle dashboard connections or API calls.

### Provisioner

Provisioners in Coder execute Terraform during workspace and template builds.
While the platform includes built-in provisioner daemons by default, there are
advantages to employing external provisioners. These external daemons provide
secure build environments and reduce server load, improving performance and
scalability. Each provisioner can handle a single concurrent workspace build,
allowing for efficient resource allocation and workload management.

### Registry

The Coder Registry is a platform where you can find starter templates and
_Modules_ for various cloud services and platforms.

Templates help create self-service development environments using
Terraform-defined infrastructure, while _Modules_ simplify template creation by
providing common features like workspace applications, third-party integrations,
or helper scripts.

Please note that the Registry is a hosted service and isn't available for
offline use.

## Scale-testing methodology

Scaling Coder involves planning and testing to ensure it can handle more load
without compromising service. This process encompasses infrastructure setup,
traffic projections, and aggressive testing to identify and mitigate potential
bottlenecks.

A dedicated Kubernetes cluster for Coder is Kubernetes cluster specifically
configured to host and manage Coder workloads. Kubernetes provides container
orchestration capabilities, allowing Coder to efficiently deploy, scale, and
manage workspaces across a distributed infrastructure. This ensures high
availability, fault tolerance, and scalability for Coder deployments. Code is
deployed on this cluster using the
[Helm chart](../install/kubernetes#install-coder-with-helm).

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

### Infrastructure and setup requirements

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

### Traffic Projections

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

[Up to 1,000 users](1k-users.md)

[Up to 2,000 users](2k-users.md)

[Up to 3,000 users](3k-users.md)

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

- `1 vCPU x 2 GB memory x 250 users`: A reasonable formula to determine resource
  allocation based on the number of users and their expected usage patterns.
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
interruptions for user connections, see [Autoscaling](../scale.md#autoscaling)
for more details.

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

### Data plane: External database

While running in production, Coder requires a access to an external PostgreSQL
database. Depending on the scale of the user-base, workspace activity, and High
Availability requirements, the amount of CPU and memory resources required by
Coder's database may differ.

#### Scaling formula

When determining scaling requirements, take into account the following
considerations:

- `2 vCPU x 8 GB RAM x 512 GB storage`: A baseline for database requirements for
  Coder deployment with less than 1000 users, and low activity level (30% active
  users). This capacity should be sufficient to support 100 external
  provisioners.
- Storage size depends on user activity, workspace builds, log verbosity,
  overhead on database encryption, etc.
- Allocate two additional CPU core to the database instance for every 1000
  active users.
- Enable _High Availability_ mode for database engine for large scale
  deployments.

If you enable [database encryption](../encryption.md) in Coder, consider
allocating an additional CPU core to every `coderd` replica.

#### Performance optimization guidelines

We provide the following general recommendations for PostgreSQL settings:

- Increase number of vCPU if CPU utilization or database latency is high.
- Allocate extra memory if database performance is poor, CPU utilization is low,
  and memory utilization is high.
- Utilize faster disk options (higher IOPS) such as SSDs or NVMe drives for
  optimal performance enhancement and possibly reduce database load.
