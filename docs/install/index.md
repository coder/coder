---
title: Install Coder in your infrastructure
---

This section is for a Coder deployment administrator who installs and runs the Coder control plane on infrastructure they control, including networks with no internet access.

> [!TIP]
> To try Coder on your own computer first, follow the [Quickstart](../get-started/index.md).
> To install only the `coder` command-line client and sign in to a deployment that already exists, use the [Coder downloads page](https://coder.com/downloads).

## Install the control plane

<div class="cards">

- [Installation requirements](./server/requirements.md): Check the CPU, memory, database, platform versions, and network access the control plane needs.
- [Air-gapped K3s quickstart](./server/kubernetes/k3s-air-gapped.md): Install Coder on one K3s node with no internet access, from staging the artifacts to a running workspace.
- [Install on Kubernetes](./server/kubernetes/index.md): The recommended platform for production, installed with the Helm chart on a managed cloud cluster, an on-premises cluster, or K3s.
- [Installation methods](./server/index.md): Compare every supported platform, from Kubernetes, Rancher, and OpenShift to Docker, Linux hosts, and cloud virtual machines.

</div>

## Run Coder in production

<div class="cards">

- [Production deployment process](./production-deployment.md): Plan, prepare, install, validate, and operate a deployment that other people depend on.
- [Upgrade Coder](./operate/upgrade.md): Move a running deployment to a newer release, with steps for each installation method.

</div>

## Common tasks

<div class="cards">

- [Air-gapped deployments](./prepare/airgap.md): Find what Coder and your templates download from the internet, and how to provide each download in a network with no internet access.
- [Use an external database](./server/external-database.md): Create a PostgreSQL role and database for the control plane, connect Coder to it, and use a custom schema if you need one.
- [TLS and reverse proxies](../admin/setup/index.md#tls--reverse-proxy): Serve Coder over HTTPS with its own TLS certificates, or behind Apache, Caddy, or NGINX.
- [Image pull secrets](./operate/image-pull-secret.md): Pull workspace images from a private registry with a Kubernetes image pull secret that your template references.

</div>

<children></children>
