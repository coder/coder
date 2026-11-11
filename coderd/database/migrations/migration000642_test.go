package migrations_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

const (
	migration000642Up   = "000642_chat_automations.up.sql"
	migration000642Down = "000642_chat_automations.down.sql"
)

// schemaFingerprint lists what a migration can change in the current schema:
// enum labels in their sort order, columns in their order with type, default
// and comment, constraints, indexes, views, triggers and functions.
func schemaFingerprint(ctx context.Context, t *testing.T, db *sql.DB) []string {
	t.Helper()
	const query = `
SELECT 'enum ' || t.typname || ': ' || string_agg(e.enumlabel, ', ' ORDER BY e.enumsortorder)
FROM pg_enum e
JOIN pg_type t ON t.oid = e.enumtypid
WHERE t.typnamespace = current_schema()::regnamespace
GROUP BY t.typname
UNION ALL
SELECT 'columns ' || c.relname || ': ' || string_agg(
	a.attname || ' ' || format_type(a.atttypid, a.atttypmod)
		|| CASE WHEN a.attnotnull THEN ' not null' ELSE '' END
		|| COALESCE(' default ' || pg_get_expr(d.adbin, d.adrelid), '')
		|| COALESCE(' comment ' || col_description(c.oid, a.attnum), ''),
	'; ' ORDER BY a.attnum)
FROM pg_class c
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE c.relnamespace = current_schema()::regnamespace AND c.relkind IN ('r', 'v')
GROUP BY c.relname
UNION ALL
SELECT 'constraint ' || conrelid::regclass::text || '.' || conname || ': ' || pg_get_constraintdef(oid)
FROM pg_constraint
WHERE connamespace = current_schema()::regnamespace
UNION ALL
SELECT 'index ' || indexname || ': ' || indexdef
FROM pg_indexes
WHERE schemaname = current_schema()
UNION ALL
SELECT 'view ' || viewname || ': ' || definition
FROM pg_views
WHERE schemaname = current_schema()
UNION ALL
SELECT 'trigger ' || tgrelid::regclass::text || '.' || tgname || ': ' || pg_get_triggerdef(oid)
FROM pg_trigger
WHERE NOT tgisinternal
UNION ALL
SELECT 'function ' || proname || ': ' || pg_get_functiondef(oid)
FROM pg_proc
WHERE pronamespace = current_schema()::regnamespace AND prokind = 'f'
ORDER BY 1`
	rows, err := db.QueryContext(ctx, query)
	require.NoError(t, err)
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, lines)
	return lines
}

func readMigration000642(t *testing.T) (up, down []byte) {
	t.Helper()
	up, err := os.ReadFile(migration000642Up)
	require.NoError(t, err)
	down, err = os.ReadFile(migration000642Down)
	require.NoError(t, err)
	return up, down
}

// The down migration recreates the api_key_scope enum from a list in the
// file and recreates chats_expanded. Up, down and up again must give the
// schema of main and then the same new schema, including the order of the
// enum labels and of the columns.
func TestMigration000642ChatAutomationsRoundTrip(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 641)
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	up, down := readMigration000642(t)

	before := schemaFingerprint(ctx, t, sqlDB)

	// A failure after the migration's last statement must leave nothing
	// behind: the migrator runs every pending migration in one transaction.
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(up))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SELECT 1 / 0`)
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	require.Equal(t, before, schemaFingerprint(ctx, t, sqlDB), "schema after a rolled back migration")

	_, err = sqlDB.ExecContext(ctx, string(up))
	require.NoError(t, err)
	migrated := schemaFingerprint(ctx, t, sqlDB)
	require.NotEqual(t, before, migrated)

	_, err = sqlDB.ExecContext(ctx, string(down))
	require.NoError(t, err)
	require.Equal(t, before, schemaFingerprint(ctx, t, sqlDB), "schema after up and down")

	_, err = sqlDB.ExecContext(ctx, string(up))
	require.NoError(t, err)
	require.Equal(t, migrated, schemaFingerprint(ctx, t, sqlDB), "schema after up, down and up")
}

// The down migration removes five enum values that rows may use, and it
// drops the flag that says a queued message still needs its prompt hooks.
func TestMigration000642DownWithData(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 642)
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	_, down := readMigration000642(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	exec := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.NoError(t, err, query)
	}
	chatID, userID := seedChatWithStatus(ctx, t, sqlDB, "waiting")
	appID := uuid.New()
	exec(`INSERT INTO oauth2_provider_apps (id, created_at, updated_at, name, icon, callback_url)
		VALUES ($1, $2, $2, 'app-642', '', 'http://localhost/callback')`, appID, now)

	keys := []struct {
		id          string
		scopes      string
		allowList   string
		withToken   bool
		wantDeleted bool
		wantScopes  string
	}{
		// One key per removed value.
		{id: "only-all", scopes: "{chat_automation:*}", wantDeleted: true},
		{id: "only-create", scopes: "{chat_automation:create}", wantDeleted: true},
		{id: "only-read", scopes: "{chat_automation:read}", wantDeleted: true},
		{id: "only-update", scopes: "{chat_automation:update}", wantDeleted: true},
		{id: "only-delete", scopes: "{chat_automation:delete}", wantDeleted: true},
		// The other scopes stay, in their order.
		{id: "mixed", scopes: "{template:read,chat_automation:read,chat:*,chat_automation:*,workspace:read}", wantScopes: "{template:read,chat:*,workspace:read}"},
		{id: "ordinary", scopes: "{coder:all}", wantScopes: "{coder:all}"},
		// The allow list is not enum-typed, and the down leaves it alone.
		{id: "ordinary-allow-list", scopes: "{template:read}", allowList: "{chat_automation:*}", wantScopes: "{template:read}"},
		// Deleting a key deletes its OAuth2 token.
		{id: "only-automation-token", scopes: "{chat_automation:update}", withToken: true, wantDeleted: true},
		{id: "ordinary-token", scopes: "{template:read}", withToken: true, wantScopes: "{template:read}"},
	}
	for _, key := range keys {
		allowList := key.allowList
		if allowList == "" {
			allowList = "{*:*}"
		}
		exec(`INSERT INTO api_keys (id, hashed_secret, user_id, last_used, expires_at, created_at, updated_at, login_type, scopes, allow_list, token_name)
			VALUES ($1, $2, $3, $4, $4, $4, $4, 'token', $5, $6, $1)`, key.id, []byte(key.id), userID, now, key.scopes, allowList)
		if key.withToken {
			exec(`INSERT INTO oauth2_provider_app_tokens (id, created_at, expires_at, hash_prefix, refresh_hash, api_key_id, user_id, app_id, scope)
				VALUES ($1, $2, $2, $3, $3, $4, $5, $6, 'template:read')`, uuid.New(), now, []byte(key.id), key.id, userID, appID)
		}
	}

	automationID := uuid.New()
	exec(`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, prompt, target_mode, target_chat_id, when_busy, webhook_auth, webhook_secret)
		SELECT $1, organization_id, owner_id, 'webhook', 'webhook', 'hello', 'existing_chat', id, 'queue', 'bearer', 'secret'
		FROM chats WHERE id = $2`, automationID, chatID)
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id, prompt_hooks_pending)
		VALUES ($1, '["automation"]', $2, $3, $4, true)`, chatID, userID, automationID, uuid.New())
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by, prompt_hooks_pending)
		VALUES ($1, '["hooks pending"]', $2, true)`, chatID, userID)
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by)
		VALUES ($1, '["ordinary"]', $2)`, chatID, userID)
	exec(`INSERT INTO chat_messages (chat_id, role, content_version, automation_id, input_id, prompt_hooks_pending)
		VALUES ($1, 'user', 1, $2, $3, true)`, chatID, automationID, uuid.New())
	exec(`UPDATE chats SET automation_id = $2, manage_automations_enabled = true WHERE id = $1`, chatID, automationID)

	_, err := sqlDB.ExecContext(ctx, string(down))
	require.NoError(t, err)

	for _, key := range keys {
		var scopes string
		err := sqlDB.QueryRowContext(ctx, `SELECT scopes::text FROM api_keys WHERE id = $1`, key.id).Scan(&scopes)
		var tokens int
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			`SELECT count(*) FROM oauth2_provider_app_tokens WHERE api_key_id = $1`, key.id).Scan(&tokens))
		if key.wantDeleted {
			require.ErrorIs(t, err, sql.ErrNoRows, key.id)
			require.Zero(t, tokens, key.id)
			continue
		}
		require.NoError(t, err, key.id)
		require.Equal(t, key.wantScopes, scopes, key.id)
		if key.withToken {
			require.Equal(t, 1, tokens, key.id)
		}
	}
	var allowList string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT allow_list::text FROM api_keys WHERE id = 'ordinary-allow-list'`).Scan(&allowList))
	require.Equal(t, "{chat_automation:*}", allowList)

	var removedLabels int
	require.NoError(t, sqlDB.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_enum e JOIN pg_type t ON t.oid = e.enumtypid
		WHERE t.typname = 'api_key_scope' AND e.enumlabel LIKE 'chat\_automation:%'`).Scan(&removedLabels))
	require.Zero(t, removedLabels)

	var queued string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT string_agg(content::text, ',') FROM chat_queued_messages WHERE chat_id = $1`, chatID).Scan(&queued))
	require.Equal(t, `["ordinary"]`, queued, "only the ordinary queued message stays")
	var messages, chats int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM chat_messages WHERE chat_id = $1`, chatID).Scan(&messages))
	require.Equal(t, 1, messages, "history messages stay")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM chats WHERE id = $1`, chatID).Scan(&chats))
	require.Equal(t, 1, chats)
}

// The down migration deletes API keys, and that delete cascades into
// oauth2_provider_app_tokens. The application deletes in the other order:
// a token delete fires a trigger that deletes the API key. The migration
// must hold both tables before it deletes anything, or the two sides wait
// for each other.
func TestMigration000642DownConcurrentTokenDelete(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 642)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, down := readMigration000642(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, userID := seedChatWithStatus(ctx, t, sqlDB, "waiting")
	appID, tokenID := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO oauth2_provider_apps (id, created_at, updated_at, name, icon, callback_url)
			VALUES ($1, $2, $2, 'app-642', '', 'http://localhost/callback')`, []any{appID, now}},
		{`INSERT INTO api_keys (id, hashed_secret, user_id, last_used, expires_at, created_at, updated_at, login_type, scopes, allow_list, token_name)
			VALUES ('automation-key', '\x01', $1, $2, $2, $2, $2, 'token', '{chat_automation:update}', '{*:*}', 'automation-key')`, []any{userID, now}},
		{`INSERT INTO oauth2_provider_app_tokens (id, created_at, expires_at, hash_prefix, refresh_hash, api_key_id, user_id, app_id, scope)
			VALUES ($1, $2, $2, '\x01', '\x01', 'automation-key', $3, $4, 'template:read')`, []any{tokenID, now, userID, appID}},
	} {
		_, err := sqlDB.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}

	// Run the lock block, then let the application start its delete, then
	// run the rest of the migration.
	const lockBlockEnd = "\n$$;\n"
	i := bytes.Index(down, []byte(lockBlockEnd))
	require.Positive(t, i, "lock block not found")
	lockBlock, rest := down[:i+len(lockBlockEnd)], down[i+len(lockBlockEnd):]

	migrationConn, migrationPID := dedicatedConn(ctx, t, sqlDB)
	migrationTx, err := migrationConn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = migrationTx.Rollback() }()
	_, err = migrationTx.ExecContext(ctx, string(lockBlock))
	require.NoError(t, err)

	appConn, appPID := dedicatedConn(ctx, t, sqlDB)
	app := async(func() error {
		_, err := appConn.ExecContext(ctx, `DELETE FROM oauth2_provider_app_tokens WHERE id = $1`, tokenID)
		return err
	})
	requireWaitsFor(ctx, t, sqlDB, appPID, migrationPID)

	migration := async(func() error {
		_, err := migrationTx.ExecContext(ctx, string(rest))
		return errors.Join(err, migrationTx.Rollback())
	})
	requireNoDeadlockVictim(ctx, t, "migration", migration)
	requireNoDeadlockVictim(ctx, t, "application", app)

	var tokens int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM oauth2_provider_app_tokens WHERE id = $1`, tokenID).Scan(&tokens))
	require.Zero(t, tokens, "the application's delete ran after the migration rolled back")
}

// A running server takes these locks on every request. The down migration
// waits for them with short timeouts instead of queueing behind them, so
// that new requests do not queue behind the migration.
func TestMigration000642DownWaitsForOpenTransaction(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 642)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, down := readMigration000642(t)
	chatID, _ := seedChatWithStatus(ctx, t, sqlDB, "waiting")

	conn, appPID := dedicatedConn(ctx, t, sqlDB)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID)
	require.NoError(t, err)

	pid, migrated := startRolledBackMigration(ctx, t, sqlDB, down)
	requireWaitsFor(ctx, t, sqlDB, pid, appPID)

	// Between attempts the migration holds nothing, so a reader of the
	// view it drops gets through.
	reader, _ := dedicatedConn(ctx, t, sqlDB)
	requireNoDeadlockVictim(ctx, t, "reader", async(func() error {
		var id uuid.UUID
		return reader.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, chatID).Scan(&id)
	}))

	requireNoDeadlockVictim(ctx, t, "application", async(func() error {
		if _, err := tx.ExecContext(ctx, `UPDATE chats SET title = 'renamed' WHERE id = $1`, chatID); err != nil {
			return err
		}
		return tx.Commit()
	}))
	requireNoDeadlockVictim(ctx, t, "migration", migrated)
}
