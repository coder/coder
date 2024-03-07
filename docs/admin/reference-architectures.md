# Reference architectures

As Coder evolves to meet the demands of modern development workflows, ensuring
scalability is paramount. Today, we're stress-testing our platform with 2000
concurrent users, preparing for deployments of up to 5000 users. This
documentation provides prescriptive solutions and reference architectures to
support successful customer deployments.

Let's dive into the core concepts and terminology essential for understanding
Coder's architecture and deployment strategies.

## Glossary

### Administrator

An administrator is a user role within the Coder platform with elevated
privileges. Admins have access to administrative functions such as user
management, template definitions, insights, and deployment configuration.

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
Coder templates and deployed on nodes by provisioners.

### Template

A template in Coder is a predefined configuration for creating workspaces.
Templates streamline the process of workspace creation by providing
pre-configured settings, tooling, and dependencies. They are built by template
administrators on top of Terraform, allowing for efficient management of
infrastructure resources. Additionally, templates can utilize Coder modules to
leverage existing features shared with other templates, enhancing flexibility
and consistency across deployments. Templates describe provisioning rules for
infrastructure resources offered by cloud providers.

### Proxy

A workspace proxy serves as a relay connection option for developers connecting
to their workspace over SSH, a workspace app, or through port forwarding. It
helps reduce network latency for geo-distributed teams by minimizing the
distance network traffic needs to travel. Notably, workspace proxies do not
handle dashboard connections or API calls.

### Provisioner

Provisioners in Coder execute Terraform during workspace and template builds.
While the platform includes built-in provisioner daemons by default, there are
advantages to employing external provisioners. These external daemons provide
secure build environments, and reduce server load, improving performance and
scalability. Each provisioner can handle a single concurrent workspace build,
allowing for efficient resource allocation and workload management.

### Registry

The Coder Registry hosts starter templates for various cloud providers and
orchestration platforms, enabling self-service cloud development environments
via Terraform-defined infrastructure. Additionally, Coder introduces _Modules_
to streamline template creation by extracting commonly used functionalities such
as web IDEs, third-party integrations, and helper scripts into reusable
components.

The Registry is hosted service and it is not available for air-gapped
deployments.

### Kubernetes cluster for Coder

A dedicated cluster for Coder is Kubernetes cluster specifically configured to
host and manage Coder workloads. Kubernetes provides container orchestration
capabilities, allowing Coder to efficiently deploy, scale, and manage workspaces
across a distributed infrastructure. This ensures high availability, fault
tolerance, and scalability for Coder deployments.

The cluster can be deployed using the Helm chart.

## Scale tests methodology

Scaling Coder involves careful planning and testing to ensure it can handle more
users without slowing down. This process encompasses infrastructure setup,
traffic projections, and aggressive testing to identify and mitigate potential
bottlenecks.

### Infrastructure and setup requirements

In a single workflow, the scale tests runner maintains a consistent load
distribution as follows:

- 80% of users open and utilize SSH connections.
- 25% of users connect to the workspace using the Web Terminal.
- 40% of users simulate traffic for workspace apps.
- 20% of users interact with the Coder UI via a headless browser.

This distribution closely mirrors natural user behavior, as observed among our
customers.

The basic setup of scale tests environment involves:

1. Scale tests runner: `c2d-standard-32` (32 vCPU, 128 GB RAM)
2. Coderd: 2 replicas (4 vCPU, 16 GB RAM)
3. Database: 1 replica (2 vCPU, 32 GB RAM)
4. Provisioner: 50 instances (0.5 vCPU, 512 MB RAM)

No pod restarts or internal errors were observed.

### Traffic Projections

In our scale tests, we simulate activity from 2000 users, 2000 workspaces, and
2000 agents, with metadata being sent 2 x every 10 s. Here are the resulting
metrics:

Coderd:

- Median CPU usage for coderd: 3 vCPU, peaking at 3.7 vCPU during dashboard
  tests.
- Median API request rate: 350 req/s during dashboard tests, 250 req/s during
  Web Terminal and workspace apps tests.
- 2000 agent API connections with latency: p90 at 60 ms, p95 at 220 ms.
- on average 2400 Web Socket connections during dashboard tests.

Provisionerd:

- Median CPU usage is 0.35 vCPU during workspace provisioning.

Database:

- Median CPU utilization: 80%.
- Median memory utilization: 40%.
- Average write_ops_count per minute: between 400 and 500 operations.
