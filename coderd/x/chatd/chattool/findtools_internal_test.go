package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestSearchTools(t *testing.T) {
	t.Parallel()
	entries := []FindToolCatalogEntry{
		{Name: "github__create_issue", Description: "Create an issue", ParameterText: "repository title body"},
		{Name: "github__search_issues", Description: "Search issue descriptions", ParameterText: "repository query"},
		{Name: "slack__post_message", Description: "Post a message", ParameterText: "channel text"},
	}

	t.Run("weights and tie break", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{Queries: []string{"issue"}})
		require.Equal(t, []string{"github__create_issue", "github__search_issues"}, []string{result.Matches[0].Name, result.Matches[1].Name})
	})
	t.Run("parameter text", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{Queries: []string{"channel"}})
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("exact names", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{Names: []string{"slack__post_message", "missing"}})
		require.Equal(t, []string{"slack__post_message"}, result.Activated)
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("empty queries", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{})
		require.Empty(t, result.Matches)
		require.Empty(t, result.Activated)
	})
	t.Run("cap", func(t *testing.T) {
		t.Parallel()
		many := make([]FindToolCatalogEntry, 25)
		for i := range many {
			many[i] = FindToolCatalogEntry{Name: fmt.Sprintf("server__tool_%02d", i), Description: "common"}
		}
		result := SearchTools(many, FindToolsArgs{Queries: []string{"common"}})
		require.Len(t, result.Matches, findToolsMaxMatches)
		require.Equal(t, "server__tool_00", result.Matches[0].Name)
	})
}

func TestFindTools(t *testing.T) {
	t.Parallel()
	var recorded FindToolsCall
	tool := FindTools(FindToolsOptions{
		Entries: []FindToolCatalogEntry{{Name: "github__create_issue", Description: "Create an issue"}},
		OnCall:  func(_ context.Context, call FindToolsCall) { recorded = call },
	})
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"queries":["issue"]}`})
	require.NoError(t, err)
	var result FindToolsResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, []string{"github__create_issue"}, result.Activated)
	require.Equal(t, 1, recorded.MatchCount)

	resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
}

func TestBuildFindToolsDescription(t *testing.T) {
	t.Parallel()
	entries := []FindToolCatalogEntry{
		{Name: "zeta__last", Description: "Last tool. More detail", Server: "zeta", ServerDescription: strings.Repeat("z", 80)},
		{Name: "alpha__second", Description: strings.Repeat("x", 100), Server: "alpha", ServerDescription: "Alpha server"},
		{Name: "alpha__first", Description: "First tool\nmore detail", Server: "alpha", ServerDescription: "Alpha server"},
	}
	description := buildFindToolsDescription(entries)
	require.Less(t, strings.Index(description, "## alpha"), strings.Index(description, "## zeta"))
	require.Less(t, strings.Index(description, "alpha__first"), strings.Index(description, "alpha__second"))
	require.Contains(t, description, "First tool")
	require.NotContains(t, description, "more detail")
	require.Contains(t, description, "...")

	many := make([]FindToolCatalogEntry, 300)
	for i := range many {
		many[i] = FindToolCatalogEntry{
			Name:        fmt.Sprintf("server__tool_%03d_%s", i, strings.Repeat("n", 40)),
			Description: strings.Repeat("description ", 20),
			Server:      "server",
		}
	}
	degraded := buildFindToolsDescription(many)
	require.Contains(t, degraded, "## server (300 tools)")
	require.NotContains(t, degraded, "server__tool_000")
}
