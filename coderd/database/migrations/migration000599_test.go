package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestMigration000599ChatTreeCascade covers the switch of the chat parent
// and root foreign keys from SET NULL to CASCADE and the down migration.
//
//nolint:tparallel,paralleltest // Subtests share one database with transaction-local fixtures.
func TestMigration000599ChatTreeCascade(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 598)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	upSQL, err := os.ReadFile("000599_chat_tree_cascade.up.sql")
	require.NoError(t, err)
	downSQL, err := os.ReadFile("000599_chat_tree_cascade.down.sql")
	require.NoError(t, err)

	const (
		ownerID    = "4a8e7d2f-0000-4000-8000-000000000001"
		orgID      = "4a8e7d2f-0000-4000-8000-000000000002"
		providerID = "4a8e7d2f-0000-4000-8000-000000000003"
		modelCfgID = "4a8e7d2f-0000-4000-8000-000000000004"
		parentID   = "4a8e7d2f-0000-4000-8000-000000000010"
		childID    = "4a8e7d2f-0000-4000-8000-000000000011"
		subagentID = "4a8e7d2f-0000-4000-8000-000000000012"
	)

	seed := func(t *testing.T, tx *sql.Tx) {
		t.Helper()

		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1::uuid, 'chat-tree@example.com', 'chat-tree', ''::bytea, NOW(), NOW())
		`, ownerID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO organizations (id, name, description, display_name, created_at, updated_at, default_org_member_roles)
			VALUES ($1::uuid, 'chat-tree-org', '', 'Chat Tree Org', NOW(), NOW(), '{}')
		`, orgID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ai_providers (id, type, name, base_url)
			VALUES ($1::uuid, 'openai', 'chat-tree-provider', 'https://example.com')
		`, providerID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_model_configs (id, model, context_limit, compression_threshold, ai_provider_id, organization_id)
			VALUES ($1::uuid, 'gpt-test', 1000, 50, $2::uuid, $3::uuid)
		`, modelCfgID, providerID, orgID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, kind, title, parent_chat_id, root_chat_id)
			VALUES
				($4::uuid, $1::uuid, $2::uuid, $3::uuid, 'chat', 'parent', NULL, NULL),
				($5::uuid, $1::uuid, $2::uuid, $3::uuid, 'chat', 'child', $4::uuid, NULL),
				($6::uuid, $1::uuid, $2::uuid, $3::uuid, 'subagent', 'subagent', $5::uuid, $5::uuid)
		`, ownerID, orgID, modelCfgID, parentID, childID, subagentID)
		require.NoError(t, err)
	}
	count := func(t *testing.T, tx *sql.Tx) int {
		t.Helper()
		var n int
		require.NoError(t, tx.QueryRow(`SELECT COUNT(*) FROM chats`).Scan(&n))
		return n
	}

	t.Run("up cascades deletes", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		seed(t, tx)
		_, err = tx.ExecContext(ctx, string(upSQL))
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, `DELETE FROM chats WHERE id = $1::uuid`, parentID)
		require.NoError(t, err)
		require.Zero(t, count(t, tx), "child and subagent follow the parent")
	})

	t.Run("down restores set null", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		seed(t, tx)
		_, err = tx.ExecContext(ctx, string(upSQL))
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(downSQL))
		require.NoError(t, err)

		// Deleting the child leaves the subagent with NULL pointers, which
		// the kind constraints then reject; delete the parent instead so
		// only the named child is affected.
		_, err = tx.ExecContext(ctx, `DELETE FROM chats WHERE id = $1::uuid`, parentID)
		require.NoError(t, err)
		require.Equal(t, 2, count(t, tx))
		var childParent sql.NullString
		require.NoError(t, tx.QueryRow(`SELECT parent_chat_id::text FROM chats WHERE id = $1::uuid`, childID).Scan(&childParent))
		require.False(t, childParent.Valid)
	})
}
