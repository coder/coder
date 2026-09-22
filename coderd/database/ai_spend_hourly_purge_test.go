package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestDeleteOldAIBridgeRecordsHourly(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	user := dbgen.User(t, db, database.User{})
	at := time.Date(2030, 1, 1, 11, 30, 0, 0, time.UTC)
	cutoff := at.Add(-24 * time.Hour)
	old := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: cutoff.Add(-time.Minute),
	}, nil)
	recent := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: cutoff,
	}, nil)
	add := func(interceptionID uuid.UUID, when time.Time, cost sql.NullInt64) {
		t.Helper()
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: interceptionID, CreatedAt: when,
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true}, CostMicros: cost,
		})
		require.NoError(t, db.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
			CreatedAt: when, EffectiveGroupID: group.ID, InitiatorID: user.ID,
			Provider: "wire", ProviderName: "configured", Model: "model", CostMicros: cost,
		}))
	}
	add(old.ID, at, sql.NullInt64{Int64: 100, Valid: true})
	add(old.ID, at, sql.NullInt64{})
	add(old.ID, at.Add(-time.Hour), sql.NullInt64{Int64: 20, Valid: true})
	add(recent.ID, at, sql.NullInt64{Int64: 300, Valid: true})

	err := db.InTx(func(tx database.Store) error {
		deleted, err := tx.DeleteOldAIBridgeRecords(ctx, cutoff)
		if err != nil {
			return err
		}
		require.EqualValues(t, 4, deleted)
		return tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx)
	}, nil)
	require.NoError(t, err)

	var cost, unpriced, count int64
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT cost_micros, unpriced_usage_count, usage_count FROM aibridge_token_usage_hourly",
	).Scan(&cost, &unpriced, &count))
	require.EqualValues(t, 300, cost)
	require.Zero(t, unpriced)
	require.EqualValues(t, 1, count)
	var groups int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly").Scan(&groups))
	require.Equal(t, 1, groups)
	deleted, err := db.DeleteOldAIBridgeRecords(ctx, cutoff)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.NoError(t, db.DeleteGroupByID(ctx, group.ID))
	err = db.InTx(func(tx database.Store) error {
		if _, err := tx.DeleteOldAIBridgeRecords(ctx, cutoff.Add(time.Second)); err != nil {
			return err
		}
		return tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx)
	}, nil)
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly").Scan(&groups))
	require.Zero(t, groups)
}
