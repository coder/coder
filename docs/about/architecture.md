# Architecture

## Agents

An agent is the Coder service that runs within a user's remote workspace.
It provides a consistent interface for coderd and clients to communicate
with workspaces regardless of operating system, architecture, or cloud.

It offers the following services along with much more:

- SSH
- Port forwarding
- Liveness checks
- `startup_script` automation

## Service Bundling

While coderd, provisionerd and Postgres can be orchestrated independently,
our default installation paths bundle them all together into one system service.
It's perfectly fine to run a production deployment this way, but there are
certain situations that necessitate decomposition:

- Reducing global client latency (distribute coderd and centralize database)
- Running untrusted provisioners (separate provisionerd from nodes with DB access)
- Achieving greater availability and efficiency (horizontally scale individual services)

## coderd

coderd is the service created by running `coder server`. It is a thin
API that connects workspaces, provisioners and users. coderd stores its state in
Postgres and is the only service that communicates with Postgres.

It offers:

- Dashboard
- HTTP API
- Dev URLs (HTTP reverse proxy to workspaces)
- Workspace Web Applications (e.g easily access code-server)
- Agent registration

## provisionerd

provisionerd is the execution context for infrastructure modifying providers.
At the moment, the only provider is Terraform (running `terraform`).

Since the provisionerd can be separated from coderd, it can run the provider
in a myriad of ways on the same Coder deployment. For example, provisioners
can have different `terraform` versions to satisfy the requirements of different
templates.

Separability is also advantageous for security. Since provisionerd has no
database access, infrastructure admins that are not necessarily Coder admins
can be safely given access to the provisionerd node. As Coder scales and
multiple infrastructure teams appear, each can be given access to their own
set of provisionerd nodes, with each set of nodes having their own cloud credentials.

## Workspaces

At the highest level, a workspace is a set of cloud resources. These resources
can be VMs, Kubernetes clusters, storage buckets, or whatever else Terraform
lets you dream up.

The resources that run the agent are described as _computational resources_,
while those that don't are called _peripheral resources_.

Each resource may also be _persistent_ or _ephemeral_ depending on whether
they're destroyed on workspace stop.
