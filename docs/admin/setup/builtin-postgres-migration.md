---
title: Migrate built-in PostgreSQL to an external database
---

This guide helps Coder deployment administrators move an existing built-in PostgreSQL database to an external PostgreSQL server.
For a new deployment without existing data, refer to [external database configuration](../../tutorials/external-database.md).

## Identify the built-in database

Check your Coder server startup output for `Using built-in PostgreSQL (...)`.
The path in parentheses identifies the PostgreSQL directory for that deployment.
Coder uses the built-in database when you haven't configured `CODER_PG_CONNECTION_URL` or `--postgres-url`.

On the **Health** page, the **Database** section reports [EDB03](../monitoring/health-check.md#edb03) for PostgreSQL versions below 14.
For a built-in database, the warning explicitly says **Built-in PostgreSQL** and directs you to this migration guide through **Docs for EDB03**.
The absence of this warning doesn't mean your database is external.

## Prepare for migration

An external database lets you manage PostgreSQL updates, backups, and availability separately from the Coder server.
The built-in database stores data on the Coder host, making that host a single point of failure without a separate backup and recovery strategy.
Moving the database alone doesn't provide backups or high availability; configure these on the external server.

PostgreSQL 13 reached [end of life](https://www.postgresql.org/support/versioning/) on November 13, 2025, and no longer receives upstream fixes.
Built-in PostgreSQL 13 binaries can fail on ARM64 Linux systems, including Raspberry Pi, and require Rosetta 2 on macOS with Apple Silicon.
An external PostgreSQL installation avoids depending on those embedded binaries after migration.
The embedded distribution doesn't include `pg_dump` or `pg_upgrade`, so install PostgreSQL client tools separately for the export.

Before you begin, prepare:

- Access to stop and restart the Coder server and read its configuration directory.
- A maintenance window that covers the dump, restore, and verification.
- An empty external database, such as `coder`, owned by the database role Coder uses.
- An external PostgreSQL server on a supported major version, such as 16, with its current minor release installed.
- `pg_dump`, `pg_restore`, and `psql` from the destination PostgreSQL major version, which must be at least the source major version.
- Enough disk space for the database dump and a protected location to store it.

For database and role setup, refer to [external database configuration](../../tutorials/external-database.md#basic-configuration).
For encrypted connections, refer to [PostgreSQL SSL configuration](../../tutorials/postgres-ssl.md).
Keep the same Coder version and server settings during migration, including any [database encryption keys](../security/database-encryption.md).

> [!WARNING]
> Stop Coder before exporting and keep it stopped until you switch databases.
> Although `pg_dump` creates a consistent snapshot, writes after that snapshot won't reach the restored database.
> Continuing to run Coder against the source can lose changes and leave workspace infrastructure inconsistent with the restored metadata.

## Export the built-in database

Run the following commands on the Coder host as the operating system user that runs Coder.
Use the same `coder` binary and configuration directory as the deployment.

1. Stop the Coder server through your service manager, container runtime, or terminal.
   Prevent any automatic restart for the maintenance window.
   The dashboard should be unavailable before you continue.

1. Set the existing configuration directory in each terminal used for the export.
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
   The source PostgreSQL server must be able to start for the dump to work.

1. In another terminal with the same `CODER_CONFIG_DIR`, capture the raw connection URL:

   ```sh
   BUILTIN_PG_URL=$(coder server postgres-builtin-url --raw-url)
   ```

   Without `--raw-url`, the command prints a complete `psql` command rather than just a URL.
   The URL contains a password, so don't share it or include it in logs.

1. Export the database to a custom-format archive in your protected backup location:

   ```sh
   umask 077
   pg_dump --dbname="$BUILTIN_PG_URL" --format=custom --file=coder.dump
   ```

   A successful command exits with status `0` and creates `coder.dump` without printing the database contents.
   Do not continue if the dump fails.
   The archive contains sensitive deployment data, including credentials and workspace metadata.

1. Stop `postgres-builtin-serve` with `Ctrl+C` after the export succeeds.
   Keep Coder stopped while you restore.

## Restore and switch databases

Restore into an empty database before allowing Coder to connect to it.
Replace the example URL with your destination database's connection string and required TLS settings.

1. Set the destination URL in the restore terminal:

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
   For a direct terminal invocation, run:

   ```sh
   export CODER_PG_CONNECTION_URL="$EXTERNAL_PG_URL"
   coder server
   ```

   Alternatively, use `coder server --postgres-url="$EXTERNAL_PG_URL"`.
   Preserve all other deployment settings and remove any conflicting database URL configuration.

1. Verify the migrated deployment before allowing normal use.
   Confirm you can log in with an existing account, find existing templates and workspaces, and connect to a workspace.
   Check that **Health** > **Database** is reachable and no longer reports EDB03.
   The startup output should no longer say `Using built-in PostgreSQL (...)`.

1. Configure and test backups for the external database before decommissioning the built-in database.

> [!WARNING]
> Do not delete the original `postgres` directory or its backup until you've verified the restore and the deployment works with the external database.
> Deleting the only recoverable copy makes a failed migration irreversible.
> After Coder writes to the external database, switching back to the old database discards those newer changes.

## Learn more

- [Deployment health and EDB03](../monitoring/health-check.md#edb03)
- [External database configuration](../../tutorials/external-database.md)
- [PostgreSQL dump options](https://www.postgresql.org/docs/current/app-pgdump.html)
- [PostgreSQL restore options](https://www.postgresql.org/docs/current/app-pgrestore.html)
