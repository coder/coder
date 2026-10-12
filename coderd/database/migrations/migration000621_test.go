package migrations_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestMigration000621ChatAutomationLastRun checks that existing
// automations start without a last run and that the shape constraint
// keeps the last-run columns together.
func TestMigration000621ChatAutomationLastRun(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 620)
	ctx := testutil.Context(t, testutil.WaitLong)
	orgID, userID, modelID, automationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-621', 'org-621', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'user621@example.com', 'user621', '\x', now(), now())`, []any{userID}},
		// Deleted, so the model needs no AI provider.
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-621', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, target_mode, new_chat_model_config_id, prompt, schedule_cron, schedule_time_zone)
			VALUES ($1, $2, $3, 'schedule', 'schedule', 'new_chat', $4, 'prompt', '0 9 * * *', 'UTC')`, []any{automationID, orgID, userID, modelID}},
	} {
		_, err := sqlDB.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}

	stepTo(t, sqlDB, 621)

	var (
		at      sql.NullTime
		outcome sql.NullString
		reason  sql.NullString
		chatID  uuid.NullUUID
	)
	err := sqlDB.QueryRowContext(ctx, `SELECT last_run_at, last_run_outcome, last_run_reason, last_run_chat_id FROM chat_automations WHERE id = $1`, automationID).
		Scan(&at, &outcome, &reason, &chatID)
	require.NoError(t, err)
	require.False(t, at.Valid)
	require.False(t, outcome.Valid)
	require.False(t, reason.Valid)
	require.False(t, chatID.Valid)

	// An outcome without a time, or a reason without a run, is rejected.
	for _, bad := range []string{
		`UPDATE chat_automations SET last_run_outcome = 'accepted' WHERE id = $1`,
		`UPDATE chat_automations SET last_run_reason = 'chat_busy' WHERE id = $1`,
	} {
		_, err = sqlDB.ExecContext(ctx, bad, automationID)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr, bad)
		require.Equal(t, "chat_automations_last_run_shape", pqErr.Constraint, bad)
	}
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET last_run_at = now(), last_run_outcome = 'skipped', last_run_reason = 'chat_busy' WHERE id = $1`, automationID)
	require.NoError(t, err)
}
