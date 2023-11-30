package healthcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/testutil"
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
		assert.Equal(t, health.SeverityOK, report.Severity)
		assert.Equal(t, ping.String(), report.Latency)
		assert.Equal(t, ping.Milliseconds(), report.LatencyMS)
		assert.Equal(t, healthcheck.DatabaseDefaultThreshold.Milliseconds(), report.ThresholdMS)
		assert.Nil(t, report.Error)
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
		assert.Equal(t, health.SeverityError, report.Severity)
		assert.Zero(t, report.Latency)
		require.NotNil(t, report.Error)
		assert.Equal(t, healthcheck.DatabaseDefaultThreshold.Milliseconds(), report.ThresholdMS)
		assert.Contains(t, *report.Error, err.Error())
		assert.Contains(t, *report.Error, health.CodeDatabasePingFailed)
	})

	t.Run("DismissedError", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
			err         = xerrors.New("ping error")
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(time.Duration(0), err)

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db, Dismissed: true})

		assert.Equal(t, health.SeverityError, report.Severity)
		assert.True(t, report.Dismissed)
		require.NotNil(t, report.Error)
		assert.Contains(t, *report.Error, health.CodeDatabasePingFailed)
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
		assert.Equal(t, health.SeverityOK, report.Severity)
		assert.Equal(t, time.Millisecond.String(), report.Latency)
		assert.EqualValues(t, 1, report.LatencyMS)
		assert.Equal(t, healthcheck.DatabaseDefaultThreshold.Milliseconds(), report.ThresholdMS)
		assert.Nil(t, report.Error)
		assert.Empty(t, report.Warnings)
	})

	t.Run("Threshold", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(time.Second, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Millisecond, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Second, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Millisecond, nil)
		db.EXPECT().Ping(gomock.Any()).Return(time.Second, nil)

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db, Threshold: time.Second})

		assert.True(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, health.SeverityWarning, report.Severity)
		assert.Equal(t, time.Second.String(), report.Latency)
		assert.EqualValues(t, 1000, report.LatencyMS)
		assert.Equal(t, time.Second.Milliseconds(), report.ThresholdMS)
		assert.Nil(t, report.Error)
		if assert.NotEmpty(t, report.Warnings) {
			assert.Contains(t, report.Warnings[0], health.CodeDatabasePingSlow)
		}
	})
}
