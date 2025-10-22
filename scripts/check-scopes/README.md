# check-scopes

Validates that the DB enum `api_key_scope` contains every `<resource>:<action>` derived from `coderd/rbac/policy/RBACPermissions`.

- Exits 0 when all scopes are present in `coderd/database/dump.sql`.
- Exits 1 and prints missing values with suggested `ALTER TYPE` statements otherwise.

## Usage

Ensure the schema dump is up-to-date, then run the check:

```sh
make -B gen/db   # forces DB dump regeneration
make lint/check-scopes
```

Or directly:

```sh
go run ./tools/check-scopes
```

Optional flags:

- `-dump path` — override path to `dump.sql` (default `coderd/database/dump.sql`).

## Remediation

When the tool reports missing values:

1. Create a DB migration extending the enum, e.g.:

   ```sql
   ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:view_insights';
   ```

2. Regenerate and re-run:

   ```sh
   make -B gen/db && make lint/check-scopes
   ```

3. Decide whether each new scope is public (exposed in the catalog) or internal-only.
   - If public, add it to the curated map in `coderd/rbac/scopes_catalog.go` (`externalLowLevel`) so it appears in the public catalog and can be requested by users.
