package migrations_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000634ChatModelOverrideComputerUse(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 633)
	ctx := testutil.Context(t, testutil.WaitLong)

	upSQL, err := os.ReadFile("000634_chat_model_override_computer_use.up.sql")
	require.NoError(t, err)
	downSQL, err := os.ReadFile("000634_chat_model_override_computer_use.down.sql")
	require.NoError(t, err)

	var orgID uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT id FROM organizations WHERE is_default = true").Scan(&orgID))
	providerID, modelID := uuid.New(), uuid.New()
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO ai_providers (id, type, name, enabled, base_url, created_at, updated_at)
		VALUES ($1, 'anthropic', $2, true, 'https://example.com', now(), now())`,
		providerID, "computer-use-"+providerID.String())
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_model_configs (
			id, model, display_name, enabled, is_default, deleted,
			context_limit, compression_threshold, ai_provider_id, organization_id,
			group_acl, user_acl, created_at, updated_at
		) VALUES ($1, 'claude-opus-5', 'Claude', true, false, false, 128000, 70, $2, $3, '{}', '{}', now(), now())`,
		modelID, providerID, orgID)
	require.NoError(t, err)

	insertOverride := func(overrideContext string) error {
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO chat_organization_model_overrides (organization_id, context, model_config_id)
			VALUES ($1, $2, $3)`, orgID, overrideContext, modelID)
		return err
	}
	requireCheckViolation := func(err error) {
		t.Helper()
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckChatOrganizationModelOverridesContextCheck), err)
	}

	// Before the migration, computer_use is not a valid context.
	requireCheckViolation(insertOverride("computer_use"))
	require.NoError(t, insertOverride("advisor"))

	_, err = sqlDB.ExecContext(ctx, string(upSQL))
	require.NoError(t, err)
	require.NoError(t, insertOverride("computer_use"))
	require.ElementsMatch(t, []string{"advisor", "computer_use"}, overrideContexts(ctx, t, sqlDB, orgID))

	// Rolling back drops computer_use rows, keeps the others, and restores
	// the old constraint.
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)
	require.Equal(t, []string{"advisor"}, overrideContexts(ctx, t, sqlDB, orgID))
	requireCheckViolation(insertOverride("computer_use"))
}

func overrideContexts(ctx context.Context, t *testing.T, sqlDB *sql.DB, orgID uuid.UUID) []string {
	t.Helper()
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT context FROM chat_organization_model_overrides WHERE organization_id = $1", orgID)
	require.NoError(t, err)
	defer rows.Close()
	var contexts []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		contexts = append(contexts, c)
	}
	require.NoError(t, rows.Err())
	return contexts
}
