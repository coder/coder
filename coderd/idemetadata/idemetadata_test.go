package idemetadata_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idemetadata"
	"github.com/coder/coder/v2/codersdk"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"CanonicalPassthrough", "vscode", "vscode"},
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
		{"TruncatesToMaxRunes", strings.Repeat("a", idemetadata.MaxAppNameLength+10), strings.Repeat("a", idemetadata.MaxAppNameLength)},
		{"TruncatesMultibyteSafely", strings.Repeat("あ", idemetadata.MaxAppNameLength+1), strings.Repeat("あ", idemetadata.MaxAppNameLength)},
		// Padding is trimmed before truncating, so it does not spend the budget.
		{"TrimsPaddingBeforeTruncating", "  " + strings.Repeat("a", idemetadata.MaxAppNameLength) + "  ", strings.Repeat("a", idemetadata.MaxAppNameLength)},
		{"TrimsWhitespaceLeftByTruncation", strings.Repeat("a", 60) + "     tail", strings.Repeat("a", 60)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, idemetadata.Normalize(tc.input))
		})
	}
}

// The codersdk usage-app vocabulary is declared independently because
// codersdk cannot import coderd packages. This pins it to the canonical
// names so the two cannot drift.
func TestCodersdkUsageAppNamesAreCanonical(t *testing.T) {
	t.Parallel()

	for sdkName, want := range map[codersdk.UsageAppName]string{
		codersdk.UsageAppNameVscode:          idemetadata.AppNameVSCode,
		codersdk.UsageAppNameJetbrains:       idemetadata.AppNameJetBrains,
		codersdk.UsageAppNameReconnectingPty: idemetadata.AppNameReconnectingPTY,
		codersdk.UsageAppNameSSH:             idemetadata.AppNameSSH,
	} {
		require.Equal(t, want, idemetadata.Normalize(string(sdkName)))
	}
}

func TestFamily(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"VSCode", "vscode", idemetadata.AppNameVSCode},
		{"VSCodeFork", "cursor", idemetadata.AppNameVSCode},
		{"VSCodeInsidersHyphenated", "vscode-insiders", idemetadata.AppNameVSCode},
		{"CaseInsensitive", "JetBrains", idemetadata.AppNameJetBrains},
		{"SSHClientJoinsSSHFamily", "zed", idemetadata.AppNameSSH},
		{"Alias", "reconnecting-pty", idemetadata.AppNameReconnectingPTY},
		{"Unknown", "SomeFutureIDE", idemetadata.AppNameUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, idemetadata.Family(tc.input))
		})
	}
}

func TestFamilyAliases(t *testing.T) {
	t.Parallel()

	// Forks share the VS Code family, and the list is sorted.
	vscode := idemetadata.FamilyAliases(idemetadata.AppNameVSCode)
	require.Contains(t, vscode, "cursor")
	require.Contains(t, vscode, idemetadata.AppNameVSCode)
	require.True(t, slices.IsSorted(vscode))

	// Zed speaks SSH, so it reports under the SSH family.
	require.Equal(t, []string{idemetadata.AppNameSSH, idemetadata.AppNameZed},
		idemetadata.FamilyAliases(idemetadata.AppNameSSH))

	require.Empty(t, idemetadata.FamilyAliases("no_such_family"))
}
