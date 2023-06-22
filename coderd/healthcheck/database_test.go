package healthcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database/dbmock"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/testutil"
)

func TestDatabase(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
			ping        = 10 * time.Millisecond
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(ping, nil).Times(5)

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db})

		assert.True(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, ping, report.Latency)
		assert.NoError(t, report.Error)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
			err         = xerrors.New("ping error")
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(time.Duration(0), err)

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db})

		assert.False(t, report.Healthy)
		assert.False(t, report.Reachable)
		assert.Zero(t, report.Latency)
		assert.ErrorIs(t, report.Error, err)
	})

	t.Run("Median", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(time.Microsecond, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Second, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Nanosecond, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Minute, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Millisecond, nil)

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db})

		assert.True(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, time.Millisecond, report.Latency)
		assert.NoError(t, report.Error)
	})
}
