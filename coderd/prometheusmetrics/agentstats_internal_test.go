package prometheusmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/agentmetrics"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

func TestStageAgentSessionCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name              string
		aggregateByLabels []string
		stats             []database.GetWorkspaceAgentStatsAndLabelsRow
		want              map[string]float64
	}{
		{
			name:              "app names families and unknown are labeled",
			aggregateByLabels: agentmetrics.LabelAgentStats,
			stats: []database.GetWorkspaceAgentStatsAndLabelsRow{{
				Username:      "alice",
				WorkspaceName: "workspace",
				AgentName:     "agent",
				SessionCounts: json.RawMessage(`{"cursor":2,"vscode":3,"zed":4,"my future editor":5}`),
			}},
			want: map[string]float64{
				"agent:cursor:vscode:alice:workspace":            2,
				"agent:vscode:vscode:alice:workspace":            3,
				"agent:zed:ssh:alice:workspace":                  4,
				"agent:my future editor:unknown:alice:workspace": 5,
			},
		},
		{
			name:              "reduced identity labels sum collisions",
			aggregateByLabels: []string{agentmetrics.LabelUsername},
			stats: []database.GetWorkspaceAgentStatsAndLabelsRow{
				{Username: "alice", WorkspaceName: "one", AgentName: "one", SessionCounts: json.RawMessage(`{"cursor":2,"vscode":3}`)},
				{Username: "alice", WorkspaceName: "two", AgentName: "two", SessionCounts: json.RawMessage(`{"cursor":7,"vscode":11}`)},
			},
			want: map[string]float64{
				"cursor:vscode:alice": 9,
				"vscode:vscode:alice": 14,
			},
		},
		{
			name:              "empty session counts create no series",
			aggregateByLabels: []string{agentmetrics.LabelUsername},
			stats: []database.GetWorkspaceAgentStatsAndLabelsRow{{
				Username:      "alice",
				SessionCounts: json.RawMessage(`null`),
			}},
			want: map[string]float64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gauge := newSessionCountGauge(tc.aggregateByLabels)
			for _, stat := range tc.stats {
				stageAgentSessionCounts(context.Background(), slogtest.Make(t, nil), stat, agentStatsLabelValues(stat, tc.aggregateByLabels), gauge)
			}
			gauge.Commit()

			require.Equal(t, tc.want, collectGaugeValues(t, gauge))
			if tc.name == "app names families and unknown are labeled" {
				require.Equal(t, []map[string]string{
					{"agent_name": "agent", "app_name": "cursor", "family": "vscode", "username": "alice", "workspace_name": "workspace"},
					{"agent_name": "agent", "app_name": "my future editor", "family": "unknown", "username": "alice", "workspace_name": "workspace"},
					{"agent_name": "agent", "app_name": "vscode", "family": "vscode", "username": "alice", "workspace_name": "workspace"},
					{"agent_name": "agent", "app_name": "zed", "family": "ssh", "username": "alice", "workspace_name": "workspace"},
				}, collectGaugeLabels(t, gauge))
			}
		})
	}
}

func TestStageAgentSessionCounts_MalformedStagesNoSessionMetrics(t *testing.T) {
	t.Parallel()

	gauge := newSessionCountGauge([]string{agentmetrics.LabelUsername})
	_, ok := stageAgentSessionCounts(context.Background(), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), database.GetWorkspaceAgentStatsAndLabelsRow{
		Username:      "alice",
		SessionCounts: json.RawMessage(`not-json`),
	}, []string{"alice"}, gauge)
	require.False(t, ok)
	gauge.Commit()
	require.Empty(t, collectGaugeValues(t, gauge))
}

func TestStageAgentSessionCounts_EmptySnapshotRemovesSeries(t *testing.T) {
	t.Parallel()

	gauge := newSessionCountGauge([]string{agentmetrics.LabelUsername})
	stat := database.GetWorkspaceAgentStatsAndLabelsRow{
		Username:      "alice",
		SessionCounts: json.RawMessage(`{"cursor":2}`),
	}
	stageAgentSessionCounts(context.Background(), slogtest.Make(t, nil), stat, []string{"alice"}, gauge)
	gauge.Commit()
	require.Equal(t, map[string]float64{"cursor:vscode:alice": 2}, collectGaugeValues(t, gauge))

	gauge.Commit()
	require.Empty(t, collectGaugeValues(t, gauge))
}

func BenchmarkStageAgentSessionCounts(b *testing.B) {
	for _, fixture := range []struct{ agents, apps int }{{1, 4}, {100, 16}, {100, 65}} {
		b.Run(fmt.Sprintf("agents=%d/apps=%d", fixture.agents, fixture.apps), func(b *testing.B) {
			stats := make([]database.GetWorkspaceAgentStatsAndLabelsRow, fixture.agents)
			labels := make([][]string, fixture.agents)
			for i := range stats {
				counts := make(map[string]int64, fixture.apps)
				for j := range fixture.apps {
					counts[fmt.Sprintf("app_%d", j)] = 1
				}
				raw, err := json.Marshal(counts)
				if err != nil {
					b.Fatal(err)
				}
				stats[i] = database.GetWorkspaceAgentStatsAndLabelsRow{SessionCounts: raw}
				labels[i] = []string{fmt.Sprintf("user_%d", i)}
			}
			log := slogtest.Make(b, nil)
			gauge := newSessionCountGauge([]string{agentmetrics.LabelUsername})
			b.ReportAllocs()
			for b.Loop() {
				for i, stat := range stats {
					stageAgentSessionCounts(context.Background(), log, stat, labels[i], gauge)
				}
				gauge.Commit()
			}
			b.StopTimer()
			registry := prometheus.NewRegistry()
			registry.MustRegister(gauge)
			families, err := registry.Gather()
			if err != nil {
				b.Fatal(err)
			}
			if len(families) != 1 || len(families[0].Metric) != fixture.agents*fixture.apps {
				b.Fatal("unexpected series count")
			}
			b.ReportMetric(float64(len(families[0].Metric)), "series")
		})
	}
}

func newSessionCountGauge(aggregateByLabels []string) *CachedGaugeVec {
	labels := append([]string{}, aggregateByLabels...)
	labels = append(labels, "app_name", "family")
	return NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_session_count"}, labels))
}

func agentStatsLabelValues(agentStat database.GetWorkspaceAgentStatsAndLabelsRow, aggregateByLabels []string) []string {
	labelValues := make([]string, 0, len(aggregateByLabels))
	for _, label := range aggregateByLabels {
		switch label {
		case agentmetrics.LabelUsername:
			labelValues = append(labelValues, agentStat.Username)
		case agentmetrics.LabelWorkspaceName:
			labelValues = append(labelValues, agentStat.WorkspaceName)
		case agentmetrics.LabelAgentName:
			labelValues = append(labelValues, agentStat.AgentName)
		}
	}
	return labelValues
}

func collectGaugeLabels(t *testing.T, collector prometheus.Collector) []map[string]string {
	t.Helper()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	require.NoError(t, err)
	if len(families) == 0 {
		return nil
	}

	labels := make([]map[string]string, 0, len(families[0].Metric))
	for _, metric := range families[0].Metric {
		values := make(map[string]string, len(metric.Label))
		for _, label := range metric.Label {
			values[label.GetName()] = label.GetValue()
		}
		labels = append(labels, values)
	}
	return labels
}

func collectGaugeValues(t *testing.T, collector prometheus.Collector) map[string]float64 {
	t.Helper()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	require.NoError(t, err)

	values := make(map[string]float64)
	if len(families) == 0 {
		return values
	}
	for _, metric := range families[0].Metric {
		key := ""
		for i, label := range metric.Label {
			if i > 0 {
				key += ":"
			}
			key += label.GetValue()
		}
		values[key] = metric.GetGauge().GetValue()
	}
	return values
}

// sessionStatsStore gates each poll so snapshots are inspected before the next
// refresh. Both query paths use the same responses.
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

func TestAgentSessionCountSnapshots(t *testing.T) {
	t.Parallel()
	for _, usage := range []bool{false, true} {
		t.Run(fmt.Sprintf("usage=%t", usage), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			store := &sessionStatsStore{responses: make(chan sessionStatsResponse)}
			registry := prometheus.NewRegistry()
			closeFn, err := AgentStats(ctx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, store, time.Now().Add(-time.Minute), time.Millisecond, []string{agentmetrics.LabelUsername}, usage)
			require.NoError(t, err)
			t.Cleanup(closeFn)
			send := func(response sessionStatsResponse) {
				t.Helper()
				select {
				case store.responses <- response:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			polls := 0.0
			waitPoll := func() {
				t.Helper()
				polls++
				require.Eventually(t, func() bool {
					metrics, err := registry.Gather()
					if err != nil {
						return false
					}
					for _, metric := range metrics {
						if metric.GetName() == "coderd_prometheusmetrics_agentstats_execution_seconds" {
							return float64(metric.Metric[0].GetHistogram().GetSampleCount()) >= polls
						}
					}
					return false
				}, testutil.WaitShort, testutil.IntervalFast)
			}
			counts := func() map[string]float64 {
				t.Helper()
				metrics, err := registry.Gather()
				require.NoError(t, err)
				result := map[string]float64{}
				for _, metric := range metrics {
					if metric.GetName() != "coderd_agentstats_session_count" {
						continue
					}
					for _, sample := range metric.Metric {
						labels := map[string]string{}
						for _, label := range sample.Label {
							labels[label.GetName()] = label.GetValue()
						}
						require.Equal(t, "alice", labels["username"])
						require.Len(t, labels, 3)
						result[labels["app_name"]+":"+labels["family"]] = sample.GetGauge().GetValue()
					}
				}
				return result
			}
			send(sessionStatsResponse{rows: []database.GetWorkspaceAgentStatsAndLabelsRow{
				{Username: "alice", SessionCounts: json.RawMessage(`{"cursor":2,"vscode":1,"future_ide":4}`)},
				{Username: "alice", SessionCounts: json.RawMessage(`{"cursor":3}`)},
			}})
			waitPoll()
			expected := map[string]float64{"cursor:vscode": 5, "vscode:vscode": 1, "future_ide:unknown": 4}
			require.Equal(t, expected, counts())
			send(sessionStatsResponse{err: xerrors.New("query failed")})
			waitPoll()
			require.Equal(t, expected, counts())
			send(sessionStatsResponse{rows: []database.GetWorkspaceAgentStatsAndLabelsRow{{Username: "alice", SessionCounts: json.RawMessage(`{"vscode":7}`)}}})
			waitPoll()
			require.Equal(t, map[string]float64{"vscode:vscode": 7}, counts())
			send(sessionStatsResponse{})
			waitPoll()
			require.Empty(t, counts())
		})
	}
}
