# Database Development Patterns

This guide owns migrations, SQL queries and generation, audit-table updates,
transaction discipline, and database-to-SDK conversion.

## Query and Generation Workflow

Keep SQL in `coderd/database/queries/*.sql`, grouped by domain. Name lookup
queries `ByX`; use `PerX` or `GroupedByX` for aggregation dimensions.

For every query, schema, or generated database model change:

1. Modify the relevant SQL files.
2. Run `make gen`.
3. Inspect all generated changes and generation errors.
4. If generation reports missing audit actions, update
   `enterprise/audit/table.go`.
5. Classify every new auditable field as `ActionTrack`, `ActionIgnore`, or
   `ActionSecret`.
6. Run `make gen` again.
7. Run `make lint` and the relevant database tests.
8. Commit the SQL, audit-table update, and generated files together.

Do not hand-edit generated database files. Run generation twice when a generator
updates inputs consumed by another generator, then confirm the second run is
clean.

## Migrations

Create paired reversible migrations in `coderd/database/migrations/`:

```text
{number}_{description}.up.sql
{number}_{description}.down.sql
```

Use the helpers:

```sh
./coderd/database/migrations/create_migration.sh "migration name"
./coderd/database/migrations/fix_migration_numbers.sh
./coderd/database/migrations/create_fixture.sh "fixture name"
```

- Keep numbers unique and sequential.
- Put one logical schema or data change in each pair.
- Make the down migration reverse the up migration.
- Add indexes and constraints required by the access pattern.
- Test data migrations with representative existing rows.

### Enum Values Used in the Same Transaction

All migrations run inside one transaction through `pgTxnDriver`. PostgreSQL
forbids using a value added with `ALTER TYPE ... ADD VALUE` in the transaction
that added it. A later migration in the same batch counts as the same
transaction. Casts, inserts, updates, and defaults can all trigger
`unsafe use of new value`.

If any migration uses a newly added enum value, recreate the enum type instead
of using `ADD VALUE`. The recreated type's values are immediately usable in the
same transaction. Precedent: `000144_user_status_dormant`.

```sql
CREATE TYPE new_my_enum AS ENUM ('existing', 'value', 'new_value');

ALTER TABLE my_table
    ALTER COLUMN col TYPE new_my_enum USING (col::text::new_my_enum);

DROP TYPE my_enum;
ALTER TYPE new_my_enum RENAME TO my_enum;
```

Recreation must leave the final schema identical. Generation should produce no
unexpected `dump.sql` drift. `migrations.Stepper` commits each migration and
cannot expose this failure. Seed a row that materializes the new value, then
apply the affected migrations in one transaction. See
`TestMigration000504AIProvidersBackfillEnumInSingleTxn`.

## Nullable Fields

Use the nullable type established by nearby generated code. Set both the value
and validity explicitly when constructing legacy `sql.Null*` values.

```go
CodeChallenge: sql.NullString{
    String: params.codeChallenge,
    Valid:  params.codeChallenge != "",
}
```

## Transactions with `InTx`

Inside `db.InTx(...)`, keep all database work on the transaction handle.

- Do not call `api.Database`, `p.db`, another outer store, or a helper that hides
  outer-store access from inside the closure.
- Pass `tx` to helpers that perform database work.
- Fetch read-only inputs before opening the transaction when they do not need
  transactional consistency.
- Review receiver helpers carefully. A helper call is unsafe when it reaches an
  outer store internally.

Using the outer store while a transaction holds a connection can block on a
second pool checkout, causing pool starvation and `idle in transaction`
incidents.

## Database-to-SDK Converters

Use explicit conversion helpers for database models returned through the SDK.
Keep nullable handling, type coercion, enum mapping, and response shaping in the
converter. Keep handlers focused on authorization and request flow.

## Authorization

All database access must use the appropriate `dbauthz` context. For public
OAuth2 endpoints that need system access, follow
[OAuth2 Development Guide](OAUTH2.md).

## Verification Commands

```sh
make gen
make lint
go test ./coderd/database/... -run TestName
```

Inspect the final diff for migration pairs, generated changes, audit
classification, and unintended schema drift.
