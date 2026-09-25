# Database Development Patterns

## Database Work Overview

### Database Generation Process

1. Modify SQL files in `coderd/database/queries/`
2. Run `make gen`
3. If errors about audit table, update `enterprise/audit/table.go`
4. Run `make gen` again
5. Run `make lint` to catch any remaining issues

## Migration Guidelines

### Creating Migration Files

**Location**: `coderd/database/migrations/`
**Format**: `{number}_{description}.{up|down}.sql`

- Number must be unique and sequential
- Always include both up and down migrations

### Helper Scripts

| Script                                                              | Purpose                                 |
|---------------------------------------------------------------------|-----------------------------------------|
| `./coderd/database/migrations/create_migration.sh "migration name"` | Creates new migration files             |
| `./coderd/database/migrations/fix_migration_numbers.sh`             | Renumbers migrations to avoid conflicts |
| `./coderd/database/migrations/create_fixture.sh "fixture name"`     | Creates test fixtures for migrations    |

### Database Query Organization

- **MUST DO**: Any changes to database - adding queries, modifying queries should be done in the `coderd/database/queries/*.sql` files
- **MUST DO**: Queries are grouped in files relating to context - e.g. `prebuilds.sql`, `users.sql`, `oauth2.sql`
- After making changes to any `coderd/database/queries/*.sql` files you must run `make gen` to generate respective ORM changes

### Query Naming

- Use `ByX` when `X` is the lookup or filter column.
- Use `PerX` or `GroupedByX` when `X` is the aggregation or grouping
  dimension.
- Avoid `ByX` names for grouped queries.

### How Migrations Run

`migrations.Up` (`coderd/database/migrations/driver.go`) takes a session-level
advisory lock so concurrent coderd replicas serialize, then applies each
pending migration in its own transaction. The `schema_migrations` row is
written in that same transaction, so the recorded version and the applied
schema cannot diverge: a migration either fully commits or leaves no trace.

Every statement runs under a Postgres `lock_timeout` (default 3s, override
with `CODER_PG_MIGRATION_LOCK_TIMEOUT`, `0` waits forever). A migration that
loses a lock race is rolled back and the remaining migrations are retried from
the last committed version, up to five times with backoff. Any other error
stops the run immediately.

#### Recovery after a failed batch

A failure partway through a batch leaves the earlier migrations committed and
`schema_migrations` at the last completed version with `dirty = false`. No
operator action is needed: the next `coder server` start (or `Up()` call)
resumes from that version. Ship a fixed build and restart.

`dirty = true` now means exactly one thing: a `-- coder:no-transaction`
migration (see below) started and did not finish. Transactional migrations
can never produce it. `Up()` refuses to run while the row is dirty, and
`EnsureClean` reports the database as not cleanly migrated. To recover:

1. Inspect the effect the migration was supposed to have. For
   `CREATE INDEX CONCURRENTLY`, check `pg_index.indisvalid`; a failed build
   leaves an `INVALID` index that `IF NOT EXISTS` would silently keep, so drop
   it with `DROP INDEX CONCURRENTLY IF EXISTS`.
2. Fix whatever made the statement fail (for example duplicate rows blocking a
   unique index).
3. Rewind so the migration runs again in full:
   `UPDATE schema_migrations SET version = <version - 1>, dirty = false;`
4. Start the server. Only set `dirty = false` without rewinding if you have
   verified by hand that the migration's effect is fully present.

### Migrations That Cannot Run in a Transaction

`CREATE INDEX CONCURRENTLY` and `DROP INDEX CONCURRENTLY` are rejected inside
a transaction block. Opt a migration out by putting this comment before any
SQL:

```sql
-- coder:no-transaction
CREATE INDEX CONCURRENTLY IF NOT EXISTS workspaces_owner_id_idx
    ON workspaces (owner_id);
```

The statement then runs in autocommit mode on the locked connection, with the
`schema_migrations` row marked dirty before it starts and clean after it
succeeds. Postgres cannot roll it back, so these rules are enforced in review
and by `TestNoTransactionMigrationsAreIdempotent`:

- **Exactly one statement per file.** Postgres runs a multi-statement string
  in an implicit transaction block, which rejects `CONCURRENTLY` just like an
  explicit one. Put a `DROP INDEX CONCURRENTLY IF EXISTS` cleanup in its own
  preceding migration if you need one.
- **Idempotent.** Use `IF NOT EXISTS` / `IF EXISTS` so a rerun after a crash
  is harmless.
- **Only for `CONCURRENTLY`.** Anything that can run in a transaction must.
- Both the up and down file need the marker if both use `CONCURRENTLY`.

### Enum Changes

Because each migration commits on its own, a value added with
`ALTER TYPE ... ADD VALUE` in one migration is usable by the next migration.
The old "unsafe use of new value" restriction from the single-transaction
driver no longer applies across files. Within a single file it still does: do
not add an enum value and use it in the same migration. Either split the use
into the next migration or recreate the type (precedent:
`000144_user_status_dormant`).

## Handling Nullable Fields

Use `sql.NullString`, `sql.NullBool`, etc. for optional database fields:

```go
CodeChallenge: sql.NullString{
    String: params.codeChallenge,
    Valid:  params.codeChallenge != "",
}
```

Set `.Valid = true` when providing values.

## Database-to-SDK Conversions

- Extract explicit db-to-SDK conversion helpers instead of inlining large
  conversion blocks inside handlers.
- Keep nullable-field handling, type coercion, and response shaping in the
  converter so handlers stay focused on request flow and authorization.

## Audit Table Updates

If adding fields to auditable types:

1. Update `enterprise/audit/table.go`
2. Add each new field with appropriate action:
   - `ActionTrack`: Field should be tracked in audit logs
   - `ActionIgnore`: Field should be ignored in audit logs
   - `ActionSecret`: Field contains sensitive data
3. Run `make gen` to verify no audit errors

## Database Architecture

### Core Components

- **PostgreSQL 13+** recommended for production
- **Migrations** managed with `migrate`
- **Database authorization** through `dbauthz` package

### Authorization Patterns

```go
// Public endpoints needing system access (OAuth2 registration)
app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)

// Authenticated endpoints with user context
app, err := api.Database.GetOAuth2ProviderAppByClientID(ctx, clientID)

// System operations in middleware
roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), userID)
```

## Common Database Issues

### Migration Issues

1. **Migration conflicts**: Use `fix_migration_numbers.sh` to renumber
2. **Missing down migration**: Always create both up and down files
3. **Schema inconsistencies**: Verify against existing schema

### Field Handling Issues

1. **Nullable field errors**: Use `sql.Null*` types consistently
2. **Missing audit entries**: Update `enterprise/audit/table.go`

### Query Issues

1. **Query organization**: Group related queries in appropriate files
2. **Generated code errors**: Run `make gen` after query changes
3. **Performance issues**: Add appropriate indexes in migrations

## Database Testing

### Test Database Setup

```go
func TestDatabaseFunction(t *testing.T) {
    db := dbtestutil.NewDB(t)

    // Test with real database
    result, err := db.GetSomething(ctx, param)
    require.NoError(t, err)
    require.Equal(t, expected, result)
}
```

## Best Practices

### Schema Design

1. **Use appropriate data types**: VARCHAR for strings, TIMESTAMP for times
2. **Add constraints**: NOT NULL, UNIQUE, FOREIGN KEY as appropriate
3. **Create indexes**: For frequently queried columns
4. **Consider performance**: Normalize appropriately but avoid over-normalization

### Query Writing

1. **Use parameterized queries**: Prevent SQL injection
2. **Handle errors appropriately**: Check for specific error types
3. **Use transactions**: For related operations that must succeed together
4. **Optimize queries**: Use EXPLAIN to understand query performance

### Transaction Safety with `InTx`

- Inside `db.InTx(...)` closures, do not use the outer store
  (`api.Database`, `p.db`, etc.) directly or indirectly. Use the `tx`
  handle for DB work inside the closure, or fetch read-only inputs before
  opening the transaction.
- Watch for helper methods on a receiver that hide outer-store access. A
  call like `p.someHelper(ctx)` is still unsafe inside `InTx` if that
  helper uses `p.db` internally.
- Using the outer store while a transaction is open can hold one
  connection and then block on another pool checkout, which can cause
  pool starvation and `idle in transaction` incidents under load.

### Migration Writing

1. **Make migrations reversible**: Always include down migration
2. **Test migrations**: On copy of production data if possible
3. **Keep migrations small**: One logical change per migration
4. **Document complex changes**: Add comments explaining rationale

## Advanced Patterns

### Complex Queries

```sql
-- Example: Complex join with aggregation
SELECT
    u.id,
    u.username,
    COUNT(w.id) as workspace_count
FROM users u
LEFT JOIN workspaces w ON u.id = w.owner_id
WHERE u.created_at > $1
GROUP BY u.id, u.username
ORDER BY workspace_count DESC;
```

### Conditional Queries

```sql
-- Example: Dynamic filtering
SELECT * FROM oauth2_provider_apps
WHERE
    ($1::text IS NULL OR name ILIKE '%' || $1 || '%')
    AND ($2::uuid IS NULL OR organization_id = $2)
ORDER BY created_at DESC;
```

### Audit Patterns

```go
// Example: Auditable database operation
func (q *sqlQuerier) UpdateUser(ctx context.Context, arg UpdateUserParams) (User, error) {
    // Implementation here

    // Audit the change
    if auditor := audit.FromContext(ctx); auditor != nil {
        auditor.Record(audit.UserUpdate{
            UserID: arg.ID,
            Old:    oldUser,
            New:    newUser,
        })
    }

    return newUser, nil
}
```

## Debugging Database Issues

### Common Debug Commands

```bash
# Run tests (starts Postgres automatically if needed)
make test

# Run specific database tests
go test ./coderd/database/... -run TestSpecificFunction

# Check query generation
make gen

# Verify audit table
make lint
```

### Debug Techniques

1. **Enable query logging**: Set appropriate log levels
2. **Use database tools**: pgAdmin, psql for direct inspection
3. **Check constraints**: UNIQUE, FOREIGN KEY violations
4. **Analyze performance**: Use EXPLAIN ANALYZE for slow queries

### Troubleshooting Checklist

- [ ] Migration files exist (both up and down)
- [ ] `make gen` run after query changes
- [ ] Audit table updated for new fields
- [ ] Nullable fields use `sql.Null*` types
- [ ] Authorization context appropriate for endpoint type
