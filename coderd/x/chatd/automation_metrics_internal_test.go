package chatd

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	coderdtestutil "github.com/coder/coder/v2/testutil"
)

// TestAutomationMetrics checks the run counter, labeled only by trigger,
// outcome, and reason, and the scan duration histogram.
func TestAutomationMetrics(t *testing.T) {
	t.Parallel()
	f := newScheduleFixture(t, database.ChatStatusRunning, scheduleStart)
	f.registry = prometheus.NewRegistry()
	ctx := coderdtestutil.Context(t, coderdtestutil.WaitLong)
	server := f.newServer(t, Limits{})
	missed := f.existingChat(ctx, t, server, "0 * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
	newChat := f.newChat(ctx, t, server, "0 0 1 1 *", "UTC")

	// The hourly schedule is missed. The minute schedule is created after
	// that scan, so its only occurrence skips the busy chat.
	f.advanceTo(ctx, t, missed.ScheduleNextRunAt.Time.Add(2*automationScheduleGrace))
	server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
	busy := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusySkip)
	f.advanceTo(ctx, t, busy.ScheduleNextRunAt.Time)
	server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
	_, err := server.RunAutomation(f.asOwner(ctx, t), newChat.ID, nil)
	require.NoError(t, err)

	runs := server.automationMetrics.runs
	require.Equal(t, 1.0, testutil.ToFloat64(runs.WithLabelValues("schedule", "skipped", "chat_busy")))
	require.Equal(t, 1.0, testutil.ToFloat64(runs.WithLabelValues("schedule", "skipped", "missed")))
	require.Equal(t, 1.0, testutil.ToFloat64(runs.WithLabelValues("run_now", "accepted", "none")))

	count, err := testutil.GatherAndCount(f.registry, "coderd_agents_automation_scan_duration_seconds")
	require.NoError(t, err)
	require.Equal(t, 1, count, "the histogram is registered once")
	require.GreaterOrEqual(t, histogramCount(t, f.registry, "coderd_agents_automation_scan_duration_seconds"), uint64(2))

	// The names and labels the docs and alerts use.
	names, err := f.registry.Gather()
	require.NoError(t, err)
	labels := map[string][]string{}
	for _, family := range names {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				labels[family.GetName()] = append(labels[family.GetName()], label.GetName())
			}
		}
	}
	for _, label := range labels["coderd_agents_automation_runs_total"] {
		require.Contains(t, []string{"trigger", "outcome", "reason"}, label, "no per-automation or per-user labels")
	}
}

func histogramCount(t *testing.T, registry *prometheus.Registry, name string) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			var total uint64
			for _, metric := range family.GetMetric() {
				total += metric.GetHistogram().GetSampleCount()
			}
			return total
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}
