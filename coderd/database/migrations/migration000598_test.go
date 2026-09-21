package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// chatKindRow is the subset of a chats row the 000598 assertions inspect.
type chatKindRow struct {
	kind     string
	parentID sql.NullString
	rootID   sql.NullString
	pinOrder int32
	userACL  string
	archived bool
}

func readChatKindRow(t *testing.T, tx *sql.Tx, id string) chatKindRow {
	t.Helper()

	var row chatKindRow
	err := tx.QueryRow(`
		SELECT kind::text, parent_chat_id::text, root_chat_id::text, pin_order, user_acl::text, archived
		FROM chats WHERE id = $1::uuid
	`, id).Scan(&row.kind, &row.parentID, &row.rootID, &row.pinOrder, &row.userACL, &row.archived)
	require.NoError(t, err)
	return row
}

// TestMigration000598ChatKind covers the kind backfill from parent_chat_id,
// the rewritten constraints, and the down migration. The testdata/fixtures
// run does not exercise the backfill because no fixture chat has a parent.
//
//nolint:tparallel,paralleltest // Subtests share one database with transaction-local fixtures.
func TestMigration000598ChatKind(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 597)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	upSQL, err := os.ReadFile("000598_chat_kind.up.sql")
	require.NoError(t, err)
	downSQL, err := os.ReadFile("000598_chat_kind.down.sql")
	require.NoError(t, err)

	const (
		ownerID     = "3f7d6c1e-0000-4000-8000-000000000001"
		orgID       = "3f7d6c1e-0000-4000-8000-000000000002"
		providerID  = "3f7d6c1e-0000-4000-8000-000000000003"
		modelCfgID  = "3f7d6c1e-0000-4000-8000-000000000004"
		parentless  = "3f7d6c1e-0000-4000-8000-000000000010"
		subagent    = "3f7d6c1e-0000-4000-8000-000000000011"
		sharedChat  = "3f7d6c1e-0000-4000-8000-000000000012"
		pinnedChat  = "3f7d6c1e-0000-4000-8000-000000000013"
		archivedSub = "3f7d6c1e-0000-4000-8000-000000000014"
	)

	// seed inserts an owner, organization, model config, and four chats:
	// a parentless chat, a subagent whose root_chat_id is NULL (the
	// pre-root_chat_id shape), a shared parentless chat, a pinned chat,
	// and an archived subagent with root_chat_id set.
	seed := func(t *testing.T, tx *sql.Tx) {
		t.Helper()

		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1::uuid, 'chat-kind@example.com', 'chat-kind', ''::bytea, NOW(), NOW())
		`, ownerID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO organizations (id, name, description, display_name, created_at, updated_at, default_org_member_roles)
			VALUES ($1::uuid, 'chat-kind-org', '', 'Chat Kind Org', NOW(), NOW(), '{}')
		`, orgID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ai_providers (id, type, name, base_url)
			VALUES ($1::uuid, 'openai', 'chat-kind-provider', 'https://example.com')
		`, providerID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_model_configs (id, model, context_limit, compression_threshold, ai_provider_id, organization_id)
			VALUES ($1::uuid, 'gpt-test', 1000, 50, $2::uuid, $3::uuid)
		`, modelCfgID, providerID, orgID)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title, parent_chat_id, root_chat_id, pin_order, user_acl, archived)
			VALUES
				($5::uuid, $1::uuid, $2::uuid, $3::uuid, 'parentless', NULL, NULL, 0, '{}', false),
				($6::uuid, $1::uuid, $2::uuid, $3::uuid, 'subagent', $5::uuid, NULL, 0, '{}', false),
				($7::uuid, $1::uuid, $2::uuid, $3::uuid, 'shared', NULL, NULL, 0, $4::jsonb, false),
				($8::uuid, $1::uuid, $2::uuid, $3::uuid, 'pinned', NULL, NULL, 3, '{}', false),
				($9::uuid, $1::uuid, $2::uuid, $3::uuid, 'archived subagent', $7::uuid, $7::uuid, 0, '{}', true)
		`, ownerID, orgID, modelCfgID, `{"`+ownerID+`": "viewer"}`, parentless, subagent, sharedChat, pinnedChat, archivedSub)
		require.NoError(t, err)
	}

	t.Run("up backfills kind and root_chat_id", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		seed(t, tx)
		_, err = tx.ExecContext(ctx, string(upSQL))
		require.NoError(t, err)

		require.Equal(t, "chat", readChatKindRow(t, tx, parentless).kind)
		require.Equal(t, "chat", readChatKindRow(t, tx, sharedChat).kind)
		pinned := readChatKindRow(t, tx, pinnedChat)
		require.Equal(t, "chat", pinned.kind)
		require.EqualValues(t, 3, pinned.pinOrder)

		sub := readChatKindRow(t, tx, subagent)
		require.Equal(t, "subagent", sub.kind)
		require.Equal(t, parentless, sub.rootID.String, "root_chat_id backfilled from parent_chat_id")

		archived := readChatKindRow(t, tx, archivedSub)
		require.Equal(t, "subagent", archived.kind)
		require.Equal(t, sharedChat, archived.rootID.String)
		require.True(t, archived.archived)

		// chats_expanded exposes kind and still inherits the parent's ACL.
		var expandedKind, expandedACL string
		err = tx.QueryRow(`SELECT kind::text, user_acl::text FROM chats_expanded WHERE id = $1::uuid`, archivedSub).
			Scan(&expandedKind, &expandedACL)
		require.NoError(t, err)
		require.Equal(t, "subagent", expandedKind)
		require.Contains(t, expandedACL, ownerID)
	})

	t.Run("constraints", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		seed(t, tx)
		_, err = tx.ExecContext(ctx, string(upSQL))
		require.NoError(t, err)

		withParent := []any{ownerID, orgID, modelCfgID, parentless}
		violations := []struct {
			name       string
			sql        string
			args       []any
			constraint string
		}{
			{
				name:       "subagent without root_chat_id",
				sql:        `INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, parent_chat_id) VALUES ($1::uuid, $2::uuid, $3::uuid, 'subagent', $4::uuid)`,
				args:       withParent,
				constraint: "chats_kind_subagent_root_check",
			},
			{
				name:       "chat with root_chat_id",
				sql:        `INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, parent_chat_id, root_chat_id) VALUES ($1::uuid, $2::uuid, $3::uuid, 'chat', $4::uuid, $4::uuid)`,
				args:       withParent,
				constraint: "chats_kind_subagent_root_check",
			},
			{
				name:       "root with parent",
				sql:        `INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, parent_chat_id) VALUES ($1::uuid, $2::uuid, $3::uuid, 'root', $4::uuid)`,
				args:       withParent,
				constraint: "chats_kind_root_parentless_check",
			},
			{
				name:       "pinned root",
				sql:        `INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, pin_order) VALUES ($1::uuid, $2::uuid, $3::uuid, 'root', 1)`,
				args:       []any{ownerID, orgID, modelCfgID},
				constraint: "chats_pin_order_parent_check",
			},
			{
				name:       "subagent with acl",
				sql:        `INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, parent_chat_id, root_chat_id, user_acl) VALUES ($1::uuid, $2::uuid, $3::uuid, 'subagent', $4::uuid, $4::uuid, '{"x": "viewer"}')`,
				args:       withParent,
				constraint: "chat_acl_only_on_root_chats",
			},
		}
		for _, v := range violations {
			_, err = tx.ExecContext(ctx, `SAVEPOINT violation`)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, v.sql, v.args...)
			require.ErrorContains(t, err, v.constraint, v.name)
			_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT violation`)
			require.NoError(t, err)
		}

		// A named child carries a parent without becoming a subagent, and
		// only one root exists per owner and organization.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, parent_chat_id, pin_order)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'chat', $4::uuid, 1)
		`, ownerID, orgID, modelCfgID, parentless)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, title)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'root', 'Root')
		`, ownerID, orgID, modelCfgID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (owner_id, organization_id, last_model_config_id, kind, title)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'root', 'Root')
		`, ownerID, orgID, modelCfgID)
		require.ErrorContains(t, err, "chats_one_tree_root_per_owner_org")
	})

	t.Run("down detaches named children and removes roots", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		seed(t, tx)
		_, err = tx.ExecContext(ctx, string(upSQL))
		require.NoError(t, err)

		const (
			rootID         = "3f7d6c1e-0000-4000-8000-000000000020"
			childID        = "3f7d6c1e-0000-4000-8000-000000000021"
			rootSubagentID = "3f7d6c1e-0000-4000-8000-000000000022"
		)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, kind, title)
			VALUES ($4::uuid, $1::uuid, $2::uuid, $3::uuid, 'root', 'Root')
		`, ownerID, orgID, modelCfgID, rootID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, kind, parent_chat_id, title)
			VALUES ($5::uuid, $1::uuid, $2::uuid, $3::uuid, 'chat', $4::uuid, 'child')
		`, ownerID, orgID, modelCfgID, rootID, childID)
		require.NoError(t, err)
		// A subagent spawned by the root goes away with the root.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, kind, parent_chat_id, root_chat_id, title)
			VALUES ($5::uuid, $1::uuid, $2::uuid, $3::uuid, 'subagent', $4::uuid, $4::uuid, 'root subagent')
		`, ownerID, orgID, modelCfgID, rootID, rootSubagentID)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, string(downSQL))
		require.NoError(t, err)

		var count int
		require.NoError(t, tx.QueryRow(`SELECT COUNT(*) FROM chats WHERE id IN ($1::uuid, $2::uuid)`, rootID, rootSubagentID).Scan(&count))
		require.Zero(t, count, "root and its subagent removed")

		var childParent, subParent, subRoot sql.NullString
		require.NoError(t, tx.QueryRow(`SELECT parent_chat_id::text FROM chats WHERE id = $1::uuid`, childID).Scan(&childParent))
		require.False(t, childParent.Valid, "named child detached")
		require.NoError(t, tx.QueryRow(`SELECT parent_chat_id::text, root_chat_id::text FROM chats WHERE id = $1::uuid`, subagent).Scan(&subParent, &subRoot))
		require.Equal(t, parentless, subParent.String, "subagent keeps its parent")
		require.Equal(t, parentless, subRoot.String)

		var hasKind bool
		require.NoError(t, tx.QueryRow(`
			SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'chats' AND column_name = 'kind')
		`).Scan(&hasKind))
		require.False(t, hasKind)

		var expanded int
		require.NoError(t, tx.QueryRow(`SELECT COUNT(*) FROM chats_expanded`).Scan(&expanded))
		require.Equal(t, 6, expanded)
	})
}
