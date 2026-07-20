package migrations_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000548ChatModelConfigCoexistence(t *testing.T) {
	t.Parallel()

	const priorMigrationVersion = 547

	sqlDB := testSQLDB(t)
	next, err := migrations.Stepper(sqlDB)
	require.NoError(t, err)
	for {
		version, more, err := next()
		require.NoError(t, err)
		if !more {
			t.Fatalf("migration %d not found", priorMigrationVersion)
		}
		if version == priorMigrationVersion {
			break
		}
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	activeOrgID := uuid.New()
	secondOrgID := uuid.New()
	deletedOrgID := uuid.New()
	globalDefaultID := uuid.New()
	globalDeletedID := uuid.New()
	providerID := uuid.New()
	userID := uuid.New()
	chatID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO organizations (
			id, name, description, created_at, updated_at, is_default,
			display_name, icon, deleted, default_org_member_roles
		) VALUES
			($1, 'coexistence-active', '', $4, $4, false, 'Coexistence Active', '', false, '{}'),
			($2, 'coexistence-second', '', $4, $4, false, 'Coexistence Second', '', false, '{}'),
			($3, 'coexistence-deleted', '', $4, $4, false, 'Coexistence Deleted', '', true, '{}')
	`, activeOrgID, secondOrgID, deletedOrgID, now)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO users (
			id, email, username, hashed_password, created_at, updated_at,
			status, rbac_roles, login_type
		) VALUES ($1, 'coexistence@example.com', 'coexistence-user', '', $2, $2, 'active', '{}', 'password')
	`, userID, now)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO ai_providers (
			id, type, name, display_name, enabled, deleted, base_url, settings
		) VALUES ($1, 'openai', 'coexistence-provider', 'Coexistence Provider', true, false, 'https://example.com/', '')
	`, providerID)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_model_configs (
			id, model, display_name, enabled, is_default, deleted, deleted_at,
			context_limit, compression_threshold, options, ai_provider_id,
			created_at, updated_at
		) VALUES
			($1, 'coexistence-default', 'Coexistence Default', true, true, false, null, 200000, 70, '{}', $3, $4, $4),
			($2, 'coexistence-deleted', 'Coexistence Deleted', false, false, true, $4, 200000, 70, '{}', null, $4, $4)
	`, globalDefaultID, globalDeletedID, providerID, now)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chats (
			id, owner_id, title, status, created_at, updated_at,
			last_model_config_id, organization_id
		) VALUES ($1, $2, 'Coexistence Chat', 'waiting', $3, $3, $4, $5);
	`, chatID, userID, now, globalDefaultID, activeOrgID)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (
			chat_id, model_config_id, created_at, role, content, visibility,
			created_by, content_version
		) VALUES ($1, $2, $3, 'assistant', '{"type":"text","text":"fixture"}', 'both', $4, 1)
	`, chatID, globalDefaultID, now, userID)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_queued_messages (
			chat_id, content, created_at, model_config_id, created_by
		) VALUES ($1, '{"type":"text","text":"queued fixture"}', $2, $3, $4)
	`, chatID, now, globalDefaultID, userID)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_debug_runs (
			chat_id, model_config_id, kind, status, started_at, updated_at
		) VALUES ($1, $2, 'generation', 'completed', $3, $3)
	`, chatID, globalDefaultID, now)
	require.NoError(t, err)

	upSQL, err := os.ReadFile("000548_chat_model_config_coexistence.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(upSQL))
	require.NoError(t, err)

	var globalCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE organization_id IS NULL
			AND id = ANY($1)
	`, pq.Array([]uuid.UUID{globalDefaultID, globalDeletedID})).Scan(&globalCount)
	require.NoError(t, err)
	require.Equal(t, 2, globalCount)

	var unchangedReferences int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT
			(chats.last_model_config_id = $2)::integer
			+ (chat_messages.model_config_id = $2)::integer
			+ (chat_queued_messages.model_config_id = $2)::integer
			+ (chat_debug_runs.model_config_id = $2)::integer
		FROM chats
		JOIN chat_messages ON chat_messages.chat_id = chats.id
		JOIN chat_queued_messages ON chat_queued_messages.chat_id = chats.id
		JOIN chat_debug_runs ON chat_debug_runs.chat_id = chats.id
		WHERE chats.id = $1
	`, chatID, globalDefaultID).Scan(&unchangedReferences)
	require.NoError(t, err)
	require.Equal(t, 4, unchangedReferences)

	type copyRow struct {
		id                   uuid.UUID
		organizationID       uuid.UUID
		legacyModelConfigID  uuid.UUID
		deleted              bool
		isDefault            bool
		inheritsLegacyConfig bool
		userACL              []byte
		groupACL             []byte
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT
			id, organization_id, legacy_model_config_id, deleted, is_default,
			inherits_legacy_config, user_acl, group_acl
		FROM chat_model_configs
		WHERE legacy_model_config_id = ANY($1)
			AND organization_id = ANY($2)
		ORDER BY organization_id, legacy_model_config_id
	`,
		pq.Array([]uuid.UUID{globalDefaultID, globalDeletedID}),
		pq.Array([]uuid.UUID{activeOrgID, secondOrgID}),
	)
	require.NoError(t, err)
	defer rows.Close()

	var copies []copyRow
	for rows.Next() {
		var row copyRow
		require.NoError(t, rows.Scan(
			&row.id,
			&row.organizationID,
			&row.legacyModelConfigID,
			&row.deleted,
			&row.isDefault,
			&row.inheritsLegacyConfig,
			&row.userACL,
			&row.groupACL,
		))
		copies = append(copies, row)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, copies, 4)

	seen := make(map[[2]uuid.UUID]bool)
	for _, copy := range copies {
		require.NotEqual(t, copy.legacyModelConfigID, copy.id)
		require.Contains(t, []uuid.UUID{activeOrgID, secondOrgID}, copy.organizationID)
		require.True(t, copy.inheritsLegacyConfig)
		require.JSONEq(t, `{}`, string(copy.userACL))
		require.JSONEq(t,
			fmt.Sprintf(`{%q:["read"]}`, copy.organizationID.String()),
			string(copy.groupACL),
		)
		if copy.legacyModelConfigID == globalDefaultID {
			require.False(t, copy.deleted)
			require.True(t, copy.isDefault)
		} else {
			require.Equal(t, globalDeletedID, copy.legacyModelConfigID)
			require.True(t, copy.deleted)
			require.False(t, copy.isDefault)
		}
		seen[[2]uuid.UUID{copy.organizationID, copy.legacyModelConfigID}] = true
	}
	require.Len(t, seen, 4)

	var deletedOrgCopies int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM chat_model_configs WHERE organization_id = $1
	`, deletedOrgID).Scan(&deletedOrgCopies)
	require.NoError(t, err)
	require.Zero(t, deletedOrgCopies)

	var inheritanceRows int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_config_org_default_inheritance
		WHERE organization_id = ANY($1)
			AND inherits_legacy_default = true
	`, pq.Array([]uuid.UUID{activeOrgID, secondOrgID})).Scan(&inheritanceRows)
	require.NoError(t, err)
	require.Equal(t, 2, inheritanceRows)

	assertConstraintViolation := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, pq.ErrorCode("23514"), pqErr.Code)
	}
	assertUniqueViolation := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, pq.ErrorCode("23505"), pqErr.Code)
	}

	insertRowSQL := `
		INSERT INTO chat_model_configs (
			id, model, display_name, enabled, is_default, deleted,
			context_limit, compression_threshold, options, ai_provider_id,
			organization_id, legacy_model_config_id, inherits_legacy_config
		) VALUES ($1, $2, $2, true, $3, false, 200000, 70, '{}', $4, $5, $6, $7)
	`
	assertConstraintViolation(insertRowSQL,
		uuid.New(), "invalid-global-lineage", false, providerID, nil, globalDefaultID, false,
	)
	assertConstraintViolation(insertRowSQL,
		uuid.New(), "invalid-native-inheritance", false, providerID, activeOrgID, nil, true,
	)
	assertUniqueViolation(insertRowSQL,
		uuid.New(), "duplicate-lineage", false, providerID, activeOrgID, globalDefaultID, true,
	)
	assertUniqueViolation(insertRowSQL,
		uuid.New(), "second-global-default", true, providerID, nil, nil, false,
	)
	assertUniqueViolation(insertRowSQL,
		uuid.New(), "second-org-default", true, providerID, activeOrgID, nil, false,
	)

	thirdOrgID := uuid.New()
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO organizations (
			id, name, description, created_at, updated_at,
			display_name, icon, deleted, default_org_member_roles
		) VALUES ($1, 'coexistence-third', '', $2, $2, 'Coexistence Third', '', false, '{}')
	`, thirdOrgID, now)
	require.NoError(t, err)

	var thirdOrgCopies int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE organization_id = $1
			AND legacy_model_config_id = $2
			AND inherits_legacy_config = true
			AND group_acl = jsonb_build_object($1::text, jsonb_build_array('read'))
	`, thirdOrgID, globalDefaultID).Scan(&thirdOrgCopies)
	require.NoError(t, err)
	require.Equal(t, 1, thirdOrgCopies)

	var thirdOrgDeletedCopies int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE organization_id = $1
			AND legacy_model_config_id = $2
	`, thirdOrgID, globalDeletedID).Scan(&thirdOrgDeletedCopies)
	require.NoError(t, err)
	require.Zero(t, thirdOrgDeletedCopies)

	var thirdOrgInheritance bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT inherits_legacy_default
		FROM chat_model_config_org_default_inheritance
		WHERE organization_id = $1
	`, thirdOrgID).Scan(&thirdOrgInheritance)
	require.NoError(t, err)
	require.True(t, thirdOrgInheritance)

	downSQL, err := os.ReadFile("000548_chat_model_config_coexistence.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)

	var remainingGlobalRows int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE id = ANY($1)
	`, pq.Array([]uuid.UUID{globalDefaultID, globalDeletedID})).Scan(&remainingGlobalRows)
	require.NoError(t, err)
	require.Equal(t, 2, remainingGlobalRows)

	var coexistenceColumns int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
			AND table_name = 'chat_model_configs'
			AND column_name = ANY($1)
	`, pq.Array([]string{
		"organization_id",
		"user_acl",
		"group_acl",
		"legacy_model_config_id",
		"inherits_legacy_config",
	})).Scan(&coexistenceColumns)
	require.NoError(t, err)
	require.Zero(t, coexistenceColumns)

	var legacyDefaultIndex int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = current_schema()
			AND indexname = 'idx_chat_model_configs_single_default'
	`).Scan(&legacyDefaultIndex)
	require.NoError(t, err)
	require.Equal(t, 1, legacyDefaultIndex)
}
