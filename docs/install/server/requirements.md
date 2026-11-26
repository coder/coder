---
title: Installation requirements
---

This page lists what a Coder deployment administrator needs before installing the Coder control plane.
Each guide in [Install the control plane](./index.md) repeats the requirements for its own platform.
For the items other teams usually provide, such as DNS records, certificates, and an identity provider application, refer to [Prepare prerequisites](../prepare/index.md).

## Control plane

| Resource         | Requirement                                                 |
|------------------|-------------------------------------------------------------|
| CPU and memory   | 2&nbsp;CPU cores and 4&nbsp;GiB of memory to evaluate Coder |
| Operating system | Linux, for every guide that Coder maintains in this section |
| Architecture     | `amd64`, `arm64`, or `armv7`                                |

On Kubernetes, the Helm chart requests and limits the Coder container to 2&nbsp;CPU cores and 4&nbsp;GiB of memory unless you set `coder.resources` in your values file.
To try Coder on macOS or Windows, follow the [Quickstart](../../get-started/index.md) instead.

To size a production deployment, refer to the [Coder Validated Architecture](../plan/sizing/index.md).
Its reference architectures cover deployments of up to 1,000, 2,000, 3,000, and 10,000 users.

## Database

Coder stores its data in PostgreSQL 13 or later.

- On Kubernetes, you provide the database.
  The [Kubernetes guide](./kubernetes/index.md#2-create-a-postgresql-instance) covers a managed PostgreSQL service and an in-cluster database for a proof of concept.
- The [Docker Compose example](./docker.md#install-coder-via-docker-compose) runs PostgreSQL in a container next to Coder.
- On a single host, `coder server` starts a built-in PostgreSQL database when `CODER_PG_CONNECTION_URL` isn't set.
  The built-in database downloads PostgreSQL from the internet the first time it starts, so a deployment without internet access needs an external database.

For production, use an [external PostgreSQL database](./external-database.md) with a backup and restore plan.

## Platform

| Platform                                           | Requirements                                                                                                                                         |
|----------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Kubernetes](./kubernetes/index.md)                | Kubernetes 1.19 or later, and Helm 3.5 or later on the machine you install from                                                                      |
| [Rancher](./rancher.md)                            | SUSE Rancher Manager 2.10 or later with the Rancher Prime distribution, a workload cluster that runs Kubernetes 1.19 or later, and Helm 3.5 or later |
| [OpenShift](./openshift.md)                        | OpenShift 4.7 or later, Helm 3.5 or later, the OpenShift CLI (`oc`), and the `coder` CLI                                                             |
| [Docker](./docker.md)                              | Docker on a Linux host                                                                                                                               |
| [Red Hat Enterprise Linux (RHEL) family](./rpm.md) | A [supported distribution](./rpm.md#supported-distributions) and `root` or `sudo` access                                                             |
| [Cloud platforms](./cloud/index.md)                | The cloud account permissions that each guide lists                                                                                                  |

## Network

- An [access URL](../../admin/setup/index.md#access-url): a hostname or IP address that both users and workspaces can reach.
- Without an access URL, `coder server` creates a public URL through a tunnel, which needs internet access.
- A [wildcard access URL](../../admin/setup/index.md#wildcard-access-url), such as `*.coder.example.com`, if you want to forward ports from the dashboard or serve workspace apps on subdomains.
  The wildcard hostname needs a DNS record that resolves to Coder, and a TLS certificate that covers it.

## Internet access

Coder runs without internet access.
Features that call a service outside your network, such as a hosted AI provider, work only if your network can reach that service.
A deployment without internet access needs the following:

- An external PostgreSQL database.
- A way to install the Terraform providers that your templates use.
- Templates that get their modules and tools from inside your network.

Refer to [Air-gapped deployments](../prepare/airgap.md).

## License

You can install Coder without a license.
Some features, including [AI Governance](../../ai-governance/index.md), need a Premium license.
Coder validates license keys locally, without an outbound connection, so a license also works in a deployment without internet access.
Refer to [Licensing](../prepare/licensing.md).
