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
		result := SearchTools(entries, FindToolsArgs{Queries: []string{"issue"}}, 0)
		require.Len(t, result.Matches, 2)
		require.Equal(t, []string{"github__create_issue", "github__search_issues"}, []string{result.Matches[0].Name, result.Matches[1].Name})
	})
	t.Run("parameter text", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{Queries: []string{"channel"}}, 0)
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("exact names", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{Names: []string{"slack__post_message", "missing"}}, 0)
		require.Equal(t, []string{"slack__post_message"}, result.Activated)
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("empty queries", func(t *testing.T) {
		t.Parallel()
		result := SearchTools(entries, FindToolsArgs{}, 0)
		require.Empty(t, result.Matches)
		require.Empty(t, result.Activated)
	})
	t.Run("cap", func(t *testing.T) {
		t.Parallel()
		many := make([]FindToolCatalogEntry, 25)
		for i := range many {
			many[i] = FindToolCatalogEntry{Name: fmt.Sprintf("server__tool_%02d", i), Description: "common"}
		}
		result := SearchTools(many, FindToolsArgs{Queries: []string{"common"}}, 0)
		require.Len(t, result.Matches, findToolsMaxMatches)
		require.Equal(t, "server__tool_00", result.Matches[0].Name)
	})
	t.Run("names capped and prioritized over queries", func(t *testing.T) {
		t.Parallel()
		many := make([]FindToolCatalogEntry, 25)
		names := make([]string, 0, len(many))
		for i := range many {
			many[i] = FindToolCatalogEntry{Name: fmt.Sprintf("server__tool_%02d", i), Description: "common"}
			names = append(names, many[i].Name)
		}
		result := SearchTools(many, FindToolsArgs{Queries: []string{"common"}, Names: []string{"server__tool_24"}}, 0)
		require.Len(t, result.Matches, findToolsMaxMatches)
		require.Equal(t, "server__tool_24", result.Matches[0].Name)
		require.Contains(t, result.Activated, "server__tool_24")

		capped := SearchTools(many, FindToolsArgs{Names: names}, 0)
		require.Len(t, capped.Matches, findToolsMaxMatches)
		require.Len(t, capped.Activated, findToolsMaxMatches)
	})
	t.Run("server metadata", func(t *testing.T) {
		t.Parallel()
		serverEntries := []FindToolCatalogEntry{
			{Name: "tracker__create", Description: "Create an item", Server: "tracker", ServerDescription: "Project tracking"},
			{Name: "docs__create", Description: "Create a project document", Server: "docs", ServerDescription: "Documentation"},
		}
		result := SearchTools(serverEntries, FindToolsArgs{Queries: []string{"tracking"}}, 0)
		require.Equal(t, []string{"tracker__create"}, result.Activated)

		result = SearchTools(serverEntries, FindToolsArgs{Queries: []string{"project"}}, 0)
		require.Equal(t, "docs__create", result.Matches[0].Name,
			"tool description match outranks server metadata match")
		require.Len(t, result.Matches, 2)
	})
	t.Run("server prefix scope", func(t *testing.T) {
		t.Parallel()
		scopedEntries := []FindToolCatalogEntry{
			{Name: "ci__status", Description: "Pipeline status", Server: "ci"},
			{Name: "github__get_commit", Description: "Get commit status", Server: "github"},
		}
		result := SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"github: status"}}, 0)
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"a known server prefix restricts matches to that server")

		result = SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"github:"}}, 0)
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"a bare server prefix lists that server's tools")

		result = SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"error: status"}}, 0)
		require.Len(t, result.Matches, 2,
			"an unknown prefix is searched as plain keywords")
	})
	t.Run("unicode terms", func(t *testing.T) {
		t.Parallel()
		unicodeEntries := []FindToolCatalogEntry{
			{Name: "docs__検索", Description: "ドキュメント検索"},
			{Name: "docs__erstellen", Description: "Dokument ERSTELLEN"},
		}
		result := SearchTools(unicodeEntries, FindToolsArgs{Queries: []string{"検索"}}, 0)
		require.Equal(t, []string{"docs__検索"}, result.Activated)

		result = SearchTools(unicodeEntries, FindToolsArgs{Queries: []string{"Erstellen"}}, 0)
		require.Equal(t, []string{"docs__erstellen"}, result.Activated)
	})
	t.Run("schema token budget", func(t *testing.T) {
		t.Parallel()
		weighted := []FindToolCatalogEntry{
			{Name: "server__big_a", Description: "big", SchemaTokens: 60},
			{Name: "server__big_b", Description: "big", SchemaTokens: 60},
			{Name: "server__huge", Description: "big", SchemaTokens: 500},
		}
		result := SearchTools(weighted, FindToolsArgs{Queries: []string{"big"}}, 100)
		require.Equal(t, []string{"server__big_a"}, result.Activated,
			"matches stop once the schema budget is spent")

		result = SearchTools(weighted, FindToolsArgs{Names: []string{"server__huge"}}, 100)
		require.Equal(t, []string{"server__huge"}, result.Activated,
			"the first match is kept even when it alone exceeds the budget")
	})
	t.Run("result descriptions are summarized", func(t *testing.T) {
		t.Parallel()
		long := []FindToolCatalogEntry{{
			Name:        "server__verbose",
			Description: strings.Repeat("word ", 100),
		}}
		result := SearchTools(long, FindToolsArgs{Names: []string{"server__verbose"}}, 0)
		require.LessOrEqual(t, len([]rune(result.Matches[0].Description)), 80)
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

	manyServers := make([]FindToolCatalogEntry, 500)
	for i := range manyServers {
		manyServers[i] = FindToolCatalogEntry{
			Name:   fmt.Sprintf("server_%03d_%s__tool", i, strings.Repeat("s", 40)),
			Server: fmt.Sprintf("server_%03d_%s", i, strings.Repeat("s", 40)),
		}
	}
	countsExceeded := buildFindToolsDescription(manyServers)
	require.Contains(t, countsExceeded, "500 deferred tools across 500 servers.")
	require.NotContains(t, countsExceeded, "## server_000")
	require.LessOrEqual(t, estimatedFindToolsTokens(countsExceeded), float64(findToolsCatalogTokens))
}
