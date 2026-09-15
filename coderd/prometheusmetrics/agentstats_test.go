package prometheusmetrics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/agentmetrics"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/testutil"
)

// sessionStatsStore hands each poll of the agent stats collector the next
// scripted response so the test controls exactly what every snapshot sees.
type sessionStatsStore struct {
	database.Store
	responses chan sessionStatsResponse
}

type sessionStatsResponse struct {
	rows []database.GetWorkspaceAgentStatsAndLabelsRow
	err  error
}

func (s *sessionStatsStore) GetWorkspaceAgentStatsAndLabels(ctx context.Context, _ time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	select {
	case response := <-s.responses:
		return response.rows, response.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *sessionStatsStore) GetWorkspaceAgentUsageStatsAndLabels(ctx context.Context, since time.Time) ([]database.GetWorkspaceAgentUsageStatsAndLabelsRow, error) {
	rows, err := s.GetWorkspaceAgentStatsAndLabels(ctx, since)
	converted := make([]database.GetWorkspaceAgentUsageStatsAndLabelsRow, len(rows))
	for i, row := range rows {
		converted[i] = database.GetWorkspaceAgentUsageStatsAndLabelsRow(row)
	}
	return converted, err
}

// TestAgentSessionCountSnapshots covers the per-app gauge across successive
// polls: agents sharing the configured identity labels are summed, unknown
// names are kept, malformed rows are skipped, and a failed query or an empty
// window retains the last snapshot while a non-empty one replaces it.
func TestAgentSessionCountSnapshots(t *testing.T) {
	t.Parallel()
	for _, usage := range []bool{false, true} {
		t.Run(fmt.Sprintf("usage=%t", usage), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			store := &sessionStatsStore{responses: make(chan sessionStatsResponse)}
			registry := prometheus.NewRegistry()
			closeFn, err := prometheusmetrics.AgentStats(ctx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, store, time.Now().Add(-time.Minute), time.Millisecond, []string{agentmetrics.LabelUsername}, usage)
			require.NoError(t, err)
			t.Cleanup(closeFn)

			polls := uint64(0)
			// poll feeds one response to the collector and waits for that
			// poll to finish, using the execution histogram as the signal.
			poll := func(response sessionStatsResponse) {
				t.Helper()
				select {
				case store.responses <- response:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				polls++
				require.Eventually(t, func() bool {
					for _, metric := range gather(t, registry) {
						if metric.GetName() == "coderd_prometheusmetrics_agentstats_execution_seconds" {
							return metric.Metric[0].GetHistogram().GetSampleCount() >= polls
						}
					}
					return false
				}, testutil.WaitShort, testutil.IntervalFast)
			}
			// counts returns the per-app gauge keyed by "app_name:family".
			counts := func() map[string]float64 {
				t.Helper()
				result := map[string]float64{}
				for _, metric := range gather(t, registry) {
					if metric.GetName() != "coderd_agentstats_session_count" {
						continue
					}
					for _, sample := range metric.Metric {
						labels := map[string]string{}
						for _, label := range sample.Label {
							labels[label.GetName()] = label.GetValue()
						}
						require.Equal(t, map[string]string{"username": "alice", "app_name": labels["app_name"], "family": labels["family"]}, labels)
						result[labels["app_name"]+":"+labels["family"]] = sample.GetGauge().GetValue()
					}
				}
				return result
			}
			row := func(raw string) database.GetWorkspaceAgentStatsAndLabelsRow {
				return database.GetWorkspaceAgentStatsAndLabelsRow{Username: "alice", SessionCounts: json.RawMessage(raw)}
			}

			poll(sessionStatsResponse{rows: []database.GetWorkspaceAgentStatsAndLabelsRow{
				row(`{"cursor":2,"vscode":1,"future_ide":4}`),
				row(`{"cursor":3}`),
				row(`not-json`),
			}})
			expected := map[string]float64{"cursor:vscode": 5, "vscode:vscode": 1, "future_ide:unknown": 4}
			require.Equal(t, expected, counts())

			poll(sessionStatsResponse{err: xerrors.New("query failed")})
			require.Equal(t, expected, counts(), "failed query must retain the last snapshot")

			poll(sessionStatsResponse{})
			require.Equal(t, expected, counts(), "empty window must retain the last snapshot")

			poll(sessionStatsResponse{rows: []database.GetWorkspaceAgentStatsAndLabelsRow{row(`{"vscode":7}`)}})
			require.Equal(t, map[string]float64{"vscode:vscode": 7}, counts(), "apps no longer reported must be dropped")
		})
	}
}

func gather(t *testing.T, registry *prometheus.Registry) []*dto.MetricFamily {
	t.Helper()
	metrics, err := registry.Gather()
	require.NoError(t, err)
	return metrics
}
