---
title: Install Coder in your infrastructure
---

This section is for a Coder deployment administrator who installs and runs the Coder control plane on infrastructure they control.
To install only the `coder` command-line client and sign in to a deployment that already exists, use the [Coder downloads page](https://coder.com/downloads).

## Install the control plane now

You can install the control plane before you finish planning.
To choose a guide, answer three questions:

1. Can the deployment reach the internet?
   If it can't, also read [Deployments without internet access](#deployments-without-internet-access).
1. Does Coder run in containers or on virtual machines?
1. Does Coder run in the cloud or on premises?

Choose the guide that matches your answers, then follow it to the end.
Each guide finishes with a deployment you can sign in to.

- **[Kubernetes](./server/kubernetes/index.md)** (recommended): run Coder in containers on a Kubernetes cluster, in the cloud or on premises, with the Helm chart.
  The guide works on most clusters, including Amazon Elastic Kubernetes Service (EKS), Azure Kubernetes Service (AKS), Google Kubernetes Engine (GKE), and K3s.
  For a cluster that you manage with [Rancher](./server/rancher.md) or run on [OpenShift](./server/openshift.md), follow that guide instead.
- **[Docker](./server/docker.md)**: run Coder and PostgreSQL in containers on one Linux host.
- **[RHEL-family Linux](./server/rpm.md)**: install Coder from an RPM package and run it as a `systemd` service on one Red Hat Enterprise Linux (RHEL) family host or virtual machine.
- **[Cloud platforms](./server/cloud/index.md)**: launch Coder from a marketplace listing or on a virtual machine in AWS, Google Cloud, or Azure.
  To install on a cluster you already run in the cloud, use the Kubernetes guide.

Kubernetes is the recommended platform for a deployment that other people depend on, and the [Coder Validated Architecture](./plan/sizing/index.md) reference designs assume it.
Before you start, check the [installation requirements](./server/requirements.md) for the platform you choose.

### Deployments without internet access

All Coder features work in a network with no internet access.
If your deployment has none, read [Air-gapped deployments](./prepare/airgap.md) before you install, and use it alongside your platform guide.
An air-gapped deployment needs an external PostgreSQL database and a way to install the Terraform providers that your templates use without the public Terraform registry.

To try an air-gapped deployment from start to finish on one machine, follow [Install Coder on K3s in an air-gapped environment](./server/kubernetes/k3s-air-gapped.md).

## Choose your path

| If you want to                                                           | Start here                                                                                       | What you skip                           |
|--------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------|-----------------------------------------|
| Try Coder on your own machine                                            | [Quickstart](../get-started/index.md)                                                            | Planning, prerequisites, and validation |
| Evaluate Coder in your infrastructure without internet access            | [Install Coder on K3s in an air-gapped environment](./server/kubernetes/k3s-air-gapped.md)       | Planning and prerequisites              |
| Deploy for production, with single sign-on and sign-off from other teams | [The five phases](#the-five-phases), starting with [Plan your deployment](./plan/index.md)       | Nothing                                 |
| Move an evaluation deployment to production                              | [Move from evaluation to production](#move-from-evaluation-to-production)                        | Reinstalling Coder                      |
| Check whether Coder fits your infrastructure                             | [Installation requirements](./server/requirements.md) and [Architecture](./plan/architecture.md) | Installation steps                      |
| Upgrade a deployment that's already running                              | [Upgrading](./operate/upgrade.md)                                                                | Planning and installation               |

## After the control plane is running

- [Create your first template](../templates/creating-templates.md#from-a-starter-template) from a starter template, then create a workspace from it to confirm the deployment works end to end.
- In a deployment without internet access, make the providers, modules, and tools your template downloads available inside your network before that first workspace build.
  For example, the Kubernetes starter template installs code-server from the internet when a workspace starts.
  Refer to [Air-gapped deployments](./prepare/airgap.md).
- [Validate your deployment](./validate/index.md) before other people depend on it.

## Move from evaluation to production

An evaluation deployment skips the decisions a production deployment needs, and you don't have to start over to make them.
The [Quickstart](../get-started/index.md), for example, runs Coder on one machine and launches a first workspace in about 10&nbsp;minutes.
When `CODER_ACCESS_URL` isn't set, `coder server` opens a tunnel and gives you a `try.coder.app` URL, so you don't need DNS records or TLS certificates.
Because anyone can reach that URL, the server prints a setup token in its output, and the setup page asks for the token before it creates the first account.
You create the first account with a password and can switch it to GitHub sign-in afterward under **Account** > **Security**, so you don't set up an identity provider application, and you don't need a license.

Coder also uses its built-in PostgreSQL database unless you set `CODER_PG_CONNECTION_URL`.
The built-in database lives on the same machine as Coder, so a deployment that people depend on needs a backup plan for that machine.

To move an evaluation deployment to production, start with the database.
Follow [Migrate from the built-in database to an external database](../admin/setup/index.md#migrate-from-the-built-in-database-to-an-external-database), then read [Plan your deployment](./plan/index.md) and [Prepare prerequisites](./prepare/index.md) for the rest.
Before real users depend on the deployment, harden it and test it under load in [Validate your deployment](./validate/index.md).

## The five phases

For a deployment others will rely on, the five phases cover the work in order.
Each phase produces the information the next one needs.
The phases are recommendations, not a gate: you can install the control plane at any point, and use the phases for what comes before and after it.

| Phase                                              | What it answers                                                          | When you can skip it                                                                                        |
|----------------------------------------------------|--------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| [1. Plan your deployment](./plan/index.md)         | Which components run where, how large they need to be, who operates them | You've already chosen a platform and you're deploying for a small number of users                           |
| [2. Prepare prerequisites](./prepare/index.md)     | What must exist before installation starts, and who owns each piece      | A proof of concept can use the built-in database and skip the license and the identity provider application |
| [3. Install the control plane](./server/index.md)  | How to install Coder on the platform your team operates                  | Never: every deployment needs this phase                                                                    |
| [4. Validate your deployment](./validate/index.md) | Whether the deployment is hardened, works, and holds up under load       | An evaluation, until real users depend on the deployment                                                    |
| [5. Operate and maintain](./operate/index.md)      | How to keep the deployment healthy, upgraded, and correctly sized        | Until the deployment is running                                                                             |

Each phase page ends with a link to the next one, so you can also enter anywhere and keep reading in order.
Most of the prerequisites in phase 2 belong to another team, so start the slowest requests early.

## Decisions worth making deliberately

Most choices in this section are easy to revisit.
A few take real work to change after the fact, so it's worth making them on purpose:

- **Where the database runs.**
  Moving from the built-in database to an external one is possible, but it means stopping Coder, dumping the database, and restoring it elsewhere.
  If you already know the deployment will serve a team, start with an external PostgreSQL database.
- **Topology, size, and network position.**
  Changing how the control plane, database, and workspaces are laid out, or whether the deployment is public, private, or air-gapped, can mean rebuilding parts of it.
  Refer to [Plan your deployment](./plan/index.md).

<children></children>

## Who does the work

A Coder deployment usually crosses team boundaries.
The following actors appear throughout this section, so you can anticipate which conversations you need to start and how long they're likely to take.

| Actor                            | Responsible for                                                              | Appears in phase |
|----------------------------------|------------------------------------------------------------------------------|------------------|
| Coder platform owner             | The deployment itself: installation, configuration, upgrades, and support    | 1 to 5           |
| Infrastructure team              | Clusters, virtual machines, networking, load balancers, and egress rules     | 1 to 4           |
| DNS and TLS owner                | The Coder hostname, the wildcard record for workspace apps, and certificates | 2                |
| Database administrator           | PostgreSQL provisioning, sizing, backups, and restore tests                  | 2, 4, 5          |
| Identity provider administrator  | The OpenID Connect (OIDC) application, claims, and group mappings            | 2                |
| Security and compliance reviewer | Audit requirements, network policy, data handling, and sign-off              | 2, 4             |
| Licensing contact                | Purchasing and renewing the Coder license                                    | 2                |
| Template author                  | The workspace templates developers use once the deployment is running        | 4, 5             |

These are roles, not people.
One person can serve as several actors, which is common for a proof of concept run by a single platform engineer.
Several people can also serve as one actor, which is common for an infrastructure team that splits networking and compute ownership.
Use the table to identify who has to agree, not to count headcount.
