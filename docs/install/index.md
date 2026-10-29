---
title: Install Coder in your infrastructure
---

This section covers how to install and run a Coder deployment, from the first decisions to a steady-state operating environment.
You don't have to follow it from start to finish.
Pick the path that matches what you're doing today, and use the rest of the section as a reference when you need it.

## Choose your path

| If you want to                               | Start here                                                 | What you skip                                    |
|----------------------------------------------|------------------------------------------------------------|--------------------------------------------------|
| See what Coder does on a single machine      | [Quickstart](../get-started/index.md)                      | Planning, prerequisites, and validation          |
| Install on a platform you've already chosen  | [Install the control plane](./server/index.md)             | Planning, unless you need to size the deployment |
| Deploy for a team, one step at a time        | [The five phases](#the-five-phases), starting with phase 1 | Nothing                                          |
| Choose a platform or size a deployment       | [Plan your deployment](./plan/index.md)                    | Installation steps                               |
| Upgrade a deployment that's already running  | [Upgrading](./operate/upgrade.md)                          | Everything except the upgrade guidance           |
| Install only the `coder` command-line client | [Coder CLI](./cli.md)                                      | The rest of this section                         |

Each phase page ends with a link to the next one, so you can also enter anywhere and keep reading in order.

## Run Coder on a single server

The fastest way to run a Coder deployment is on one machine.
The [Quickstart](../get-started/index.md) installs Coder, starts it, and launches a first workspace in about 10&nbsp;minutes.
A single-server deployment fits an evaluation, a proof of concept, or a small team.

On a single server, Coder uses its built-in PostgreSQL database unless you set `CODER_PG_CONNECTION_URL`.
That removes the database from your prerequisites, which is the main reason the Quickstart is quick.
It also means the database lives on the same machine as Coder, so a deployment that people depend on needs a backup plan for that machine.

When you outgrow a single server, you don't start over.
Move the data to an external PostgreSQL database by following [Migrate from the built-in database to an external database](../admin/setup/index.md#migrate-from-the-built-in-database-to-an-external-database), then read [Plan your deployment](./plan/index.md) to decide how to grow.

## The five phases

For a deployment others will rely on, work through the phases in order.
Each phase produces the information the next one needs.

| Phase                                              | What it answers                                                          | What you finish with                                                           |
|----------------------------------------------------|--------------------------------------------------------------------------|--------------------------------------------------------------------------------|
| [1. Plan your deployment](./plan/index.md)         | Which components run where, how large they need to be, who operates them | A deployment shape and a size target you can hand to an infrastructure team    |
| [2. Prepare prerequisites](./prepare/index.md)     | What must exist before installation starts, and who owns each piece      | DNS, certificates, a database, a license, and an identity provider application |
| [3. Install the control plane](./server/index.md)  | How to install Coder on the platform your team operates                  | A running Coder deployment you can sign in to                                  |
| [4. Validate your deployment](./validate/index.md) | Whether the deployment is hardened, works, and holds up under load       | Evidence that the deployment is ready for real users                           |
| [5. Operate and maintain](./operate/index.md)      | How to keep the deployment healthy, upgraded, and correctly sized        | An upgrade and scaling routine, plus a removal path                            |

Phases 1 and 2 are advisory rather than a gate.
Nothing stops you from installing the control plane first, but the recommendations in those phases are what keep a deployment from needing to be rebuilt later.

### Phase 1: Plan your deployment

Planning decides the topology, size, provisioning model, and network position of the deployment.
It's the phase to read when you don't yet know what to install, or when more than one team has to agree on the answer.
You finish with a deployment shape and a size target.

Skip it if you've already chosen a platform and you're deploying for a small number of users.
Come back to it when the deployment grows, because topology, size, and network position are the decisions that are hardest to change after installation.

### Phase 2: Prepare prerequisites

Preparation gathers what Coder depends on: a hostname and DNS records, TLS certificates, a PostgreSQL database, a license, and an identity provider application.
Most of these belong to another team, so their lead time usually sets your timeline.
Start the slowest requests early.

You can skip any item that doesn't apply to your deployment.
A proof of concept can use the built-in database and skip the license and the identity provider application, so it needs little of this phase.
A production deployment needs most of it.

### Phase 3: Install the control plane

The pages in this phase are alternatives, not sequential steps.
Choose the platform your team already operates, then follow that page to the end.
You finish with a deployment you can reach and sign in to.

This is the one phase you can't skip.
If you know your platform, you can start here and return to phases 1 and 2 for the details you need.

### Phase 4: Validate your deployment

Validation hardens the deployment, then proves it works and holds up under load.
It's how you find out whether the size target you chose in phase 1 was right.

Skip it for an evaluation.
Do it before real users depend on the deployment.

### Phase 5: Operate and maintain

Day-two operations cover upgrades, provisioning capacity, and removal.
Read it after your deployment is running, and again before each upgrade.

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
| Identity provider administrator  | The OIDC application, claims, and group mappings                             | 2                |
| Security and compliance reviewer | Audit requirements, network policy, data handling, and sign-off              | 2, 4             |
| Licensing contact                | Purchasing and renewing the Coder license                                    | 2                |
| Template author                  | The workspace templates developers use once the deployment is running        | 4, 5             |

These are roles, not people.
One person may serve as several actors, which is common for a proof of concept run by a single platform engineer.
Several people may also serve as one actor, which is common for an infrastructure team that splits networking and compute ownership.
Use the table to identify who has to agree, not to count headcount.
