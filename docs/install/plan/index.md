# Plan your deployment

Planning decides the shape of your deployment: which components run where, how large they need to be, and which choices are expensive to change later.
Work through this phase before you pick a platform in [Install the control plane](../server/index.md).

## What you decide here

- **Topology**: where the control plane, the database, and workspaces run, and whether workspaces run in the same region as their users.
  Refer to [Architecture](./architecture.md).
- **Size**: how much CPU, memory, and database capacity the deployment needs for the number of users you expect.
  Refer to [Coder Validated Architecture](./sizing/index.md).
- **Provisioning model**: whether workspace builds run on the control plane or on [external provisioners](../operate/provisioners/index.md), which matters when builds need credentials the control plane shouldn't hold.
- **Network position**: whether the deployment is reachable from the public internet, from a private network only, or is [air-gapped](../prepare/airgap.md).

## Who to involve

The Coder platform owner drives this phase, but the topology and size decisions need the infrastructure team, and a network position decision usually needs a security reviewer.
Bring them in now: these are the decisions that are hardest to revisit after installation.

## Before you move on

You should be able to state, in a sentence each, where Coder will run, how many users it's sized for, and how users will reach it.
If any of those is still open, the answer usually lives in [Architecture](./architecture.md) or in the [Coder Validated Architecture](./sizing/index.md) reference designs.

Next: [Prepare prerequisites](../prepare/index.md).

<children></children>
