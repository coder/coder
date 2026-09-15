---
title: Install Coder in your infrastructure
---

This section takes a Coder deployment from the first decision to steady-state operation.
The five phases are meant to be read in order: each one produces the information that the next one needs.

> [!TIP]
> If you only need the `coder` command-line client on your own machine, refer to [Coder CLI](./cli.md).
> You don't need the rest of this section.
>
> If you want a Coder deployment on a single machine as quickly as possible, follow the [Quickstart](../get-started/index.md) instead.
> It installs Coder and launches a first workspace without the planning and preparation work described here.
> Come back to this section when other people depend on the deployment.

## The five phases

| Phase                                              | What it answers                                                          | What you finish with                                                           |
|----------------------------------------------------|--------------------------------------------------------------------------|--------------------------------------------------------------------------------|
| [1. Plan your deployment](./plan/index.md)         | Which components run where, how large they need to be, who operates them | A deployment shape and a size target you can hand to an infrastructure team    |
| [2. Prepare prerequisites](./prepare/index.md)     | What must exist before installation starts, and who owns each piece      | DNS, certificates, a database, a license, and an identity provider application |
| [3. Install the control plane](./server/index.md)  | How to install Coder on the platform your team operates                  | A running control plane you can sign in to                                     |
| [4. Validate your deployment](./validate/index.md) | Whether the deployment is hardened, works, and holds up under load       | Evidence that the deployment is ready for real users                           |
| [5. Operate and maintain](./operate/index.md)      | How to keep the deployment healthy, upgraded, and correctly sized        | An upgrade and scaling routine, plus a removal path                            |

Phases 1 and 2 are advisory rather than a gate.
Nothing stops you from installing the control plane first, but the recommendations in those phases are what keep a deployment from needing to be rebuilt later.
Don't know where to start?
Read [Plan your deployment](./plan/index.md) and [Prepare prerequisites](./prepare/index.md) first.

<children></children>

## Who does the work

A Coder deployment usually crosses team boundaries.
The following actors appear throughout this section, so you can tell early which conversations you need to start and how long they're likely to take.

| Actor                            | Responsible for                                                              | Appears in phase |
|----------------------------------|------------------------------------------------------------------------------|------------------|
| Coder platform owner             | The deployment itself: installation, configuration, upgrades, and support    | 1 to 5           |
| Infrastructure team              | Clusters, virtual machines, networking, load balancers, and egress rules     | 1 to 4           |
| DNS and TLS owner                | The Coder hostname, the wildcard record for workspace apps, and certificates | 2                |
| Database administrator           | PostgreSQL provisioning, sizing, backups, and restore tests                  | 2, 4, 5          |
| Identity provider administrator  | The OIDC or SAML application, claims, and group mappings                     | 2                |
| Security and compliance reviewer | Audit requirements, network policy, data handling, and sign-off              | 2, 4             |
| Licensing contact                | Purchasing and renewing the Coder license                                    | 2                |
| Template author                  | The workspace templates developers use once the deployment is running        | 4, 5             |

These are roles, not people.
One person may serve as several actors, which is common for a proof of concept run by a single platform engineer.
Several people may also serve as one actor, which is common for an infrastructure team that splits networking and compute ownership.
Use the table to identify who has to agree, not to count headcount.
