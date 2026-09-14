package codersdk_test

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// Duplicated from the package so the limit stays unexported.
const maxAppNameLength = 64

func TestNormalizeAppName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"NormalizedPassthrough", "vscode", "vscode"},
		{"KnownNameCaseInsensitive", "JetBrains", "jetbrains"},
		{"LegacyAlias", "reconnecting-pty", "reconnecting_pty"},
		{"HyphenatedKnownName", "vscode-insiders", "vscode_insiders"},
		{"UnknownHyphensFolded", "my-future-ide", "my_future_ide"},
		{"UnknownLowercased", "Cursor Nightly", "cursor nightly"},
		{"UnknownPreservesUnicode", "エディタ", "エディタ"},
		{"StripsNullBytes", "cur\x00sor", "cursor"},
		{"TrimsWhitespace", " vscode\t", "vscode"},
		{"Empty", "", "unknown"},
		{"OnlyNullBytes", "\x00\x00", "unknown"},
		{"OnlyWhitespace", "   ", "unknown"},
		{"TruncatesToMaxRunes", strings.Repeat("a", maxAppNameLength+10), strings.Repeat("a", maxAppNameLength)},
		{"TruncatesMultibyteSafely", strings.Repeat("あ", maxAppNameLength+1), strings.Repeat("あ", maxAppNameLength)},
		// Padding is trimmed before truncating, so it does not spend the budget.
		{"TrimsPaddingBeforeTruncating", "  " + strings.Repeat("a", maxAppNameLength) + "  ", strings.Repeat("a", maxAppNameLength)},
		{"TrimsWhitespaceLeftByTruncation", strings.Repeat("a", 60) + "     tail", strings.Repeat("a", 60)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.NormalizeAppName(tc.input))
		})
	}
}

// The usage-app vocabulary is declared independently of the normalized app
// names because its values are part of the HTTP API. This pins the two
// together so they cannot drift.
func TestUsageAppNamesAreNormalized(t *testing.T) {
	t.Parallel()

	for sdkName, want := range map[codersdk.UsageAppName]string{
		codersdk.UsageAppNameVscode:          "vscode",
		codersdk.UsageAppNameJetbrains:       "jetbrains",
		codersdk.UsageAppNameReconnectingPty: "reconnecting_pty",
		codersdk.UsageAppNameSSH:             "ssh",
	} {
		require.Equal(t, want, codersdk.NormalizeAppName(string(sdkName)))
	}
}

func TestAppNameFamily(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  codersdk.AppFamilyName
	}{
		{"VSCode", "vscode", codersdk.AppFamilyVSCode},
		{"VSCodeFork", "cursor", codersdk.AppFamilyVSCode},
		{"VSCodeInsidersHyphenated", "vscode-insiders", codersdk.AppFamilyVSCode},
		{"CaseInsensitive", "JetBrains", codersdk.AppFamilyJetBrains},
		{"SSHClientJoinsSSHFamily", "zed", codersdk.AppFamilySSH},
		{"Alias", "reconnecting-pty", codersdk.AppFamilyReconnectingPTY},
		{"Unknown", "SomeFutureIDE", codersdk.AppFamilyUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.AppNameFamily(tc.input))
		})
	}
}

func TestAppNamesInFamily(t *testing.T) {
	t.Parallel()

	// Forks share the VS Code family, and the list is sorted.
	vscode := codersdk.AppNamesInFamily(codersdk.AppFamilyVSCode)
	require.Contains(t, vscode, "cursor")
	require.Contains(t, vscode, "vscode")
	require.True(t, slices.IsSorted(vscode))

	// Zed speaks SSH, so it reports under the SSH family.
	require.Equal(t, []string{"ssh", "zed"},
		codersdk.AppNamesInFamily(codersdk.AppFamilySSH))

	require.Empty(t, codersdk.AppNamesInFamily("no_such_family"))
}

// The SQL queries that report per-family session counts hardcode one probe
// expression and one output column per family, because sqlc output columns
// are static. This pins the Go registry to the families those queries know
// about.
//
// When adding a family, update, in this order:
//  1. attributedAppFamilies in appname.go.
//  2. Every query that reports per-family session counts: the fams CTE list,
//     the probe expression, and the output column, in
//     coderd/database/queries/workspaceagentstats.sql and insights.sql.
//  3. The Go readers of those columns, then this list.
//
// dbauthz validation rejects a registry that does not match
// AttributedAppFamilies, so a family added to Go but not to SQL fails at
// runtime too.
func TestAttributedAppFamiliesMatchQueries(t *testing.T) {
	t.Parallel()

	require.Equal(t, []codersdk.AppFamilyName{
		codersdk.AppFamilyVSCode,
		codersdk.AppFamilyJetBrains,
		codersdk.AppFamilySSH,
		codersdk.AppFamilyReconnectingPTY,
	}, codersdk.AttributedAppFamilies())

	// The registry keys are what the queries index by name.
	require.ElementsMatch(t, codersdk.AttributedAppFamilies(),
		slices.Collect(maps.Keys(codersdk.SessionCountAppFamilies())),
		"registry keys must match the attributed families")
}

func TestSessionCountAppFamilies(t *testing.T) {
	t.Parallel()

	families := codersdk.SessionCountAppFamilies()
	require.Len(t, families, 4, "every attributed family must be present")
	require.Contains(t, families, codersdk.AppFamilyVSCode)
	require.Contains(t, families, codersdk.AppFamilyJetBrains)
	require.Contains(t, families, codersdk.AppFamilySSH)
	require.Contains(t, families, codersdk.AppFamilyReconnectingPTY)
	require.Equal(t, codersdk.AppNamesInFamily(codersdk.AppFamilyVSCode), families[codersdk.AppFamilyVSCode])
}

func TestSessionCountAppFamiliesJSON(t *testing.T) {
	t.Parallel()

	raw := codersdk.SessionCountAppFamiliesJSON()
	require.NotEmpty(t, raw)

	var decoded map[codersdk.AppFamilyName][]string
	require.NoError(t, json.Unmarshal(raw, &decoded), "registry must marshal to a valid jsonb object")
	require.Equal(t, codersdk.SessionCountAppFamilies(), decoded)
}

func TestSessionCountsByFamily(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		appCounts map[string]int64
		want      map[codersdk.AppFamilyName]int64
	}{
		{
			name:      "Empty",
			appCounts: map[string]int64{},
			want:      map[codersdk.AppFamilyName]int64{},
		},
		{
			name:      "Nil",
			appCounts: nil,
			want:      map[codersdk.AppFamilyName]int64{},
		},
		{
			name:      "SingleApp",
			appCounts: map[string]int64{"jetbrains": 3},
			want:      map[codersdk.AppFamilyName]int64{codersdk.AppFamilyJetBrains: 3},
		},
		{
			// Simultaneous forks are additive within their family.
			name:      "OverlappingAppsInOneFamily",
			appCounts: map[string]int64{"vscode": 1, "cursor": 2, "windsurf": 4},
			want:      map[codersdk.AppFamilyName]int64{codersdk.AppFamilyVSCode: 7},
		},
		{
			name:      "UnregisteredAppIsUnknown",
			appCounts: map[string]int64{"some_future_ide": 5, "ssh": 1},
			want: map[codersdk.AppFamilyName]int64{
				codersdk.AppFamilyUnknown: 5,
				codersdk.AppFamilySSH:     1,
			},
		},
		{
			// Storage normalizes app names, but folding must not depend on it.
			name:      "DenormalizedNamesFold",
			appCounts: map[string]int64{"VSCode-Insiders": 2, "vscode_insiders": 1},
			want:      map[codersdk.AppFamilyName]int64{codersdk.AppFamilyVSCode: 3},
		},
		{
			name:      "AllFamilies",
			appCounts: map[string]int64{"vscode": 1, "jetbrains": 2, "zed": 3, "reconnecting_pty": 4, "": 5},
			want: map[codersdk.AppFamilyName]int64{
				codersdk.AppFamilyVSCode:          1,
				codersdk.AppFamilyJetBrains:       2,
				codersdk.AppFamilySSH:             3,
				codersdk.AppFamilyReconnectingPTY: 4,
				codersdk.AppFamilyUnknown:         5,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.SessionCountsByFamily(tc.appCounts))
		})
	}
}

// Every app name the registry attributes must fold back into the family it is
// registered under, so no attributed family can go uncounted.
func TestSessionCountsByFamilyCoversEveryAttributedFamily(t *testing.T) {
	t.Parallel()

	appCounts := map[string]int64{}
	want := map[codersdk.AppFamilyName]int64{}
	for family, appNames := range codersdk.SessionCountAppFamilies() {
		for _, appName := range appNames {
			appCounts[appName] = 1
			want[family]++
		}
	}
	require.Equal(t, want, codersdk.SessionCountsByFamily(appCounts))
}

func TestSessionCountsByFamilyJSON(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		appCounts json.RawMessage
		want      map[codersdk.AppFamilyName]int64
	}{
		{"Counts", json.RawMessage(`{"cursor":2,"vscode":1,"ssh":4}`), map[codersdk.AppFamilyName]int64{
			codersdk.AppFamilyVSCode: 3,
			codersdk.AppFamilySSH:    4,
		}},
		{"UnknownApp", json.RawMessage(`{"some_future_ide":9}`), map[codersdk.AppFamilyName]int64{
			codersdk.AppFamilyUnknown: 9,
		}},
		{"EmptyObject", json.RawMessage(`{}`), map[codersdk.AppFamilyName]int64{}},
		// A query with no matching rows aggregates to SQL NULL, which is not
		// an error, just no sessions.
		{"JSONNull", json.RawMessage(`null`), map[codersdk.AppFamilyName]int64{}},
		{"Absent", nil, map[codersdk.AppFamilyName]int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := codersdk.SessionCountsByFamilyJSON(tc.appCounts)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSessionCountsByFamilyJSONMalformed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		appCounts json.RawMessage
	}{
		{"Truncated", json.RawMessage(`{"vscode":`)},
		{"NotAnObject", json.RawMessage(`["vscode"]`)},
		{"NonNumericCount", json.RawMessage(`{"vscode":"1"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := codersdk.SessionCountsByFamilyJSON(tc.appCounts)
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}
