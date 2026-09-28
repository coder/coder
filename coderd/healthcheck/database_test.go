package healthcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
			assert.Equal(t, report.Warnings[0].Code, health.CodeDatabasePingSlow)
		}
	})

	for _, tt := range []struct {
		name    string
		builtin bool
	}{
		{name: "Builtin", builtin: true},
		{name: "External"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("EOLVersionWarns", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)
				report := healthcheck.DatabaseReport{}
				db := dbmock.NewMockStore(gomock.NewController(t))
				db.EXPECT().Ping(gomock.Any()).Return(10*time.Millisecond, nil).Times(5)

				report.Run(ctx, &healthcheck.DatabaseReportOptions{
					DB: db, ServerVersionNum: 130004, Builtin: tt.builtin,
				})

				assert.True(t, report.Healthy)
				assert.True(t, report.Reachable)
				assert.Equal(t, health.SeverityWarning, report.Severity)
				assert.Nil(t, report.Error)
				require.Len(t, report.Warnings, 1)
				warning := report.Warnings[0]
				assert.Equal(t, health.CodeDatabasePostgresVersionEOL, warning.Code)
				if tt.builtin {
					assert.Equal(t, "Built-in PostgreSQL version is end-of-life; migrate to an external PostgreSQL database using the migration guide linked in the EDB03 documentation.", warning.Message)
					assert.Equal(t, "https://coder.com/docs/admin/monitoring/health-check#edb03", warning.URL(""))
				} else {
					assert.Equal(t, "PostgreSQL version is end-of-life; upgrade your PostgreSQL server to a supported version (14+).", warning.Message)
					assert.NotContains(t, warning.Message, "migration guide")
					assert.NotContains(t, warning.Message, "Built-in")
				}
			})

			for _, version := range []struct {
				name string
				num  int
			}{
				{name: "UnknownVersion", num: 0},
				{name: "SupportedVersionBoundary", num: 140000},
				{name: "SupportedVersion", num: 160003},
			} {
				t.Run(version.name+"NoWarning", func(t *testing.T) {
					t.Parallel()

					ctx := testutil.Context(t, testutil.WaitLong)
					report := healthcheck.DatabaseReport{}
					db := dbmock.NewMockStore(gomock.NewController(t))
					db.EXPECT().Ping(gomock.Any()).Return(10*time.Millisecond, nil).Times(5)

					report.Run(ctx, &healthcheck.DatabaseReportOptions{
						DB: db, ServerVersionNum: version.num, Builtin: tt.builtin,
					})

					assert.True(t, report.Healthy)
					assert.True(t, report.Reachable)
					assert.Equal(t, health.SeverityOK, report.Severity)
					assert.Empty(t, report.Warnings)
				})
			}
		})
	}

	t.Run("EOLVersionDoesNotDowngradeError", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			report      = healthcheck.DatabaseReport{}
			db          = dbmock.NewMockStore(gomock.NewController(t))
		)
		defer cancel()

		db.EXPECT().Ping(gomock.Any()).Return(time.Duration(0), xerrors.New("ping error"))

		report.Run(ctx, &healthcheck.DatabaseReportOptions{DB: db, ServerVersionNum: 130004})

		assert.Equal(t, health.SeverityError, report.Severity)
	})
}
