---
title: Migrate built-in PostgreSQL
---

This guide helps Coder deployment administrators migrate an existing built-in PostgreSQL database while preserving deployment data.
You can upgrade the built-in database to PostgreSQL 16 or move to an external PostgreSQL server.
For a new deployment without existing data, refer to [external database configuration](../../tutorials/external-database.md).

## Identify the built-in database

Check your Coder server startup output for `Using built-in PostgreSQL (...)`.
The path in parentheses identifies the PostgreSQL directory for that deployment.
Coder uses the built-in database when you haven't configured `CODER_PG_CONNECTION_URL` or `--postgres-url`.

On the **Health** page, the **Database** section reports [EDB03](../monitoring/health-check.md#edb03) for PostgreSQL versions below 14.
For a built-in database, the warning explicitly says **Built-in PostgreSQL** and directs you to this migration guide through **Docs for EDB03**.
The absence of this warning doesn't mean your database is external.

## Why migrate

PostgreSQL 13 reached [end of life](https://www.postgresql.org/support/versioning/) on November 13, 2025, and no longer receives upstream fixes.
New built-in databases use PostgreSQL 16, but existing PostgreSQL 13 databases stay on 13 until you migrate their data.
Restarting Coder alone doesn't upgrade them.

Choose one migration option:

- [Option 1: Upgrade the built-in database to PostgreSQL 16](#option-1-upgrade-the-built-in-database-to-postgresql-16) keeps the database on the Coder host.
  You export the data and restore it into a fresh built-in database without provisioning an external server.
- [Option 2: Migrate to an external PostgreSQL database](#option-2-migrate-to-an-external-postgresql-database) lets you manage PostgreSQL updates, backups, and availability separately from Coder.
  Moving the database alone doesn't provide backups or high availability; configure these on the external server.

The built-in database stores data on the Coder host, making that host a single point of failure without a separate backup and recovery strategy.
This remains true after upgrading the built-in database.

## Prepare for migration

Before you begin, prepare:

- Access to stop and restart the Coder server and read its configuration directory.
- A maintenance window that covers the dump, restore, and verification.
- `pg_dump`, `pg_restore`, and `psql` from the destination PostgreSQL major version, which must be at least the source major version.
  For Option 1, use PostgreSQL 16 client tools.
- Enough disk space for the original database, its backup, the dump, and the restored database.
- A protected backup location outside the deployment's `postgres` directory.
- For Option 2, an empty database owned by Coder's database role on a supported PostgreSQL server, such as 16.
  Use the current minor release for that major version.

Install PostgreSQL client tools separately; the embedded distribution doesn't include `pg_dump` or `pg_upgrade`.
The source PostgreSQL server must be able to start for the dump to work.
Built-in PostgreSQL 13 binaries can fail on ARM64 Linux systems, including Raspberry Pi, and require Rosetta 2 on macOS with Apple Silicon.

For Option 2 database and role setup, refer to [external database configuration](../../tutorials/external-database.md#basic-configuration).
For encrypted connections, refer to [PostgreSQL SSL configuration](../../tutorials/postgres-ssl.md).
Keep the same Coder version and server settings during migration, including any [database encryption keys](../security/database-encryption.md).
For Option 1, the Coder binary must support PostgreSQL 16 for new built-in databases.

> [!WARNING]
> Stop Coder before exporting and keep it stopped until the restore completes.
> Although `pg_dump` creates a consistent snapshot, writes after that snapshot won't reach the restored database.
> Continuing to run Coder against the source can lose changes and leave workspace infrastructure inconsistent with the restored metadata.

## Export the built-in database

Complete these steps for either option.
Run the commands on the Coder host as the operating system user that runs Coder.
Use the same `coder` binary and configuration directory as the deployment.

1. Stop the Coder server through your service manager, container runtime, or terminal.
   The dashboard should be unavailable before you continue.

1. Prevent any automatic restart for the maintenance window.

1. Set the existing configuration directory in each terminal used for the migration.
   Replace the placeholder with the directory containing the `postgres` directory from the startup output.

   ```sh
   export CODER_CONFIG_DIR='/path/to/existing/coder-config'
   ```

   The equivalent global flag is `--global-config`.
   Do not point these commands at a new directory, which would create an empty database instead of serving your existing data.

1. Back up the stopped deployment's entire `postgres` directory to a separate, protected location.
   Keep this backup and the original directory until you've verified the migration.

1. Start only the built-in PostgreSQL server in a separate terminal:

   ```sh
   coder server postgres-builtin-serve
   ```

   The command prints a `psql` connection command after PostgreSQL starts and remains running until interrupted.
   Leave this terminal open while you export.
   The Coder application stays stopped.

1. In another terminal with the same `CODER_CONFIG_DIR`, capture the raw connection URL:

   ```sh
   BUILTIN_PG_URL=$(coder server postgres-builtin-url --raw-url)
   ```

   Without `--raw-url`, the command prints a complete `psql` command rather than a URL.
   The URL contains a password, so don't share it or include it in logs.

1. Export the database to a custom-format archive in your protected backup location:

   ```sh
   umask 077
   pg_dump --dbname="$BUILTIN_PG_URL" --format=custom --file=coder.dump
   ```

   A successful command exits with status `0` and creates `coder.dump` without printing the database contents.
   Do not continue if the dump fails.
   The archive contains sensitive deployment data, including credentials and workspace metadata.

1. After the export succeeds, stop `postgres-builtin-serve` with `Ctrl+C` and wait for it to exit.
   Keep Coder stopped while you complete one of the following options.

## Option 1: Upgrade the built-in database to PostgreSQL 16

This option upgrades the database on the same host through a dump and restore, not by converting the existing data files.
Complete [Export the built-in database](#export-the-built-in-database) first.
Use the same `CODER_CONFIG_DIR` in each terminal and run restore commands from the directory containing `coder.dump`.

> [!WARNING]
> Do not start `coder server` before restoring the dump.
> It creates Coder tables in the empty database, which conflict with the tables in the archive.
> Use `postgres-builtin-serve` to start only PostgreSQL during the restore.

1. Move the entire stopped `postgres` directory to a backup name that doesn't already exist:

   ```sh
   mv "$CODER_CONFIG_DIR/postgres" "$CODER_CONFIG_DIR/postgres-v13-backup"
   ```

   This preserves the original data, binaries, password, and port files.
   Moving only `postgres/data` leaves the old binaries in place, so move the entire directory.
   Do not delete or edit `PG_VERSION` in the original data directory.

1. Start only the built-in PostgreSQL server in a separate terminal:

   ```sh
   coder server postgres-builtin-serve
   ```

   With no existing data directory, Coder downloads PostgreSQL 16 binaries and initializes an empty `coder` database.
   Leave this terminal open until the restore and database checks finish.

1. In the restore terminal, capture the new connection URL:

   ```sh
   BUILTIN_PG_URL=$(coder server postgres-builtin-url --raw-url)
   ```

   The new database has a new password and may use a different port.
   Do not reuse the source URL.

1. Confirm the new server version before restoring:

   ```sh
   psql "$BUILTIN_PG_URL" -X --set=ON_ERROR_STOP=1 --command='SHOW server_version;'
   ```

   The result must report PostgreSQL `16.x`.
   Do not continue if it reports another major version.

1. Restore the archive into the empty built-in database:

   ```sh
   pg_restore --dbname="$BUILTIN_PG_URL" --no-owner --no-acl --single-transaction --exit-on-error coder.dump
   ```

   The restore assigns ownership to the new `coder` role without replaying the source ownership and grants.
   A successful restore exits with status `0`.
   If the restore fails, do not start Coder against the destination.

1. Confirm the destination connection and restored tables:

   ```sh
   psql "$BUILTIN_PG_URL" -X --set=ON_ERROR_STOP=1 --command='SELECT current_database(), current_user;'
   psql "$BUILTIN_PG_URL" -X --set=ON_ERROR_STOP=1 --command='\dt'
   ```

   The first command should report `coder` for both the database and role.
   The second should list the restored Coder tables, not an empty database.

1. Stop `postgres-builtin-serve` with `Ctrl+C` and wait for it to exit.

1. Restart Coder through your usual service manager, container runtime, or terminal.
   For a direct terminal invocation with the original server settings, run:

   ```sh
   coder server
   ```

   Keep `CODER_PG_CONNECTION_URL` and `--postgres-url` unset to continue using the built-in database.
   The startup output should still say `Using built-in PostgreSQL (...)`.

1. Complete [Verify the migrated deployment](#verify-the-migrated-deployment) before allowing normal use.

## Option 2: Migrate to an external PostgreSQL database

Complete [Export the built-in database](#export-the-built-in-database) first.
Restore into an empty database before allowing Coder to connect to it.
Run restore commands from the directory containing `coder.dump`.

1. Set the destination URL in the restore terminal.
   Replace the example URL with your destination database's connection string and required TLS settings.

   ```sh
   EXTERNAL_PG_URL='postgres://coder:password@db.example.com:5432/coder?sslmode=verify-full'
   ```

1. Restore the archive as the role that Coder uses:

   ```sh
   pg_restore --dbname="$EXTERNAL_PG_URL" --no-owner --no-acl --single-transaction --exit-on-error coder.dump
   ```

   `--no-owner` and `--no-acl` avoid replaying the built-in database's ownership and grants on the external server.
   A successful restore exits with status `0`.
   If the restore fails, do not start Coder against the destination.

1. Confirm the destination connection and restored tables:

   ```sh
   psql "$EXTERNAL_PG_URL" -X --set=ON_ERROR_STOP=1 --command='SELECT current_database(), current_user;'
   psql "$EXTERNAL_PG_URL" -X --set=ON_ERROR_STOP=1 --command='\dt'
   ```

   The first command should report the intended database and role.
   The second should list the restored Coder tables, not an empty database.

1. Set `CODER_PG_CONNECTION_URL` in the Coder service or container configuration to the destination URL.
   For a direct terminal invocation, set:

   ```sh
   export CODER_PG_CONNECTION_URL="$EXTERNAL_PG_URL"
   ```

   Preserve all other deployment settings and remove any conflicting database URL configuration.

1. Restart Coder through your usual service manager, container runtime, or terminal.
   For a direct terminal invocation, run:

   ```sh
   coder server
   ```

   Alternatively, use `coder server --postgres-url="$EXTERNAL_PG_URL"`.
   The startup output should no longer say `Using built-in PostgreSQL (...)`.

1. Complete [Verify the migrated deployment](#verify-the-migrated-deployment) before allowing normal use.

## Verify the migrated deployment

Complete these checks after either migration option.

1. Confirm you can log in with an existing account and find existing templates and workspaces.

1. Connect to an existing workspace.

1. Check that **Health** > **Database** is reachable and no longer reports EDB03.

1. Configure and test backups for the destination database before decommissioning the original data.

> [!WARNING]
> Do not delete the original PostgreSQL directory or its backup until you've verified the restore and the deployment works with the destination database.
> Deleting the only recoverable copy makes a failed migration irreversible.
> After Coder writes to the destination database, switching back to the old database discards those newer changes.

## Learn more

- [Deployment health and EDB03](../monitoring/health-check.md#edb03)
- [External database configuration](../../tutorials/external-database.md)
- [PostgreSQL dump options](https://www.postgresql.org/docs/current/app-pgdump.html)
- [PostgreSQL restore options](https://www.postgresql.org/docs/current/app-pgrestore.html)
