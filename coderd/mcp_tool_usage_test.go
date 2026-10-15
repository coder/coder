package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostMCPToolUsage(t *testing.T) {
	t.Parallel()

	t.Run("Aggregates", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			TelemetryReporter: newFakeTelemetryReporter(ctx, t, 10),
		})
		_ = coderdtest.CreateFirstUser(t, client)
		_ = api.MCPToolUsage.Collect()

		err := codersdk.NewExperimentalClient(client).ReportMCPToolUsage(ctx, codersdk.MCPToolUsageReport{
			Entries: []codersdk.MCPToolUsageReportEntry{
				{Tool: toolsdk.ToolNameListWorkspaces, Client: "claude-code", Calls: 3, ToolErrors: 1, AuthzErrors: 1},
				// Unknown tools and clients collapse to the fallback.
				{Tool: "custom_tool_with_user_data", Client: "my-private-agent", Calls: 2, ValidationErrors: 2},
			},
		})
		require.NoError(t, err)

		summaries := api.MCPToolUsage.Collect()
		require.Len(t, summaries, 2)
		for _, summary := range summaries {
			assert.Equal(t, telemetry.MCPTransportStdio, summary.Transport)
		}
		assert.Equal(t, toolsdk.ToolNameListWorkspaces, summaries[0].Tool)
		assert.Equal(t, mcp.ClientClaudeCode, summaries[0].Client)
		assert.EqualValues(t, 3, summaries[0].Calls)
		assert.EqualValues(t, 1, summaries[0].ToolErrors)
		assert.EqualValues(t, 1, summaries[0].AuthzErrors)
		assert.Equal(t, mcp.ToolOther, summaries[1].Tool)
		assert.Equal(t, mcp.ClientOther, summaries[1].Client)
		assert.EqualValues(t, 2, summaries[1].Calls)
		assert.EqualValues(t, 2, summaries[1].ValidationErrors)
	})

	t.Run("TelemetryDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = api.MCPToolUsage.Collect()

		err := codersdk.NewExperimentalClient(client).ReportMCPToolUsage(ctx, codersdk.MCPToolUsageReport{
			Entries: []codersdk.MCPToolUsageReportEntry{
				{Tool: toolsdk.ToolNameListWorkspaces, Client: "cursor", Calls: 1},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, api.MCPToolUsage.Collect())
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			TelemetryReporter: newFakeTelemetryReporter(ctx, t, 10),
		})
		_ = coderdtest.CreateFirstUser(t, client)
		_ = api.MCPToolUsage.Collect()
		expClient := codersdk.NewExperimentalClient(client)

		tooMany := make([]codersdk.MCPToolUsageReportEntry, 257)
		for i := range tooMany {
			tooMany[i] = codersdk.MCPToolUsageReportEntry{Tool: toolsdk.ToolNameListWorkspaces, Calls: 1}
		}
		for name, entries := range map[string][]codersdk.MCPToolUsageReportEntry{
			"TooManyEntries": tooMany,
			"NoCalls":        {{Tool: toolsdk.ToolNameListWorkspaces}},
			"TooManyCalls":   {{Tool: toolsdk.ToolNameListWorkspaces, Calls: 100_001}},
			"NegativeErrors": {{Tool: toolsdk.ToolNameListWorkspaces, Calls: 1, ToolErrors: -1}},
			"MoreErrors":     {{Tool: toolsdk.ToolNameListWorkspaces, Calls: 1, ToolErrors: 1, AuthzErrors: 1}},
		} {
			err := expClient.ReportMCPToolUsage(ctx, codersdk.MCPToolUsageReport{Entries: entries})
			sdkErr, ok := codersdk.AsError(err)
			require.True(t, ok, name)
			assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), name)
		}
		// A rejected report records nothing, including its valid entries.
		assert.Empty(t, api.MCPToolUsage.Collect())
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client := coderdtest.New(t, nil)
		anon := codersdk.New(client.URL)
		err := codersdk.NewExperimentalClient(anon).ReportMCPToolUsage(ctx, codersdk.MCPToolUsageReport{})
		sdkErr, ok := codersdk.AsError(err)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})
}
