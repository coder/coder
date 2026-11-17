---
title: Disaster recovery
---

This page is for Coder deployment administrators and database administrators who plan how a self-hosted deployment recovers from an outage or data loss.
It explains what to back up, what high availability does and doesn't protect against, what users experience during a control plane outage, and what a database restore leaves behind.
To respond to a security incident by removing access to workspaces, refer to [Cut off access to workspaces during a security incident](../../admin/security/cut-off-workspace-access.md).

## Back up the database

The control plane keeps its state in PostgreSQL.
Users, organizations, templates and template versions, workspaces and their build history, and the Terraform state of each workspace build are all stored there.
`coderd`, the process that runs the control plane, keeps no other persistent state, so disaster recovery for Coder is mostly database recovery.

Schedule regular, automated backups of the Coder database.
If you use a managed PostgreSQL service, such as Amazon RDS or Azure Database for PostgreSQL, turn on its daily automated backups.
If you run the built-in database because `CODER_PG_CONNECTION_URL` isn't set, its data lives in the Coder config directory on the control plane host, so back up that host or [migrate to an external database](../../admin/setup/index.md#migrate-from-the-built-in-database-to-an-external-database).

Some values that protect the database contents live outside it.
If you use [database encryption](../../admin/security/database-encryption.md), store the [external token encryption keys](../../reference/cli/server/index.md#--external-token-encryption-keys) with the same care as the backups.
A restored database is unreadable for encrypted fields without the keys that were in use when the backup was taken.

### Back up before every upgrade

Back up the database immediately before you upgrade Coder.
An upgrade runs database migrations, and Coder doesn't support running an earlier version against a database that a later version has migrated.
To return to an earlier version, restore a backup taken on that version.
Refer to [Upgrade best practices](./upgrade/best-practices.md).

## Test your restore process

A backup is useful only if you can restore from it.
Your database administrator owns a documented restore procedure and tests it on a schedule, not only the backup schedule.

A restore test confirms 3 things:

- The backup restores into a new PostgreSQL instance without errors.
- A Coder control plane on the same version as the backup starts against the restored database.
- An administrator can sign in, and the **Workspaces** page lists the workspaces that existed when the backup was taken.

## What high availability protects against

[High availability](../../admin/networking/high-availability.md) (HA) runs multiple `coderd` replicas behind a load balancer.
All replicas share one PostgreSQL endpoint.
HA gives you horizontal scale and automatic failover inside a single region.

| HA protects against                   | HA doesn't protect against                          |
|---------------------------------------|-----------------------------------------------------|
| The failure of an individual replica  | The loss of the region that hosts the control plane |
| Maintenance that restarts one replica | The loss or corruption of the database              |
| Load that exceeds a single replica    | Replicating the database to another region          |

Coder doesn't provide a built-in mechanism for cross-region failover.
To survive a region outage, use your database provider's cross-region replication or backup copies, and plan how you bring up a control plane in the recovery region against the recovered database.

## What users experience during a control plane outage

Workspaces are cloud resources, such as virtual machines, Kubernetes pods, and volumes, that the template defines.
Workspace compute runs independently of the control plane, so it keeps running when `coderd` is unavailable.
What a user can still do depends on how their session connects.

### Direct connections

When network address translation (NAT) traversal succeeds, Coder sets up a direct WireGuard tunnel between the client and the workspace agent.
Traffic on that tunnel goes straight between the 2 endpoints and doesn't pass through the control plane.
A user with an active direct session, such as `coder ssh`, can usually keep working during a control plane outage.

### Relayed connections

When a direct connection isn't possible, traffic goes through a Designated Encrypted Relay for Packets (DERP) relay.
By default, the control plane runs the relay, so relayed sessions stop when the control plane stops.
If you configure [custom relays](../../admin/networking/index.md#custom-relays) that run outside the control plane, such as Tailscale's DERP relays or your own, relayed sessions on those relays can continue during the outage.

### Impact by component

- **Connections through a lost replica**:
  losing one replica in an HA deployment disconnects the dashboard, `coder ssh`, port-forwarding, and IDE sessions that go through it.
  Clients reconnect when they reach a healthy replica.
- **New connections and the dashboard**:
  users can't sign in, open the dashboard, start new sessions, or reach workspace apps through the control plane.
  The workspace compute keeps running.
- **Workspace builds**:
  starting, stopping, updating, and deleting workspaces need the control plane and the database.
  These operations fail or wait until the control plane is back.
- **Workspace agents**:
  the workspace agent keeps running and retries its connection to the control plane.
  Until it reconnects, it can't report status, metadata, or new apps.

## Orphaned workspaces after a database restore

A database restore rolls back Coder's record of the deployment to the time of the backup.
It doesn't roll back the infrastructure that Coder provisioned.

Any workspace created after the backup is unknown to the restored database, even though its virtual machine, pod, or volume might still exist and run.
These resources are orphaned:

- They don't appear in the dashboard.
- Autostart, autostop, and quotas don't apply to them.
- Coder can't delete them, because the restored database holds no Terraform state for them.

Workspaces deleted after the backup have the opposite problem.
The restored database lists them, but their infrastructure is gone.

To limit the impact of a restore:

1. Before you restore, list the workspaces created after the backup time.
   Use your cloud provider console, Kubernetes, or infrastructure tags.
1. Tell users the backup time before a planned restore.
   They recreate any workspace that they created after that time.
1. After you restore, delete orphaned infrastructure directly on its platform, such as the cloud console or `kubectl`.
1. For a restored workspace whose infrastructure is gone, run [`coder delete --orphan`](../../reference/cli/delete.md) to remove the record without running Terraform.

## Learn more

- [High availability](../../admin/networking/high-availability.md)
- [Upgrade best practices](./upgrade/best-practices.md)
- [Coder Validated Architecture](../plan/sizing/index.md)
- [Networking](../../admin/networking/index.md)
