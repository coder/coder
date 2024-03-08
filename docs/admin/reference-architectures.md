# Reference architectures

This document provides prescriptive solutions and reference architectures to
support successful deployments of up to 2000 users and outlines at a high-level
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

- Median CPU usage for _coderd_: 3 vCPU, peaking at 3.7 vCPU during dashboard
  tests.
- Median API request rate: 350 req/s during dashboard tests, 250 req/s during
  Web Terminal and workspace apps tests.
- 2000 agent API connections with latency: p90 at 60 ms, p95 at 220 ms.
- on average 2400 Web Socket connections during dashboard tests.

Provisionerd:

- Median CPU usage is 0.35 vCPU during workspace provisioning.

Database:

- Median CPU utilization is 80%, with a significant portion dedicated to writing
  metadata.
- Memory utilization averages at 40%.
- `write_ops_count` between 6.7 and 8.4 operations per second.
