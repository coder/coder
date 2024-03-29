# Architecture

The Coder deployment model is flexible and offers various components that
platform administrators can deploy and scale depending on the use cases. This
page describes possible deployments, challenges, and risks associated with them.

Learn more about our [Reference Architectures](../admin/architectures/index.md)
and platform scaling capabilities.

## Components

### coderd

_coderd_ is the service created by running `coder server`. It is a thin API that
connects workspaces, provisioners and users. _coderd_ stores its state in
Postgres and is the only service that communicates with Postgres.

It offers:

- Dashboard (UI)
- HTTP API
- Dev URLs (HTTP reverse proxy to workspaces)
- Workspace Web Applications (e.g easily access `code-server`)
- Agent registration

### provisionerd

_provisionerd_ is the execution context for infrastructure modifying providers.
At the moment, the only provider is Terraform (running `terraform`).

By default, the Coder server runs multiple provisioner daemons.
[External provisioners](../admin/provisioners.md) can be added for security or
scalability purposes.

### Agents

An agent is the Coder service that runs within a user's remote workspace. It
provides a consistent interface for coderd and clients to communicate with
workspaces regardless of operating system, architecture, or cloud.

It offers the following services along with much more:

- SSH
- Port forwarding
- Liveness checks
- `startup_script` automation

Templates are responsible for
[creating and running agents](../templates/index.md#coder-agent) within
workspaces.

### Service Bundling

While _coderd_ and Postgres can be orchestrated independently, our default
installation paths bundle them all together into one system service. It's
perfectly fine to run a production deployment this way, but there are certain
situations that necessitate decomposition:

- Reducing global client latency (distribute coderd and centralize database)
- Achieving greater availability and efficiency (horizontally scale individual
  services)

### Workspaces

At the highest level, a workspace is a set of cloud resources. These resources
can be VMs, Kubernetes clusters, storage buckets, or whatever else Terraform
lets you dream up.

The resources that run the agent are described as _computational resources_,
while those that don't are called _peripheral resources_.

Each resource may also be _persistent_ or _ephemeral_ depending on whether
they're destroyed on workspace stop.

## Deployment models

### Single region architecture

![Architecture Diagram](../images/architecture-single-region.png)

#### Components

This architecture consists of a single load balancer, several _Coder Server_
replicas, and _Coder workspaces_ deployed in the same region.

##### Workload resources

- Use Terraform to deploy at least one **Coder Server Replica** per availability
  zone with _Coder Server_ instances and provisioners. High availability is
  recommended but not essential for small deployments.
- Single replica deployment is a special case that can address a
  tiny/small/proof-of-concept installation on a single virtual machine serving
  less than 100 workspace users.

**Coder workspace**

- For small deployments consider a lightweight workspace runtime like
  [Sysbox](https://github.com/nestybox/sysbox) container runtime. Learn more how
  to enable
  [docker-in-docker using Sysbox](https://asciinema.org/a/kkTmOxl8DhEZiM2fLZNFlYzbo?speed=2).

**HA Database**

- Enable replication across all availability zones.
- Monitor replication lag, node status, and resource utilization metrics.
- Implement robust backup and disaster recovery strategies to protect against
  data loss.

##### Workload supporting resources

**Load balancer**

- Distributes and load balances traffic from agents and clients to _Coder
  Server_ replicas across zones.
- Layer 7 load balancing. Decrypt SSL traffic, and re-encrypt using internal
  certificate.
- Sessions persistence (sticky sessions) can be disabled as _Coder Server_
  instances are stateless.
- WebSocket and long-time connections must be supported.

**Single sign-on**

- Integrate with existing Single Sign-On (SSO) solution used within the
  organization, OAuth 2.0 or OpenID Connect standards are supported.
- Learn more about [Authentication in Coder](../admin/auth.md).

### Multi-region architecture

![Architecture Diagram](../images/architecture-multi-region.png)

#### Components

This architecture is for globally distributed developer teams using Coder
workspaces on daily basis. It features a single load balancer with regionally
deployed _Workspace Proxy Replicas_, several _Coder Server_ replicas, and _Coder
workspaces_ provisioned in different regions.

Note: The _multi-region architecture_ assumes the same deployment principles as
the _single region architecture_, but it extends them to multi region deployment
with workspace proxies. Provisioners deployed in regions closed to developers
offer the fastest developer experience.

##### Workload resources

**Workspace proxy**

- Workspace proxy offers developers the option to establish a fast relay
  connection when accessing their workspace via SSH, a workspace application, or
  port forwarding.
- Dashboard connections, API calls (e.g. _list workspaces_) are not served over
  proxies.
- Proxies do not establish connections to the database.
- Do not share authentication tokens between proxy instances.

##### Workload supporting resources

**Proxy load balancer**

- Distributes and load balances workspace relay traffic in a single region
  across availability zones.
- Layer 7 load balancing. Decrypt SSL traffic, and re-encrypt using internal
  certificate.
