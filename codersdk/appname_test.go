package codersdk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

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
		{"TruncatesToMaxRunes", strings.Repeat("a", codersdk.MaxAppNameLength+10), strings.Repeat("a", codersdk.MaxAppNameLength)},
		{"TruncatesMultibyteSafely", strings.Repeat("あ", codersdk.MaxAppNameLength+1), strings.Repeat("あ", codersdk.MaxAppNameLength)},
		// Padding is trimmed before truncating, so it does not spend the budget.
		{"TrimsPaddingBeforeTruncating", "  " + strings.Repeat("a", codersdk.MaxAppNameLength) + "  ", strings.Repeat("a", codersdk.MaxAppNameLength)},
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
		codersdk.UsageAppNameVscode:          codersdk.AppNameVSCode,
		codersdk.UsageAppNameJetbrains:       codersdk.AppNameJetBrains,
		codersdk.UsageAppNameReconnectingPty: codersdk.AppNameReconnectingPTY,
		codersdk.UsageAppNameSSH:             codersdk.AppNameSSH,
	} {
		require.Equal(t, want, codersdk.NormalizeAppName(string(sdkName)))
	}
}

func TestAppNameFamily(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"VSCode", "vscode", codersdk.AppNameVSCode},
		{"VSCodeFork", "cursor", codersdk.AppNameVSCode},
		{"VSCodeInsidersHyphenated", "vscode-insiders", codersdk.AppNameVSCode},
		{"CaseInsensitive", "JetBrains", codersdk.AppNameJetBrains},
		{"SSHClientJoinsSSHFamily", "zed", codersdk.AppNameSSH},
		{"Alias", "reconnecting-pty", codersdk.AppNameReconnectingPTY},
		{"Unknown", "SomeFutureIDE", codersdk.AppNameUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.AppNameFamily(tc.input))
		})
	}
}
