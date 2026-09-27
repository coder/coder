package database_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestIncrementAIBridgeTokenUsageHourlyConcurrentAndRollback(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	user := dbgen.User(t, db, database.User{})
	at := time.Date(2030, 1, 1, 12, 45, 0, 0, time.UTC)
	intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: at,
	}, nil)
	write := func() error {
		return db.InTx(func(tx database.Store) error {
			if _, err := tx.LockAIBridgeInterceptionForUsage(ctx, intc.ID); err != nil {
				return err
			}
			_, err := tx.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
				ID: uuid.New(), InterceptionID: intc.ID, CreatedAt: at, Metadata: []byte("{}"),
				EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			})
			if err != nil {
				return err
			}
			return tx.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
				CreatedAt: at, EffectiveGroupID: group.ID, InitiatorID: user.ID,
				Provider: "wire", ProviderName: "configured", Model: "model",
			})
		}, nil)
	}
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- write()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	rollback := xerrors.New("rollback")
	err := db.InTx(func(tx database.Store) error {
		if _, err := tx.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
			ID: uuid.New(), InterceptionID: intc.ID, CreatedAt: at, Metadata: []byte("{}"),
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
		}); err != nil {
			return err
		}
		if err := tx.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
			CreatedAt: at, EffectiveGroupID: group.ID, InitiatorID: user.ID,
			Provider: "wire", ProviderName: "configured", Model: "model",
		}); err != nil {
			return err
		}
		return rollback
	}, nil)
	require.ErrorIs(t, err, rollback)
	var count, unpriced int64
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT usage_count, unpriced_usage_count FROM aibridge_token_usage_hourly",
	).Scan(&count, &unpriced))
	require.EqualValues(t, workers, count)
	require.EqualValues(t, workers, unpriced)
	var rawCount int64
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM aibridge_token_usages WHERE interception_id = $1", intc.ID,
	).Scan(&rawCount))
	require.EqualValues(t, workers, rawCount)
}
