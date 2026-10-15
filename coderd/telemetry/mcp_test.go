package telemetry_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMCPToolUsage(t *testing.T) {
	t.Parallel()

	t.Run("AggregatesAndResets", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		start := clock.Now().UTC()
		usage := telemetry.NewMCPToolUsage(clock)

		remote := telemetry.MCPToolUsageKey{Transport: telemetry.MCPTransportRemote, Tool: "coder_list_workspaces", Client: "cursor"}
		stdio := telemetry.MCPToolUsageKey{Transport: telemetry.MCPTransportStdio, Tool: "coder_list_workspaces", Client: "claude-code"}
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1})
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1, ToolErrors: 1})
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1, AuthzErrors: 1})
		usage.Add(stdio, telemetry.MCPToolUsageCounts{Calls: 3, ValidationErrors: 2})
		// Invalid counts are dropped.
		usage.Add(remote, telemetry.MCPToolUsageCounts{})
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1, ToolErrors: -1})
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1, ToolErrors: 1, AuthzErrors: 1})

		clock.Advance(time.Minute)
		summaries := usage.Collect()
		require.Len(t, summaries, 2)

		got := summaries[0]
		assert.NotEqual(t, got.ID, summaries[1].ID)
		assert.Equal(t, start, got.PeriodStart)
		assert.Equal(t, start.Add(time.Minute), got.PeriodEnd)
		assert.Equal(t, telemetry.MCPTransportRemote, got.Transport)
		assert.Equal(t, "coder_list_workspaces", got.Tool)
		assert.Equal(t, "cursor", got.Client)
		assert.EqualValues(t, 3, got.Calls)
		assert.EqualValues(t, 1, got.ToolErrors)
		assert.EqualValues(t, 0, got.ValidationErrors)
		assert.EqualValues(t, 1, got.AuthzErrors)

		got = summaries[1]
		assert.Equal(t, telemetry.MCPTransportStdio, got.Transport)
		assert.Equal(t, "claude-code", got.Client)
		assert.EqualValues(t, 3, got.Calls)
		assert.EqualValues(t, 2, got.ValidationErrors)

		// The next period starts where the previous one ended.
		clock.Advance(time.Minute)
		usage.Add(remote, telemetry.MCPToolUsageCounts{Calls: 1})
		summaries = usage.Collect()
		require.Len(t, summaries, 1)
		assert.Equal(t, start.Add(time.Minute), summaries[0].PeriodStart)
		assert.EqualValues(t, 1, summaries[0].Calls)

		assert.Empty(t, usage.Collect())
	})

	t.Run("BoundedKeys", func(t *testing.T) {
		t.Parallel()

		usage := telemetry.NewMCPToolUsage(nil)
		for i := range 5000 {
			usage.Add(telemetry.MCPToolUsageKey{
				Transport: telemetry.MCPTransportRemote,
				Tool:      fmt.Sprintf("tool-%d", i),
				Client:    "other",
			}, telemetry.MCPToolUsageCounts{Calls: 1})
		}
		assert.Len(t, usage.Collect(), 1024)
	})

	t.Run("OnlyAggregateFields", func(t *testing.T) {
		t.Parallel()

		// Guard against fields that could carry tool arguments, results,
		// or user identifiers being added to the summary.
		data, err := json.Marshal(telemetry.MCPToolUsageSummary{})
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(data, &fields))
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		assert.ElementsMatch(t, []string{
			"id", "period_start", "period_end", "transport", "tool", "client",
			"calls", "tool_errors", "validation_errors", "authz_errors",
		}, keys)
	})

	t.Run("Snapshot", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		usage := telemetry.NewMCPToolUsage(nil)
		usage.Add(telemetry.MCPToolUsageKey{
			Transport: telemetry.MCPTransportRemote,
			Tool:      "coder_get_authenticated_user",
			Client:    "vscode",
		}, telemetry.MCPToolUsageCounts{Calls: 2, AuthzErrors: 1})

		_, snapshot := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.MCPToolUsage = usage
			return opts
		})
		require.Len(t, snapshot.MCPToolUsageSummaries, 1)
		got := snapshot.MCPToolUsageSummaries[0]
		assert.Equal(t, "coder_get_authenticated_user", got.Tool)
		assert.Equal(t, "vscode", got.Client)
		assert.Equal(t, telemetry.MCPTransportRemote, got.Transport)
		assert.EqualValues(t, 2, got.Calls)
		assert.EqualValues(t, 1, got.AuthzErrors)
		// The snapshot drained the aggregator.
		assert.Empty(t, usage.Collect())
	})
}
