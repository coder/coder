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

## Handling Nullable Fields

Use `sql.NullString`, `sql.NullBool`, etc. for optional database fields:

```go
CodeChallenge: sql.NullString{
    String: params.codeChallenge,
    Valid:  params.codeChallenge != "",
}
```

Set `.Valid = true` when providing values.

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
# Check database connection
make test-postgres

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
