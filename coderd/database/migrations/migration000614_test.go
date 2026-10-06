package migrations_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

// TestMigration000614RemoveChatAutomations upgrades a database that used chat
// automations through the production migration runner, which applies 000614
// and 000615 in one transaction. Automation data and credentials must go,
// while the chats, their messages, ordinary queued messages, and the other
// scopes and permissions on the same credentials must stay.
func TestMigration000614RemoveChatAutomations(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 613)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)
	orgID, userID, modelID := uuid.New(), uuid.New(), uuid.New()
	chatID, otherChatID := uuid.New(), uuid.New()
	existingAutomationID, newChatAutomationID, inputID := uuid.New(), uuid.New(), uuid.New()
	mixedRoleID, automationRoleID := uuid.New(), uuid.New()
	mixedAppID, automationAppID := uuid.New(), uuid.New()
	const (
		automationPermission = `{"negate": false, "resource_type": "chat_automation", "action": "read"}`
		userPermission       = `{"negate": false, "resource_type": "user", "action": "read"}`
	)

	exec := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.NoError(t, err, query)
	}
	exec(`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
		VALUES ($1, 'org-614', 'org-614', '', $2, $2, '{}')`, orgID, now)
	exec(`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
		VALUES ($1, 'user614@example.com', 'user614', '\x', $2, $2)`, userID, now)
	// A deleted model config needs no AI provider.
	exec(`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
		VALUES ($1, $2, 'model-614', 1000, 50, true)`, modelID, orgID)
	exec(`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title, automation_id, manage_automations_enabled)
		VALUES ($1, $2, $3, $4, 'automated', $5, true)`, chatID, userID, orgID, modelID, newChatAutomationID)
	exec(`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title)
		VALUES ($1, $2, $3, $4, 'ordinary')`, otherChatID, userID, orgID, modelID)
	exec(`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, enabled, target_mode, target_chat_id, when_busy, webhook_use, webhook_secret_hash, prompt)
		VALUES ($1, $2, $3, 'webhook', 'webhook', true, 'existing_chat', $4, 'queue', 'multi', '\x01', 'hello')`,
		existingAutomationID, orgID, userID, chatID)
	exec(`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, enabled, target_mode, new_chat_model_config_id, schedule_cron, schedule_time_zone, prompt)
		VALUES ($1, $2, $3, 'schedule', 'schedule', true, 'new_chat', $4, '0 9 * * *', 'UTC', 'hello')`,
		newChatAutomationID, orgID, userID, modelID)
	exec(`INSERT INTO chat_messages (chat_id, role, content_version, automation_id, input_id)
		VALUES ($1, 'user', 1, $2, $3)`, chatID, existingAutomationID, inputID)
	exec(`INSERT INTO chat_messages (chat_id, role, content_version) VALUES ($1, 'user', 1)`, chatID)
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id, queue_generation)
		VALUES ($1, '"automation"', $2, $3, $4, 1)`, chatID, userID, existingAutomationID, inputID)
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by) VALUES ($1, '"ordinary"', $2)`, chatID, userID)
	exec(`INSERT INTO chat_queued_messages (chat_id, content, created_by) VALUES ($1, '"other chat"', $2)`, otherChatID, userID)
	// Later transitions advance snapshot_version without touching the
	// queue, so queue_version falls behind it.
	exec(`UPDATE chats SET snapshot_version = snapshot_version + 5`)

	// The custom roles keep their other permissions.
	exec(`INSERT INTO custom_roles (id, name, display_name, organization_id, site_permissions, org_permissions, user_permissions, member_permissions)
		VALUES ($1, 'mixed-role', 'Mixed Role', $2, $3, $4, $5, $6)`,
		mixedRoleID, orgID, "["+automationPermission+", "+userPermission+"]", "["+automationPermission+"]", "["+automationPermission+", "+userPermission+"]", "["+userPermission+", "+automationPermission+"]")
	exec(`INSERT INTO custom_roles (id, name, display_name, organization_id, site_permissions, org_permissions, user_permissions, member_permissions)
		VALUES ($1, 'automation-role', 'Automation Role', $2, $3, '[]', '[]', '[]')`,
		automationRoleID, orgID, "["+automationPermission+"]")

	apps := []struct {
		id        uuid.UUID
		name      string
		scope     string
		wantScope string
	}{
		{mixedAppID, "mixed-app", "chat_automation:read template:read", "template:read"},
		// An empty scope reads as unrestricted, so a whitespace-only one
		// stands in for "nothing allowed".
		{automationAppID, "automation-app", "chat_automation:read chat_automation:create", " "},
	}
	for _, app := range apps {
		exec(`INSERT INTO oauth2_provider_apps (id, created_at, updated_at, name, icon, callback_url, scope)
			VALUES ($1, $2, $2, $3, '', 'http://localhost/callback', $4)`, app.id, now, app.name, app.scope)
	}

	keys := []struct {
		id            string
		scopes        string
		allowList     string
		wantDeleted   bool
		wantScopes    string
		wantAllowList string
	}{
		{id: "mixed-key", scopes: "{chat_automation:read,chat_project_memory:read}", allowList: "{*:*}", wantScopes: "{chat_project_memory:read}", wantAllowList: "{*:*}"},
		{id: "automation-key", scopes: "{chat_automation:*,chat_automation:delete}", allowList: "{*:*}", wantDeleted: true},
		{id: "mixed-allow-key", scopes: "{template:read}", allowList: "{chat_automation:*,user:*}", wantScopes: "{template:read}", wantAllowList: "{user:*}"},
		{id: "automation-allow-key", scopes: "{template:read}", allowList: "{chat_automation:3f0c9b2e-5d41-4a7b-9c1e-8d2f6a4b7c5e}", wantDeleted: true},
		{id: "ordinary-key", scopes: "{coder:all}", allowList: "{*:*}", wantScopes: "{coder:all}", wantAllowList: "{*:*}"},
	}
	for _, key := range keys {
		exec(`INSERT INTO api_keys (id, hashed_secret, user_id, last_used, expires_at, created_at, updated_at, login_type, scopes, allow_list, token_name)
			VALUES ($1, $2, $3, $4, $4, $4, $4, 'token', $5, $6, $1)`, key.id, []byte(key.id), userID, now, key.scopes, key.allowList)
	}

	// Each grant is stored as both an authorization code and a token, keyed
	// by the grant name.
	grants := []struct {
		key         string
		scope       string
		wantDeleted bool
		wantScope   string
	}{
		{key: "mixed-grant", scope: "chat_automation:read template:read", wantScope: "template:read"},
		{key: "automation-grant", scope: "chat_automation:read chat_automation:update", wantDeleted: true},
	}
	for _, grant := range grants {
		exec(`INSERT INTO oauth2_provider_app_codes (id, created_at, expires_at, secret_prefix, hashed_secret, user_id, app_id, scope)
			VALUES ($1, $2, $2, $3, $3, $4, $5, $6)`, uuid.New(), now, []byte(grant.key), userID, mixedAppID, grant.scope)
		// oauth2_provider_app_tokens.api_key_id is a NOT NULL foreign key,
		// so this key only satisfies it and is not asserted.
		exec(`INSERT INTO api_keys (id, hashed_secret, user_id, last_used, expires_at, created_at, updated_at, login_type, scopes, allow_list, token_name)
			VALUES ($1, $2, $3, $4, $4, $4, $4, 'oauth2_provider_app', '{template:read}', '{*:*}', $1)`, grant.key, []byte(grant.key), userID, now)
		exec(`INSERT INTO oauth2_provider_app_tokens (id, created_at, expires_at, hash_prefix, refresh_hash, api_key_id, user_id, app_id, scope)
			VALUES ($1, $2, $2, $3, $3, $4, $5, $6, $7)`, uuid.New(), now, []byte(grant.key), grant.key, userID, mixedAppID, grant.scope)
	}

	require.NoError(t, migrations.Up(sqlDB))

	queryStrings := func(query string, args ...any) []string {
		t.Helper()
		rows, err := sqlDB.QueryContext(ctx, query, args...)
		require.NoError(t, err, query)
		defer rows.Close()
		var got []string
		for rows.Next() {
			var s string
			require.NoError(t, rows.Scan(&s))
			got = append(got, s)
		}
		require.NoError(t, rows.Err())
		return got
	}
	assertMigrated := func(t *testing.T) {
		t.Helper()

		// Chats and their messages stay. Only automation-queued messages
		// are deleted, and the queue version trigger records the change.
		require.ElementsMatch(t, []string{"automated", "ordinary"}, queryStrings(`SELECT title FROM chats`))
		require.ElementsMatch(t, []string{chatID.String(), chatID.String()}, queryStrings(`SELECT chat_id::text FROM chat_messages`))
		require.ElementsMatch(t, []string{`"ordinary"`, `"other chat"`}, queryStrings(`SELECT content::text FROM chat_queued_messages`))
		var queueVersion, snapshotVersion int64
		require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT queue_version, snapshot_version FROM chats WHERE id = $1`, chatID).Scan(&queueVersion, &snapshotVersion))
		require.Equal(t, snapshotVersion, queueVersion, "the deleted queued message must advance queue_version")
		require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT queue_version, snapshot_version FROM chats WHERE id = $1`, otherChatID).Scan(&queueVersion, &snapshotVersion))
		require.Less(t, queueVersion, snapshotVersion, "a chat without automation-queued messages keeps its queue_version")

		for column, want := range map[string]string{
			"site_permissions":   "[" + userPermission + "]",
			"org_permissions":    "[]",
			"user_permissions":   "[" + userPermission + "]",
			"member_permissions": "[" + userPermission + "]",
		} {
			var got string
			require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT `+column+`::text FROM custom_roles WHERE id = $1`, mixedRoleID).Scan(&got))
			require.JSONEq(t, want, got, column)
		}
		require.Equal(t, []string{"[]"}, queryStrings(`SELECT site_permissions::text FROM custom_roles WHERE id = $1`, automationRoleID))

		for _, app := range apps {
			require.Equal(t, []string{app.wantScope}, queryStrings(`SELECT scope FROM oauth2_provider_apps WHERE id = $1`, app.id), app.name)
		}
		for _, key := range keys {
			var scopes, allowList string
			err := sqlDB.QueryRowContext(ctx, `SELECT scopes::text, allow_list::text FROM api_keys WHERE id = $1`, key.id).Scan(&scopes, &allowList)
			if key.wantDeleted {
				require.ErrorIs(t, err, sql.ErrNoRows, key.id)
				continue
			}
			require.NoError(t, err, key.id)
			require.Equal(t, key.wantScopes, scopes, key.id)
			require.Equal(t, key.wantAllowList, allowList, key.id)
		}
		for _, grant := range grants {
			for _, table := range []struct{ name, prefixColumn string }{
				{"oauth2_provider_app_codes", "secret_prefix"},
				{"oauth2_provider_app_tokens", "hash_prefix"},
			} {
				got := queryStrings(`SELECT scope FROM `+table.name+` WHERE `+table.prefixColumn+` = $1`, []byte(grant.key))
				if grant.wantDeleted {
					require.Empty(t, got, "%s %s", table.name, grant.key)
					continue
				}
				require.Equal(t, []string{grant.wantScope}, got, "%s %s", table.name, grant.key)
			}
		}
	}
	assertSchemaRemoved := func(t *testing.T, removed bool) {
		t.Helper()
		var tables, columns, scopes int
		require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE tablename = 'chat_automations'`).Scan(&tables))
		require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_name IN ('chats', 'chats_expanded', 'chat_messages', 'chat_queued_messages')
			AND column_name IN ('automation_id', 'manage_automations_enabled', 'input_id', 'queue_generation')`).Scan(&columns))
		require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM unnest(enum_range(NULL::api_key_scope)) s WHERE s::text LIKE 'chat\_automation:%'`).Scan(&scopes))
		if removed {
			require.Zero(t, tables+columns+scopes)
			return
		}
		// One table, 9 provenance and switch columns (2 of them on
		// chats_expanded), and 5 scopes.
		require.Equal(t, []int{1, 9, 5}, []int{tables, columns, scopes})
	}
	assertMigrated(t)
	assertSchemaRemoved(t, true)

	// The down migrations restore only the schema: removed data and
	// credentials stay removed. The up migrations then apply again.
	for _, file := range []string{
		"000615_remove_chat_automation_permissions.down.sql",
		"000614_remove_chat_automations.down.sql",
	} {
		query, err := os.ReadFile(file)
		require.NoError(t, err)
		exec(string(query))
	}
	assertSchemaRemoved(t, false)
	require.Empty(t, queryStrings(`SELECT id::text FROM chat_automations`))
	for _, file := range []string{
		"000614_remove_chat_automations.up.sql",
		"000615_remove_chat_automation_permissions.up.sql",
	} {
		query, err := os.ReadFile(file)
		require.NoError(t, err)
		exec(string(query))
	}
	assertSchemaRemoved(t, true)
	assertMigrated(t)
}
