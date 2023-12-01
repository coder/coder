package dbpurge_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Ensures no goroutines leak.
func TestPurge(t *testing.T) {
	t.Parallel()
	purger := dbpurge.New(context.Background(), slogtest.Make(t, nil), dbmem.New())
	err := purger.Close()
	require.NoError(t, err)
}

func TestDeleteOldProvisionerDaemons(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	now := dbtime.Now()

	// given
	_, err := db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		Name:      "external-0",
		CreatedAt: now.Add(-7 * 24 * time.Hour),
		UpdatedAt: sql.NullTime{Valid: true, Time: now.Add(-7 * 24 * time.Hour).Add(time.Minute)},
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		Name:      "external-1",
		CreatedAt: now.Add(-8 * 24 * time.Hour),
		UpdatedAt: sql.NullTime{Valid: true, Time: now.Add(-8 * 24 * time.Hour)},
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		Name:      "external-2",
		CreatedAt: now.Add(-9 * 24 * time.Hour),
		UpdatedAt: sql.NullTime{Valid: true, Time: now.Add(-9 * 24 * time.Hour)},
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		Name:      "external-3",
		CreatedAt: now.Add(-6 * 24 * time.Hour),
		UpdatedAt: sql.NullTime{Valid: true, Time: now.Add(-6 * 24 * time.Hour)},
	})
	require.NoError(t, err)

	// when
	closer := dbpurge.New(ctx, logger, db)
	defer closer.Close()

	// then
	require.Eventually(t, func() bool {
		daemons, err := db.GetProvisionerDaemons(ctx)
		if err != nil {
			return false
		}
		return len(daemons) == 2
	}, testutil.WaitShort, testutil.IntervalFast)

}
