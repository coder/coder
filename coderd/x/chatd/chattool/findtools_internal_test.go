package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
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
		result, _ := SearchTools(entries, FindToolsArgs{Queries: []string{"issue"}}, SearchBudget{})
		require.Len(t, result.Matches, 2)
		require.Equal(t, []string{"github__create_issue", "github__search_issues"}, []string{result.Matches[0].Name, result.Matches[1].Name})
	})
	t.Run("parameter text", func(t *testing.T) {
		t.Parallel()
		result, _ := SearchTools(entries, FindToolsArgs{Queries: []string{"channel"}}, SearchBudget{})
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("exact names", func(t *testing.T) {
		t.Parallel()
		result, _ := SearchTools(entries, FindToolsArgs{Names: []string{"slack__post_message", "missing"}}, SearchBudget{})
		require.Equal(t, []string{"slack__post_message"}, result.Activated)
		require.Equal(t, "slack__post_message", result.Matches[0].Name)
	})
	t.Run("empty queries", func(t *testing.T) {
		t.Parallel()
		result, _ := SearchTools(entries, FindToolsArgs{}, SearchBudget{})
		require.Empty(t, result.Matches)
		require.Empty(t, result.Activated)
	})
	t.Run("cap", func(t *testing.T) {
		t.Parallel()
		many := make([]FindToolCatalogEntry, 25)
		for i := range many {
			many[i] = FindToolCatalogEntry{Name: fmt.Sprintf("server__tool_%02d", i), Description: "common"}
		}
		result, _ := SearchTools(many, FindToolsArgs{Queries: []string{"common"}}, SearchBudget{})
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
		result, _ := SearchTools(many, FindToolsArgs{Queries: []string{"common"}, Names: []string{"server__tool_24"}}, SearchBudget{})
		require.Len(t, result.Matches, findToolsMaxMatches)
		require.Equal(t, "server__tool_24", result.Matches[0].Name)
		require.Contains(t, result.Activated, "server__tool_24")

		capped, _ := SearchTools(many, FindToolsArgs{Names: names}, SearchBudget{})
		require.Len(t, capped.Matches, findToolsMaxMatches)
		require.Len(t, capped.Activated, findToolsMaxMatches)
	})
	t.Run("names list is bounded", func(t *testing.T) {
		t.Parallel()
		entries := []FindToolCatalogEntry{
			{Name: "server__target", Description: "does things"},
		}
		unknown := make([]string, findToolsMaxNames)
		for i := range unknown {
			unknown[i] = fmt.Sprintf("missing_%02d", i)
		}
		result, _ := SearchTools(entries, FindToolsArgs{Names: append(slices.Clone(unknown), "server__target")}, SearchBudget{})
		require.Empty(t, result.Activated,
			"a name past the inspection cap is not looked up")

		result, _ = SearchTools(entries, FindToolsArgs{Names: append(unknown[:findToolsMaxNames-1], "server__target")}, SearchBudget{})
		require.Equal(t, []string{"server__target"}, result.Activated,
			"a name within the inspection cap still activates")
	})
	t.Run("server metadata", func(t *testing.T) {
		t.Parallel()
		serverEntries := []FindToolCatalogEntry{
			{Name: "tracker__create", Description: "Create an item", Server: "tracker", ServerDescription: "Project tracking"},
			{Name: "docs__create", Description: "Create a project document", Server: "docs", ServerDescription: "Documentation"},
		}
		result, _ := SearchTools(serverEntries, FindToolsArgs{Queries: []string{"tracking"}}, SearchBudget{})
		require.Equal(t, []string{"tracker__create"}, result.Activated)

		result, _ = SearchTools(serverEntries, FindToolsArgs{Queries: []string{"project"}}, SearchBudget{})
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
		result, _ := SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"github: status"}}, SearchBudget{})
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"a known server prefix restricts matches to that server")

		result, _ = SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"github:"}}, SearchBudget{})
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"a bare server prefix lists that server's tools")

		result, _ = SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"error: status"}}, SearchBudget{})
		require.Len(t, result.Matches, 2,
			"an unknown prefix is searched as plain keywords")

		result, _ = SearchTools(scopedEntries, FindToolsArgs{Queries: []string{"GitHub: status"}}, SearchBudget{})
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"a case-variant prefix still scopes to its server when no exact-case name collides")
	})
	t.Run("case-colliding server names", func(t *testing.T) {
		t.Parallel()
		caseEntries := []FindToolCatalogEntry{
			{Name: "GitHub__enterprise_status", Description: "Enterprise status", Server: "GitHub"},
			{Name: "github__get_commit", Description: "Get commit status", Server: "github"},
		}
		result, _ := SearchTools(caseEntries, FindToolsArgs{Queries: []string{"GitHub: status"}}, SearchBudget{})
		require.Equal(t, []string{"GitHub__enterprise_status"}, result.Activated,
			"an exact-case prefix scopes only to its own server")

		result, _ = SearchTools(caseEntries, FindToolsArgs{Queries: []string{"github: status"}}, SearchBudget{})
		require.Equal(t, []string{"github__get_commit"}, result.Activated,
			"the case-colliding sibling stays reachable by its own exact name")

		result, _ = SearchTools(caseEntries, FindToolsArgs{Queries: []string{"GITHUB: status"}}, SearchBudget{})
		require.Len(t, result.Activated, 2,
			"a prefix matching no exact-case name falls back to spanning the case-colliding servers")
	})
	t.Run("folded scopes with different byte lengths", func(t *testing.T) {
		t.Parallel()
		// The long s folds with S and s but is two UTF-8 bytes, so a
		// byte-length prefix slice can never line the two forms up.
		// Scope-only queries keep the assertion sharp: an unscoped
		// fallback tokenizes to a term that matches nothing because
		// ToLower does not case-fold the long s.
		foldedEntries := []FindToolCatalogEntry{
			{Name: "ſerver__tool", Description: "does things", Server: "ſerver"},
		}
		result, _ := SearchTools(foldedEntries, FindToolsArgs{Queries: []string{"Server:"}}, SearchBudget{})
		require.Equal(t, []string{"ſerver__tool"}, result.Activated,
			"a folded scope with fewer bytes than the server name still scopes")

		asciiEntries := []FindToolCatalogEntry{
			{Name: "server__tool", Description: "does things", Server: "server"},
		}
		result, _ = SearchTools(asciiEntries, FindToolsArgs{Queries: []string{"ſerver:"}}, SearchBudget{})
		require.Equal(t, []string{"server__tool"}, result.Activated,
			"a folded scope with more bytes than the server name still scopes")
	})
	t.Run("bounded query work", func(t *testing.T) {
		t.Parallel()
		entries := []FindToolCatalogEntry{
			{Name: "server__match", Description: "Matches the last token", Server: "server"},
		}
		// The matching term is placed beyond both caps, so a match
		// proves the caps were not applied.
		overflowQuery := strings.Repeat("filler ", findToolsMaxQueryTokens) + "matches"
		result, _ := SearchTools(entries, FindToolsArgs{Queries: []string{overflowQuery}}, SearchBudget{})
		require.Empty(t, result.Activated,
			"tokens beyond the per-query cap are not scored")

		queries := make([]string, findToolsMaxQueries+1)
		for i := range queries {
			queries[i] = "filler"
		}
		queries[len(queries)-1] = "matches"
		result, _ = SearchTools(entries, FindToolsArgs{Queries: queries}, SearchBudget{})
		require.Empty(t, result.Activated,
			"queries beyond the per-call cap are not scored")

		result, _ = SearchTools(entries, FindToolsArgs{Queries: []string{"matches"}}, SearchBudget{})
		require.Equal(t, []string{"server__match"}, result.Activated,
			"capped search still scores in-bound tokens")
	})
	t.Run("whitespace-colliding server names", func(t *testing.T) {
		t.Parallel()
		paddedEntries := []FindToolCatalogEntry{
			{Name: "_everything___ping", Description: "Ping status", Server: " everything "},
			{Name: "everything__status", Description: "Get status", Server: "everything"},
		}
		result, _ := SearchTools(paddedEntries, FindToolsArgs{Queries: []string{"everything: status"}}, SearchBudget{})
		require.Equal(t, []string{"everything__status"}, result.Activated,
			"the exact-form scope matches only its own server, not a whitespace-padded sibling")

		result, _ = SearchTools(paddedEntries, FindToolsArgs{Queries: []string{" everything : ping"}}, SearchBudget{})
		require.Equal(t, []string{"_everything___ping"}, result.Activated,
			"the raw query prefix is matched before trimming, so the padded server stays selectable")
	})
	t.Run("server names containing colons", func(t *testing.T) {
		t.Parallel()
		colonEntries := []FindToolCatalogEntry{
			{Name: "jira_prod__list_issues", Description: "List issues", Server: "jira:prod"},
			{Name: "jira__list_issues", Description: "List issues", Server: "jira"},
			{Name: "ci__status", Description: "Issue pipeline status", Server: "ci"},
		}
		result, _ := SearchTools(colonEntries, FindToolsArgs{Queries: []string{"jira:prod: issues"}}, SearchBudget{})
		require.Equal(t, []string{"jira_prod__list_issues"}, result.Activated,
			"the longest cataloged server name wins over its colon-split prefix")

		result, _ = SearchTools(colonEntries, FindToolsArgs{Queries: []string{"jira: issues"}}, SearchBudget{})
		require.Equal(t, []string{"jira__list_issues"}, result.Activated,
			"the shorter server still scopes its own queries")
	})
	t.Run("unicode terms", func(t *testing.T) {
		t.Parallel()
		unicodeEntries := []FindToolCatalogEntry{
			{Name: "docs__検索", Description: "ドキュメント検索"},
			{Name: "docs__erstellen", Description: "Dokument ERSTELLEN"},
		}
		result, _ := SearchTools(unicodeEntries, FindToolsArgs{Queries: []string{"検索"}}, SearchBudget{})
		require.Equal(t, []string{"docs__検索"}, result.Activated)

		result, _ = SearchTools(unicodeEntries, FindToolsArgs{Queries: []string{"Erstellen"}}, SearchBudget{})
		require.Equal(t, []string{"docs__erstellen"}, result.Activated)
	})
	t.Run("schema token budget", func(t *testing.T) {
		t.Parallel()
		weighted := []FindToolCatalogEntry{
			{Name: "server__big_a", Description: "big", SchemaTokens: 60},
			{Name: "server__big_b", Description: "big", SchemaTokens: 60},
			{Name: "server__huge", Description: "big", SchemaTokens: 500},
		}
		result, _ := SearchTools(weighted, FindToolsArgs{Queries: []string{"big"}}, SearchBudget{SchemaTokens: 100, AllowFirstOverBudget: true})
		require.Equal(t, []string{"server__big_a"}, result.Activated,
			"matches stop once the schema budget is spent")

		result, _ = SearchTools(weighted, FindToolsArgs{Names: []string{"server__huge"}}, SearchBudget{SchemaTokens: 100, AllowFirstOverBudget: true})
		require.Equal(t, []string{"server__huge"}, result.Activated,
			"the first match is kept even when it alone exceeds the budget")
	})
	t.Run("result descriptions are summarized", func(t *testing.T) {
		t.Parallel()
		long := []FindToolCatalogEntry{{
			Name:        "server__verbose",
			Description: strings.Repeat("word ", 100),
		}}
		result, _ := SearchTools(long, FindToolsArgs{Names: []string{"server__verbose"}}, SearchBudget{})
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

func TestFindToolsSerialToolCalls(t *testing.T) {
	t.Parallel()
	serial, ok := FindTools(FindToolsOptions{}).(interface{ SerialToolCalls() bool })
	require.True(t, ok, "find_tools must opt into serial execution so shared-budget admission follows tool-call order")
	require.True(t, serial.SerialToolCalls())
}

func TestFindToolsDirectCallReservation(t *testing.T) {
	t.Parallel()
	newTool := func(budget float64) (fantasy.AgentTool, interface{ ObserveStepToolCalls([]string) }) {
		tool := FindTools(FindToolsOptions{
			Entries: []FindToolCatalogEntry{
				{Name: "server__a", SchemaTokens: 60},
				{Name: "server__b", SchemaTokens: 50},
				{Name: "server__c", SchemaTokens: 30},
			},
			SchemaTokenBudget: budget,
		})
		observer, ok := tool.(interface{ ObserveStepToolCalls([]string) })
		require.True(t, ok, "find_tools must observe step tool calls to reserve direct-call schema weight")
		return tool, observer
	}

	t.Run("direct calls charge the budget before searches", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		observer.ObserveStepToolCalls([]string{"server__a", "server__a", "unknown", FindToolsName})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "a search claim exceeding the budget left by direct calls must fail loudly")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__c"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "duplicate direct-call names are charged once")
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__c"}, result.Activated)
	})

	t.Run("rejected calls still reach OnCall", func(t *testing.T) {
		t.Parallel()
		var calls []FindToolsCall
		tool := FindTools(FindToolsOptions{
			Entries: []FindToolCatalogEntry{
				{Name: "server__a", SchemaTokens: 60},
				{Name: "server__b", SchemaTokens: 50},
			},
			SchemaTokenBudget: 60,
			OnCall:            func(_ context.Context, call FindToolsCall) { calls = append(calls, call) },
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"queries":"github"}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "a type mismatch is rejected by the argument decoder")
		require.Len(t, calls, 4, "rejected calls count toward call totals")
		require.Empty(t, calls[0].Rejection)
		require.Equal(t, "budget", calls[1].Rejection)
		require.Equal(t, []string{"server__b"}, calls[1].Names)
		require.Empty(t, calls[1].Activated, "a rejected call reports no activations")
		require.Equal(t, "arguments", calls[2].Rejection, "empty-argument calls are counted as rejected")
		require.Empty(t, calls[2].Activated)
		require.Equal(t, "arguments", calls[3].Rejection,
			"calls rejected during argument decoding are counted before the handler is reached")
	})

	t.Run("a touched budget skips oversized matches and admits later fits", func(t *testing.T) {
		t.Parallel()
		tool := FindTools(FindToolsOptions{
			Entries: []FindToolCatalogEntry{
				{Name: "server__a", SchemaTokens: 60},
				{Name: "server__b", SchemaTokens: 50},
				{Name: "server__c", SchemaTokens: 30},
			},
			SchemaTokenBudget: 100,
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b","server__c"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "an oversized top match must not fail the call when a later match fits")
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__c"}, result.Activated,
			"the oversized first match is skipped and the fitting later match admitted")
	})

	t.Run("an errored direct call refunds its reservation", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		settler, ok := tool.(interface {
			ObserveStepToolResults(names []string, errored []bool)
		})
		require.True(t, ok, "find_tools must observe step results to refund errored reservations")
		observer.ObserveStepToolCalls([]string{"server__a"})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "the pre-execution reservation holds while the outcome is unknown")

		settler.ObserveStepToolResults([]string{"server__a", "unknown"}, []bool{true, true})
		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "the refunded reservation admits later searches")
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__b"}, result.Activated)
	})

	t.Run("a name that executed successfully keeps its reservation", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		settler, ok := tool.(interface {
			ObserveStepToolResults(names []string, errored []bool)
		})
		require.True(t, ok)
		observer.ObserveStepToolCalls([]string{"server__a", "server__a"})
		settler.ObserveStepToolResults([]string{"server__a", "server__a"}, []bool{false, true})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "a successful execution pins the reservation even when a later call errors")
	})

	t.Run("mixed outcomes admit at the first successful call position", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		settler, ok := tool.(interface {
			ObserveStepToolResults(names []string, errored []bool)
		})
		require.True(t, ok)
		// A errors, B succeeds, then A succeeds: derivation postpones
		// the errored A by call ID, admits B, and budget-rejects the
		// later A (60 over the 50 already charged), so only B's schema
		// reaches the next request.
		observer.ObserveStepToolCalls([]string{"server__a", "server__b", "server__a"})
		settler.ObserveStepToolResults([]string{"server__a", "server__b", "server__a"}, []bool{true, false, false})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Empty(t, result.Activated,
			"a name derivation budget-rejects at its successful call position is unclaimable, not free")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__b"}, result.Activated, "the admitted call stays free")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__c"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__c"}, result.Activated,
			"only the admitted prefix is charged, so the leftover budget admits new claims")
	})

	t.Run("aggregate overflow frees only the prefix derivation retains", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		settler, ok := tool.(interface {
			ObserveStepToolResults(names []string, errored []bool)
		})
		require.True(t, ok)
		observer.ObserveStepToolCalls([]string{"server__a", "server__b"})
		settler.ObserveStepToolResults([]string{"server__a", "server__b"}, []bool{false, false})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Empty(t, result.Activated,
			"a direct call past the retained prefix cannot be reported activated: derivation sheds it")
		require.Equal(t, 3, result.TotalDeferred, "unclaimable entries still count as deferred")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__a"}, result.Activated, "the retained prefix stays free")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__c"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__c"}, result.Activated,
			"the skipped call's weight is not charged, so later searches keep the leftover budget")
	})

	t.Run("a name claimed by an earlier search is free for later searches", func(t *testing.T) {
		t.Parallel()
		tool, _ := newTool(60)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "derivation deduplicates by name, so a repeated claim costs nothing")
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__a"}, result.Activated)

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__c"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "the repeated claim must not have refunded the spent budget")
	})

	t.Run("an errored prefix call promotes the next observed name", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(100)
		settler, ok := tool.(interface {
			ObserveStepToolResults(names []string, errored []bool)
		})
		require.True(t, ok)
		observer.ObserveStepToolCalls([]string{"server__a", "server__b"})
		settler.ObserveStepToolResults([]string{"server__a", "server__b"}, []bool{true, false})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__b"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__b"}, result.Activated,
			"the errored call leaves the prefix, so the succeeding call becomes free")

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError,
			"the errored call is claimable at full weight, which exceeds the leftover budget")
	})

	t.Run("reserved names stay activatable after the budget is spent", func(t *testing.T) {
		t.Parallel()
		tool, observer := newTool(50)
		observer.ObserveStepToolCalls([]string{"server__a"})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__a"]}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "derivation retains direct calls, so reporting them activated is free")
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, []string{"server__a"}, result.Activated)

		resp, err = tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__c"]}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "an over-reserved budget admits no new schema weight")
	})
}

func TestFindToolsSharedSchemaBudget(t *testing.T) {
	t.Parallel()
	tool := FindTools(FindToolsOptions{
		Entries: []FindToolCatalogEntry{
			{Name: "server__a", SchemaTokens: 60},
			{Name: "server__b", SchemaTokens: 60},
			{Name: "server__c", SchemaTokens: 60},
			{Name: "server__d", SchemaTokens: 60},
		},
		SchemaTokenBudget: 200,
	})
	activated := func(input string) []string {
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		var result FindToolsResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		return result.Activated
	}

	require.Equal(t, []string{"server__a", "server__b"}, activated(`{"names":["server__a","server__b"]}`))
	require.Equal(t, []string{"server__c"}, activated(`{"names":["server__c","server__d"]}`),
		"the second call spends the remaining shared budget, not a fresh one")

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__d"]}`})
	require.NoError(t, err)
	require.True(t, resp.IsError,
		"a call whose claims cannot fit the remaining budget errors instead of over-claiming")

	huge := FindTools(FindToolsOptions{
		Entries: []FindToolCatalogEntry{
			{Name: "server__huge", SchemaTokens: 500},
			{Name: "server__other", SchemaTokens: 60},
		},
		SchemaTokenBudget: 200,
	})
	resp, err = huge.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__huge"]}`})
	require.NoError(t, err)
	var hugeResult FindToolsResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &hugeResult))
	require.Equal(t, []string{"server__huge"}, hugeResult.Activated,
		"an untouched budget may over-claim once; derivation's newest-keep retains the sole claim")
	resp, err = huge.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__huge"]}`})
	require.NoError(t, err)
	require.False(t, resp.IsError, "repeating an already claimed name costs nothing")
	resp, err = huge.Run(context.Background(), fantasy.ToolCall{Input: `{"names":["server__other"]}`})
	require.NoError(t, err)
	require.True(t, resp.IsError, "the spent budget rejects further new activations")
}

func TestBuildFindToolsDescription(t *testing.T) {
	t.Parallel()
	entries := []FindToolCatalogEntry{
		{Name: "zeta__last", Description: "Last tool. More detail", Server: "zeta", ServerDescription: strings.Repeat("z", 80)},
		{Name: "alpha__second", Description: strings.Repeat("x", 100), Server: "alpha", ServerDescription: "Alpha server"},
		{Name: "alpha__first", Description: "First tool\nmore detail", Server: "alpha", ServerDescription: "Alpha server"},
	}
	description := buildFindToolsDescription(entries, 0)
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
	degraded := buildFindToolsDescription(many, 0)
	require.Contains(t, degraded, "## server (300 tools)")
	require.NotContains(t, degraded, "server__tool_000")

	manyServers := make([]FindToolCatalogEntry, 500)
	for i := range manyServers {
		manyServers[i] = FindToolCatalogEntry{
			Name:   fmt.Sprintf("server_%03d_%s__tool", i, strings.Repeat("s", 40)),
			Server: fmt.Sprintf("server_%03d_%s", i, strings.Repeat("s", 40)),
		}
	}
	countsExceeded := buildFindToolsDescription(manyServers, 0)
	require.Contains(t, countsExceeded, "500 deferred tools across 500 servers.")
	require.NotContains(t, countsExceeded, "## server_000")
	require.LessOrEqual(t, estimatedFindToolsTokens(countsExceeded), float64(findToolsCatalogTokens))

	smallWindow := buildFindToolsDescription(entries, 150)
	require.NotContains(t, smallWindow, "First tool",
		"a small context window budget forces catalog degradation below the 4000-token default")
}
