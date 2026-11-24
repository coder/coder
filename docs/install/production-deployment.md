---
title: Production deployment process
---

This page is for a Coder deployment administrator who prepares a deployment that other people will depend on.
It covers the five phases of a production deployment, which phases an evaluation can skip, and who takes part in each one.
To evaluate Coder first, follow the [Quickstart](../get-started/index.md) on your own computer, or [Install Coder on K3s in an air-gapped environment](./server/kubernetes/k3s-air-gapped.md) on one machine in your network.

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
| [5. Operate and maintain](./operate/index.md)      | How to keep the deployment healthy, upgraded, and correctly sized        | Never: it starts once the deployment is running                                                             |

Each phase page ends with a link to the next one, so you can also enter anywhere and keep reading in order.
Most of the prerequisites in phase 2 belong to another team, so start the slowest requests early.

## Move from evaluation to production

An evaluation deployment skips the decisions a production deployment needs, and you don't have to start over to make them.
The [Quickstart](../get-started/index.md), for example, runs Coder on one machine and launches a first workspace in under 10&nbsp;minutes.
When `CODER_ACCESS_URL` isn't set, `coder server` opens a tunnel and gives you a `try.coder.app` URL, so you don't need DNS records or TLS certificates.
Because anyone can reach that URL, the server prints a setup token in its output, and the setup page asks for the token before it creates the first account.
You create the first account with a password and can switch it to GitHub sign-in afterward under **Account** > **Security**, so you don't set up an identity provider application, and you don't need a license.

Coder also uses its built-in PostgreSQL database unless you set `CODER_PG_CONNECTION_URL`.
The built-in database lives on the same machine as Coder, so a deployment that people depend on needs a backup plan for that machine.

To move an evaluation deployment to production, start with the database.
Follow [Migrate from the built-in database to an external database](../admin/setup/index.md#migrate-from-the-built-in-database-to-an-external-database), then read [Plan your deployment](./plan/index.md) and [Prepare prerequisites](./prepare/index.md) for the rest.
Before real users depend on the deployment, harden it and test it under load in [Validate your deployment](./validate/index.md).

## Decisions worth making deliberately

Most installation choices are easy to revisit.
A few take real work to change after the fact, so it's worth making them on purpose:

- **Where the database runs.**
  Moving from the built-in database to an external one is possible, but it means stopping Coder, dumping the database, and restoring it elsewhere.
  If you already know the deployment will serve a team, start with an external PostgreSQL database.
- **Topology, size, and network position.**
  Changing how the control plane, database, and workspaces are laid out, or whether the deployment is public, private, or air-gapped, can mean rebuilding parts of it.
  Refer to [Plan your deployment](./plan/index.md).

## Who does the work

A Coder deployment usually crosses team boundaries.
The following actors appear throughout the five phases, so you can anticipate which conversations you need to start and how long they're likely to take.

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

Next: [Plan your deployment](./plan/index.md).
