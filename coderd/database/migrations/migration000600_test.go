package migrations_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

func testMigration000600AIBridgeTokenUsageHourly(t *testing.T, sqlDB *sql.DB, next migrationStepper) {
	ctx := testutil.Context(t, testutil.WaitLong)
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	user := dbgen.User(t, db, database.User{})
	at := time.Date(2026, 3, 10, 9, 45, 0, 0, time.UTC)
	intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: at.Add(-24 * time.Hour),
	}, nil)
	for _, cost := range []sql.NullInt64{{Int64: 100, Valid: true}, {Int64: 0, Valid: true}, {}} {
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID, EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			CreatedAt: at, CostMicros: cost,
		})
	}
	other := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: at,
	}, nil)
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: other.ID, CreatedAt: at, CostMicros: sql.NullInt64{Int64: 500, Valid: true},
	})

	version, more, err := next()
	require.NoError(t, err)
	require.True(t, more)
	require.EqualValues(t, 600, version)

	check := func() {
		t.Helper()
		var orgID, groupID, userID uuid.UUID
		var hour time.Time
		var provider, providerName, model, client string
		var cost, unpriced, count int64
		err := sqlDB.QueryRowContext(ctx,
			"SELECT organization_id, effective_group_id, initiator_id, hour, provider, provider_name, model, client, cost_micros, unpriced_usage_count, usage_count FROM aibridge_token_usage_hourly",
		).Scan(&orgID, &groupID, &userID, &hour, &provider, &providerName, &model, &client, &cost, &unpriced, &count)
		require.NoError(t, err)
		require.Equal(t, org.ID, orgID)
		require.Equal(t, group.ID, groupID)
		require.Equal(t, user.ID, userID)
		require.True(t, at.Truncate(time.Hour).Equal(hour))
		require.Equal(t, "wire", provider)
		require.Equal(t, "configured", providerName)
		require.Equal(t, "model", model)
		require.Equal(t, "Unknown", client)
		require.EqualValues(t, 100, cost)
		require.EqualValues(t, 1, unpriced)
		require.EqualValues(t, 3, count)
	}
	check()

	down, err := os.ReadFile("000600_aibridge_token_usage_hourly.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(down))
	require.NoError(t, err)
	var absent bool
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT to_regclass('aibridge_token_usage_hourly') IS NULL").Scan(&absent))
	require.True(t, absent)
	up, err := os.ReadFile("000600_aibridge_token_usage_hourly.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(up))
	require.NoError(t, err)
	check()
}
