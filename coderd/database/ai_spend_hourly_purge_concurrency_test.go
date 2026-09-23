package database_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestDeleteOldAIBridgeRecordsConcurrentWithIngestion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		writerFirst bool
	}{{"WriterFirst", true}, {"PurgeFirst", false}} {
		t.Run(tc.name, func(t *testing.T) {
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
				InitiatorID: user.ID, Provider: "wire", ProviderName: "configured", Model: "model", StartedAt: at.Add(-48 * time.Hour),
			}, nil)
			cutoff := at.Add(-24 * time.Hour)
			writerReady := make(chan error, 1)
			writerRelease := make(chan struct{})
			writerDone := make(chan error, 1)
			writer := func() {
				writerDone <- db.InTx(func(tx database.Store) error {
					if _, err := tx.LockAIBridgeInterceptionForUsage(ctx, intc.ID); err != nil {
						writerReady <- err
						return err
					}
					_, err := tx.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
						ID: uuid.New(), InterceptionID: intc.ID, CreatedAt: at, Metadata: []byte("{}"),
						EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
					})
					if err == nil {
						err = tx.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
							CreatedAt: at, EffectiveGroupID: group.ID, InitiatorID: user.ID,
							Provider: "wire", ProviderName: "configured", Model: "model",
						})
					}
					writerReady <- err
					if err != nil {
						return err
					}
					select {
					case <-writerRelease:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}, nil)
			}
			purgeReady := make(chan error, 1)
			purgeRelease := make(chan struct{})
			purgeDone := make(chan error, 1)
			purge := func() {
				purgeDone <- db.InTx(func(tx database.Store) error {
					lockedIDs, err := tx.LockOldAIBridgeInterceptionsForPurge(ctx, cutoff)
					if err == nil {
						_, err = tx.DeleteOldAIBridgeRecords(ctx, lockedIDs)
					}
					if err == nil {
						err = tx.DeleteEmptyAIBridgeTokenUsageHourly(ctx)
					}
					purgeReady <- err
					if err != nil {
						return err
					}
					select {
					case <-purgeRelease:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}, nil)
			}
			waitBlocked := func(queryNeedle string) {
				t.Helper()
				ticker := time.NewTicker(testutil.IntervalFast)
				defer ticker.Stop()
				for {
					var n int
					err := sqlDB.QueryRowContext(ctx,
						"SELECT COUNT(*) FROM pg_stat_activity a WHERE a.datname = current_database() AND a.state = 'active' AND a.query LIKE '%' || $1 || '%' AND cardinality(pg_blocking_pids(a.pid)) > 0", queryNeedle,
					).Scan(&n)
					require.NoError(t, err)
					if n > 0 {
						t.Logf("observed blocked backend: %s", queryNeedle)
						return
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatalf("did not observe blocked backend %q: %v", queryNeedle, ctx.Err())
					}
				}
			}
			if tc.writerFirst {
				go writer()
				require.NoError(t, <-writerReady)
				go purge()
				waitBlocked("FOR UPDATE")
				close(writerRelease)
				require.NoError(t, <-writerDone)
				require.NoError(t, <-purgeReady)
				close(purgeRelease)
				require.NoError(t, <-purgeDone)
			} else {
				go purge()
				require.NoError(t, <-purgeReady)
				go writer()
				waitBlocked("FOR KEY SHARE")
				close(purgeRelease)
				require.NoError(t, <-purgeDone)
				err := <-writerReady
				require.ErrorIs(t, err, sql.ErrNoRows)
				close(writerRelease)
				require.ErrorIs(t, <-writerDone, sql.ErrNoRows)
			}
			var interceptions, raw, orphanRaw, hourly int64
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_interceptions WHERE id=$1", intc.ID).Scan(&interceptions))
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usages WHERE interception_id=$1", intc.ID).Scan(&raw))
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM aibridge_token_usages tu LEFT JOIN aibridge_interceptions ai ON tu.interception_id=ai.id WHERE tu.interception_id=$1 AND ai.id IS NULL", intc.ID).Scan(&orphanRaw))
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COALESCE(SUM(usage_count), 0) FROM aibridge_token_usage_hourly WHERE effective_group_id=$1", group.ID).Scan(&hourly))
			t.Logf("result: interceptions=%d raw=%d orphanRaw=%d hourly=%d", interceptions, raw, orphanRaw, hourly)
			if tc.writerFirst {
				require.Equal(t, int64(0), interceptions)
				require.Equal(t, int64(0), raw, fmt.Sprintf("purge missed committed usage: orphanRaw=%d hourly=%d", orphanRaw, hourly))
				require.Equal(t, int64(0), hourly)
			} else {
				require.Zero(t, interceptions)
				require.Zero(t, raw)
				require.Zero(t, hourly)
			}
		})
	}
}
