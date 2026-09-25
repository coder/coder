package database_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgeHourlyWideDimensions(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	user := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
	wide := func(seed string) string {
		var b string
		for i := range 27 {
			digest := sha256.Sum256([]byte(seed + strconv.Itoa(i)))
			b += hex.EncodeToString(digest[:])
		}
		return b[:1700]
	}
	providerName, model := wide("provider"), wide("model")
	at := time.Date(2030, 1, 1, 12, 45, 0, 0, time.UTC)
	for _, modelName := range []string{model, model + "x"} {
		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID, Provider: "anthropic", ProviderName: providerName,
			Model: modelName, Client: sql.NullString{String: "cli", Valid: true},
			StartedAt: at.Add(-48 * time.Hour),
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID, CreatedAt: at,
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			CostMicros:       sql.NullInt64{Int64: 123, Valid: true},
		})
		require.NoError(t, db.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
			CreatedAt: at, EffectiveGroupID: group.ID, InitiatorID: user.ID,
			Provider: "anthropic", ProviderName: providerName, Model: modelName,
			Client:     sql.NullString{String: "cli", Valid: true},
			CostMicros: sql.NullInt64{Int64: 123, Valid: true},
		}))
	}
	var groups, cost int64
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*), SUM(cost_micros) FROM aibridge_token_usage_hourly WHERE effective_group_id = $1", group.ID,
	).Scan(&groups, &cost))
	require.EqualValues(t, 2, groups)
	require.EqualValues(t, 246, cost)
	rows, err := db.ListOrganizationAISpendUsers(ctx, database.ListOrganizationAISpendUsersParams{
		OrganizationID: org.ID, PeriodStart: at.Truncate(time.Hour),
		PeriodEnd:    at.Truncate(time.Hour).Add(time.Hour),
		ProviderName: providerName, Model: model,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 123, rows[0].CostMicros)
	require.NoError(t, db.InTx(func(tx database.Store) error {
		ids, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, database.LockOldAIBridgeInterceptionsForPurgeParams{BeforeTime: at.Add(-24 * time.Hour), LimitCount: 10000})
		if err != nil {
			return err
		}
		result, err := tx.DeleteOldAIBridgeRecords(ctx, ids)
		if err != nil {
			return err
		}
		return tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx, result.EmptyHourlyIds)
	}, nil))
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usage_hourly WHERE effective_group_id = $1", group.ID).Scan(&groups))
	require.Zero(t, groups)
}
