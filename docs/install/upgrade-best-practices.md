---
title: Upgrading Best Practices
---

This guide provides best practices for upgrading Coder, along with
troubleshooting steps for common issues encountered during upgrades,
particularly with database migrations in high availability (HA) deployments.

## Rolling and live upgrades are not supported

> [!WARNING]
> Coder does not support rolling or "live" upgrades while users have active
> sessions, workspace connections, or in-progress workspace builds. Upgrading
> Coder restarts `coderd`, which disconnects active browser sessions, CLI
> connections, and port-forwards, and can interrupt in-progress workspace
> builds. In high availability (HA) deployments, active connections from
> existing replicas can also hold database locks that block migrations run by
> the new version.
>
> Always perform upgrades during a scheduled maintenance window when no users
> are actively connected, as described below.

## Safe upgrade procedure for production deployments

Follow this procedure for every production upgrade:

1. **Announce a maintenance window.** Notify users in advance and pick a time
   with the fewest active developers, per the
   [scheduling guidance](#before-you-upgrade) below.
1. **Confirm there is no active user activity.** Before starting the upgrade,
   verify that there are no in-progress workspace builds and no active user
   sessions, SSH connections, or port-forwards. Check the **Workspaces** page
   in the dashboard for any builds in progress, and review
   [connection logs](../admin/monitoring/connection-logs.md) if you have a
   Premium license. If your organization requires it, ask users to stop their
   workspaces or disconnect before you proceed.
1. **Take a database backup.** Always take a database snapshot immediately
   before upgrading, since Coder does not support rollbacks. See your
   database provider's documentation for backup instructions (for example,
   `pg_dump` for self-managed PostgreSQL).
1. **Apply the upgrade.** Follow the
   [reinstall steps](./upgrade.md#reinstall-coder-to-upgrade) for your install
   method, or the
   [pre-upgrade strategy for Kubernetes HA deployments](#pre-upgrade-strategy-for-kubernetes-ha-deployments)
   if you run multiple replicas.
1. **Verify the deployment is healthy.** After the upgrade completes, confirm
   the new version is running and healthy:

   ```sh
   coder version
   ```

   Check the [health check page](../admin/monitoring/health-check.md) in the
   dashboard, and confirm `coderd` logs show no errors and that migrations
   completed successfully.
1. **Communicate completion.** Let users know the maintenance window has
   ended and that they can resume using their workspaces.

## Before you upgrade

> [!TIP]
> To check your current Coder version, use `coder version` from the CLI, check
> the bottom-right of the Coder dashboard, or query the `/api/v2/buildinfo`
> endpoint. See the [version command](../reference/cli/version.md) for details.

- **Schedule upgrades during off-peak hours.** Upgrades can cause a noticeable
  disruption to the developer experience. Plan your maintenance window when
  the fewest developers are actively using their workspaces.
- **The larger the version jump, the more migrations will run.** If you are
  upgrading across multiple minor versions, expect longer migration times.
- **Large upgrades should complete in minutes** (typically 4-7 minutes). If your
  upgrade is taking significantly longer, there may be an issue requiring
  investigation.
- **Check for known issues affecting your upgrade path.** Some version upgrades
  have known issues that may require a larger maintenance window or additional
  steps. For example, upgrades from v2.26.0 to v2.27.8 may encounter issues with
  the `api_keys` table—upgrading to v2.26.6 first can help mitigate this.
  Contact [Coder support](../support/index.md) for guidance on your specific
  upgrade path.

## Pre-upgrade strategy for Kubernetes HA deployments

Standard Kubernetes rolling updates may fail when exclusive database locks are
required because old replicas keep connections open. For production deployments
running multiple replicas (HA), active connections from existing pods can
prevent the new pod from acquiring necessary locks.

### Recommended strategy for major upgrades

1. **Scale down before upgrading:** Before running `helm upgrade`, scale your
   Coder deployment down to eliminate database connection contention from
   existing pods.

   - **Scale to zero** for a clean cutover with no active database connections
     when the upgrade starts. This momentarily ensures no application access to
     the database, allowing migrations to acquire locks immediately:

     ```sh
     kubectl scale deployment coder --replicas=0
     ```

   - **Scale to one** if you prefer to minimize downtime. This keeps one pod
     running but eliminates contention from multiple replicas:

     ```sh
     kubectl scale deployment coder --replicas=1
     ```

1. **Perform upgrade:** Run your standard Helm upgrade command. When scaling to
   zero, this will bring up a fresh pod that can run migrations without
   competing for database locks.

1. **Scale back:** Once the upgrade is healthy, scale back to your desired
   replica count.

## Kubernetes liveness probes and long-running migrations

Liveness probes can cause pods to be killed during long-running database
migrations. Starting with Coder v2.30.0, liveness probes are *disabled by
default* in the Helm chart.

This change was made because:

- Liveness probes can kill pods during legitimate long-running migrations
- If a Coder pod becomes unresponsive (due to a deadlock, etc.), it's better to
  investigate the issue rather than have Kubernetes silently restart the pod

If you have enabled liveness probes in your deployment and observe pods
restarting with `CrashLoopBackOff` during an upgrade, the liveness probe may be
killing the pod prematurely.

### Diagnosing liveness probe issues

To confirm whether Kubernetes is killing pods due to liveness probe failures,
check the Kubernetes events and pod logs:

```sh
# Check events for the Coder deployment
kubectl get events --field-selector involvedObject.name=coder -n <namespace>

# Check pod logs for migration progress
kubectl logs -l app.kubernetes.io/name=coder -n <namespace> --previous
```

Look for events indicating `Liveness probe failed` or `Container coder failed
liveness probe, will be restarted`.

### Recommended approach

If you have liveness probes enabled and experience issues during upgrades,
disable them before upgrading:

```sh
kubectl edit deployment coder
```

Remove the `livenessProbe` section entirely, then proceed with the upgrade.

> [!NOTE]
> For versions prior to v2.30.0, liveness probes were enabled by default. You
> can disable them by editing the Deployment directly with `kubectl edit
> deployment coder` or by using a ConfigMap override. See the
> [Helm chart values](https://artifacthub.io/packages/helm/coder-v2/coder?modal=values&path=coder.livenessProbe)
> for configuration options available in v2.30.0+.

### Workaround steps

1. **Remove or adjust liveness probes:** Temporarily remove the `livenessProbe`
   from your Deployment configuration to prevent Kubernetes from restarting the
   pod during migrations.

1. **Isolate the migration:** Ensure all extra replica sets are shut down. If
   you have clear evidence of database locks from old pods, scale the deployment
   to 1 replica to prevent old pods from holding locks on the tables being
   upgraded.

1. **Clear database locks:** Monitor database activity. If the migration remains
   blocked by locks despite scaling down, you may need to manually terminate
   existing connections. See
   [Recovering from failed database migrations](#recovering-from-failed-database-migrations)
   below for instructions.

## Recovering from failed database migrations

If an upgrade gets stuck in a restart loop due to database locks:

1. **Scale to zero:** Scale the Coder deployment to 0 to stop all application
   activity.

   ```sh
   kubectl scale deployment coder --replicas=0
   ```

1. **Clear connections:** Terminate existing connections to the Coder database
   to release any lingering locks. This PostgreSQL command drops all active
   connections to the database:

   > [!CAUTION]
   > This command is intrusive and should be used as a last resort. Contact
   > [Coder support](../support/index.md) before running destructive database
   > commands in production. SQL commands may vary depending on your PostgreSQL
   > version and configuration.

   ```sql
   SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE datname = 'coder'
   AND pid <> pg_backend_pid();
   ```

1. **Check schema migrations:** Verify the level of upgrade and check if `dirty`
   is true. If this has progressed, this now indicates your current Coder
   installation state.

   > [!NOTE]
   > The SQL commands below are for informational purposes. If you are unsure
   > about querying your database directly, contact
   > [Coder support](../support/index.md) for assistance.

   ```sql
   SELECT * FROM schema_migrations;
   ```

1. **Ensure image version:** Confirm the Deployment image is set to the
   appropriate version (old or new, depending on the database migration state
   found in step 3). Match your tag in the
   [migrations directory](../../coderd/database/migrations)
   to the value in the `schema_migrations` output.

1. **Resume the upgrade:** Follow the
   [pre-upgrade strategy](#recommended-strategy-for-major-upgrades) to scale
   back up and continue the upgrade process.

## When to contact support

If you encounter any of the following issues, contact
[Coder support](../support/index.md):

- Locking issues that cannot be mitigated by the steps in this guide
- Migrations taking significantly longer than expected (more than 15 minutes)
  without evidence of lock contention—this may indicate database resource
  constraints requiring investigation
- Resource consumption issues (excessive memory, CPU, or OOM kills) during
  upgrades
- Any other upgrade problems not covered by this documentation

When contacting support, please collect and provide:

- `coderd` logs with details on the stages where the upgrade stalled
- PostgreSQL logs if available
- The Coder versions involved (source and target)
- Your deployment configuration (number of replicas, resource limits)
