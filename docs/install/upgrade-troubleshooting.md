# Upgrade Troubleshooting Steps

This guide covers common issues encountered during Coder upgrades, particularly
with database migrations in high availability (HA) deployments, and provides
step-by-step troubleshooting and recovery procedures.

## Pre-upgrade strategy for HA deployments

Standard rolling updates may fail when exclusive database locks are required
because old replicas keep connections open. For production deployments running
multiple replicas (HA), active connections from existing pods can prevent the
new pod from acquiring necessary locks.

### Recommended strategy for major upgrades

1. **Scale to one replica:** Before running `helm upgrade`, manually scale your
   Coder deployment to 1 replica. This ensures only one pod handles the
   migration and eliminates contention from other active replicas.

   ```shell
   kubectl scale deployment coder --replicas=1
   ```

1. **Perform upgrade:** Run your standard Helm upgrade command.

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

1. **Scale to one:** Scale the deployment to 1 to restart the migration process
   in isolation.

   ```shell
   kubectl scale deployment coder --replicas=1
   ```

## Next steps

- Review the [upgrade documentation](./upgrade.md) for standard upgrade
  procedures.
- For Kubernetes-specific upgrade guidance, see
  [Upgrading Coder via Helm](./kubernetes.md#upgrading-coder-via-helm).
