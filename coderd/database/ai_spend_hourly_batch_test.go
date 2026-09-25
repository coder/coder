package database_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgePurgeCappedBatches(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	user := dbgen.User(t, db, database.User{})
	cutoff := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	sharedHour := cutoff.Add(2 * time.Hour)
	olderHour := cutoff.Add(time.Hour)
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.New(), uuid.New(), uuid.New(),
	}
	startedAt := []time.Time{cutoff.Add(-4 * time.Hour), cutoff.Add(-4 * time.Hour), cutoff.Add(-2 * time.Hour), cutoff.Add(-time.Hour), cutoff}
	for i, id := range ids {
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID: id, InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: startedAt[i],
		}, nil)
		at := sharedHour
		if i == 2 {
			at = olderHour
		}
		cost := int64(10)
		if i == 4 {
			cost = 50
		}
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: id, CreatedAt: at,
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			CostMicros:       sql.NullInt64{Int64: cost, Valid: true},
		})
		require.NoError(t, db.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
			CreatedAt: at, EffectiveGroupID: group.ID, InitiatorID: user.ID,
			Provider: "wire", ProviderName: "configured", Model: "model", CostMicros: sql.NullInt64{Int64: cost, Valid: true},
		}))
	}
	// Populate unrelated rollups so the plan can expose a full-table scan.
	_, err := sqlDB.ExecContext(ctx, "INSERT INTO aibridge_token_usage_hourly (organization_id, hour, effective_group_id, initiator_id, provider, provider_name, model, client, usage_count) SELECT $1, now(), gen_random_uuid(), $2, 'wire', 'configured', 'other', 'client', 1 FROM generate_series(1, 1000)", org.ID, user.ID)
	require.NoError(t, err)
	var unrelatedEmptyID int64
	require.NoError(t, sqlDB.QueryRowContext(ctx, "INSERT INTO aibridge_token_usage_hourly (organization_id, hour, effective_group_id, initiator_id, provider, provider_name, model, client, usage_count) VALUES ($1, now(), $2, $3, 'wire', 'configured', 'other', 'client', 0) RETURNING id", org.ID, uuid.New(), user.ID).Scan(&unrelatedEmptyID))

	for tick, wantIDs := range [][]uuid.UUID{{ids[1], ids[0]}, {ids[2], ids[3]}, {}} {
		require.NoError(t, db.InTx(func(tx database.Store) error {
			locked, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: cutoff, LimitCount: 2})
			if err != nil {
				return err
			}
			if len(wantIDs) == 0 {
				require.Empty(t, locked)
			} else {
				require.Equal(t, wantIDs, locked)
			}
			if len(locked) == 0 {
				return nil
			}
			result, err := tx.DeleteOldAIBridgeRecords(ctx, locked)
			if err != nil {
				return err
			}
			require.EqualValues(t, 4, result.TotalDeleted)
			require.Len(t, result.EmptyHourlyIds, tick)
			if len(result.EmptyHourlyIds) > 0 {
				if err := tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx, result.EmptyHourlyIds); err != nil {
					return err
				}
			}
			return nil
		}, nil))
		var rawCost, rawCount, hourlyCost, hourlyCount int64
		require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COALESCE(SUM(cost_micros), 0), COUNT(*) FROM aibridge_token_usages WHERE effective_group_id=$1", group.ID).Scan(&rawCost, &rawCount))
		require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COALESCE(SUM(cost_micros), 0), COALESCE(SUM(usage_count), 0) FROM aibridge_token_usage_hourly WHERE effective_group_id=$1", group.ID).Scan(&hourlyCost, &hourlyCount))
		require.Equal(t, rawCost, hourlyCost, "tick %d", tick)
		require.Equal(t, rawCount, hourlyCount, "tick %d", tick)
		require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly WHERE id=$1", unrelatedEmptyID).Scan(&hourlyCount))
		require.EqualValues(t, 1, hourlyCount)
	}
	var cost, count int64
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT cost_micros, usage_count FROM aibridge_token_usage_hourly WHERE effective_group_id=$1", group.ID).Scan(&cost, &count))
	require.EqualValues(t, 50, cost)
	require.EqualValues(t, 1, count)
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly WHERE model='other' AND usage_count=1").Scan(&count))
	require.EqualValues(t, 1000, count)

	rows, err := sqlDB.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS) DELETE FROM aibridge_token_usage_hourly WHERE id=ANY($1::bigint[]) AND usage_count=0", pq.Array([]int64{-1}))
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan = append(plan, line)
	}
	require.NoError(t, rows.Err())
	planText := strings.Join(plan, "\n")
	t.Logf("targeted cleanup plan:\n%s", planText)
	require.Contains(t, planText, "Index Scan using aibridge_token_usage_hourly_pkey")
}
