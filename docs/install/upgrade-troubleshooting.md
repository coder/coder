# Upgrading Best Practices

This guide provides best practices for upgrading Coder, along with
troubleshooting steps for common issues encountered during upgrades,
particularly with database migrations in high availability (HA) deployments.

## Before you upgrade

- **Schedule upgrades during off-peak hours.** Upgrades cause a noticeable
  disruption to the developer experience. Plan your maintenance window when
  fewest developers are actively using their workspaces.
- **The larger the version jump, the more migrations will run.** If you are
  upgrading across multiple minor versions, expect longer migration times.
- **Large upgrades should complete in minutes** (typically 4-7 minutes). If your
  upgrade is taking significantly longer, there may be an issue requiring
  investigation.
- **Upgrades from v2.26.0 to v2.27.8** may encounter specific issues with the
  `api_keys` table. If you are upgrading within this range, consider upgrading
  to v2.26.6 first to mitigate potential issues with this table.

## Pre-upgrade strategy for HA deployments

Standard rolling updates may fail when exclusive database locks are required
because old replicas keep connections open. For production deployments running
multiple replicas (HA), active connections from existing pods can prevent the
new pod from acquiring necessary locks.

### Recommended strategy for major upgrades

1. **Scale down before upgrading:** Before running `helm upgrade`, scale your
   Coder deployment down to eliminate database connection contention from
   existing pods.

   - **Scale to zero** for a clean cutover with no active database connections
     when the upgrade starts. This momentarily ensures no application access to
     the database, allowing migrations to acquire locks immediately:

     ```shell
     kubectl scale deployment coder --replicas=0
     ```

   - **Scale to one** if you prefer to minimize downtime. This keeps one pod
     running but eliminates contention from multiple replicas:

     ```shell
     kubectl scale deployment coder --replicas=1
     ```

1. **Perform upgrade:** Run your standard Helm upgrade command. When scaling to
   zero, this will bring up a fresh pod that can run migrations without
   competing for database locks.

1. **Scale back:** Once the upgrade is healthy, scale back to your desired
   replica count.

## Liveness probe configuration for long-running migrations

Large database migrations may exceed default `livenessProbe` timeouts. If you
observe pods restarting with `CrashLoopBackOff` during an upgrade and logs
indicate a migration in progress, Kubernetes might be killing the pod
prematurely.

### Configuration example

Increase the liveness probe threshold to cover a reasonable duration (for
example, 15 minutes):

```yaml
livenessProbe:
  failureThreshold: 90 # 90 checks
  httpGet:
    path: /healthz
    port: http
    scheme: HTTP
  periodSeconds: 10 # 90 * 10s = 900 seconds (15 minutes)
  successThreshold: 1
  timeoutSeconds: 1
```

### Workaround steps

1. **Adjust liveness probes:** Temporarily increase the `failureThreshold` in
   your `values.yaml` (liveness probe) or Deployment configuration (for example,
   set to 200-300 with intervals greater than 10 seconds). This ensures the
   `coderd` instance is not restarted by Kubernetes while the migration is
   running.

1. **Isolate the migration:** Ensure all extra replica sets are shut down. If
   you have clear evidence of database locks from old pods, scale the deployment
   to 1 replica to prevent old pods from holding locks on the tables being
   upgraded.

1. **Clear database locks:** Monitor database activity. If the migration remains
   blocked by locks despite scaling down, you may need to manually terminate
   (disconnect) existing connections to the database to resolve deadlocks.

## Recovering from failed database migrations

If an upgrade gets stuck in a restart loop due to database locks:

1. **Scale to zero:** Scale the Coder deployment to 0 to stop all application
   activity.

   ```shell
   kubectl scale deployment coder --replicas=0
   ```

1. **Clear connections:** Terminate existing connections to the Coder database
   to release any lingering locks. This PostgreSQL command drops all active
   connections to the database:

   > [!CAUTION]
   > This command is intrusive and should be used as a last resort.

   ```sql
   SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE datname = 'coder'
   AND pid <> pg_backend_pid();
   ```

1. **Check schema migrations:** Verify the level of upgrade and check if `dirty`
   is true. If this has progressed, this now indicates your current Coder
   installation state.

   ```sql
   SELECT * FROM schema_migrations;
   ```

1. **Ensure image version:** Confirm the Deployment image is set to the
   appropriate version (old or new, depending on the database migration state
   found in step 3). Match your tag in the
   [migrations directory](https://github.com/coder/coder/tree/main/coderd/database/migrations)
   to the value in the `schema_migrations` output.

1. **Resume the upgrade:** Follow the
   [pre-upgrade strategy](#recommended-strategy-for-major-upgrades) to scale
   back up and continue the upgrade process.

## When to contact support

If you encounter any of the following issues, contact
[Coder support](../support/index.md):

- Locking issues that cannot be mitigated by the steps in this guide
- Migrations taking significantly longer than expected (more than 10-15 minutes)
- Resource consumption issues (excessive memory, CPU, or OOM kills) during
  upgrades
- Any other upgrade problems not covered by this documentation

When contacting support, please collect and provide:

- `coderd` logs with details on the stages where the upgrade stalled
- PostgreSQL logs if available
- The Coder versions involved (source and target)
- Your deployment configuration (number of replicas, resource limits)
