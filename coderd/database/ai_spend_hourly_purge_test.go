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
		lockedIDs, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: cutoff, LimitCount: 10000})
		if err != nil {
			return err
		}
		deleted, err := tx.DeleteOldAIBridgeRecords(ctx, lockedIDs)
		if err != nil {
			return err
		}
		require.EqualValues(t, 4, deleted.TotalDeleted)
		return tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx, deleted.EmptyHourlyIds)
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
	var deleted database.DeleteOldAIBridgeRecordsRow
	err = db.InTx(func(tx database.Store) error {
		lockedIDs, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: cutoff, LimitCount: 10000})
		if err != nil {
			return err
		}
		deleted, err = tx.DeleteOldAIBridgeRecords(ctx, lockedIDs)
		return err
	}, nil)
	require.NoError(t, err)
	require.Zero(t, deleted.TotalDeleted)
	require.NoError(t, db.DeleteGroupByID(ctx, group.ID))
	err = db.InTx(func(tx database.Store) error {
		lockedIDs, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: cutoff.Add(time.Second), LimitCount: 10000})
		if err != nil {
			return err
		}
		result, err := tx.DeleteOldAIBridgeRecords(ctx, lockedIDs)
		if err != nil {
			return err
		}
		return tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx, result.EmptyHourlyIds)
	}, nil)
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly").Scan(&groups))
	require.Zero(t, groups)
}

func TestDeleteOldAIBridgeRecordsExcludesNewlyInsertedInterceptions(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	user := dbgen.User(t, db, database.User{})
	cutoff := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	old := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, StartedAt: cutoff.Add(-time.Hour),
	}, nil)
	addedID := uuid.New()
	require.NoError(t, db.InTx(func(tx database.Store) error {
		ids, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: cutoff, LimitCount: 10000})
		if err != nil {
			return err
		}
		require.Equal(t, []uuid.UUID{old.ID}, ids)
		// Use a separate connection to insert an expired row after lock selection.
		_, err = sqlDB.ExecContext(ctx,
			"INSERT INTO aibridge_interceptions (id, initiator_id, provider, model, started_at) VALUES ($1, $2, $3, $4, $5)",
			addedID, user.ID, "wire", "model", cutoff.Add(-time.Hour),
		)
		if err != nil {
			return err
		}
		_, err = tx.DeleteOldAIBridgeRecords(ctx, ids)
		return err
	}, nil))
	_, err := db.GetAIBridgeInterceptionByID(ctx, old.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.GetAIBridgeInterceptionByID(ctx, addedID)
	require.NoError(t, err)
}
