package codersdk_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
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
		{"StripsControlCharacters", "vs\ncode\r\t", "vscode"},
		// Only the escape byte itself is a control character, so the rest of
		// an ANSI sequence survives as ordinary text.
		{"StripsANSIEscape", "\x1b[31mvscode\x1b[0m", "[31mvscode[0m"},
		{"OnlyControlCharacters", "\n\r\t\x1b", "unknown"},
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

// Family sets are derived from the one registry, so callers that need the app
// names in a family do not need a second list.
func TestRegistryFamilySets(t *testing.T) {
	t.Parallel()

	registry := codersdk.SessionCountAppFamilies()
	inFamily := func(want codersdk.AppFamilyName) []string {
		var names []string
		for appName, family := range registry {
			if family == want {
				names = append(names, appName)
			}
		}
		slices.Sort(names)
		return names
	}

	// Forks share the VS Code family.
	vscode := inFamily(codersdk.AppFamilyVSCode)
	require.Contains(t, vscode, "cursor")
	require.Contains(t, vscode, "vscode")

	// Zed speaks SSH, so it reports under the SSH family.
	require.Equal(t, []string{"ssh", "zed"}, inFamily(codersdk.AppFamilySSH))

	require.Empty(t, inFamily("no_such_family"))
}

func TestSessionCountAppFamilies(t *testing.T) {
	t.Parallel()

	registry := codersdk.SessionCountAppFamilies()
	require.NotEmpty(t, registry)
	for appName, family := range registry {
		require.Equal(t, family, codersdk.AppNameFamily(appName),
			"registry entry %q must agree with lookup", appName)
	}

	// The registry is a copy, so a caller cannot corrupt attribution.
	registry["cursor"] = codersdk.AppFamilySSH
	require.Equal(t, codersdk.AppFamilyVSCode, codersdk.AppNameFamily("cursor"))
	require.Equal(t, codersdk.AppFamilyVSCode, codersdk.SessionCountAppFamilies()["cursor"])
}

func TestSumByFamily(t *testing.T) {
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
			require.Equal(t, tc.want, codersdk.SumByFamily(tc.appCounts))
		})
	}
}

// A family is registered by adding app names alone, with no SQL, column, or
// second list to update.
func TestSumByFamilyCoversEveryRegisteredFamily(t *testing.T) {
	t.Parallel()

	appCounts := map[string]int64{}
	want := map[codersdk.AppFamilyName]int64{}
	for appName, family := range codersdk.SessionCountAppFamilies() {
		appCounts[appName] = 1
		want[family]++
	}
	require.Equal(t, want, codersdk.SumByFamily(appCounts))
}

func TestDecodeAppMap(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]json.RawMessage{
		"NotJSON":     json.RawMessage(`{`),
		"NotAnObject": json.RawMessage(`[1, 2]`),
		"WrongValue":  json.RawMessage(`{"vscode": "sixty"}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// A malformed payload must not decode to zero usage, or an
			// encoding bug would look like an idle deployment.
			got, err := codersdk.DecodeAppMap[int64](raw)
			require.Error(t, err)
			require.Nil(t, got)
		})
	}

	t.Run("UsageSeconds", func(t *testing.T) {
		t.Parallel()

		got, err := codersdk.DecodeAppMap[int64](json.RawMessage(`{"cursor": 60}`))
		require.NoError(t, err)
		require.Equal(t, map[string]int64{"cursor": 60}, got)
	})

	// A query with no rows aggregates to SQL NULL: no usage, not an error.
	for name, raw := range map[string]json.RawMessage{
		"Absent":      nil,
		"EmptyObject": json.RawMessage(`{}`),
		"JSONNull":    json.RawMessage(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := codersdk.DecodeAppMap[int64](raw)
			require.NoError(t, err)
			require.Equal(t, map[string]int64{}, got)
			require.Zero(t, got["ssh"])
		})
	}
}

func TestUnionByFamily(t *testing.T) {
	t.Parallel()

	shared, cursorOnly, sshOnly := uuid.New(), uuid.New(), uuid.New()
	got := codersdk.UnionByFamily(map[string][]uuid.UUID{
		"vscode": {shared},
		"cursor": {shared, cursorOnly},
		"ssh":    {sshOnly},
		// An app the registry does not know still lands somewhere.
		"some_new_ide": {cursorOnly},
	})

	// A template both apps saw appears once for the family.
	require.ElementsMatch(t, []uuid.UUID{shared, cursorOnly}, got[codersdk.AppFamilyVSCode])
	require.Equal(t, []uuid.UUID{sshOnly}, got[codersdk.AppFamilySSH])
	require.Equal(t, []uuid.UUID{cursorOnly}, got[codersdk.AppFamilyUnknown])
	require.Empty(t, got[codersdk.AppFamilyJetBrains])
}

func TestSessionCountApps(t *testing.T) {
	t.Parallel()

	apps := codersdk.SessionCountApps(map[string]int64{"cursor": 2, "vscodium": 1, "future_ide": 3})
	require.Equal(t, map[string]codersdk.SessionCountApp{
		"cursor":   {Count: 2, DisplayName: "Cursor", Icon: "/icon/cursor.svg", Family: codersdk.AppFamilyVSCode},
		"vscodium": {Count: 1, DisplayName: "VSCodium", Family: codersdk.AppFamilyVSCode},
		// An app the registry does not know shows its own name.
		"future_ide": {Count: 3, DisplayName: "future_ide", Family: codersdk.AppFamilyUnknown},
	}, apps)
	require.Equal(t, map[string]codersdk.SessionCountApp{}, codersdk.SessionCountApps(nil))
}

func TestSessionCountAppIcons(t *testing.T) {
	t.Parallel()

	// Every icon is a bundled, clean path, never an arbitrary URL.
	for name := range codersdk.SessionCountAppFamilies() {
		app := codersdk.SessionCountApps(map[string]int64{name: 1})[name]
		require.NotEmpty(t, app.DisplayName, name)
		if app.Icon == "" {
			continue
		}
		require.True(t, strings.HasPrefix(app.Icon, "/icon/"), name)
		require.Equal(t, filepath.Clean(app.Icon), app.Icon)
		require.FileExists(t, filepath.Join("..", "site", "static", app.Icon), "icon for %s must be bundled", name)
	}
}
