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

## Proxy

A workspace proxy serves as a relay connection option for developers connecting
to their workspace over SSH, a workspace app, or through port forwarding. It
helps reduce network latency for geo-distributed teams by minimizing the
distance network traffic needs to travel. Notably, workspace proxies do not
handle dashboard connections or API calls.

## Provisioner

Provisioners in Coder execute Terraform during workspace and template builds.
While the platform includes built-in provisioner daemons by default, there are
advantages to employing external provisioners. These external daemons provide
secure build environments, and reduce server load, improving performance and
scalability. Each provisioner can handle a single concurrent workspace build,
allowing for efficient resource allocation and workload management.

## Registry

A registry in Coder is a centralized repository for storing and managing
container images used within the platform. By leveraging a registry, users can
easily share, distribute, and deploy containerized applications across their
development workflows, ensuring consistency and reliability.

## Kubernetes Cluster for Coder

A Kubernetes cluster for Coder is a dedicated cluster specifically configured to
host and manage Coder workloads. Kubernetes provides container orchestration
capabilities, allowing Coder to efficiently deploy, scale, and manage workspaces
across a distributed infrastructure. This ensures high availability, fault
tolerance, and scalability for Coder deployments.
