package healthcheck_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

func TestPubsub(t *testing.T) {
	t.Parallel()
	changed := time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)
	for _, backend := range []string{pubsub.BackendPostgres, pubsub.BackendNATS} {
		for _, connected := range []bool{true, false} {
			name := backend + "/disconnected"
			if connected {
				name = backend + "/connected"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var report healthcheck.PubsubReport
				report.Run(context.Background(), &healthcheck.PubsubReportOptions{
					Pubsub: &healthReportingPubsub{report: pubsub.HealthReport{
						Backend: backend, Connected: connected, LastConnectionStateChange: changed,
					}},
					Dismissed: true,
				})
				require.Equal(t, backend, report.Backend)
				require.Equal(t, &connected, report.Connected)
				require.Equal(t, &changed, report.LastConnectionStateChange)
				require.True(t, report.Dismissed)
				if connected {
					require.Equal(t, health.SeverityOK, report.Severity)
					require.Nil(t, report.Error)
				} else {
					require.Equal(t, health.SeverityError, report.Severity)
					require.NotNil(t, report.Error)
				}
				encoded, err := json.Marshal(report)
				require.NoError(t, err)
				require.Contains(t, string(encoded), `"last_connection_state_change":"2026-09-24T07:00:00Z"`)
			})
		}
	}

	t.Run("Unsupported", func(t *testing.T) {
		t.Parallel()
		var report healthcheck.PubsubReport
		report.Run(context.Background(), &healthcheck.PubsubReportOptions{Pubsub: pubsub.NewInMemory()})
		require.Equal(t, health.SeverityWarning, report.Severity)
		require.Empty(t, report.Backend)
		require.Nil(t, report.Connected)
		require.Nil(t, report.LastConnectionStateChange)
		require.Len(t, report.Warnings, 1)
	})

	t.Run("Missing", func(t *testing.T) {
		t.Parallel()
		var report healthcheck.PubsubReport
		report.Run(context.Background(), &healthcheck.PubsubReportOptions{})
		require.Equal(t, health.SeverityError, report.Severity)
		require.NotNil(t, report.Error)
		require.Nil(t, report.Connected)
	})
}

type healthReportingPubsub struct {
	pubsub.Pubsub
	report pubsub.HealthReport
}

func (p *healthReportingPubsub) ReportHealth() pubsub.HealthReport {
	return p.report
}
